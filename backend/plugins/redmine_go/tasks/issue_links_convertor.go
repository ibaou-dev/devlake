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
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/models/domainlayer"
	"github.com/apache/devlake/core/models/domainlayer/crossdomain"
	"github.com/apache/devlake/core/models/domainlayer/didgen"
	"github.com/apache/devlake/core/models/domainlayer/ticket"
	"github.com/apache/devlake/core/plugin"
	"github.com/apache/devlake/core/utils"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// ConvertIssueRelationshipsMeta describes the relationship convertor.
var ConvertIssueRelationshipsMeta = plugin.SubTaskMeta{
	Name:             "convertIssueRelationships",
	EntryPoint:       ConvertIssueRelationships,
	EnabledByDefault: true,
	Description:      "Convert _tool_redmine_go_issue_relations into the domain layer table issue_relationships",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
	DependencyTables: []string{models.RedmineGoIssueRelation{}.TableName()},
	ProductTables:    []string{ticket.IssueRelationship{}.TableName()},
}

// ConvertIssueCommentsMeta describes the comment convertor.
var ConvertIssueCommentsMeta = plugin.SubTaskMeta{
	Name:             "convertIssueComments",
	EntryPoint:       ConvertIssueComments,
	EnabledByDefault: true,
	Description:      "Convert the notes of _tool_redmine_go_journals into the domain layer table issue_comments; private notes only when the scope config includes them",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
	DependencyTables: []string{models.RedmineGoJournal{}.TableName(), models.RedmineGoIssue{}.TableName()},
	ProductTables:    []string{ticket.IssueComment{}.TableName()},
}

// ConvertIssueCommitsMeta describes the issue-commit convertor.
var ConvertIssueCommitsMeta = plugin.SubTaskMeta{
	Name:             "convertIssueCommits",
	EntryPoint:       ConvertIssueCommits,
	EnabledByDefault: true,
	Description:      "Convert _tool_redmine_go_issue_changesets into the domain layer table issue_commits",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_CROSS},
	DependencyTables: []string{models.RedmineGoIssueChangeset{}.TableName(), models.RedmineGoIssue{}.TableName()},
	ProductTables:    []string{crossdomain.IssueCommit{}.TableName()},
}

// ConvertWorklogsMeta describes the worklog convertor.
var ConvertWorklogsMeta = plugin.SubTaskMeta{
	Name:             "convertWorklogs",
	EntryPoint:       ConvertWorklogs,
	EnabledByDefault: true,
	Description:      "Convert _tool_redmine_go_time_entries into the domain layer table issue_worklogs",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
	DependencyTables: []string{models.RedmineGoTimeEntry{}.TableName(), models.RedmineGoIssue{}.TableName()},
	ProductTables:    []string{ticket.IssueWorklog{}.TableName()},
}

var (
	_ plugin.SubTaskEntryPoint = ConvertIssueRelationships
	_ plugin.SubTaskEntryPoint = ConvertIssueComments
	_ plugin.SubTaskEntryPoint = ConvertIssueCommits
	_ plugin.SubTaskEntryPoint = ConvertWorklogs
)

// scopeParams is the params value of the raw rows of the task's project, which
// domain rows inherit through their origin.
func scopeParams(data *RedmineGoTaskData) string {
	return utils.ToJsonString(models.RedmineGoApiParams{
		ConnectionId: data.Options.ConnectionId,
		ProjectId:    data.Options.ProjectId,
	})
}

// deleteDomainByOrigin deletes the rows of a domain table that were derived
// from the given raw tables for the project. The divider does the same, but
// only once a run emits a row of the table, so a project whose last row
// vanished would otherwise keep it.
func deleteDomainByOrigin(db dal.Dal, model interface{}, rawTables []string, params string) errors.Error {
	return db.Delete(model, dal.Where("_raw_data_table IN ? AND _raw_data_params = ?", rawTables, params))
}

// issuesInScope is the join that restricts a tool table with connection_id and
// issue_id columns to the issues of the task's project.
const issuesInScope = "INNER JOIN _tool_redmine_go_issues i ON (i.connection_id = %s.connection_id AND i.id = %s.issue_id)"

// scopedClauses selects the rows of a tool table (alias given) that belong to
// issues of the task's project, without a column list so the same clauses
// serve a count and a cursor.
func scopedClauses(table, alias string, data *RedmineGoTaskData, extra ...dal.Clause) []dal.Clause {
	clauses := []dal.Clause{
		dal.From(table + " " + alias),
		dal.Join(fmt.Sprintf(issuesInScope, alias, alias)),
		dal.Where(alias+".connection_id = ? AND i.project_id = ?", data.Options.ConnectionId, data.Options.ProjectId),
	}
	return append(clauses, extra...)
}

// scopedRows opens a cursor over the rows of a tool table (alias given) that
// belong to issues of the task's project.
func scopedRows(db dal.Dal, alias string, clauses []dal.Clause) (dal.Rows, errors.Error) {
	return db.Cursor(append([]dal.Clause{dal.Select(alias + ".*")}, clauses...)...)
}

