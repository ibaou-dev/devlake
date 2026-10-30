/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tasks

import (
	"reflect"
	"strconv"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/models/domainlayer"
	"github.com/apache/devlake/core/models/domainlayer/didgen"
	"github.com/apache/devlake/core/models/domainlayer/ticket"
	"github.com/apache/devlake/core/plugin"
	"github.com/apache/devlake/core/utils"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/mapping"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// ConvertIssueChangelogsMeta describes the changelog convertor.
var ConvertIssueChangelogsMeta = plugin.SubTaskMeta{
	Name:             "convertIssueChangelogs",
	EntryPoint:       ConvertIssueChangelogs,
	EnabledByDefault: true,
	Description:      "Convert _tool_redmine_go_journal_details into the domain layer table issue_changelogs, with the standard status on status changes",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
	DependencyTables: []string{
		models.RedmineGoJournalDetail{}.TableName(),
		models.RedmineGoIssue{}.TableName(),
		models.RedmineGoTracker{}.TableName(),
		models.RedmineGoStatus{}.TableName(),
		rawTableName(RAW_ISSUE_DETAILS_TABLE),
	},
	ProductTables: []string{ticket.IssueChangelogs{}.TableName()},
}

var _ plugin.SubTaskEntryPoint = ConvertIssueChangelogs

// ConvertIssueChangelogs converts every journal detail of the scope's issues
// to one changelog row.
//
//   - Status changes carry the standard status (by the same mapping the issue
//     uses) in from_value and to_value, and the status names in
//     original_from_value and original_to_value, the shape DevLake's status
//     history expects.
//   - Assignee changes carry account names in from_value and to_value, and the
//     accounts' domain ids in the original values, like the Jira plugin.
//   - Every other change carries the translated label in from_value and
//     to_value, and the raw Redmine value in the original values.
func ConvertIssueChangelogs(taskCtx plugin.SubTaskContext) errors.Error {
	db := taskCtx.GetDal()
	data := taskCtx.GetData().(*RedmineGoTaskData)
	connectionId := data.Options.ConnectionId

	instance, err := loadMappingInstance(db, connectionId)
	if err != nil {
		return err
	}
	mapper := mapping.NewMapper(&data.ScopeConfig.Transformation, instance)

	changelogIdGen := didgen.NewDomainIdGenerator(&models.RedmineGoJournalDetail{})
	issueIdGen := didgen.NewDomainIdGenerator(&models.RedmineGoIssue{})
	accountIdGen := didgen.NewDomainIdGenerator(&models.RedmineGoAccount{})

	// Replace the scope's changelogs even when this run emits none: the divider
	// replaces rows only once it sees one, so an issue set that lost all its
	// journals would otherwise keep the old rows.
	params := utils.ToJsonString(models.RedmineGoApiParams{
		ConnectionId: connectionId,
		ProjectId:    data.Options.ProjectId,
	})
	if err := db.Delete(&ticket.IssueChangelogs{}, dal.Where(
		"_raw_data_table = ? AND _raw_data_params = ?", rawTableName(RAW_ISSUE_DETAILS_TABLE), params,
	)); err != nil {
		return err
	}

	// Scope the changelogs to the project through their parent issue.
	scope := []dal.Clause{
		dal.From("_tool_redmine_go_journal_details d"),
		dal.Join("INNER JOIN _tool_redmine_go_issues i ON (i.connection_id = d.connection_id AND i.id = d.issue_id)"),
		dal.Where("d.connection_id = ? AND i.project_id = ?", connectionId, data.Options.ProjectId),
	}
	cursor, err := db.Cursor(append([]dal.Clause{dal.Select("d.*")}, scope...)...)
	if err != nil {
		return err
	}
	defer cursor.Close()

	converter, err := helper.NewDataConverter(helper.DataConverterArgs{
		RawDataSubTaskArgs: helper.RawDataSubTaskArgs{
			Ctx: withCountedTotal(taskCtx, scope...),
			Options: models.RedmineGoApiParams{
				ConnectionId: connectionId,
				ProjectId:    data.Options.ProjectId,
			},
			Table: RAW_ISSUE_DETAILS_TABLE,
		},
		InputRowType: reflect.TypeOf(models.RedmineGoJournalDetail{}),
		Input:        cursor,
		Convert: func(inputRow interface{}) ([]interface{}, errors.Error) {
			detail := inputRow.(*models.RedmineGoJournalDetail)
			changelog := &ticket.IssueChangelogs{
				DomainEntity:      domainlayer.DomainEntity{Id: changelogIdGen.Generate(connectionId, detail.JournalId, detail.DetailKey)},
				IssueId:           issueIdGen.Generate(connectionId, detail.IssueId),
				AuthorName:        detail.UserName,
				FieldId:           changelogFieldId(detail),
				FieldName:         detail.FieldName,
				FromValue:         detail.OldLabel,
				ToValue:           detail.NewLabel,
				OriginalFromValue: detail.OldValue,
				OriginalToValue:   detail.NewValue,
				CreatedDate:       detail.RedmineCreatedOn,
			}
			if detail.UserId != 0 {
				changelog.AuthorId = accountIdGen.Generate(connectionId, detail.UserId)
			}
			if detail.Property == "attr" {
				switch detail.Name {
				case "status_id":
					changelog.FromValue = standardStatus(mapper, detail.OldValue, detail.OldLabel)
					changelog.ToValue = standardStatus(mapper, detail.NewValue, detail.NewLabel)
					changelog.OriginalFromValue = detail.OldLabel
					changelog.OriginalToValue = detail.NewLabel
				case "assigned_to_id":
					changelog.OriginalFromValue = accountDomainId(accountIdGen, connectionId, detail.OldValue)
					changelog.OriginalToValue = accountDomainId(accountIdGen, connectionId, detail.NewValue)
				}
			}
			return []interface{}{changelog}, nil
		},
	})
	if err != nil {
		return err
	}
	return converter.Execute()
}

// changelogFieldId is a stable identifier of the changed field, independent of
// its display name: the attribute name, `cf_<id>` for a custom field, and
// `relation_<type>` for a relation.
func changelogFieldId(detail *models.RedmineGoJournalDetail) string {
	switch detail.Property {
	case "attr":
		return detail.Name
	case "cf":
		return "cf_" + detail.Name
	case "relation":
		return "relation_" + detail.Name
	default:
		return detail.Property + "_" + detail.Name
	}
}

// standardStatus maps a status id of a journal value to a standard status. A
// change that names no status (an empty side) stays empty.
func standardStatus(mapper *mapping.Mapper, rawId, name string) string {
	if rawId == "" {
		return ""
	}
	id, err := strconv.Atoi(rawId)
	if err != nil {
		return ""
	}
	return mapper.Status(id, name, false)
}

// accountDomainId returns the domain account id of a user id value, "" for an
// empty or non-numeric value.
func accountDomainId(idGen *didgen.DomainIdGenerator, connectionId uint64, rawId string) string {
	id, err := strconv.ParseUint(rawId, 10, 64)
	if err != nil {
		return ""
	}
	return idGen.Generate(connectionId, id)
}
