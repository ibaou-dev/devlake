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
	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// detailExtraction describes an extractor that derives rows from the issue
// detail raw table.
type detailExtraction struct {
	// Config is the subtask config of the stateful extractor; a change to it
	// re-extracts every raw row.
	Config interface{}
	// Replaced lists the tool models whose rows of an issue are deleted before
	// its newest detail is extracted. Each has connection_id and issue_id
	// columns.
	Replaced []interface{}
	// Rows maps the newest detail of an issue to its tool rows.
	Rows func(item *apiIssue) []interface{}
}

// deleteBatchSize is the number of issues one DELETE of replaced rows covers.
const deleteBatchSize = 500

// extractIssueDetails runs a stateful extractor over the issue detail raw
// rows. An issue's rows are replaced as a whole each time it is extracted, so
// an edited or re-collected issue never leaves duplicates or deleted children
// behind, and only the newest raw row of an issue in the window expands: an
// older version would bring back rows the newer one no longer has.
//
// The rows of every issue in the window are deleted up front, a batch of
// issues per statement. Deleting issue by issue costs one auto-committed
// statement per issue and table, about as much as the extraction itself.
func extractIssueDetails(taskCtx plugin.SubTaskContext, spec detailExtraction) errors.Error {
	data := taskCtx.GetData().(*RedmineGoTaskData)
	db := taskCtx.GetDal()
	connectionId := data.Options.ConnectionId

	var latestVersion map[uint64]uint64
	extractor, err := helper.NewStatefulApiExtractor(&helper.StatefulApiExtractorArgs[apiIssue]{
		SubtaskCommonArgs: &helper.SubtaskCommonArgs{
			SubTaskContext: withRawRowsTotal(taskCtx, rawTableName(RAW_ISSUE_DETAILS_TABLE), scopeParams(data)),
			Table:          RAW_ISSUE_DETAILS_TABLE,
			Params: models.RedmineGoApiParams{
				ConnectionId: connectionId,
				ProjectId:    data.Options.ProjectId,
			},
			SubtaskConfig: spec.Config,
		},
		BeforeExtract: func(issue *apiIssue, _ *helper.SubtaskStateManager) errors.Error {
			// A row whose input column was unreadable is not in latestVersion,
			// so its issue was not deleted up front.
			if _, ok := latestVersion[issue.Id]; ok {
				return nil
			}
			return deleteIssueRows(db, spec.Replaced, connectionId, []uint64{issue.Id})
		},
		Extract: func(item *apiIssue, row *helper.RawData) ([]interface{}, errors.Error) {
			if latestId, ok := latestVersion[item.Id]; ok && row.ID != latestId {
				return nil, nil
			}
			return spec.Rows(item), nil
		},
	})
	if err != nil {
		return err
	}
	latestVersion, err = latestDetailVersions(db, extractor)
	if err != nil {
		return err
	}
	// Both modes: the divider's full-sync wipe fires only when a row of the
	// type is emitted, so an issue whose children all vanished would otherwise
	// keep its old rows.
	ids := make([]uint64, 0, len(latestVersion))
	for id := range latestVersion {
		ids = append(ids, id)
	}
	if err := deleteIssueRowsInBatches(db, spec.Replaced, connectionId, ids); err != nil {
		return err
	}
	return extractor.Execute()
}

// deleteIssueRowsInBatches deletes the rows of the issues, deleteBatchSize
// issues per statement.
func deleteIssueRowsInBatches(db dal.Dal, tables []interface{}, connectionId uint64, issueIds []uint64) errors.Error {
	for start := 0; start < len(issueIds); start += deleteBatchSize {
		end := min(start+deleteBatchSize, len(issueIds))
		if err := deleteIssueRows(db, tables, connectionId, issueIds[start:end]); err != nil {
			return err
		}
	}
	return nil
}

// deleteIssueRows deletes the rows of the given issues from each model's
// table; every model has connection_id and issue_id columns.
func deleteIssueRows(db dal.Dal, tables []interface{}, connectionId uint64, issueIds []uint64) errors.Error {
	for _, table := range tables {
		if err := db.Delete(table, dal.Where("connection_id = ? AND issue_id IN ?", connectionId, issueIds)); err != nil {
			return err
		}
	}
	return nil
}
