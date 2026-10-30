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
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/models"
	"github.com/apache/devlake/core/models/common"
	"github.com/apache/devlake/core/models/domainlayer/crossdomain"
	"github.com/apache/devlake/core/models/domainlayer/ticket"
	"github.com/apache/devlake/core/plugin"
	"github.com/apache/devlake/core/runner"
	"github.com/apache/devlake/helpers/e2ehelper"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	contextimpl "github.com/apache/devlake/impls/context"
	"github.com/apache/devlake/plugins/redmine_go/impl"
	redmineModels "github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/apache/devlake/plugins/redmine_go/tasks"
	"github.com/stretchr/testify/require"
)

// connectionId is the connection every fixture row belongs to.
const connectionId = 1

// scopeProjects are the Redmine projects of the fixtures: the RGO fixture
// projects of the stack Redmine (6 with its subproject 7, and 8) and the two
// Lane 5 projects (1 and 2). Identifiers are what issues.original_project
// reports.
var scopeProjects = map[uint64]string{
	1: "portal",
	2: "registry",
	6: "rgo-fixtures",
	7: "rgo-fixtures-sub",
	8: "rgo-fixtures-b",
}

// newDataFlowTester opens a redmine_go DataFlowTester and creates every table
// the subtasks read or delete from. The connection table is skipped: the
// subtasks never touch it, and its encrypted token needs a serializer the
// tester does not register.
func newDataFlowTester(t *testing.T) *e2ehelper.DataFlowTester {
	var redmineGo impl.RedmineGo
	tester := e2ehelper.NewDataFlowTester(t, "redmine_go", redmineGo)
	tables := append([]dal.Tabler(nil), domainTables()...)
	for _, table := range redmineGo.GetTablesInfo() {
		switch table.(type) {
		case *redmineModels.RedmineGoConnection, *redmineModels.RedmineGoScopeConfig:
		default:
			tables = append(tables, table)
		}
	}
	for _, table := range tables {
		require.NoError(t, tester.Db.AutoMigrate(table))
	}
	require.NoError(t, tester.Db.AutoMigrate(&models.SubtaskState{}))
	seedProjects(t, tester)
	return tester
}

// seedProjects stores the scope rows the board convertor and the journal
// translation read, as the scope API would have.
func seedProjects(t *testing.T, tester *e2ehelper.DataFlowTester) {
	tester.FlushTabler(&redmineModels.RedmineGoProject{})
	for id, identifier := range scopeProjects {
		project := &redmineModels.RedmineGoProject{
			Scope: common.Scope{ConnectionId: connectionId},
			Id:    id, Identifier: identifier, Name: "Project " + identifier,
			Url: "https://redmine.example.org/projects/" + identifier,
		}
		require.NoError(t, tester.Db.Create(project).Error)
	}
}

// newTaskData builds the task data of one project, as PrepareTaskData would:
// the REST client stays nil because raw CSV rows are imported instead of
// collecting.
func newTaskData(projectId uint64, transformation redmineModels.Transformation) *tasks.RedmineGoTaskData {
	connection := &redmineModels.RedmineGoConnection{}
	connection.Endpoint = "https://redmine.example.org"
	return &tasks.RedmineGoTaskData{
		Options:    &tasks.RedmineGoOptions{ConnectionId: connectionId, ProjectId: projectId},
		Connection: connection,
		Project:    &redmineModels.RedmineGoProject{Id: projectId, Identifier: scopeProjects[projectId]},
		ScopeConfig: &redmineModels.RedmineGoScopeConfig{
			ScopeConfig:    common.ScopeConfig{Entities: []string{plugin.DOMAIN_TYPE_TICKET, plugin.DOMAIN_TYPE_CROSS}},
			Transformation: transformation,
		},
	}
}

// zeroConfig is the mapping of a scope without a scope config.
func zeroConfig() redmineModels.Transformation {
	return redmineModels.Transformation{}
}

// transformationFrom decodes a scope-config transformation document.
func transformationFrom(t *testing.T, document string) redmineModels.Transformation {
	var transformation redmineModels.Transformation
	require.NoError(t, json.Unmarshal([]byte(document), &transformation))
	require.Nil(t, transformation.Validate(), "the example config must be valid")
	return transformation
}

// beginStatefulScenario opens a test that exercises the framework's
// incremental and full-sync transitions, which DataFlowTester.Subtask cannot
// cover because it clears the subtask state and forces a full sync on every
// call. State is flushed at entry because the e2e database is shared by
// tests and state keys do not include the test name.
func beginStatefulScenario(t *testing.T, tester *e2ehelper.DataFlowTester) {
	t.Helper()
	tester.FlushTabler(&models.SubtaskState{})
}

// runSubtaskPreservingState runs one subtask inside a stateful scenario: the
// state survives between calls, so a stateful extractor goes incremental on
// an unchanged config and full-syncs when its config changes.
func runSubtaskPreservingState(t *testing.T, tester *e2ehelper.DataFlowTester, meta plugin.SubTaskMeta, taskData interface{}) {
	t.Helper()
	subtaskCtx := contextimpl.NewStandaloneSubTaskContext(
		context.Background(),
		runner.CreateBasicRes(tester.Cfg, tester.Log, tester.Db),
		tester.Name,
		taskData,
		meta.Name,
		&models.SyncPolicy{},
	)
	require.NoError(t, meta.EntryPoint(subtaskCtx))
}

