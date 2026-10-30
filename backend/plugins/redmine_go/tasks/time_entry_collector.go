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
	"net/url"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// RAW_TIME_ENTRIES_TABLE is the raw table of time entries, without the `_raw_`
// prefix. Every run replaces the whole table of the project.
const RAW_TIME_ENTRIES_TABLE = "redmine_go_time_entries"

// CollectTimeEntriesMeta describes the time-entry collector.
var CollectTimeEntriesMeta = plugin.SubTaskMeta{
	Name:             "collectTimeEntries",
	EntryPoint:       CollectTimeEntries,
	EnabledByDefault: true,
	Description:      "Collect all time entries of the project in full: Redmine ignores updated_on on time entries",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
	ProductTables:    []string{rawTableName(RAW_TIME_ENTRIES_TABLE)},
}

// ExtractTimeEntriesMeta describes the time-entry extractor.
var ExtractTimeEntriesMeta = plugin.SubTaskMeta{
	Name:             "extractTimeEntries",
	EntryPoint:       ExtractTimeEntries,
	EnabledByDefault: true,
	Description:      "Extract the time entries logged on issues into _tool_redmine_go_time_entries, replacing the project's earlier rows",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
	DependencyTables: []string{rawTableName(RAW_TIME_ENTRIES_TABLE)},
	ProductTables:    []string{models.RedmineGoTimeEntry{}.TableName(), models.RedmineGoAccount{}.TableName()},
}

var (
	_ plugin.SubTaskEntryPoint = CollectTimeEntries
	_ plugin.SubTaskEntryPoint = ExtractTimeEntries
)

// timeEntryQuery builds the query of one page of the time-entry list. Redmine
// accepts only the numeric project id here, not the identifier;
// subproject_id=!* keeps a subproject's entries out of its parent's list, as
// for issues. The list is sorted by id so parallel pages do not overlap when
// an entry is added mid-run.
func timeEntryQuery(projectId uint64, skip, size int) url.Values {
	return url.Values{
		"project_id":    []string{fmt.Sprint(projectId)},
		"subproject_id": []string{"!*"},
		"sort":          []string{"id"},
		"limit":         []string{fmt.Sprint(size)},
		"offset":        []string{fmt.Sprint(skip)},
	}
}

// CollectTimeEntries collects every time entry of the scope's project, and
// replaces the raw rows of earlier runs. Redmine ignores the updated_on filter
// on time entries, so an edited or deleted entry can only be found by pulling
// the whole list again. A project whose time tracking the key cannot read (403
// or 404) contributes no entries, with a warning.
func CollectTimeEntries(taskCtx plugin.SubTaskContext) errors.Error {
	data := taskCtx.GetData().(*RedmineGoTaskData)
	collector, err := helper.NewApiCollector(helper.ApiCollectorArgs{
		RawDataSubTaskArgs: helper.RawDataSubTaskArgs{
			Ctx: taskCtx,
			Options: models.RedmineGoApiParams{
				ConnectionId: data.Options.ConnectionId,
				ProjectId:    data.Options.ProjectId,
			},
			Table: RAW_TIME_ENTRIES_TABLE,
		},
		ApiClient:   data.ApiClient,
		UrlTemplate: "time_entries.json",
		PageSize:    redminePageSize,
		Query: func(reqData *helper.RequestData) (url.Values, errors.Error) {
			return timeEntryQuery(data.Options.ProjectId, reqData.Pager.Skip, reqData.Pager.Size), nil
		},
		GetTotalPages:  totalPagesFromCount,
		ResponseParser: listResponseParser("time_entries"),
		AfterResponse:  IgnoreMissingItem(taskCtx.GetLogger()),
	})
	if err != nil {
		return err
	}
	return collector.Execute()
}

// apiTimeEntry is a time entry as `time_entries.json` returns it. The issue is
// null for an entry logged on the project itself.
type apiTimeEntry struct {
	Id        uint64  `json:"id"`
	Project   *apiRef `json:"project"`
	Issue     *apiRef `json:"issue"`
	User      *apiRef `json:"user"`
	Activity  *apiRef `json:"activity"`
	Hours     float64 `json:"hours"`
	Comments  string  `json:"comments"`
	SpentOn   string  `json:"spent_on"`
	CreatedOn string  `json:"created_on"`
	UpdatedOn string  `json:"updated_on"`
}

// ExtractTimeEntries extracts the time entries logged on issues. Entries
// logged on the project itself have no issue to attach a worklog to and are
// skipped; their count goes to the debug log. The project's earlier rows are
// deleted first, so an entry deleted in Redmine disappears even when this run
// extracts none.
func ExtractTimeEntries(taskCtx plugin.SubTaskContext) errors.Error {
	data := taskCtx.GetData().(*RedmineGoTaskData)
	db := taskCtx.GetDal()
	connectionId, projectId := data.Options.ConnectionId, data.Options.ProjectId

	if err := db.Delete(&models.RedmineGoTimeEntry{}, dal.Where("connection_id = ? AND project_id = ?", connectionId, projectId)); err != nil {
		return err
	}
	skipped := 0
	extractor, err := helper.NewApiExtractor(helper.ApiExtractorArgs{
		RawDataSubTaskArgs: helper.RawDataSubTaskArgs{
			Ctx: taskCtx,
			Options: models.RedmineGoApiParams{
				ConnectionId: connectionId,
				ProjectId:    projectId,
			},
			Table: RAW_TIME_ENTRIES_TABLE,
		},
		Extract: func(row *helper.RawData) ([]interface{}, errors.Error) {
			var item apiTimeEntry
			if err := errors.Convert(json.Unmarshal(row.Data, &item)); err != nil {
				return nil, err
			}
			if item.Issue == nil || item.Issue.Id == 0 {
				skipped++
				return nil, nil
			}
			entry := &models.RedmineGoTimeEntry{
				ConnectionId:     connectionId,
				Id:               item.Id,
				ProjectId:        projectId,
				IssueId:          item.Issue.Id,
				Hours:            item.Hours,
				Comments:         item.Comments,
				RedmineCreatedOn: parseTime(item.CreatedOn),
				RedmineUpdatedOn: parseTime(item.UpdatedOn),
			}
			if spentOn := parseDate(item.SpentOn); spentOn != nil {
				entry.SpentOn = *spentOn
			}
			if item.User != nil {
				entry.UserId, entry.UserName = item.User.Id, item.User.Name
			}
			if item.Activity != nil {
				entry.ActivityId, entry.ActivityName = item.Activity.Id, item.Activity.Name
			}
			results := []interface{}{entry}
			if account := accountRow(connectionId, item.User); account != nil {
				results = append(results, account)
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
	taskCtx.GetLogger().Debug("skipped %d time entries of project %d logged on the project itself, not on an issue", skipped, projectId)
	return nil
}