// ConvertIssueRelationships converts the relations of the scope's project to
// issue_relationships: source and target issue ids with Redmine's relation
// type, read from the source to the target. One row per relation, so a
// `relates` pair is never doubled.
func ConvertIssueRelationships(taskCtx plugin.SubTaskContext) errors.Error {
	db := taskCtx.GetDal()
	data := taskCtx.GetData().(*RedmineGoTaskData)
	connectionId := data.Options.ConnectionId
	issueIdGen := didgen.NewDomainIdGenerator(&models.RedmineGoIssue{})

	// Relations come from issue details and from the sweep, so their tool rows
	// carry either raw table as origin.
	if err := deleteDomainByOrigin(db, &ticket.IssueRelationship{},
		[]string{rawTableName(RAW_ISSUE_DETAILS_TABLE), rawTableName(RAW_ISSUE_SWEEP_TABLE)}, scopeParams(data)); err != nil {
		return err
	}
	scope := []dal.Clause{
		dal.From(&models.RedmineGoIssueRelation{}),
		dal.Where("connection_id = ? AND project_id = ?", connectionId, data.Options.ProjectId),
	}
	cursor, err := db.Cursor(scope...)
	if err != nil {
		return err
	}
	defer cursor.Close()
	converter, err := helper.NewDataConverter(helper.DataConverterArgs{
		RawDataSubTaskArgs: scopeRawArgs(withCountedTotal(taskCtx, scope...), data, RAW_ISSUE_DETAILS_TABLE),
		InputRowType:       reflect.TypeOf(models.RedmineGoIssueRelation{}),
		Input:              cursor,
		Convert: func(inputRow interface{}) ([]interface{}, errors.Error) {
			relation := inputRow.(*models.RedmineGoIssueRelation)
			return []interface{}{&ticket.IssueRelationship{
				SourceIssueId: issueIdGen.Generate(connectionId, relation.IssueId),
				TargetIssueId: issueIdGen.Generate(connectionId, relation.IssueToId),
				OriginalType:  relation.RelationType,
			}}, nil
		},
	})
	if err != nil {
		return err
	}
	return converter.Execute()
}

// scopeRawArgs is the raw-data identity of a conversion over the given raw
// table of the task's project. The context is the subtask's, possibly
// wrapped to report a total.
func scopeRawArgs(taskCtx plugin.SubTaskContext, data *RedmineGoTaskData, table string) helper.RawDataSubTaskArgs {
	return helper.RawDataSubTaskArgs{
		Ctx: taskCtx,
		Options: models.RedmineGoApiParams{
			ConnectionId: data.Options.ConnectionId,
			ProjectId:    data.Options.ProjectId,
		},
		Table: table,
	}
}

// ConvertIssueComments converts the journal notes of the scope's issues to
// issue_comments: one comment per journal with a non-blank note. Private notes
// are left out unless the scope config's includePrivateNotes is on, so
// switching it needs no re-extraction. A note edited after it was written
// carries its update time.
func ConvertIssueComments(taskCtx plugin.SubTaskContext) errors.Error {
	db := taskCtx.GetDal()
	data := taskCtx.GetData().(*RedmineGoTaskData)
	connectionId := data.Options.ConnectionId
	includePrivate := data.ScopeConfig.ToggleEnabled(models.ToggleIncludePrivateNotes)
	commentIdGen := didgen.NewDomainIdGenerator(&models.RedmineGoJournal{})
	issueIdGen := didgen.NewDomainIdGenerator(&models.RedmineGoIssue{})
	accountIdGen := didgen.NewDomainIdGenerator(&models.RedmineGoAccount{})

	if err := deleteDomainByOrigin(db, &ticket.IssueComment{}, []string{rawTableName(RAW_ISSUE_DETAILS_TABLE)}, scopeParams(data)); err != nil {
		return err
	}
	scope := scopedClauses(models.RedmineGoJournal{}.TableName(), "j", data, dal.Where("j.notes <> ''"))
	cursor, err := scopedRows(db, "j", scope)
	if err != nil {
		return err
	}
	defer cursor.Close()
	converter, err := helper.NewDataConverter(helper.DataConverterArgs{
		RawDataSubTaskArgs: scopeRawArgs(withCountedTotal(taskCtx, scope...), data, RAW_ISSUE_DETAILS_TABLE),
		InputRowType:       reflect.TypeOf(models.RedmineGoJournal{}),
		Input:              cursor,
		Convert: func(inputRow interface{}) ([]interface{}, errors.Error) {
			journal := inputRow.(*models.RedmineGoJournal)
			if strings.TrimSpace(journal.Notes) == "" || (journal.PrivateNotes && !includePrivate) {
				return nil, nil
			}
			comment := &ticket.IssueComment{
				DomainEntity: domainlayer.DomainEntity{Id: commentIdGen.Generate(connectionId, journal.Id)},
				IssueId:      issueIdGen.Generate(connectionId, journal.IssueId),
				Body:         journal.Notes,
				CreatedDate:  journal.RedmineCreatedOn,
			}
			if journal.UserId != 0 {
				comment.AccountId = accountIdGen.Generate(connectionId, journal.UserId)
			}
			if journal.RedmineUpdatedOn != nil && journal.RedmineUpdatedOn.After(journal.RedmineCreatedOn) {
				comment.UpdatedDate = journal.RedmineUpdatedOn
			}
			return []interface{}{comment}, nil
		},
	})
	if err != nil {
		return err
	}
	return converter.Execute()
}

