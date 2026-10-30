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
	"fmt"
	"sort"
	"testing"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/models/domainlayer"
	"github.com/apache/devlake/core/models/domainlayer/crossdomain"
	"github.com/apache/devlake/core/models/domainlayer/ticket"
	"github.com/apache/devlake/helpers/e2ehelper"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/apache/devlake/plugins/redmine_go/tasks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// relationship is a row of issue_relationships by Redmine issue id.
type relationship struct {
	Source, Target uint64
	Type           string
}

// relationships reads issue_relationships back as Redmine issue ids.
func relationships(t *testing.T, tester *e2ehelper.DataFlowTester) []relationship {
	t.Helper()
	var rows []ticket.IssueRelationship
	require.NoError(t, tester.Db.Find(&rows).Error)
	var out []relationship
	for _, row := range rows {
		var source, target uint64
		_, err := fmt.Sscanf(row.SourceIssueId, "redmine_go:RedmineGoIssue:1:%d", &source)
		require.NoError(t, err)
		_, err = fmt.Sscanf(row.TargetIssueId, "redmine_go:RedmineGoIssue:1:%d", &target)
		require.NoError(t, err)
		out = append(out, relationship{source, target, row.OriginalType})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Source < out[j].Source || out[i].Source == out[j].Source && out[i].Target < out[j].Target
	})
	return out
}

// fixtureRelationships are the relations of the RGO fixtures as Redmine lists
// them, one row per relation read from its source to its target.
var fixtureRelationships = []relationship{
	{158, 159, "relates"}, {160, 161, "duplicates"}, {162, 163, "blocks"}, {164, 165, "precedes"}, {166, 167, "copied_to"},
}

// setSweepRelations rewrites the relations of one issue in the sweep raw rows
// of a project, as a Redmine whose relations changed would list them.
func setSweepRelations(t *testing.T, tester *e2ehelper.DataFlowTester, projectId, issueId uint64, relations []map[string]interface{}) {
	t.Helper()
	params := fmt.Sprintf(`{"ConnectionId":1,"ProjectId":%d}`, projectId)
	var rows []helper.RawData
	require.NoError(t, tester.Db.Table(rawSweepTable).Where("params = ?", params).Find(&rows).Error)
	for _, row := range rows {
		var body map[string]interface{}
		require.NoError(t, json.Unmarshal(row.Data, &body))
		if uint64(body["id"].(float64)) != issueId {
			continue
		}
		body["relations"] = relations
		data, err := json.Marshal(body)
		require.NoError(t, err)
		require.NoError(t, tester.Db.Table(rawSweepTable).Where("id = ?", row.ID).Update("data", data).Error)
		return
	}
	require.Failf(t, "sweep row missing", "issue %d is not in the sweep of project %d", issueId, projectId)
}

func relation(id, from, to uint64, kind string) map[string]interface{} {
	return map[string]interface{}{"id": id, "issue_id": from, "issue_to_id": to, "relation_type": kind, "delay": nil}
}

func sweepOff() models.Transformation {
	off := false
	return models.Transformation{ReconcileSweep: &off}
}

func TestRelationsFromIssueDetails(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")
	for _, projectId := range []uint64{6, 7, 8} {
		runPlanned(tester, newTaskData(projectId, sweepOff()))
	}

	verifyToolTable(tester, &models.RedmineGoIssueRelation{})
	verifyDomainTable(tester, &ticket.IssueRelationship{}, "issue_relationships")
	assert.Equal(t, fixtureRelationships, relationships(t, tester),
		"one row per relation, read from its source: a pair that Redmine lists under both issues is not doubled")
	assert.Equal(t, int64(5), count(t, tester, &models.RedmineGoIssueRelation{}, "project_id = ?", 6))
	var precedes models.RedmineGoIssueRelation
	require.NoError(t, tester.Db.First(&precedes, "connection_id = ? AND id = ?", connectionId, 18).Error)
	require.NotNil(t, precedes.Delay)
	assert.Equal(t, 2, *precedes.Delay, "the delay of a precedes relation is kept")
}

