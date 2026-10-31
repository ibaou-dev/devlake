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
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// RAW_ISSUE_SWEEP_TABLE is the raw table of the reconciliation sweep, without
// the `_raw_` prefix. Each row is one issue of the project reduced to its id
// and relations, and every run replaces the whole table of the project.
const RAW_ISSUE_SWEEP_TABLE = "redmine_go_issue_sweep"

// IssueProbe reports whether an issue still belongs to a project in Redmine.
type IssueProbe func(issueId, projectId uint64) (bool, errors.Error)

// CollectIssueSweepMeta describes the reconciliation sweep collector.
var CollectIssueSweepMeta = plugin.SubTaskMeta{
	Name:             "collectIssueSweep",
	EntryPoint:       CollectIssueSweep,
	EnabledByDefault: true,
	Description:      "Page through the whole issue list of the project with its relations, to find relation changes and deleted issues that no updated_on reveals; skipped on a full run that listed every issue",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
	ProductTables:    []string{rawTableName(RAW_ISSUE_SWEEP_TABLE)},
}

// ExtractIssueSweepMeta describes the reconciliation sweep extractor.
var ExtractIssueSweepMeta = plugin.SubTaskMeta{
	Name:             "extractIssueSweep",
	EntryPoint:       ExtractIssueSweep,
	EnabledByDefault: true,
	Description:      "Upsert the relations the sweep found, and delete relations and issues that Redmine no longer lists, from the tool and domain layers; after a full run that listed every issue, delete only the relations of issues that left the project",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
	DependencyTables: []string{rawTableName(RAW_ISSUE_SWEEP_TABLE), models.RedmineGoIssue{}.TableName()},
	ProductTables:    []string{models.RedmineGoIssueRelation{}.TableName()},
}

var (
	_ plugin.SubTaskEntryPoint = CollectIssueSweep
	_ plugin.SubTaskEntryPoint = ExtractIssueSweep
)

// sweepIssue is one raw sweep row.
type sweepIssue struct {
	Id        uint64        `json:"id"`
	Relations []apiRelation `json:"relations"`
}

// sweepQuery builds the query of one page of the sweep: the issue list query
// of the incremental collector, without its time bound, with relations.
func sweepQuery(projectId uint64, skip, size int) url.Values {
	query := issueQuery(projectId, nil, skip, size)
	query.Set("include", "relations")
	return query
}

