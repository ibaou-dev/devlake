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
	"net/url"
	"time"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// RAW_ISSUES_TABLE is the raw table of issue list rows, without the `_raw_`
// prefix.
const RAW_ISSUES_TABLE = "redmine_go_issues"

// CollectIssuesMeta describes the incremental issue collector.
var CollectIssuesMeta = plugin.SubTaskMeta{
	Name:             "collectIssues",
	EntryPoint:       CollectIssues,
	EnabledByDefault: true,
	Description:      "Collect the issues of a Redmine project, changed since the last run",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET, plugin.DOMAIN_TYPE_CROSS},
	ProductTables:    []string{rawTableName(RAW_ISSUES_TABLE)},
}

var _ plugin.SubTaskEntryPoint = CollectIssues

// issueQuery builds the query of one page of the issue list.
//
// subproject_id=!* restricts the list to the project itself, so selecting a
// parent project and its subproject as two scopes never collects an issue
// twice. status_id=* includes closed issues. Pages are sorted by id, not by
// updated_on: they are fetched in parallel by offset, and an issue edited
// mid-run would jump to the end of an updated_on ordering and shift every
// later row one slot, so one issue at a page boundary could be missed. An id
// never moves. The updated_on bound is formatted in UTC with an explicit Z,
// which Redmine reads independent of any user time zone.
func issueQuery(projectId uint64, since *time.Time, skip, size int) url.Values {
	query := url.Values{
		"project_id":    []string{fmt.Sprint(projectId)},
		"subproject_id": []string{"!*"},
		"status_id":     []string{"*"},
		"sort":          []string{"id"},
		"limit":         []string{fmt.Sprint(size)},
		"offset":        []string{fmt.Sprint(skip)},
	}
	if since != nil {
		query.Set("updated_on", ">="+since.UTC().Format("2006-01-02T15:04:05Z"))
	}
	return query
}

// CollectIssues collects the issues of the scope's project. The first run
// collects everything (or everything after timeAfter); later runs collect
// issues updated since the last successful start, minus an overlap.
func CollectIssues(taskCtx plugin.SubTaskContext) errors.Error {
	data := taskCtx.GetData().(*RedmineGoTaskData)
	apiCollector, err := helper.NewStatefulApiCollector(helper.RawDataSubTaskArgs{
		Ctx: taskCtx,
		Options: models.RedmineGoApiParams{
			ConnectionId: data.Options.ConnectionId,
			ProjectId:    data.Options.ProjectId,
		},
		Table: RAW_ISSUES_TABLE,
	})
	if err != nil {
		return err
	}
	since := apiCollector.GetSince()
	if apiCollector.IsIncremental() {
		since = changedIssuesSince(true, since)
	}
	taskCtx.GetLogger().Info("collecting issues of project %d updated since %v (incremental: %v)",
		data.Options.ProjectId, since, apiCollector.IsIncremental())

	err = apiCollector.InitCollector(helper.ApiCollectorArgs{
		ApiClient:   data.ApiClient,
		UrlTemplate: "issues.json",
		PageSize:    redminePageSize,
		Query: func(reqData *helper.RequestData) (url.Values, errors.Error) {
			return issueQuery(data.Options.ProjectId, since, reqData.Pager.Skip, reqData.Pager.Size), nil
		},
		GetTotalPages:  totalPagesFromCount,
		ResponseParser: listResponseParser("issues"),
	})
	if err != nil {
		return err
	}
	if err := apiCollector.Execute(); err != nil {
		return err
	}
	rows, err := taskCtx.GetDal().Count(dal.From(rawTableName(RAW_ISSUES_TABLE)), dal.Where("params = ?", scopeParams(data)))
	if err != nil {
		return err
	}
	data.FullIssueRun = listedEveryIssue(apiCollector.IsIncremental(), since, rows)
	return nil
}

// listedEveryIssue reports whether a collection of the issue list covered the
// whole project: a full-sync run (not incremental) without a time bound that
// found at least one issue. With a time bound the list stops at timeAfter, and
// an empty list leaves the stored issues unreplaced, so in neither case does
// the run prove what is gone from Redmine.
func listedEveryIssue(incremental bool, since *time.Time, rows int64) bool {
	return !incremental && since == nil && rows > 0
}