func TestRelationSweepAddsAndRemovesRelationsWithoutAnUpdatedOnChange(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")
	for _, projectId := range []uint64{6, 7, 8} {
		runPlanned(tester, newTaskData(projectId, zeroConfig()))
	}
	assert.Equal(t, fixtureRelationships, relationships(t, tester), "the sweep of unchanged projects changes nothing")
	assert.Equal(t, int64(26), count(t, tester, &ticket.Issue{}, ""), "and deletes no issue")
	assert.Equal(t, int64(26), count(t, tester, &models.RedmineGoIssue{}, ""))

	// A relation is added in Redmine: neither issue's updated_on moves, so only
	// the sweep sees it. Redmine lists it under both issues.
	setSweepRelations(t, tester, 6, 152, []map[string]interface{}{relation(91, 152, 153, "relates")})
	setSweepRelations(t, tester, 6, 153, []map[string]interface{}{relation(91, 152, 153, "relates")})
	runPlanned(tester, newTaskData(6, zeroConfig()))
	assert.Contains(t, relationships(t, tester), relationship{152, 153, "relates"})
	assert.Len(t, relationships(t, tester), 6)
	assert.Equal(t, int64(1), count(t, tester, &models.RedmineGoIssueRelation{}, "id = ?", 91))

	// A relation is removed: it vanishes from the tool and the domain layer,
	// and the others stay.
	setSweepRelations(t, tester, 6, 158, nil)
	setSweepRelations(t, tester, 6, 159, nil)
	runPlanned(tester, newTaskData(6, zeroConfig()))
	assert.NotContains(t, relationships(t, tester), relationship{158, 159, "relates"})
	assert.Zero(t, count(t, tester, &models.RedmineGoIssueRelation{}, "id = ?", 15))
	assert.Len(t, relationships(t, tester), 5)

	// A relation between projects belongs to the project of its source issue,
	// even though the target's list shows it too.
	cross := relation(92, 154, 152, "blocks")
	setSweepRelations(t, tester, 7, 154, []map[string]interface{}{cross})
	setSweepRelations(t, tester, 6, 152, []map[string]interface{}{relation(91, 152, 153, "relates"), cross})
	runPlanned(tester, newTaskData(6, zeroConfig()))
	runPlanned(tester, newTaskData(7, zeroConfig()))
	assert.Equal(t, int64(1), count(t, tester, &models.RedmineGoIssueRelation{}, "id = ? AND project_id = ?", 92, 7), "owned by the source's project")
	assert.Contains(t, relationships(t, tester), relationship{154, 152, "blocks"})
	runPlanned(tester, newTaskData(6, zeroConfig()))
	assert.Equal(t, int64(1), count(t, tester, &models.RedmineGoIssueRelation{}, "id = ?", 92), "the target's project neither duplicates nor deletes it")
	assert.Contains(t, relationships(t, tester), relationship{154, 152, "blocks"})

	// And its removal is seen by the source's project.
	setSweepRelations(t, tester, 7, 154, nil)
	setSweepRelations(t, tester, 6, 152, []map[string]interface{}{relation(91, 152, 153, "relates")})
	runPlanned(tester, newTaskData(7, zeroConfig()))
	assert.Zero(t, count(t, tester, &models.RedmineGoIssueRelation{}, "id = ?", 92))
	assert.NotContains(t, relationships(t, tester), relationship{154, 152, "blocks"})
}

// issueFootprint counts the rows that exist for a Redmine issue in every table
// the plugin fills, by table name.
func issueFootprint(t *testing.T, tester *e2ehelper.DataFlowTester, issueId uint64) map[string]int64 {
	t.Helper()
	domainId := fmt.Sprintf("redmine_go:RedmineGoIssue:%d:%d", connectionId, issueId)
	byIssue := func(model interface{}) int64 {
		return count(t, tester, model, "connection_id = ? AND issue_id = ?", connectionId, issueId)
	}
	return map[string]int64{
		"_tool_redmine_go_issues":           count(t, tester, &models.RedmineGoIssue{}, "connection_id = ? AND id = ?", connectionId, issueId),
		"_tool_redmine_go_journals":         byIssue(&models.RedmineGoJournal{}),
		"_tool_redmine_go_journal_details":  byIssue(&models.RedmineGoJournalDetail{}),
		"_tool_redmine_go_issue_relations":  byIssue(&models.RedmineGoIssueRelation{}),
		"_tool_redmine_go_issue_changesets": byIssue(&models.RedmineGoIssueChangeset{}),
		"_tool_redmine_go_time_entries":     byIssue(&models.RedmineGoTimeEntry{}),
		"issues":                            count(t, tester, &ticket.Issue{}, "id = ?", domainId),
		"board_issues":                      count(t, tester, &ticket.BoardIssue{}, "issue_id = ?", domainId),
		"issue_changelogs":                  count(t, tester, &ticket.IssueChangelogs{}, "issue_id = ?", domainId),
		"issue_comments":                    count(t, tester, &ticket.IssueComment{}, "issue_id = ?", domainId),
		"issue_worklogs":                    count(t, tester, &ticket.IssueWorklog{}, "issue_id = ?", domainId),
		"issue_relationships":               count(t, tester, &ticket.IssueRelationship{}, "source_issue_id = ?", domainId),
		"issue_assignees":                   count(t, tester, &ticket.IssueAssignee{}, "issue_id = ?", domainId),
		"issue_commits":                     count(t, tester, &crossdomain.IssueCommit{}, "issue_id = ?", domainId),
		"incidents":                         count(t, tester, &ticket.Incident{}, "id = ?", domainId),
		"incident_assignees":                count(t, tester, &ticket.IncidentAssignee{}, "incident_id = ?", domainId),
	}
}