// parseSweepPage reduces a page of the issue list to one raw row per issue,
// keeping the id and the relations only: the sweep runs every time and would
// otherwise store every issue's description again.
func parseSweepPage(res *http.Response) ([]json.RawMessage, errors.Error) {
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Convert(err)
	}
	var page struct {
		Issues []sweepIssue `json:"issues"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, errors.Default.Wrap(err, "could not decode the Redmine issue list")
	}
	rows := make([]json.RawMessage, 0, len(page.Issues))
	for _, issue := range page.Issues {
		encoded, err := json.Marshal(issue)
		if err != nil {
			return nil, errors.Default.Wrap(err, "could not encode a sweep row")
		}
		rows = append(rows, encoded)
	}
	return rows, nil
}

// removeOrphanRelations is what the sweep leaves to do after a full run that
// listed every issue of the project. The full run replaced the project's issue
// rows and every issue's relations, so a relation can only be stale when its
// source issue is no longer one of the project's issues: it was deleted in
// Redmine or moved away, and a relation row written by an earlier sweep is not
// among those the details extractor wipes by origin. Such a relation is
// deleted here, so the domain layer rebuilt after it does not keep it.
func removeOrphanRelations(taskCtx plugin.SubTaskContext, connectionId, projectId uint64) errors.Error {
	db := taskCtx.GetDal()
	clauses := []dal.Clause{
		dal.From(&models.RedmineGoIssueRelation{}),
		dal.Where("connection_id = ? AND project_id = ?", connectionId, projectId),
		dal.Where("issue_id NOT IN (SELECT id FROM "+models.RedmineGoIssue{}.TableName()+" WHERE connection_id = ? AND project_id = ?)", connectionId, projectId),
	}
	orphans, err := db.Count(clauses...)
	if err != nil {
		return err
	}
	if orphans > 0 {
		if err := db.Delete(&models.RedmineGoIssueRelation{}, clauses[1:]...); err != nil {
			return err
		}
	}
	taskCtx.GetLogger().Info("issue sweep of project %d skipped after a full run: %d relations of issues that left the project deleted", projectId, orphans)
	return nil
}

// CollectIssueSweep pages through the project's whole issue list, with
// relations, replacing the project's sweep rows. Unlike the incremental issue
// collector it ignores updated_on, because Redmine changes none when a
// relation is added or removed or when an issue is deleted. A 403 or 404 fails
// the subtask: an unreadable list must never read as an empty project.
//
// It sends nothing when this run already listed every issue of the project in
// full (see RedmineGoTaskData.FullIssueRun): the full run replaced the issue
// rows and re-read every issue's relations, and the list would repeat the
// issue list's pages.
func CollectIssueSweep(taskCtx plugin.SubTaskContext) errors.Error {
	data := taskCtx.GetData().(*RedmineGoTaskData)
	if data.FullIssueRun {
		taskCtx.GetLogger().Info("issue sweep of project %d skipped: this run listed every issue of it in full", data.Options.ProjectId)
		return nil
	}
	collector, err := helper.NewApiCollector(helper.ApiCollectorArgs{
		RawDataSubTaskArgs: helper.RawDataSubTaskArgs{
			Ctx: taskCtx,
			Options: models.RedmineGoApiParams{
				ConnectionId: data.Options.ConnectionId,
				ProjectId:    data.Options.ProjectId,
			},
			Table: RAW_ISSUE_SWEEP_TABLE,
		},
		ApiClient:   data.ApiClient,
		UrlTemplate: "issues.json",
		PageSize:    redminePageSize,
		Query: func(reqData *helper.RequestData) (url.Values, errors.Error) {
			return sweepQuery(data.Options.ProjectId, reqData.Pager.Skip, reqData.Pager.Size), nil
		},
		GetTotalPages:  totalPagesFromCount,
		ResponseParser: parseSweepPage,
	})
	if err != nil {
		return err
	}
	return collector.Execute()
}

// ExtractIssueSweep upserts the relations of the sweep and removes what the
// sweep no longer finds:
//
//   - A relation is owned by the project of its source issue (relation.issue_id)
//     and emitted when that issue is in the sweep, whatever project the target
//     is in. Relations of the project that the sweep did not list are deleted.
//   - An issue stored for the project but missing from the sweep is deleted from
//     the tool and domain layers. It was deleted in Redmine, or moved to another
//     project: a move leaves this scope's rows, and the target project, if it is
//     a scope, collects the issue because the move changed its updated_on. The
//     list is paged by offset, so an issue deleted while the sweep runs can shift
//     a neighbour off a page; each candidate is therefore confirmed with one
//     request before it is deleted, and a confirmed-present issue is kept
//     together with its relations.
//
// After a full run that listed every issue of the project there is no sweep
// to extract: only removeOrphanRelations runs.
func ExtractIssueSweep(taskCtx plugin.SubTaskContext) errors.Error {
	data := taskCtx.GetData().(*RedmineGoTaskData)
	db := taskCtx.GetDal()
	connectionId, projectId := data.Options.ConnectionId, data.Options.ProjectId
	if data.FullIssueRun {
		return removeOrphanRelations(taskCtx, connectionId, projectId)
	}

	// Without the collector's raw table the extractor would see an empty
	// project and delete every issue of it.
	if !db.HasTable(rawTableName(RAW_ISSUE_SWEEP_TABLE)) {
		return errors.Default.New("the issue sweep has no collected rows: run collectIssueSweep before extractIssueSweep")
	}
	seenIssues := map[uint64]struct{}{}
	seenRelations := map[uint64]struct{}{}
	extractor, err := helper.NewApiExtractor(helper.ApiExtractorArgs{
		RawDataSubTaskArgs: helper.RawDataSubTaskArgs{
			Ctx: withRawRowsTotal(taskCtx, rawTableName(RAW_ISSUE_SWEEP_TABLE), scopeParams(data)),
			Options: models.RedmineGoApiParams{
				ConnectionId: connectionId,
				ProjectId:    projectId,
			},
			Table: RAW_ISSUE_SWEEP_TABLE,
		},
		Extract: func(row *helper.RawData) ([]interface{}, errors.Error) {
			var issue sweepIssue
			if err := errors.Convert(json.Unmarshal(row.Data, &issue)); err != nil {
				return nil, err
			}
			seenIssues[issue.Id] = struct{}{}
			var rows []interface{}
			for i := range issue.Relations {
				if relation := relationRow(connectionId, projectId, issue.Id, &issue.Relations[i]); relation != nil {
					seenRelations[relation.Id] = struct{}{}
					rows = append(rows, relation)
				}
			}
			return rows, nil
		},
	})
	if err != nil {
		return err
	}
	if err := extractor.Execute(); err != nil {
		return err
	}

	missing, err := missingIssues(db, connectionId, projectId, seenIssues)
	if err != nil {
		return err
	}
	var removed []uint64
	kept := map[uint64]struct{}{}
	for _, issueId := range missing {
		present := false
		if data.IssueProbe != nil {
			if present, err = data.IssueProbe(issueId, projectId); err != nil {
				return err
			}
		}
		if present {
			kept[issueId] = struct{}{}
			taskCtx.GetLogger().Warn(nil, "issue %d is missing from the sweep of project %d but Redmine still has it there; kept", issueId, projectId)
			continue
		}
		removed = append(removed, issueId)
	}

	staleRelations, err := staleRelationIds(db, connectionId, projectId, seenRelations, kept)
	if err != nil {
		return err
	}
	if err := deleteByIds(db, &models.RedmineGoIssueRelation{}, "id", staleRelations, dal.Where("connection_id = ?", connectionId)); err != nil {
		return err
	}
	if err := removeIssues(db, connectionId, removed); err != nil {
		return err
	}
	taskCtx.GetLogger().Info("issue sweep of project %d: %d issues and %d relations listed, %d stale relations and %d removed issues deleted",
		projectId, len(seenIssues), len(seenRelations), len(staleRelations), len(removed))
	return nil
}

// missingIssues lists the issues stored for the project that a sweep did not
// list, in id order.
func missingIssues(db dal.Dal, connectionId, projectId uint64, listed map[uint64]struct{}) ([]uint64, errors.Error) {
	return idsNotIn(db, &models.RedmineGoIssue{}, listed,
		dal.Where("connection_id = ? AND project_id = ?", connectionId, projectId))
}

// staleRelationIds lists the relations stored for the project that a sweep did
// not list, leaving out those whose source issue was kept.
func staleRelationIds(db dal.Dal, connectionId, projectId uint64, listed, keptIssues map[uint64]struct{}) ([]uint64, errors.Error) {
	cursor, err := db.Cursor(
		dal.Select("id, issue_id"),
		dal.From(&models.RedmineGoIssueRelation{}),
		dal.Where("connection_id = ? AND project_id = ?", connectionId, projectId),
	)
	if err != nil {
		return nil, err
	}
	defer cursor.Close()
	var stale []uint64
	for cursor.Next() {
		var row struct {
			Id      uint64
			IssueId uint64
		}
		if err := db.Fetch(cursor, &row); err != nil {
			return nil, errors.Default.Wrap(err, "error reading relations")
		}
		if _, ok := listed[row.Id]; ok {
			continue
		}
		if _, ok := keptIssues[row.IssueId]; ok {
			continue
		}
		stale = append(stale, row.Id)
	}
	return stale, nil
}

// idsNotIn streams the `id` column of a table's rows that match the clauses and
// returns those not in listed.
func idsNotIn(db dal.Dal, model interface{}, listed map[uint64]struct{}, clauses ...dal.Clause) ([]uint64, errors.Error) {
	cursor, err := db.Cursor(append([]dal.Clause{dal.Select("id"), dal.From(model), dal.Orderby("id ASC")}, clauses...)...)
	if err != nil {
		return nil, err
	}
	defer cursor.Close()
	var ids []uint64
	for cursor.Next() {
		var row struct{ Id uint64 }
		if err := db.Fetch(cursor, &row); err != nil {
			return nil, errors.Default.Wrap(err, "error reading ids")
		}
		if _, ok := listed[row.Id]; ok {
			continue
		}
		ids = append(ids, row.Id)
	}
	return ids, nil
}

// NewIssueProbe returns an IssueProbe that asks Redmine for the issue. A 404
// or 403 means the issue is deleted or no longer visible, which for the sweep
// is the same as gone; any other failure is an error, never a guess.
func NewIssueProbe(client *helper.ApiAsyncClient) IssueProbe {
	return func(issueId, projectId uint64) (bool, errors.Error) {
		res, err := client.Get(fmt.Sprintf("issues/%d.json", issueId), nil, nil)
		if err != nil {
			return false, errors.Default.Wrap(err, fmt.Sprintf("could not confirm issue %d with Redmine", issueId))
		}
		defer res.Body.Close()
		switch res.StatusCode {
		case http.StatusOK:
			var body struct {
				Issue struct {
					Project *apiRef `json:"project"`
				} `json:"issue"`
			}
			if err := helper.UnmarshalResponse(res, &body); err != nil {
				return false, err
			}
			return body.Issue.Project != nil && body.Issue.Project.Id == projectId, nil
		case http.StatusNotFound, http.StatusForbidden:
			return false, nil
		case http.StatusUnauthorized:
			return false, errors.Unauthorized.New("authentication failed, please check the Redmine API key")
		default:
			return false, errors.Default.New(fmt.Sprintf("Redmine answered %d while confirming issue %d", res.StatusCode, issueId))
		}
	}
}
