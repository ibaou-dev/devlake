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
	"math"
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

// FieldEnvironmentClass is the field_id of the environment class rows written
// to issue_custom_array_fields.
const FieldEnvironmentClass = "environment_class"

// ConvertIssuesMeta describes the issue convertor.
var ConvertIssuesMeta = plugin.SubTaskMeta{
	Name:             "convertIssues",
	EntryPoint:       ConvertIssues,
	EnabledByDefault: true,
	Description:      "Convert _tool_redmine_go_issues into the domain layer tables issues, board_issues, issue_assignees and issue_custom_array_fields, applying the scope config's mapping",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
	DependencyTables: []string{
		models.RedmineGoIssue{}.TableName(),
		models.RedmineGoJournalDetail{}.TableName(),
		models.RedmineGoTracker{}.TableName(),
		models.RedmineGoStatus{}.TableName(),
		models.RedmineGoCustomField{}.TableName(),
		rawTableName(RAW_ISSUES_TABLE),
	},
	ProductTables: []string{
		ticket.Issue{}.TableName(),
		ticket.BoardIssue{}.TableName(),
		ticket.IssueAssignee{}.TableName(),
		ticket.IssueCustomArrayField{}.TableName(),
	},
}

var _ plugin.SubTaskEntryPoint = ConvertIssues