// importMetadata loads the metadata raw tables and extracts them, as the
// pipeline does before any issue is touched.
func importMetadata(t *testing.T, tester *e2ehelper.DataFlowTester) {
	t.Helper()
	for _, table := range []string{"trackers", "statuses", "priorities", "custom_fields", "versions", "issue_categories"} {
		tester.ImportCsvIntoRawTable("./raw_tables/_raw_redmine_go_"+table+".csv", "_raw_redmine_go_"+table)
	}
	for _, table := range []dal.Tabler{
		&redmineModels.RedmineGoTracker{}, &redmineModels.RedmineGoStatus{}, &redmineModels.RedmineGoPriority{},
		&redmineModels.RedmineGoCustomField{}, &redmineModels.RedmineGoVersion{}, &redmineModels.RedmineGoIssueCategory{},
	} {
		tester.FlushTabler(table)
	}
	for _, projectId := range []uint64{1, 2, 6, 7, 8} {
		data := newTaskData(projectId, zeroConfig())
		for _, meta := range []plugin.SubTaskMeta{
			tasks.ExtractTrackersMeta, tasks.ExtractStatusesMeta, tasks.ExtractPrioritiesMeta,
			tasks.ExtractCustomFieldsMeta, tasks.ExtractVersionsMeta, tasks.ExtractIssueCategoriesMeta,
		} {
			tester.Subtask(meta, data)
		}
	}
}

// loadIssueRaw imports the issue and detail raw tables of a fixture set
// ("" for the RGO fixtures, "lane5/" for the Lane 5 projects) and clears
// every table the extractors and convertors fill.
func loadIssueRaw(t *testing.T, tester *e2ehelper.DataFlowTester, set string) {
	t.Helper()
	tester.ImportCsvIntoRawTable("./raw_tables/"+set+"_raw_redmine_go_issues.csv", "_raw_redmine_go_issues")
	tester.ImportCsvIntoRawTable("./raw_tables/"+set+"_raw_redmine_go_issue_details.csv", "_raw_redmine_go_issue_details")
	for _, table := range []string{rawSweepTable, rawTimeEntriesTable} {
		tester.FlushRawTable(table)
	}
	if set == "" {
		// The RGO fixtures also have a sweep and time entries; their state is
		// what Redmine lists now, so a run that sweeps finds nothing to prune.
		importSweep(tester)
		importTimeEntries(tester)
	}
	for _, table := range []dal.Tabler{
		&redmineModels.RedmineGoIssue{}, &redmineModels.RedmineGoJournal{}, &redmineModels.RedmineGoJournalDetail{},
		&redmineModels.RedmineGoAccount{}, &redmineModels.RedmineGoObservedField{},
		&redmineModels.RedmineGoIssueRelation{}, &redmineModels.RedmineGoIssueChangeset{}, &redmineModels.RedmineGoTimeEntry{},
	} {
		tester.FlushTabler(table)
	}
	for _, table := range domainTables() {
		tester.FlushTabler(table)
	}
}

// domainTables lists the domain-layer tables the plugin writes or prunes.
func domainTables() []dal.Tabler {
	return []dal.Tabler{
		&ticket.Board{}, &ticket.BoardIssue{}, &ticket.Issue{}, &ticket.IssueAssignee{},
		&ticket.IssueChangelogs{}, &ticket.IssueCustomArrayField{}, &crossdomain.Account{},
		&ticket.IssueRelationship{}, &ticket.IssueWorklog{}, &ticket.IssueComment{},
		&ticket.Sprint{}, &ticket.BoardSprint{}, &ticket.SprintIssue{},
		&ticket.Incident{}, &ticket.IncidentAssignee{}, &crossdomain.IssueCommit{},
	}
}

// extractIssues runs the issue and journal extractors for each project.
func extractIssues(tester *e2ehelper.DataFlowTester, projectIds ...uint64) {
	for _, projectId := range projectIds {
		data := newTaskData(projectId, zeroConfig())
		tester.Subtask(tasks.ExtractIssuesMeta, data)
		tester.Subtask(tasks.ExtractIssueJournalsMeta, data)
	}
}

// convertIssues runs every convertor for each project with a transformation.
func convertIssues(tester *e2ehelper.DataFlowTester, transformation redmineModels.Transformation, projectIds ...uint64) {
	for _, projectId := range projectIds {
		data := newTaskData(projectId, transformation)
		tester.Subtask(tasks.ConvertProjectsMeta, data)
		tester.Subtask(tasks.ConvertAccountsMeta, data)
		tester.Subtask(tasks.ConvertIssuesMeta, data)
		tester.Subtask(tasks.ConvertIssueChangelogsMeta, data)
	}
}

func count(t *testing.T, tester *e2ehelper.DataFlowTester, model interface{}, where string, args ...interface{}) int64 {
	t.Helper()
	var n int64
	query := tester.Db.Model(model)
	if where != "" {
		query = query.Where(where, args...)
	}
	require.NoError(t, query.Count(&n).Error)
	return n
}

// nextRawId returns an id past every row of a raw table: the CSV import
// writes explicit ids, which leaves a PostgreSQL sequence at 1.
func nextRawId(t *testing.T, tester *e2ehelper.DataFlowTester, table string) uint64 {
	t.Helper()
	var maxId uint64
	require.NoError(t, tester.Db.Table(table).Select("COALESCE(MAX(id), 0)").Row().Scan(&maxId))
	return maxId + 1
}

// deleteRawContaining deletes the raw rows of a table whose data holds the
// text, as a collection that no longer returns the item would.
func deleteRawContaining(t *testing.T, tester *e2ehelper.DataFlowTester, table, text string) {
	t.Helper()
	var rows []helper.RawData
	require.NoError(t, tester.Db.Table(table).Find(&rows).Error)
	deleted := 0
	for _, row := range rows {
		if bytes.Contains(row.Data, []byte(text)) {
			require.NoError(t, tester.Db.Table(table).Where("id = ?", row.ID).Delete(&helper.RawData{}).Error)
			deleted++
		}
	}
	require.Positive(t, deleted, "no raw row of %s contains %q", table, text)
}
