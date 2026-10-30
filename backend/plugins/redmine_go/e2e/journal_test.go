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
	"testing"

	"github.com/apache/devlake/helpers/e2ehelper"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/apache/devlake/plugins/redmine_go/tasks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// appendDetailVersion stores a new raw version of an issue detail, the way a
// later collection would: the issue's latest raw row, edited by edit, as a
// new row. The raw table keeps every version.
func appendDetailVersion(t *testing.T, tester *e2ehelper.DataFlowTester, issueId uint64, edit func(issue map[string]interface{})) {
	t.Helper()
	const table = "_raw_redmine_go_issue_details"
	var rows []helper.RawData
	require.NoError(t, tester.Db.Table(table).Order("id").Find(&rows).Error)
	var latest *helper.RawData
	for i := range rows {
		var input struct {
			Id uint64 `json:"id"`
		}
		require.NoError(t, json.Unmarshal(rows[i].Input, &input))
		if input.Id == issueId {
			latest = &rows[i]
		}
	}
	require.NotNil(t, latest, "no raw detail of issue %d", issueId)
	var issue map[string]interface{}
	require.NoError(t, json.Unmarshal(latest.Data, &issue))
	edit(issue)
	data, err := json.Marshal(issue)
	require.NoError(t, err)
	require.NoError(t, tester.Db.Table(table).Create(&helper.RawData{
		ID: nextRawId(t, tester, table), Params: latest.Params, Data: data, Url: latest.Url, Input: latest.Input,
	}).Error)
}

// journalNotes returns the stored journal ids of an issue with their notes.
func journalNotes(t *testing.T, tester *e2ehelper.DataFlowTester, issueId uint64) map[uint64]string {
	t.Helper()
	var journals []models.RedmineGoJournal
	require.NoError(t, tester.Db.Find(&journals, "connection_id = ? AND issue_id = ?", connectionId, issueId).Error)
	notes := map[uint64]string{}
	for _, journal := range journals {
		notes[journal.Id] = journal.Notes
	}
	return notes
}

func assertNoDuplicateDetails(t *testing.T, tester *e2ehelper.DataFlowTester) {
	t.Helper()
	var total, distinct int64
	require.NoError(t, tester.Db.Raw("SELECT COUNT(*) FROM _tool_redmine_go_journal_details").Row().Scan(&total))
	require.NoError(t, tester.Db.Raw("SELECT COUNT(*) FROM (SELECT DISTINCT connection_id, journal_id, detail_key FROM _tool_redmine_go_journal_details) AS keys").Row().Scan(&distinct))
	assert.Equal(t, total, distinct)
}

// An issue's journals are replaced as a whole when it is re-collected: an
// edited note updates in place, a journal that vanished is removed, a new one
// appears, and no row is duplicated. Only the newest raw version of an issue
// expands when several wait in one window, and a changed metadata name
// re-extracts the journals from the raw rows without a new collection.
func TestJournalsAreReplacedPerIssueOnReExtraction(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")
	beginStatefulScenario(t, tester)
	data := newTaskData(6, zeroConfig())
	runSubtaskPreservingState(t, tester, tasks.ExtractIssuesMeta, data)
	runSubtaskPreservingState(t, tester, tasks.ExtractIssueJournalsMeta, data)

	require.Equal(t, map[uint64]string{
		678: "RGO public note (edited)", 679: "RGO private note", 680: "RGO second public note", 685: "m2 incremental probe",
	}, journalNotes(t, tester, 175))
	journalsBefore := count(t, tester, &models.RedmineGoJournal{}, "")
	otherDetails := count(t, tester, &models.RedmineGoJournalDetail{}, "issue_id <> ?", 175)

	// Version 2: a note edited, a journal removed, a status change added.
	appendDetailVersion(t, tester, 175, func(issue map[string]interface{}) {
		journals := issue["journals"].([]interface{})
		journals[0].(map[string]interface{})["notes"] = "edited again"
		kept := []interface{}{journals[0], journals[1], journals[3]}
		kept = append(kept, map[string]interface{}{
			"id": 900, "user": map[string]interface{}{"id": 15, "name": "Fixture A"}, "notes": "closing",
			"created_on": "2026-10-31T09:00:00Z", "private_notes": false,
			"details": []interface{}{map[string]interface{}{"property": "attr", "name": "status_id", "old_value": "2", "new_value": "5"}},
		})
		issue["journals"] = kept
	})
	runSubtaskPreservingState(t, tester, tasks.ExtractIssueJournalsMeta, data)

	assert.Equal(t, map[uint64]string{
		678: "edited again", 679: "RGO private note", 685: "m2 incremental probe", 900: "closing",
	}, journalNotes(t, tester, 175))
	assert.Equal(t, journalsBefore, count(t, tester, &models.RedmineGoJournal{}, ""), "one journal removed, one added")
	assert.Equal(t, otherDetails+2, count(t, tester, &models.RedmineGoJournalDetail{}, ""), "175's two details: the earlier status change and the new one")
	assert.Equal(t, otherDetails, count(t, tester, &models.RedmineGoJournalDetail{}, "issue_id <> ?", 175), "other issues are untouched")
	var added models.RedmineGoJournalDetail
	require.NoError(t, tester.Db.First(&added, "connection_id = ? AND journal_id = ?", connectionId, 900).Error)
	assert.Equal(t, "status", added.FieldName)
	assert.Equal(t, "In Progress", added.OldLabel)
	assert.Equal(t, "Closed", added.NewLabel)
	assert.Equal(t, "Fixture A", added.UserName)
	assertNoDuplicateDetails(t, tester)

	// Nothing new was collected: a rerun changes nothing.
	runSubtaskPreservingState(t, tester, tasks.ExtractIssueJournalsMeta, data)
	assert.Equal(t, journalsBefore, count(t, tester, &models.RedmineGoJournal{}, ""))
	assert.Equal(t, otherDetails+2, count(t, tester, &models.RedmineGoJournalDetail{}, ""))
	assertNoDuplicateDetails(t, tester)

	// Two versions wait in one window: the older one must not bring back a
	// journal the newer one no longer has.
	appendDetailVersion(t, tester, 175, func(issue map[string]interface{}) {
		issue["journals"] = append(issue["journals"].([]interface{}), map[string]interface{}{
			"id": 901, "user": map[string]interface{}{"id": 1, "name": "Redmine Admin"}, "notes": "short lived",
			"created_on": "2026-10-31T10:00:00Z", "private_notes": false, "details": []interface{}{},
		})
	})
	appendDetailVersion(t, tester, 175, func(issue map[string]interface{}) {
		journals := issue["journals"].([]interface{})
		issue["journals"] = journals[:len(journals)-1]
	})
	runSubtaskPreservingState(t, tester, tasks.ExtractIssueJournalsMeta, data)
	_, resurrected := journalNotes(t, tester, 175)[901]
	assert.False(t, resurrected, "the stale version stays dead")
	assert.Len(t, journalNotes(t, tester, 175), 4)

	// A renamed status re-extracts every journal from raw, with no collection.
	require.NoError(t, tester.Db.Model(&models.RedmineGoStatus{}).Where("connection_id = ? AND id = ?", connectionId, 2).
		Update("name", "Doing").Error)
	runSubtaskPreservingState(t, tester, tasks.ExtractIssueJournalsMeta, data)
	require.NoError(t, tester.Db.First(&added, "connection_id = ? AND journal_id = ?", connectionId, 900).Error)
	assert.Equal(t, "Doing", added.OldLabel, "labels follow the metadata")
	assertNoDuplicateDetails(t, tester)
}

