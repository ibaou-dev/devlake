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

package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/apache/devlake/helpers/e2ehelper"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/api"
	"github.com/apache/devlake/plugins/redmine_go/impl"
	"github.com/apache/devlake/plugins/redmine_go/tasks"
	"github.com/stretchr/testify/require"
)

const (
	rawSweepTable       = "_raw_redmine_go_issue_sweep"
	rawTimeEntriesTable = "_raw_redmine_go_time_entries"
	rawIssuesTable      = "_raw_redmine_go_issues"
	rawDetailsTable     = "_raw_redmine_go_issue_details"
)

// importSweep loads the sweep of the RGO fixture projects as the stack Redmine
// listed it: issue 170 under project 8, issue 171 deleted.
func importSweep(tester *e2ehelper.DataFlowTester) {
	tester.ImportCsvIntoRawTable("./raw_tables/_raw_redmine_go_issue_sweep.csv", rawSweepTable)
}

// importTimeEntries loads the time entries of project 6 as Redmine listed them:
// two on issue 176 (one edited) and one logged on the project itself.
func importTimeEntries(tester *e2ehelper.DataFlowTester) {
	tester.ImportCsvIntoRawTable("./raw_tables/_raw_redmine_go_time_entries.csv", rawTimeEntriesTable)
}

// runPlanned runs the subtasks a blueprint would run for the task's scope
// config, in the plugin's order, with the real gating by toggles. The
// collectors are left out: e2e tests import raw rows instead of collecting.
func runPlanned(tester *e2ehelper.DataFlowTester, data *tasks.RedmineGoTaskData) {
	transformation := data.ScopeConfig.Transformation
	metas := api.EnabledSubtasks((impl.RedmineGo{}).SubTaskMetas(), tasks.SubtaskToggles, &transformation)
	for _, meta := range metas {
		if strings.HasPrefix(meta.Name, "collect") {
			continue
		}
		tester.Subtask(meta, data)
	}
}

// plannedNames lists the names of the subtasks runPlanned would run.
func plannedNames(data *tasks.RedmineGoTaskData) []string {
	transformation := data.ScopeConfig.Transformation
	var names []string
	for _, meta := range api.EnabledSubtasks((impl.RedmineGo{}).SubTaskMetas(), tasks.SubtaskToggles, &transformation) {
		names = append(names, meta.Name)
	}
	return names
}

// cloneRaw appends to a raw table a copy of the newest row of issue fromId,
// re-labelled as toId, as an earlier collection of an issue that was since
// deleted or moved would have stored it. edit changes the copy before it is
// stored.
func cloneRaw(t *testing.T, tester *e2ehelper.DataFlowTester, table, params string, fromId, toId uint64, edit func(issue map[string]interface{})) {
	t.Helper()
	var rows []helper.RawData
	require.NoError(t, tester.Db.Table(table).Order("id").Find(&rows).Error)
	var source *helper.RawData
	for i := range rows {
		var body struct {
			Id uint64 `json:"id"`
		}
		require.NoError(t, json.Unmarshal(rows[i].Data, &body))
		if body.Id == fromId {
			source = &rows[i]
		}
	}
	require.NotNil(t, source, "no raw row of issue %d in %s", fromId, table)
	var issue map[string]interface{}
	require.NoError(t, json.Unmarshal(source.Data, &issue))
	issue["id"] = toId
	if edit != nil {
		edit(issue)
	}
	data, err := json.Marshal(issue)
	require.NoError(t, err)
	input := source.Input
	if table == rawDetailsTable {
		input = []byte(`{"id":` + idText(toId) + `}`)
	}
	require.NoError(t, tester.Db.Table(table).Create(&helper.RawData{
		ID: nextRawId(t, tester, table), Params: params, Data: data, Url: source.Url, Input: input,
	}).Error)
}

func idText(id uint64) string {
	encoded, _ := json.Marshal(id)
	return string(encoded)
}

// deleteRawIssue deletes the raw rows of one issue under a scope's params, as a
// fresh collection of the scope would no longer return them.
func deleteRawIssue(t *testing.T, tester *e2ehelper.DataFlowTester, table, params string, issueId uint64) {
	t.Helper()
	var rows []helper.RawData
	require.NoError(t, tester.Db.Table(table).Where("params = ?", params).Find(&rows).Error)
	deleted := 0
	for _, row := range rows {
		var body struct {
			Id uint64 `json:"id"`
		}
		require.NoError(t, json.Unmarshal(row.Data, &body))
		if body.Id == issueId {
			require.NoError(t, tester.Db.Table(table).Where("id = ?", row.ID).Delete(&helper.RawData{}).Error)
			deleted++
		}
	}
	require.Positive(t, deleted, "no raw row of issue %d under %s in %s", issueId, params, table)
}
