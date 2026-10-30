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
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// ExtractIssueRelationsMeta describes the relation extractor of issue details.
var ExtractIssueRelationsMeta = plugin.SubTaskMeta{
	Name:             "extractIssueRelations",
	EntryPoint:       ExtractIssueRelations,
	EnabledByDefault: true,
	Description:      "Extract the relations of the issues collected in this run into _tool_redmine_go_issue_relations",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
	DependencyTables: []string{rawTableName(RAW_ISSUE_DETAILS_TABLE)},
	ProductTables:    []string{models.RedmineGoIssueRelation{}.TableName()},
}

// ExtractIssueChangesetsMeta describes the changeset extractor of issue details.
var ExtractIssueChangesetsMeta = plugin.SubTaskMeta{
	Name:             "extractIssueChangesets",
	EntryPoint:       ExtractIssueChangesets,
	EnabledByDefault: true,
	Description:      "Extract the repository changesets linked to the issues collected in this run into _tool_redmine_go_issue_changesets",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_CROSS},
	DependencyTables: []string{rawTableName(RAW_ISSUE_DETAILS_TABLE)},
	ProductTables:    []string{models.RedmineGoIssueChangeset{}.TableName(), models.RedmineGoAccount{}.TableName()},
}

var (
	_ plugin.SubTaskEntryPoint = ExtractIssueRelations
	_ plugin.SubTaskEntryPoint = ExtractIssueChangesets
)

// ExtractIssueRelations extracts the relations an issue detail lists, for the
// issues collected in this run. It keeps only the relations whose source is
// the issue itself, since a relation also appears under its target. Relations
// of unchanged issues, and relations removed in Redmine, are the reconciliation
// sweep's job: a relation change does not bump the issue's updated_on, so this
// extractor only sees them when the issue changes for another reason.
func ExtractIssueRelations(taskCtx plugin.SubTaskContext) errors.Error {
	data := taskCtx.GetData().(*RedmineGoTaskData)
	return extractIssueDetails(taskCtx, detailExtraction{
		Config:   issueExtractorConfig{Version: extractorVersion},
		Replaced: []interface{}{&models.RedmineGoIssueRelation{}},
		Rows: func(item *apiIssue) []interface{} {
			projectId := data.Options.ProjectId
			if item.Project != nil && item.Project.Id != 0 {
				projectId = item.Project.Id
			}
			var rows []interface{}
			for i := range item.Relations {
				if row := relationRow(data.Options.ConnectionId, projectId, item.Id, &item.Relations[i]); row != nil {
					rows = append(rows, row)
				}
			}
			return rows
		},
	})
}

// relationRow maps a relation to its tool row, or nil when the relation is not
// owned by the issue that lists it, or is unreadable.
func relationRow(connectionId, projectId, issueId uint64, relation *apiRelation) *models.RedmineGoIssueRelation {
	if relation.Id == 0 || relation.IssueId != issueId || relation.IssueToId == 0 {
		return nil
	}
	return &models.RedmineGoIssueRelation{
		ConnectionId: connectionId,
		Id:           relation.Id,
		ProjectId:    projectId,
		IssueId:      relation.IssueId,
		IssueToId:    relation.IssueToId,
		RelationType: relation.RelationType,
		Delay:        relation.Delay,
	}
}

// ExtractIssueChangesets extracts the changesets linked to the issues
// collected in this run. Redmine lists them on the issue detail only.
func ExtractIssueChangesets(taskCtx plugin.SubTaskContext) errors.Error {
	data := taskCtx.GetData().(*RedmineGoTaskData)
	connectionId := data.Options.ConnectionId
	return extractIssueDetails(taskCtx, detailExtraction{
		Config:   issueExtractorConfig{Version: extractorVersion},
		Replaced: []interface{}{&models.RedmineGoIssueChangeset{}},
		Rows: func(item *apiIssue) []interface{} {
			var rows []interface{}
			for _, changeset := range item.Changesets {
				if changeset.Revision == "" {
					continue
				}
				row := &models.RedmineGoIssueChangeset{
					ConnectionId: connectionId,
					IssueId:      item.Id,
					Revision:     changeset.Revision,
					Comments:     changeset.Comments,
				}
				if changeset.CommittedOn != nil {
					committed := changeset.CommittedOn.UTC()
					row.CommittedOn = &committed
				}
				if changeset.User != nil {
					row.UserId, row.UserName = changeset.User.Id, changeset.User.Name
					if account := accountRow(connectionId, changeset.User); account != nil {
						rows = append(rows, account)
					}
				}
				rows = append(rows, row)
			}
			return rows
		},
	})
}
