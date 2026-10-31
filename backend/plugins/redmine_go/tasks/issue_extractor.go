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
	"encoding/json"
	"time"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// extractorVersion identifies the extraction logic of the stateful
// extractors. It is part of their subtask config: raising it makes the next
// run re-extract every raw row, so a fix to an extractor reaches rows that
// were extracted before it.
const extractorVersion = 1

// issueExtractorConfig is the subtask config of the issue extractor.
type issueExtractorConfig struct {
	Version int
}

// apiIssue decodes an issue as the list (`issues.json`) and the detail
// (`issues/:id.json`) return it; only the detail carries journals.
type apiIssue struct {
	Id                  uint64          `json:"id"`
	Project             *apiRef         `json:"project"`
	Tracker             *apiRef         `json:"tracker"`
	Status              *apiStatusRef   `json:"status"`
	Priority            *apiRef         `json:"priority"`
	Author              *apiRef         `json:"author"`
	AssignedTo          *apiRef         `json:"assigned_to"`
	Category            *apiRef         `json:"category"`
	FixedVersion        *apiRef         `json:"fixed_version"`
	Parent              *apiRef         `json:"parent"`
	Subject             string          `json:"subject"`
	Description         string          `json:"description"`
	StartDate           string          `json:"start_date"`
	DueDate             string          `json:"due_date"`
	DoneRatio           int             `json:"done_ratio"`
	IsPrivate           bool            `json:"is_private"`
	EstimatedHours      *float64        `json:"estimated_hours"`
	TotalEstimatedHours *float64        `json:"total_estimated_hours"`
	SpentHours          *float64        `json:"spent_hours"`
	TotalSpentHours     *float64        `json:"total_spent_hours"`
	CustomFields        json.RawMessage `json:"custom_fields"`
	CreatedOn           time.Time       `json:"created_on"`
	UpdatedOn           time.Time       `json:"updated_on"`
	ClosedOn            *time.Time      `json:"closed_on"`
	Journals            []apiJournal    `json:"journals"`
	Relations           []apiRelation   `json:"relations"`
	Changesets          []apiChangeset  `json:"changesets"`
}

// apiRelation is an issue relation as the list and the detail return it. The
// relation appears under both of its issues with the same id; relation_type
// reads from issue_id to issue_to_id.
type apiRelation struct {
	Id           uint64 `json:"id"`
	IssueId      uint64 `json:"issue_id"`
	IssueToId    uint64 `json:"issue_to_id"`
	RelationType string `json:"relation_type"`
	Delay        *int   `json:"delay"`
}

// apiChangeset is a repository changeset linked to an issue, as the detail
// returns it. The user is absent when the committer maps to no Redmine user.
type apiChangeset struct {
	Revision    string     `json:"revision"`
	User        *apiRef    `json:"user"`
	Comments    string     `json:"comments"`
	CommittedOn *time.Time `json:"committed_on"`
}

// apiCustomFieldValue is one element of an issue's custom_fields array.
type apiCustomFieldValue struct {
	Id       uint64          `json:"id"`
	Name     string          `json:"name"`
	Multiple bool            `json:"multiple"`
	Value    json.RawMessage `json:"value"`
}

// ExtractIssuesMeta describes the issue extractor.
var ExtractIssuesMeta = plugin.SubTaskMeta{
	Name:             "extractIssues",
	EntryPoint:       ExtractIssues,
	EnabledByDefault: true,
	Description:      "Extract issues into _tool_redmine_go_issues, with the accounts and custom fields they mention",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET, plugin.DOMAIN_TYPE_CROSS},
	DependencyTables: []string{rawTableName(RAW_ISSUES_TABLE)},
	ProductTables: []string{
		models.RedmineGoIssue{}.TableName(),
		models.RedmineGoAccount{}.TableName(),
		models.RedmineGoObservedField{}.TableName(),
	},
}

var _ plugin.SubTaskEntryPoint = ExtractIssues