// seedDeletedAndMovedIssues stores what an earlier run of project 6 collected
// of two issues that have since left it: 171, which was deleted, and 170,
// which moved to project 8. 171 carries notes, a status change, a relation, a
// changeset, a time entry and an incident, so every table that must be pruned
// has a row.
func seedDeletedAndMovedIssues(t *testing.T, tester *e2ehelper.DataFlowTester) {
	t.Helper()
	params := `{"ConnectionId":1,"ProjectId":6}`
	inProject6 := func(issue map[string]interface{}) {
		issue["project"] = map[string]interface{}{"id": 6, "name": "RGO Fixtures"}
	}
	cloneRaw(t, tester, rawIssuesTable, params, 175, 171, nil)
	cloneRaw(t, tester, rawIssuesTable, params, 170, 170, inProject6)
	cloneRaw(t, tester, rawDetailsTable, params, 175, 171, func(issue map[string]interface{}) {
		for _, journal := range issue["journals"].([]interface{}) {
			journal := journal.(map[string]interface{})
			journal["id"] = journal["id"].(float64) + 100000
		}
		issue["relations"] = []interface{}{relation(95, 171, 152, "relates")}
		issue["changesets"] = []interface{}{map[string]interface{}{
			"revision": "ghost0000000000000000000000000000000000", "comments": "refs #171", "committed_on": "2026-10-30T01:00:00Z",
		}}
	})
	cloneRaw(t, tester, rawDetailsTable, params, 170, 170, inProject6)

	for _, ghost := range []struct{ id, issueId uint64 }{{352, 171}, {354, 170}} {
		require.NoError(t, tester.Db.Table(rawTimeEntriesTable).Create(&helper.RawData{
			ID: nextRawId(t, tester, rawTimeEntriesTable), Params: params, Url: "time_entries.json", Input: []byte("{}"),
			Data: []byte(fmt.Sprintf(`{"id":%d,"project":{"id":6},"issue":{"id":%d},"user":{"id":1,"name":"Redmine Admin"},`+
				`"activity":{"id":9,"name":"Development"},"hours":1,"comments":"ghost","spent_on":"2026-10-30",`+
				`"created_on":"2026-10-30T00:40:52Z","updated_on":"2026-10-30T00:40:52Z"}`, ghost.id, ghost.issueId)),
		}).Error)
	}
}

func seedIncident(t *testing.T, tester *e2ehelper.DataFlowTester, issueId uint64) {
	t.Helper()
	domainId := fmt.Sprintf("redmine_go:RedmineGoIssue:%d:%d", connectionId, issueId)
	require.NoError(t, tester.Db.Create(&ticket.Incident{DomainEntity: domainlayer.DomainEntity{Id: domainId}}).Error)
	require.NoError(t, tester.Db.Create(&ticket.IncidentAssignee{IncidentId: domainId, AssigneeId: "redmine_go:RedmineGoAccount:1:1"}).Error)
}