// A journal that adds several values to one multi-value custom field yields
// one row per value, with distinct stable keys; a second extraction produces
// the same keys.
func TestJournalDetailKeysSurviveReExtraction(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")
	extractIssues(tester, 6)

	keysOf := func() []string {
		var keys []string
		require.NoError(t, tester.Db.Model(&models.RedmineGoJournalDetail{}).Order("journal_id, detail_key").
			Pluck("CONCAT(journal_id, '/', detail_key)", &keys).Error)
		return keys
	}
	first := keysOf()
	require.NotEmpty(t, first)

	// Journal 677 of issue 174 changes RGO Environment three times: one value
	// out, two values in.
	var rows []models.RedmineGoJournalDetail
	require.NoError(t, tester.Db.Find(&rows, "connection_id = ? AND journal_id = ? AND name = ?", connectionId, 677, "13").Error)
	require.Len(t, rows, 3)
	assert.Len(t, map[string]bool{rows[0].DetailKey: true, rows[1].DetailKey: true, rows[2].DetailKey: true}, 3)
	for _, row := range rows {
		assert.Equal(t, "RGO Environment", row.FieldName)
		assert.Equal(t, "cf", row.Property)
	}
	assertNoDuplicateDetails(t, tester)

	extractIssues(tester, 6)
	assert.Equal(t, first, keysOf(), "keys are stable across re-extractions")
}

// The issue extractor goes incremental too: a new raw version of an issue
// updates its tool row, and the others stay as they were.
func TestIssueExtractionIsIncremental(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")
	beginStatefulScenario(t, tester)
	data := newTaskData(6, zeroConfig())
	runSubtaskPreservingState(t, tester, tasks.ExtractIssuesMeta, data)
	require.Equal(t, int64(24), count(t, tester, &models.RedmineGoIssue{}, "project_id = ?", 6))

	// Mark the stored rows so a re-extraction of them would be visible.
	require.NoError(t, tester.Db.Model(&models.RedmineGoIssue{}).Where("1 = 1").Update("subject", "untouched").Error)

	const table = "_raw_redmine_go_issues"
	var rows []helper.RawData
	require.NoError(t, tester.Db.Table(table).Order("id").Find(&rows).Error)
	var issue map[string]interface{}
	var source helper.RawData
	for _, row := range rows {
		require.NoError(t, json.Unmarshal(row.Data, &issue))
		if issue["id"].(float64) == 153 {
			source = row
			break
		}
	}
	issue["subject"] = "changed in Redmine"
	issue["status"] = map[string]interface{}{"id": 5, "name": "Closed", "is_closed": true}
	issue["closed_on"] = "2026-10-31T08:00:00Z"
	issue["updated_on"] = "2026-10-31T08:00:00Z"
	data2, err := json.Marshal(issue)
	require.NoError(t, err)
	require.NoError(t, tester.Db.Table(table).Create(&helper.RawData{
		ID: nextRawId(t, tester, table), Params: source.Params, Data: data2, Url: source.Url,
	}).Error)

	runSubtaskPreservingState(t, tester, tasks.ExtractIssuesMeta, data)
	assert.Equal(t, int64(24), count(t, tester, &models.RedmineGoIssue{}, "project_id = ?", 6), "no issue added or lost")
	assert.Equal(t, int64(1), count(t, tester, &models.RedmineGoIssue{}, "subject = ?", "changed in Redmine"))
	assert.Equal(t, int64(23), count(t, tester, &models.RedmineGoIssue{}, "project_id = ? AND subject = ?", 6, "untouched"), "only the changed issue was extracted again")
	var changed models.RedmineGoIssue
	require.NoError(t, tester.Db.First(&changed, "connection_id = ? AND id = ?", connectionId, 153).Error)
	assert.Equal(t, "Closed", changed.StatusName)
	assert.True(t, changed.StatusIsClosed)
	require.NotNil(t, changed.RedmineClosedOn)
}
