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
	"io"
	"net/http"
	"net/url"
	"reflect"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// RAW_ISSUE_DETAILS_TABLE is the raw table of issue detail rows, without the
// `_raw_` prefix. Each row is one issue with its journals, children,
// changesets and relations, so later milestones extract relations and
// changesets without collecting again.
const RAW_ISSUE_DETAILS_TABLE = "redmine_go_issue_details"

// issueDetailInclude lists everything a detail request asks for.
const issueDetailInclude = "journals,children,changesets,relations"

// issueDetailInput is the collector's input row: one per issue to fetch.
type issueDetailInput struct {
	Id uint64 `json:"id"`
}

// CollectIssueDetailsMeta describes the issue detail collector.
var CollectIssueDetailsMeta = plugin.SubTaskMeta{
	Name:             "collectIssueDetails",
	EntryPoint:       CollectIssueDetails,
	EnabledByDefault: true,
	Description:      "Collect journals, children, changesets and relations of the issues collected in this run, one request per issue",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET, plugin.DOMAIN_TYPE_CROSS},
	DependencyTables: []string{models.RedmineGoIssue{}.TableName()},
	ProductTables:    []string{rawTableName(RAW_ISSUE_DETAILS_TABLE)},
}

var _ plugin.SubTaskEntryPoint = CollectIssueDetails

// CollectIssueDetails fetches the detail of each issue the issue collector
// re-collected: every issue of the scope on a full run, and on an incremental
// run those whose updated_on falls in the same window the list used. A 404 or
// 403 (the issue was deleted, or the key lost access) skips the issue with a
// warning.
func CollectIssueDetails(taskCtx plugin.SubTaskContext) errors.Error {
	data := taskCtx.GetData().(*RedmineGoTaskData)
	db := taskCtx.GetDal()
	apiCollector, err := helper.NewStatefulApiCollector(helper.RawDataSubTaskArgs{
		Ctx: taskCtx,
		Options: models.RedmineGoApiParams{
			ConnectionId: data.Options.ConnectionId,
			ProjectId:    data.Options.ProjectId,
		},
		Table: RAW_ISSUE_DETAILS_TABLE,
	})
	if err != nil {
		return err
	}
	if apiCollector.IsIncremental() {
		data.FullIssueRun = false
	}

	clauses := []dal.Clause{
		dal.Select("id"),
		dal.From(&models.RedmineGoIssue{}),
		dal.Where("connection_id = ? AND project_id = ?", data.Options.ConnectionId, data.Options.ProjectId),
		dal.Orderby("id ASC"),
	}
	if since := changedIssuesSince(apiCollector.IsIncremental(), apiCollector.GetSince()); since != nil {
		clauses = append(clauses, dal.Where("redmine_updated_on >= ?", *since))
	}
	cursor, err := db.Cursor(clauses...)
	if err != nil {
		return err
	}
	input, err := helper.NewDalCursorIterator(db, cursor, reflect.TypeOf(issueDetailInput{}))
	if err != nil {
		return err
	}
	taskCtx.GetLogger().Info("collecting issue details (incremental: %v)", apiCollector.IsIncremental())

	err = apiCollector.InitCollector(helper.ApiCollectorArgs{
		ApiClient:   data.ApiClient,
		UrlTemplate: "issues/{{ .Input.Id }}.json",
		Input:       input,
		Query: func(*helper.RequestData) (url.Values, errors.Error) {
			return url.Values{"include": []string{issueDetailInclude}}, nil
		},
		ResponseParser: parseIssueDetail,
		AfterResponse:  IgnoreMissingItem(taskCtx.GetLogger()),
	})
	if err != nil {
		return err
	}
	return apiCollector.Execute()
}

// parseIssueDetail unwraps `{"issue": {...}}` into the single raw row the
// detail extractor reads.
func parseIssueDetail(res *http.Response) ([]json.RawMessage, errors.Error) {
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Convert(err)
	}
	var envelope struct {
		Issue json.RawMessage `json:"issue"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, errors.Default.Wrap(err, "could not decode the Redmine issue response")
	}
	if len(envelope.Issue) == 0 {
		return nil, nil
	}
	return []json.RawMessage{envelope.Issue}, nil
}