func TestSweepRemovesDeletedAndMovedIssuesFromBothLayers(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")
	seedDeletedAndMovedIssues(t, tester)

	// The earlier run: project 6 collected both issues, with the sweep off.
	runPlanned(tester, newTaskData(6, sweepOff()))
	seedIncident(t, tester, 171)
	for _, issueId := range []uint64{170, 171} {
		footprint := issueFootprint(t, tester, issueId)
		assert.Equal(t, int64(1), footprint["_tool_redmine_go_issues"], "issue %d was collected", issueId)
		assert.Equal(t, int64(1), footprint["issues"], "issue %d", issueId)
		assert.Equal(t, int64(1), footprint["board_issues"], "issue %d", issueId)
		assert.Positive(t, footprint["issue_changelogs"], "issue %d", issueId)
		assert.Equal(t, int64(1), footprint["issue_worklogs"], "issue %d", issueId)
	}
	footprint := issueFootprint(t, tester, 171)
	for table, rows := range footprint {
		if table == "issue_assignees" {
			continue // the cloned issue has no assignee
		}
		assert.Positive(t, rows, "the deleted issue must leave rows in %s for the sweep to prune", table)
	}
	assert.Equal(t, int64(26), count(t, tester, &ticket.Issue{}, ""), "24 issues of the project and the two that left it")

	// A project whose sweep is off prunes nothing, however stale its rows.
	runPlanned(tester, newTaskData(6, sweepOff()))
	assert.Equal(t, int64(26), count(t, tester, &ticket.Issue{}, ""))

	// The sweep lists neither: both leave the tool and the domain layer at
	// once, before any conversion runs again.
	tester.Subtask(tasks.ExtractIssueSweepMeta, newTaskData(6, zeroConfig()))
	for _, issueId := range []uint64{170, 171} {
		for table, rows := range issueFootprint(t, tester, issueId) {
			assert.Zero(t, rows, "issue %d must be gone from %s", issueId, table)
		}
	}
	assert.Equal(t, int64(24), count(t, tester, &models.RedmineGoIssue{}, "project_id = ?", 6), "the other issues stay")
	assert.Equal(t, int64(24), count(t, tester, &ticket.Issue{}, ""))
	assert.Zero(t, count(t, tester, &models.RedmineGoIssueRelation{}, "id = ?", 95))

	// The next full run, with the stale raw rows gone as a fresh collection
	// leaves them, converges to the same state; then project 8 collects the
	// moved issue and it appears under its new board.
	for _, table := range []string{rawIssuesTable, rawDetailsTable} {
		for _, issueId := range []uint64{170, 171} {
			deleteRawIssue(t, tester, table, `{"ConnectionId":1,"ProjectId":6}`, issueId)
		}
	}
	for _, text := range []string{`"id":352`, `"id":354`} {
		deleteRawContaining(t, tester, rawTimeEntriesTable, text)
	}
	runPlanned(tester, newTaskData(6, zeroConfig()))
	assert.Equal(t, int64(24), count(t, tester, &ticket.Issue{}, ""))
	assert.Zero(t, count(t, tester, &ticket.BoardIssue{}, "issue_id = ?", "redmine_go:RedmineGoIssue:1:170"))

	runPlanned(tester, newTaskData(8, zeroConfig()))
	moved := issueByKey(t, tester, "170")
	assert.Equal(t, "rgo-fixtures-b", moved.OriginalProject)
	assert.Equal(t, int64(1), count(t, tester, &ticket.BoardIssue{}, "issue_id = ? AND board_id = ?", moved.Id, "redmine_go:RedmineGoProject:1:8"))
	assert.Zero(t, count(t, tester, &ticket.BoardIssue{}, "issue_id = ? AND board_id = ?", moved.Id, "redmine_go:RedmineGoProject:1:6"),
		"the moved issue is no longer on project 6's board")
	var tool models.RedmineGoIssue
	require.NoError(t, tester.Db.First(&tool, "connection_id = ? AND id = ?", connectionId, 170).Error)
	assert.Equal(t, uint64(8), tool.ProjectId)
}

func TestSweepKeepsAnIssueRedmineStillHasInTheProject(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")
	seedDeletedAndMovedIssues(t, tester)
	runPlanned(tester, newTaskData(6, sweepOff()))

	// An issue deleted while the sweep pages through the list shifts a
	// neighbour off its page. The probe tells that apart from a deletion.
	var probed []uint64
	data := newTaskData(6, zeroConfig())
	data.IssueProbe = func(issueId, projectId uint64) (bool, errors.Error) {
		require.Equal(t, uint64(6), projectId)
		probed = append(probed, issueId)
		return issueId == 171, nil
	}
	tester.Subtask(tasks.ExtractIssueSweepMeta, data)

	assert.ElementsMatch(t, []uint64{170, 171}, probed, "every missing issue is confirmed with Redmine before it is deleted")
	assert.Equal(t, int64(1), issueFootprint(t, tester, 171)["_tool_redmine_go_issues"], "the issue Redmine still has is kept")
	assert.Equal(t, int64(1), issueFootprint(t, tester, 171)["_tool_redmine_go_issue_relations"], "with its relation")
	assert.Equal(t, int64(1), issueFootprint(t, tester, 171)["issues"])
	assert.Zero(t, issueFootprint(t, tester, 170)["_tool_redmine_go_issues"], "the one it no longer has in the project is deleted")
}

