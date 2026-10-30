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
	"testing"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/models/common"
	"github.com/apache/devlake/helpers/e2ehelper"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/apache/devlake/plugins/redmine_go/tasks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// verifyToolTable compares a tool table with its committed snapshot, which
// the tester writes on the first run.
func verifyToolTable(tester *e2ehelper.DataFlowTester, table dal.Tabler) {
	tester.VerifyTableWithOptions(table, e2ehelper.TableOptions{
		CSVRelPath:  "./snapshot_tables/" + table.TableName() + ".csv",
		IgnoreTypes: []interface{}{common.NoPKModel{}},
	})
}

// The metadata raw tables are a real capture of the stack Redmine: trackers,
// statuses, priorities and custom fields of an admin key, versions of five
// projects, and the categories of the RGO fixtures.
func TestMetadataDataFlow(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)

	for _, table := range []dal.Tabler{
		&models.RedmineGoTracker{}, &models.RedmineGoStatus{}, &models.RedmineGoPriority{},
		&models.RedmineGoCustomField{}, &models.RedmineGoVersion{}, &models.RedmineGoIssueCategory{},
	} {
		verifyToolTable(tester, table)
	}

	// Explicit invariants, so the snapshot is never the only oracle.
	var tracker models.RedmineGoTracker
	require.NoError(t, tester.Db.First(&tracker, "connection_id = ? AND name = ?", connectionId, "Bug").Error)
	assert.Equal(t, "New", tracker.DefaultStatusName, "a tracker keeps its default status")
	assert.NotZero(t, tracker.DefaultStatusId)

	var closed []models.RedmineGoStatus
	require.NoError(t, tester.Db.Find(&closed, "connection_id = ? AND is_closed = ?", connectionId, true).Error)
	names := make([]string, 0, len(closed))
	for _, status := range closed {
		names = append(names, status.Name)
	}
	assert.ElementsMatch(t, []string{"Closed", "Rejected", "Archived (rgo)"}, names, "is_closed is kept")

	assert.Equal(t, int64(5), count(t, tester, &models.RedmineGoPriority{}, ""))
	assert.Equal(t, int64(1), count(t, tester, &models.RedmineGoPriority{}, "is_default = ?", true))

	// Custom fields keep their format, multiplicity, labelled possible values
	// and trackers; fields of other customized types are dropped.
	var severity models.RedmineGoCustomField
	require.NoError(t, tester.Db.First(&severity, "connection_id = ? AND name = ?", connectionId, "RGO Severity").Error)
	assert.Equal(t, "enumeration", severity.FieldFormat)
	assert.False(t, severity.Multiple)
	assert.JSONEq(t, `[{"value":"7","label":"Minor"},{"value":"8","label":"Major"},{"value":"9","label":"Critical"}]`, severity.PossibleValues)
	assert.Contains(t, severity.Trackers, `"RGO Fixture"`)
	var environment models.RedmineGoCustomField
	require.NoError(t, tester.Db.First(&environment, "connection_id = ? AND name = ?", connectionId, "RGO Environment").Error)
	assert.True(t, environment.Multiple)
	assert.Zero(t, count(t, tester, &models.RedmineGoCustomField{}, "customized_type <> ?", "issue"))

	// Versions keep status, due date and sharing, in the owning project.
	var versions []models.RedmineGoVersion
	require.NoError(t, tester.Db.Find(&versions, "connection_id = ? AND project_id = ?", connectionId, 6).Error)
	statuses := map[string]string{}
	for _, version := range versions {
		statuses[version.Name] = version.Status
	}
	assert.Equal(t, "open", statuses["RGO 1.0 (open)"])
	assert.Equal(t, "closed", statuses["RGO 0.9 (closed)"])
	assert.Equal(t, "locked", statuses["RGO 0.8 (locked)"])

	// Issue categories belong to their project and keep the default assignee.
	var category models.RedmineGoIssueCategory
	require.NoError(t, tester.Db.First(&category, "connection_id = ? AND name = ?", connectionId, "RGO Backend").Error)
	assert.Equal(t, uint64(6), category.ProjectId)
	assert.Equal(t, "Fixture A", category.AssignedToName)
}

// Metadata is replaced on every run: an item that disappeared from Redmine
// disappears from the tool layer, and shared connection-level rows survive
// the next scope's run.
func TestMetadataIsReplacedPerRun(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	before := count(t, tester, &models.RedmineGoStatus{}, "")
	require.Positive(t, before)

	// Redmine lost a status: its raw row is gone from the next collection.
	deleteRawContaining(t, tester, "_raw_redmine_go_statuses", "Rejected by QA")
	tester.Subtask(tasks.ExtractStatusesMeta, newTaskData(6, zeroConfig()))
	assert.Equal(t, before-1, count(t, tester, &models.RedmineGoStatus{}, ""))
	assert.Zero(t, count(t, tester, &models.RedmineGoStatus{}, "name = ?", "Rejected by QA"))
}