// ConvertIssueCommits converts the changesets linked to the scope's issues to
// issue_commits, keyed by the issue and the revision, which is the commit
// sha for Git repositories. The message and commit time stay in the tool table.
func ConvertIssueCommits(taskCtx plugin.SubTaskContext) errors.Error {
	db := taskCtx.GetDal()
	data := taskCtx.GetData().(*RedmineGoTaskData)
	connectionId := data.Options.ConnectionId
	issueIdGen := didgen.NewDomainIdGenerator(&models.RedmineGoIssue{})

	if err := deleteDomainByOrigin(db, &crossdomain.IssueCommit{}, []string{rawTableName(RAW_ISSUE_DETAILS_TABLE)}, scopeParams(data)); err != nil {
		return err
	}
	scope := scopedClauses(models.RedmineGoIssueChangeset{}.TableName(), "c", data)
	cursor, err := scopedRows(db, "c", scope)
	if err != nil {
		return err
	}
	defer cursor.Close()
	converter, err := helper.NewDataConverter(helper.DataConverterArgs{
		RawDataSubTaskArgs: scopeRawArgs(withCountedTotal(taskCtx, scope...), data, RAW_ISSUE_DETAILS_TABLE),
		InputRowType:       reflect.TypeOf(models.RedmineGoIssueChangeset{}),
		Input:              cursor,
		Convert: func(inputRow interface{}) ([]interface{}, errors.Error) {
			changeset := inputRow.(*models.RedmineGoIssueChangeset)
			return []interface{}{&crossdomain.IssueCommit{
				IssueId:   issueIdGen.Generate(connectionId, changeset.IssueId),
				CommitSha: changeset.Revision,
			}}, nil
		},
	})
	if err != nil {
		return err
	}
	return converter.Execute()
}

// ConvertWorklogs converts the time entries logged on the scope's issues to
// issue_worklogs. The time spent is the entry's hours in whole minutes; the day
// it was spent on is the started date and the entry's creation the logged date.
func ConvertWorklogs(taskCtx plugin.SubTaskContext) errors.Error {
	db := taskCtx.GetDal()
	data := taskCtx.GetData().(*RedmineGoTaskData)
	connectionId := data.Options.ConnectionId
	worklogIdGen := didgen.NewDomainIdGenerator(&models.RedmineGoTimeEntry{})
	issueIdGen := didgen.NewDomainIdGenerator(&models.RedmineGoIssue{})
	accountIdGen := didgen.NewDomainIdGenerator(&models.RedmineGoAccount{})

	if err := deleteDomainByOrigin(db, &ticket.IssueWorklog{}, []string{rawTableName(RAW_TIME_ENTRIES_TABLE)}, scopeParams(data)); err != nil {
		return err
	}
	scope := scopedClauses(models.RedmineGoTimeEntry{}.TableName(), "t", data)
	cursor, err := scopedRows(db, "t", scope)
	if err != nil {
		return err
	}
	defer cursor.Close()
	converter, err := helper.NewDataConverter(helper.DataConverterArgs{
		RawDataSubTaskArgs: scopeRawArgs(withCountedTotal(taskCtx, scope...), data, RAW_TIME_ENTRIES_TABLE),
		InputRowType:       reflect.TypeOf(models.RedmineGoTimeEntry{}),
		Input:              cursor,
		Convert: func(inputRow interface{}) ([]interface{}, errors.Error) {
			entry := inputRow.(*models.RedmineGoTimeEntry)
			spentOn := entry.SpentOn
			worklog := &ticket.IssueWorklog{
				DomainEntity:     domainlayer.DomainEntity{Id: worklogIdGen.Generate(connectionId, entry.Id)},
				Comment:          entry.Comments,
				TimeSpentMinutes: int(math.Round(entry.Hours * 60)),
				LoggedDate:       entry.RedmineCreatedOn,
				StartedDate:      &spentOn,
				IssueId:          issueIdGen.Generate(connectionId, entry.IssueId),
			}
			if entry.UserId != 0 {
				worklog.AuthorId = accountIdGen.Generate(connectionId, entry.UserId)
			}
			return []interface{}{worklog}, nil
		},
	})
	if err != nil {
		return err
	}
	return converter.Execute()
}