func TestTimeEntriesBecomeWorklogs(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")
	// A deleted entry that an earlier run stored, and an entry on an issue of
	// another project: neither may produce a worklog.
	params := `{"ConnectionId":1,"ProjectId":6}`
	entry := func(id, issueId int, hours float64) *helper.RawData {
		return &helper.RawData{
			ID: nextRawId(t, tester, rawTimeEntriesTable) + uint64(id), Params: params, Url: "time_entries.json", Input: []byte("{}"),
			Data: []byte(fmt.Sprintf(`{"id":%d,"issue":{"id":%d},"user":{"id":1,"name":"Redmine Admin"},"activity":{"id":9,"name":"Development"},`+
				`"hours":%v,"comments":"extra","spent_on":"2026-10-29","created_on":"2026-10-29T10:00:00Z","updated_on":"2026-10-29T10:00:00Z"}`, id, issueId, hours)),
		}
	}
	require.NoError(t, tester.Db.Table(rawTimeEntriesTable).Create(entry(352, 176, 1)).Error)
	require.NoError(t, tester.Db.Table(rawTimeEntriesTable).Create(entry(353, 154, 4)).Error)

	data := newTaskData(6, zeroConfig())
	runPlanned(tester, data)
	verifyToolTable(tester, &models.RedmineGoTimeEntry{})
	verifyDomainTable(tester, &ticket.IssueWorklog{}, "issue_worklogs")
	assert.Equal(t, int64(4), count(t, tester, &models.RedmineGoTimeEntry{}, ""),
		"entries on issues are kept, the project-level entry (350) is skipped")
	assert.Zero(t, count(t, tester, &models.RedmineGoTimeEntry{}, "id = ?", 350))
	assert.Equal(t, int64(3), count(t, tester, &ticket.IssueWorklog{}, ""), "an entry on an issue outside the project makes no worklog")
	var edited ticket.IssueWorklog
	require.NoError(t, tester.Db.First(&edited, "id = ?", "redmine_go:RedmineGoTimeEntry:1:351").Error)
	assert.Equal(t, 150, edited.TimeSpentMinutes, "2.5 hours")
	assert.Equal(t, "RGO time entry (edited)", edited.Comment)
	assert.Equal(t, "redmine_go:RedmineGoIssue:1:176", edited.IssueId)
	assert.Equal(t, "redmine_go:RedmineGoAccount:1:1", edited.AuthorId)
	assert.Equal(t, "Development", toolEntry(t, tester, 351).ActivityName, "the activity stays in the tool table")

	// The next collection: entry 349 was edited, entry 352 deleted in Redmine.
	// Redmine ignores updated_on on time entries, so only a full pull sees both.
	deleteRawContaining(t, tester, rawTimeEntriesTable, `"id":352`)
	var raw helper.RawData
	require.NoError(t, tester.Db.Table(rawTimeEntriesTable).Where("data LIKE ?", `%"id":349,%`).First(&raw).Error)
	require.NoError(t, tester.Db.Table(rawTimeEntriesTable).Where("id = ?", raw.ID).
		Update("data", []byte(`{"id":349,"project":{"id":6},"issue":{"id":176},"user":{"id":1,"name":"Redmine Admin"},`+
			`"activity":{"id":10,"name":"Design"},"hours":3,"comments":"RGO issue-level time (edited again)","spent_on":"2026-10-30",`+
			`"created_on":"2026-10-30T00:40:52Z","updated_on":"2026-10-30T09:00:00Z"}`)).Error)
	runPlanned(tester, data)
	assert.Equal(t, int64(3), count(t, tester, &models.RedmineGoTimeEntry{}, ""))
	assert.Zero(t, count(t, tester, &models.RedmineGoTimeEntry{}, "id = ?", 352), "the deleted entry is gone from the tool layer")
	assert.Zero(t, count(t, tester, &ticket.IssueWorklog{}, "id = ?", "redmine_go:RedmineGoTimeEntry:1:352"), "and from the domain layer")
	assert.Equal(t, int64(2), count(t, tester, &ticket.IssueWorklog{}, ""))
	var changed ticket.IssueWorklog
	require.NoError(t, tester.Db.First(&changed, "id = ?", "redmine_go:RedmineGoTimeEntry:1:349").Error)
	assert.Equal(t, 180, changed.TimeSpentMinutes, "the edit replaced the old hours")
	assert.Equal(t, "RGO issue-level time (edited again)", changed.Comment)
	assert.Equal(t, "Design", toolEntry(t, tester, 349).ActivityName)

	// A project whose entries are all gone loses all its worklogs.
	require.NoError(t, tester.Db.Exec("DELETE FROM "+rawTimeEntriesTable).Error)
	runPlanned(tester, data)
	assert.Zero(t, count(t, tester, &models.RedmineGoTimeEntry{}, ""))
	assert.Zero(t, count(t, tester, &ticket.IssueWorklog{}, ""))
}