// ExtractIssues extracts the collected issue rows. It keeps every standard
// field and the custom fields verbatim; mapping to standard types and
// statuses happens at conversion. A re-collected issue has several raw rows,
// and they apply in collection order, so the newest wins by upsert.
func ExtractIssues(taskCtx plugin.SubTaskContext) errors.Error {
	data := taskCtx.GetData().(*RedmineGoTaskData)
	connectionId := data.Options.ConnectionId
	extractor, err := helper.NewStatefulApiExtractor(&helper.StatefulApiExtractorArgs[apiIssue]{
		SubtaskCommonArgs: &helper.SubtaskCommonArgs{
			SubTaskContext: withRawRowsTotal(taskCtx, rawTableName(RAW_ISSUES_TABLE), scopeParams(data)),
			Table:          RAW_ISSUES_TABLE,
			Params: models.RedmineGoApiParams{
				ConnectionId: connectionId,
				ProjectId:    data.Options.ProjectId,
			},
			SubtaskConfig: issueExtractorConfig{Version: extractorVersion},
		},
		Extract: func(item *apiIssue, row *helper.RawData) ([]interface{}, errors.Error) {
			issue := issueRow(connectionId, data.Options.ProjectId, item)
			results := []interface{}{issue}
			for _, user := range []*apiRef{item.Author, item.AssignedTo} {
				if account := accountRow(connectionId, user); account != nil {
					results = append(results, account)
				}
			}
			var fields []apiCustomFieldValue
			if len(item.CustomFields) > 0 {
				if err := json.Unmarshal(item.CustomFields, &fields); err != nil {
					return nil, errors.Default.Wrap(err, "could not decode custom_fields of issue "+idString(item.Id))
				}
			}
			for _, field := range fields {
				results = append(results, &models.RedmineGoObservedField{
					ConnectionId: connectionId, Id: field.Id, Name: field.Name, Multiple: field.Multiple,
				})
			}
			return results, nil
		},
	})
	if err != nil {
		return err
	}
	if err := extractor.Execute(); err != nil {
		return err
	}
	if data.FullIssueRun {
		stale, err := countStaleIssues(taskCtx.GetDal(), connectionId, data.Options.ProjectId, scopeParams(data))
		if err != nil {
			return err
		}
		data.FullIssueRun = stale == 0
	}
	return nil
}

// countStaleIssues counts the project's issue rows that no current raw issue
// row produced. A full collection replaces the raw rows, so such a row is an
// issue the list no longer returns: deleted in Redmine or moved away. A
// stateful extractor in full-sync mode deletes it, but one in incremental mode
// does not, and a first run is incremental: the extractor starts from the
// state its collector saved. The sweep is skipped only when none is left.
func countStaleIssues(db dal.Dal, connectionId, projectId uint64, params string) (int64, errors.Error) {
	return db.Count(
		dal.From(&models.RedmineGoIssue{}),
		dal.Where("connection_id = ? AND project_id = ?", connectionId, projectId),
		dal.Where("(_raw_data_id IS NULL OR _raw_data_id NOT IN (SELECT id FROM "+rawTableName(RAW_ISSUES_TABLE)+" WHERE params = ?))", params),
	)
}

// issueRow maps an API issue to its tool row. The project is the issue's own,
// which differs from the scope only for an issue moved since it was listed.
func issueRow(connectionId, scopeProjectId uint64, item *apiIssue) *models.RedmineGoIssue {
	issue := &models.RedmineGoIssue{
		ConnectionId:        connectionId,
		Id:                  item.Id,
		ProjectId:           scopeProjectId,
		Subject:             item.Subject,
		Description:         item.Description,
		StartDate:           parseDate(item.StartDate),
		DueDate:             parseDate(item.DueDate),
		DoneRatio:           item.DoneRatio,
		IsPrivate:           item.IsPrivate,
		EstimatedHours:      item.EstimatedHours,
		TotalEstimatedHours: item.TotalEstimatedHours,
		SpentHours:          item.SpentHours,
		TotalSpentHours:     item.TotalSpentHours,
		CustomFieldsJson:    jsonText(item.CustomFields),
		RedmineCreatedOn:    item.CreatedOn.UTC(),
		RedmineUpdatedOn:    item.UpdatedOn.UTC(),
	}
	if item.ClosedOn != nil {
		closed := item.ClosedOn.UTC()
		issue.RedmineClosedOn = &closed
	}
	if item.Project != nil {
		issue.ProjectId, issue.ProjectName = item.Project.Id, item.Project.Name
	}
	if item.Tracker != nil {
		issue.TrackerId, issue.TrackerName = item.Tracker.Id, item.Tracker.Name
	}
	if item.Status != nil {
		issue.StatusId, issue.StatusName, issue.StatusIsClosed = item.Status.Id, item.Status.Name, item.Status.IsClosed
	}
	if item.Priority != nil {
		issue.PriorityId, issue.PriorityName = item.Priority.Id, item.Priority.Name
	}
	if item.Author != nil {
		issue.AuthorId, issue.AuthorName = item.Author.Id, item.Author.Name
	}
	if item.AssignedTo != nil {
		issue.AssigneeId, issue.AssigneeName = item.AssignedTo.Id, item.AssignedTo.Name
	}
	if item.Category != nil {
		issue.CategoryId, issue.CategoryName = item.Category.Id, item.Category.Name
	}
	if item.FixedVersion != nil {
		issue.FixedVersionId, issue.FixedVersionName = item.FixedVersion.Id, item.FixedVersion.Name
	}
	if item.Parent != nil {
		issue.ParentId = item.Parent.Id
	}
	return issue
}

// accountRow derives a tool account from an embedded {id, name} user, or nil
// when the reference is absent.
func accountRow(connectionId uint64, user *apiRef) *models.RedmineGoAccount {
	if user == nil || user.Id == 0 {
		return nil
	}
	return &models.RedmineGoAccount{ConnectionId: connectionId, Id: user.Id, Name: user.Name}
}
