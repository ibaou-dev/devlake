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

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/models/domainlayer"
	"github.com/apache/devlake/core/models/domainlayer/didgen"
	"github.com/apache/devlake/core/models/domainlayer/ticket"
	"github.com/apache/devlake/core/plugin"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// ConvertSprintsMeta describes the versions-as-sprints convertor.
var ConvertSprintsMeta = plugin.SubTaskMeta{
	Name:             "convertSprints",
	EntryPoint:       ConvertSprints,
	EnabledByDefault: true,
	Description:      "Convert Redmine versions into sprints, board_sprints and sprint_issues: the target version of an issue is its sprint",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
	DependencyTables: []string{models.RedmineGoVersion{}.TableName(), models.RedmineGoIssue{}.TableName()},
	ProductTables: []string{
		ticket.Sprint{}.TableName(),
		ticket.BoardSprint{}.TableName(),
		ticket.SprintIssue{}.TableName(),
	},
}

var _ plugin.SubTaskEntryPoint = ConvertSprints

// Redmine version statuses.
const (
	versionOpen   = "open"
	versionLocked = "locked"
	versionClosed = "closed"
)

// sprintStatus maps a Redmine version status to a sprint status: an open
// version is ACTIVE, and a locked or closed one is CLOSED, since neither takes
// new issues. An unknown status counts as open.
func sprintStatus(versionStatus string) string {
	switch versionStatus {
	case versionLocked, versionClosed:
		return "CLOSED"
	default:
		return "ACTIVE"
	}
}

// sprintRow maps a version to a sprint. Redmine versions have no start date,
// so the sprint starts when the version was created; the due date is the
// planned end, and the completion of a version that no longer takes issues.
func sprintRow(connectionId uint64, endpoint, boardId string, version *models.RedmineGoVersion, idGen *didgen.DomainIdGenerator) *ticket.Sprint {
	sprint := &ticket.Sprint{
		DomainEntity:    domainlayer.DomainEntity{Id: idGen.Generate(connectionId, version.Id)},
		Name:            version.Name,
		Url:             versionUrl(endpoint, version.Id),
		Status:          sprintStatus(version.Status),
		StartedDate:     version.RedmineCreatedOn,
		EndedDate:       version.DueDate,
		OriginalBoardID: boardId,
	}
	if sprint.Status == "CLOSED" {
		sprint.CompletedDate = version.DueDate
	}
	return sprint
}

// ConvertSprints turns the versions of the scope's project into sprints, when
// the scope config's versionsAsSprints is on: the versions the project owns
// and the ones its issues target, each attached to the project's board, and
// every issue targeting a known version into that version's sprint.
func ConvertSprints(taskCtx plugin.SubTaskContext) errors.Error {
	db := taskCtx.GetDal()
	data := taskCtx.GetData().(*RedmineGoTaskData)
	connectionId, projectId := data.Options.ConnectionId, data.Options.ProjectId
	boardId := didgen.NewDomainIdGenerator(&models.RedmineGoProject{}).Generate(connectionId, projectId)
	sprintIdGen := didgen.NewDomainIdGenerator(&models.RedmineGoVersion{})
	issueIdGen := didgen.NewDomainIdGenerator(&models.RedmineGoIssue{})
	params := scopeParams(data)

	if err := deleteDomainByOrigin(db, &ticket.Sprint{}, []string{rawTableName(RAW_VERSIONS_TABLE)}, params); err != nil {
		return err
	}
	if err := db.Delete(&ticket.BoardSprint{}, dal.Where("board_id = ?", boardId)); err != nil {
		return err
	}
	versions, err := db.Cursor(
		dal.From(&models.RedmineGoVersion{}),
		dal.Where(`connection_id = ? AND (project_id = ? OR id IN (
			SELECT fixed_version_id FROM _tool_redmine_go_issues WHERE connection_id = ? AND project_id = ? AND fixed_version_id <> 0))`,
			connectionId, projectId, connectionId, projectId),
	)
	if err != nil {
		return err
	}
	defer versions.Close()
	versionConverter, err := helper.NewDataConverter(helper.DataConverterArgs{
		RawDataSubTaskArgs: scopeRawArgs(taskCtx, data, RAW_VERSIONS_TABLE),
		InputRowType:       reflect.TypeOf(models.RedmineGoVersion{}),
		Input:              versions,
		Convert: func(inputRow interface{}) ([]interface{}, errors.Error) {
			version := inputRow.(*models.RedmineGoVersion)
			sprint := sprintRow(connectionId, data.Connection.Endpoint, boardId, version, sprintIdGen)
			return []interface{}{sprint, &ticket.BoardSprint{BoardId: boardId, SprintId: sprint.Id}}, nil
		},
	})
	if err != nil {
		return err
	}
	if err := versionConverter.Execute(); err != nil {
		return err
	}

	if err := deleteDomainByOrigin(db, &ticket.SprintIssue{}, []string{rawTableName(RAW_ISSUES_TABLE)}, params); err != nil {
		return err
	}
	issues, err := db.Cursor(
		dal.Select("i.*"),
		dal.From("_tool_redmine_go_issues i"),
		dal.Join("INNER JOIN _tool_redmine_go_versions v ON (v.connection_id = i.connection_id AND v.id = i.fixed_version_id)"),
		dal.Where("i.connection_id = ? AND i.project_id = ?", connectionId, projectId),
	)
	if err != nil {
		return err
	}
	defer issues.Close()
	issueConverter, err := helper.NewDataConverter(helper.DataConverterArgs{
		RawDataSubTaskArgs: scopeRawArgs(taskCtx, data, RAW_ISSUES_TABLE),
		InputRowType:       reflect.TypeOf(models.RedmineGoIssue{}),
		Input:              issues,
		Convert: func(inputRow interface{}) ([]interface{}, errors.Error) {
			issue := inputRow.(*models.RedmineGoIssue)
			return []interface{}{&ticket.SprintIssue{
				SprintId: sprintIdGen.Generate(connectionId, issue.FixedVersionId),
				IssueId:  issueIdGen.Generate(connectionId, issue.Id),
			}}, nil
		},
	})
	if err != nil {
		return err
	}
	return issueConverter.Execute()
}