func toolEntry(t *testing.T, tester *e2ehelper.DataFlowTester, id uint64) models.RedmineGoTimeEntry {
	t.Helper()
	var entry models.RedmineGoTimeEntry
	require.NoError(t, tester.Db.First(&entry, "connection_id = ? AND id = ?", connectionId, id).Error)
	return entry
}

func TestPublicPrivateAndEditedNotesBecomeComments(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")

	for _, projectId := range []uint64{6, 7, 8} {
		runPlanned(tester, newTaskData(projectId, zeroConfig()))
	}
	verifyDomainTable(tester, &ticket.IssueComment{}, "issue_comments")
	var comments []ticket.IssueComment
	require.NoError(t, tester.Db.Order("id").Find(&comments, "issue_id = ?", "redmine_go:RedmineGoIssue:1:175").Error)
	bodies := map[string]ticket.IssueComment{}
	for _, comment := range comments {
		bodies[comment.Body] = comment
	}
	assert.Contains(t, bodies, "RGO public note (edited)")
	assert.Contains(t, bodies, "RGO second public note")
	assert.NotContains(t, bodies, "RGO private note", "private notes are left out by default")
	edited := bodies["RGO public note (edited)"]
	require.NotNil(t, edited.UpdatedDate, "an edited note carries its update time")
	assert.True(t, edited.UpdatedDate.After(edited.CreatedDate))
	assert.Nil(t, bodies["RGO second public note"].UpdatedDate, "an unedited note has none")
	assert.Equal(t, "redmine_go:RedmineGoAccount:1:1", edited.AccountId)
	assert.Zero(t, count(t, tester, &ticket.IssueComment{}, "body = ?", ""), "journals without a note make no comment")
	withoutPrivate := count(t, tester, &ticket.IssueComment{}, "")

	// Switching includePrivateNotes on adds the private note, with no
	// re-extraction: it is applied at conversion.
	on := true
	includePrivate := models.Transformation{IncludePrivateNotes: &on}
	toolBefore := count(t, tester, &models.RedmineGoJournal{}, "")
	for _, projectId := range []uint64{6, 7, 8} {
		tester.Subtask(tasks.ConvertIssueCommentsMeta, newTaskData(projectId, includePrivate))
	}
	assert.Equal(t, withoutPrivate+1, count(t, tester, &ticket.IssueComment{}, ""))
	verifyDomainTable(tester, &ticket.IssueComment{}, "issue_comments_with_private")
	assert.Equal(t, toolBefore, count(t, tester, &models.RedmineGoJournal{}, ""))

	// And off again removes it.
	for _, projectId := range []uint64{6, 7, 8} {
		tester.Subtask(tasks.ConvertIssueCommentsMeta, newTaskData(projectId, zeroConfig()))
	}
	assert.Equal(t, withoutPrivate, count(t, tester, &ticket.IssueComment{}, ""))
}

func TestChangesetsBecomeIssueCommits(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")
	for _, projectId := range []uint64{6, 7, 8} {
		runPlanned(tester, newTaskData(projectId, zeroConfig()))
	}

	verifyToolTable(tester, &models.RedmineGoIssueChangeset{})
	verifyDomainTable(tester, &crossdomain.IssueCommit{}, "issue_commits")
	var commits []crossdomain.IssueCommit
	require.NoError(t, tester.Db.Order("issue_id, commit_sha").Find(&commits).Error)
	var got []string
	for _, commit := range commits {
		got = append(got, commit.IssueId[len("redmine_go:RedmineGoIssue:1:"):]+" "+commit.CommitSha[:7])
	}
	assert.Equal(t, []string{"177 6cde351", "177 7042446", "178 6c08579", "178 7042446"}, got,
		"the revision is the commit sha; a commit that refs two issues is a row for each")
	var changeset models.RedmineGoIssueChangeset
	require.NoError(t, tester.Db.First(&changeset, "connection_id = ? AND issue_id = ? AND revision LIKE ?", connectionId, 177, "6cde351%").Error)
	assert.Equal(t, "RGO fixture commit one, refs #177", changeset.Comments, "the message stays in the tool table")
	require.NotNil(t, changeset.CommittedOn)
}