// ConvertIssues converts the scope's issues. Types, statuses, severity,
// component and environment class come from the scope config's mapping, with
// the built-in fallbacks for whatever it leaves unmapped, evaluated here and
// not at extraction, so changing the mapping needs no re-extraction.
func ConvertIssues(taskCtx plugin.SubTaskContext) errors.Error {
	db := taskCtx.GetDal()
	data := taskCtx.GetData().(*RedmineGoTaskData)
	connectionId := data.Options.ConnectionId
	if data.Project == nil {
		return errors.Default.New("redmine_go task data carries no project scope")
	}

	instance, err := loadMappingInstance(db, connectionId)
	if err != nil {
		return err
	}
	mapper := mapping.NewMapper(&data.ScopeConfig.Transformation, instance)

	issueIdGen := didgen.NewDomainIdGenerator(&models.RedmineGoIssue{})
	accountIdGen := didgen.NewDomainIdGenerator(&models.RedmineGoAccount{})
	boardId := didgen.NewDomainIdGenerator(&models.RedmineGoProject{}).Generate(connectionId, data.Options.ProjectId)

	// The divider replaces a domain table's rows only when the run emits a row
	// of that table. Assignees and environment classes are emitted for some
	// issues only, so when none qualifies any more their old rows would
	// survive: replace them explicitly.
	params := utils.ToJsonString(models.RedmineGoApiParams{
		ConnectionId: connectionId,
		ProjectId:    data.Options.ProjectId,
	})
	for _, table := range []interface{}{&ticket.IssueAssignee{}, &ticket.IssueCustomArrayField{}} {
		if err := db.Delete(table, dal.Where("_raw_data_table = ? AND _raw_data_params = ?", rawTableName(RAW_ISSUES_TABLE), params)); err != nil {
			return err
		}
	}

	cursor, err := db.Cursor(
		dal.From(&models.RedmineGoIssue{}),
		dal.Where("connection_id = ? AND project_id = ?", connectionId, data.Options.ProjectId),
	)
	if err != nil {
		return err
	}
	defer cursor.Close()

	converter, err := helper.NewDataConverter(helper.DataConverterArgs{
		RawDataSubTaskArgs: helper.RawDataSubTaskArgs{
			Ctx: taskCtx,
			Options: models.RedmineGoApiParams{
				ConnectionId: connectionId,
				ProjectId:    data.Options.ProjectId,
			},
			Table: RAW_ISSUES_TABLE,
		},
		InputRowType: reflect.TypeOf(models.RedmineGoIssue{}),
		Input:        cursor,
		Convert: func(inputRow interface{}) ([]interface{}, errors.Error) {
			issue := inputRow.(*models.RedmineGoIssue)
			input := mappingIssue(issue)
			stdType := mapper.IssueType(input)
			closed := mapper.StatusIsClosed(int(issue.StatusId), issue.StatusIsClosed)

			var changes []mapping.StatusChange
			if issue.RedmineClosedOn == nil && closed {
				var err errors.Error
				if changes, err = statusChanges(db, connectionId, issue.Id); err != nil {
					return nil, err
				}
			}
			resolution := mapper.ResolutionDate(issue.RedmineClosedOn, closed, changes)

			created, updated := issue.RedmineCreatedOn, issue.RedmineUpdatedOn
			domainIssue := &ticket.Issue{
				DomainEntity:            domainlayer.DomainEntity{Id: issueIdGen.Generate(connectionId, issue.Id)},
				Url:                     issueUrl(data.Connection.Endpoint, issue.Id),
				IssueKey:                idString(issue.Id),
				Title:                   issue.Subject,
				Description:             issue.Description,
				Type:                    stdType,
				OriginalType:            issue.TrackerName,
				Status:                  mapper.Status(int(issue.StatusId), issue.StatusName, issue.StatusIsClosed),
				OriginalStatus:          issue.StatusName,
				Priority:                issue.PriorityName,
				Severity:                mapper.Severity(input),
				Component:               mapper.Component(input),
				ResolutionDate:          resolution,
				CreatedDate:             &created,
				UpdatedDate:             &updated,
				LeadTimeMinutes:         mapping.LeadTimeMinutes(created, resolution),
				OriginalEstimateMinutes: hoursToMinutes(issue.EstimatedHours),
				TimeSpentMinutes:        hoursToMinutes(issue.SpentHours),
				IsSubtask:               stdType == ticket.SUBTASK,
				DueDate:                 issue.DueDate,
				FixVersions:             issue.FixedVersionName,
				OriginalProject:         data.Project.Identifier,
			}
			if issue.ParentId != 0 {
				domainIssue.ParentIssueId = issueIdGen.Generate(connectionId, issue.ParentId)
			}
			if issue.AuthorId != 0 {
				domainIssue.CreatorId = accountIdGen.Generate(connectionId, issue.AuthorId)
				domainIssue.CreatorName = issue.AuthorName
			}
			if issue.AssigneeId != 0 {
				domainIssue.AssigneeId = accountIdGen.Generate(connectionId, issue.AssigneeId)
				domainIssue.AssigneeName = issue.AssigneeName
			}

			results := []interface{}{
				domainIssue,
				&ticket.BoardIssue{BoardId: boardId, IssueId: domainIssue.Id},
			}
			if domainIssue.AssigneeId != "" {
				results = append(results, &ticket.IssueAssignee{
					IssueId:      domainIssue.Id,
					AssigneeId:   domainIssue.AssigneeId,
					AssigneeName: domainIssue.AssigneeName,
				})
			}
			for _, class := range mapper.EnvironmentClasses(input) {
				results = append(results, &ticket.IssueCustomArrayField{
					IssueId:    domainIssue.Id,
					FieldId:    FieldEnvironmentClass,
					FieldValue: class,
				})
			}
			return results, nil
		},
	})
	if err != nil {
		return err
	}
	return converter.Execute()
}

// hoursToMinutes converts Redmine's decimal hours to whole minutes, nil when
// the issue has no value.
func hoursToMinutes(hours *float64) *int64 {
	if hours == nil {
		return nil
	}
	minutes := int64(math.Round(*hours * 60))
	return &minutes
}

// statusChanges lists the issue's transitions into a status, from its
// journals, for the resolution-date fallback.
func statusChanges(db dal.Dal, connectionId, issueId uint64) ([]mapping.StatusChange, errors.Error) {
	var details []models.RedmineGoJournalDetail
	if err := db.All(&details, dal.Where(
		"connection_id = ? AND issue_id = ? AND property = ? AND name = ?",
		connectionId, issueId, "attr", "status_id",
	)); err != nil {
		return nil, err
	}
	changes := make([]mapping.StatusChange, 0, len(details))
	for _, detail := range details {
		statusId, parseErr := strconv.ParseUint(detail.NewValue, 10, 64)
		if parseErr != nil {
			continue
		}
		changes = append(changes, mapping.StatusChange{At: detail.RedmineCreatedOn, ToStatusID: int(statusId)})
	}
	return changes, nil
}