func TestVersionsBecomeSprintsOnlyWhenTheToggleIsOn(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")

	// Off by default: fix_versions is filled, and nothing else.
	runPlanned(tester, newTaskData(6, zeroConfig()))
	assert.Equal(t, "RGO 0.9 (closed)", issueByKey(t, tester, "152").FixVersions)
	assert.Zero(t, count(t, tester, &ticket.Sprint{}, ""))
	assert.Zero(t, count(t, tester, &ticket.BoardSprint{}, ""))
	assert.Zero(t, count(t, tester, &ticket.SprintIssue{}, ""))
	assert.NotContains(t, plannedNames(newTaskData(6, zeroConfig())), "convertSprints")

	on := true
	data := newTaskData(6, models.Transformation{VersionsAsSprints: &on})
	runPlanned(tester, data)
	verifyDomainTable(tester, &ticket.Sprint{}, "sprints")
	verifyDomainTable(tester, &ticket.BoardSprint{}, "board_sprints")
	verifyDomainTable(tester, &ticket.SprintIssue{}, "sprint_issues")

	status := map[string]string{}
	var sprints []ticket.Sprint
	require.NoError(t, tester.Db.Find(&sprints).Error)
	for _, sprint := range sprints {
		status[sprint.Name] = sprint.Status
	}
	assert.Equal(t, map[string]string{
		"RGO 1.0 (open)": "ACTIVE", "RGO 2.0 (open)": "ACTIVE", "RGO 0.9 (closed)": "CLOSED", "RGO 0.8 (locked)": "CLOSED",
	}, status, "open is active; closed and locked take no more issues")
	var closed ticket.Sprint
	require.NoError(t, tester.Db.First(&closed, "name = ?", "RGO 0.9 (closed)").Error)
	assert.Equal(t, "redmine_go:RedmineGoProject:1:6", closed.OriginalBoardID)
	assert.Equal(t, "https://redmine.example.org/versions/23", closed.Url)
	assert.NotNil(t, closed.StartedDate)

	var members []ticket.SprintIssue
	require.NoError(t, tester.Db.Find(&members).Error)
	sprintOf := map[string]string{}
	for _, member := range members {
		sprintOf[member.IssueId[len("redmine_go:RedmineGoIssue:1:"):]] = member.SprintId[len("redmine_go:RedmineGoVersion:1:"):]
	}
	assert.Equal(t, map[string]string{"152": "23", "153": "24"}, sprintOf, "an issue is in the sprint of its target version")
	assert.Equal(t, int64(4), count(t, tester, &ticket.BoardSprint{}, "board_id = ?", "redmine_go:RedmineGoProject:1:6"))

	// Switched off again, a run still keeps fix_versions.
	runPlanned(tester, newTaskData(6, zeroConfig()))
	assert.Equal(t, "RGO 0.9 (closed)", issueByKey(t, tester, "152").FixVersions)
}

func TestVersionsOfOtherProjectsAreSprintsOfTheBoardsThatUseThem(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")
	runPlanned(tester, newTaskData(8, models.Transformation{}))
	on := true
	runPlanned(tester, newTaskData(8, models.Transformation{VersionsAsSprints: &on}))
	assert.Zero(t, count(t, tester, &ticket.Sprint{}, ""), "project 8 owns no version and its issue has no target version")
	assert.Zero(t, count(t, tester, &ticket.BoardSprint{}, ""))
}

func TestDisabledTogglesProduceNoRows(t *testing.T) {
	off := false
	allOff := models.Transformation{
		ReconcileSweep: &off, CollectTimeEntries: &off, CollectChangesets: &off, CollectComments: &off,
	}
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")
	seedDeletedAndMovedIssues(t, tester)

	for _, projectId := range []uint64{6, 7, 8} {
		data := newTaskData(projectId, allOff)
		names := plannedNames(data)
		for _, gated := range []string{
			"collectIssueSweep", "extractIssueSweep", "collectTimeEntries", "extractTimeEntries", "convertWorklogs",
			"extractIssueChangesets", "convertIssueCommits", "convertIssueComments", "convertSprints",
		} {
			assert.NotContains(t, names, gated, "%s is gated by a toggle that is off", gated)
		}
		runPlanned(tester, data)
	}
	assert.Zero(t, count(t, tester, &models.RedmineGoTimeEntry{}, ""))
	assert.Zero(t, count(t, tester, &ticket.IssueWorklog{}, ""))
	assert.Zero(t, count(t, tester, &models.RedmineGoIssueChangeset{}, ""))
	assert.Zero(t, count(t, tester, &crossdomain.IssueCommit{}, ""))
	assert.Zero(t, count(t, tester, &ticket.IssueComment{}, ""))
	assert.Zero(t, count(t, tester, &ticket.Sprint{}, ""))
	assert.Positive(t, count(t, tester, &ticket.Issue{}, ""), "issues, journals and relations from details still convert")
	assert.Positive(t, count(t, tester, &ticket.IssueChangelogs{}, ""))
	assert.Positive(t, count(t, tester, &ticket.IssueRelationship{}, ""))
	assert.Equal(t, int64(27), count(t, tester, &ticket.Issue{}, ""), "with the sweep off, the two issues that left project 6 stay")
}

func TestEnvFlagsForceTogglesOffWhateverTheScopeConfigSays(t *testing.T) {
	on := true
	enabled := models.Transformation{
		ReconcileSweep: &on, CollectTimeEntries: &on, CollectChangesets: &on, CollectComments: &on,
		IncludePrivateNotes: &on, VersionsAsSprints: &on,
	}
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")

	// Everything on: every gated subtask is planned, and rows appear.
	all := newTaskData(6, enabled)
	assert.Subset(t, plannedNames(all), []string{
		"collectIssueSweep", "extractIssueSweep", "collectTimeEntries", "extractTimeEntries", "convertWorklogs",
		"extractIssueChangesets", "convertIssueCommits", "convertIssueComments", "convertSprints",
	})
	runPlanned(tester, all)
	assert.Positive(t, count(t, tester, &ticket.IssueWorklog{}, ""))
	assert.Positive(t, count(t, tester, &ticket.IssueComment{}, "issue_id = ?", "redmine_go:RedmineGoIssue:1:175"))
	assert.Positive(t, count(t, tester, &ticket.Sprint{}, ""))
	assert.Equal(t, int64(1), count(t, tester, &ticket.IssueComment{}, "body = ?", "RGO private note"))

	// The same scope config under REDMINE_GO_* flags set to false: nothing is
	// planned, and the private note is left out.
	flags := map[string]string{
		"REDMINE_GO_RECONCILE_SWEEP": "false", "REDMINE_GO_COLLECT_TIME_ENTRIES": "false", "REDMINE_GO_COLLECT_CHANGESETS": "false",
		"REDMINE_GO_COLLECT_COMMENTS": "false", "REDMINE_GO_INCLUDE_PRIVATE_NOTES": "false", "REDMINE_GO_VERSIONS_AS_SPRINTS": "false",
	}
	forced := newTaskData(6, enabled)
	forced.ScopeConfig.Transformation.ApplyEnvFlags(func(name string) string { return flags[name] })
	for _, gated := range []string{"extractIssueSweep", "extractTimeEntries", "convertWorklogs", "convertIssueCommits", "convertIssueComments", "convertSprints"} {
		assert.NotContains(t, plannedNames(forced), gated)
	}
	for _, table := range []dal.Tabler{&ticket.IssueWorklog{}, &ticket.IssueComment{}, &ticket.Sprint{}, &crossdomain.IssueCommit{}, &models.RedmineGoTimeEntry{}} {
		tester.FlushTabler(table)
	}
	runPlanned(tester, forced)
	assert.Zero(t, count(t, tester, &ticket.IssueWorklog{}, ""))
	assert.Zero(t, count(t, tester, &ticket.IssueComment{}, ""))
	assert.Zero(t, count(t, tester, &ticket.Sprint{}, ""))
	assert.Zero(t, count(t, tester, &crossdomain.IssueCommit{}, ""))
	assert.Zero(t, count(t, tester, &models.RedmineGoTimeEntry{}, ""))

	// The private note is a conversion option, so check it where it applies.
	privateOnly := newTaskData(6, enabled)
	privateOnly.ScopeConfig.Transformation.ApplyEnvFlags(func(name string) string { return map[string]string{"REDMINE_GO_INCLUDE_PRIVATE_NOTES": "false"}[name] })
	tester.Subtask(tasks.ConvertIssueCommentsMeta, privateOnly)
	assert.Zero(t, count(t, tester, &ticket.IssueComment{}, "body = ?", "RGO private note"))
	assert.Positive(t, count(t, tester, &ticket.IssueComment{}, ""))
}

// Lane 5 has relations and a long note history but no changesets, private
// notes or time entries in its capture: the counts are those of the capture.
func TestLane5RelationsAndComments(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "lane5/")
	for _, projectId := range []uint64{1, 2} {
		runPlanned(tester, newTaskData(projectId, sweepOff()))
	}

	assert.Equal(t, int64(6), count(t, tester, &ticket.IssueRelationship{}, "original_type = ?", "relates"))
	assert.Equal(t, int64(6), count(t, tester, &ticket.IssueRelationship{}, ""), "one row per relation of the capture")
	assert.Equal(t, int64(423), count(t, tester, &ticket.IssueComment{}, ""), "every journal with a note")
	assert.Zero(t, count(t, tester, &ticket.IssueComment{}, "body = ''"))
	assert.Zero(t, count(t, tester, &crossdomain.IssueCommit{}, ""))
	assert.Zero(t, count(t, tester, &ticket.IssueWorklog{}, ""))
}
