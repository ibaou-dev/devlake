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
	"strings"
	"testing"
	"time"

	"github.com/apache/devlake/core/models/domainlayer/didgen"
	"github.com/apache/devlake/core/plugin"
	mockplugin "github.com/apache/devlake/mocks/core/plugin"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelationRowKeepsOnlyRelationsTheIssueOwns(t *testing.T) {
	delay := 2
	owned := &apiRelation{Id: 18, IssueId: 164, IssueToId: 165, RelationType: "precedes", Delay: &delay}
	row := relationRow(1, 6, 164, owned)
	require.NotNil(t, row)
	assert.Equal(t, &models.RedmineGoIssueRelation{
		ConnectionId: 1, Id: 18, ProjectId: 6, IssueId: 164, IssueToId: 165, RelationType: "precedes", Delay: &delay,
	}, row)

	assert.Nil(t, relationRow(1, 6, 165, owned), "the target lists the same relation, and does not own it")
	assert.Nil(t, relationRow(1, 6, 164, &apiRelation{IssueId: 164, IssueToId: 165}), "a relation without an id cannot be de-duplicated")
	assert.Nil(t, relationRow(1, 6, 164, &apiRelation{Id: 1, IssueId: 164}), "a relation without a target")
}

func TestSprintStatus(t *testing.T) {
	for status, want := range map[string]string{
		"open": "ACTIVE", "locked": "CLOSED", "closed": "CLOSED", "": "ACTIVE", "unknown": "ACTIVE",
	} {
		assert.Equal(t, want, sprintStatus(status), "version status %q", status)
	}
}

func TestSprintRow(t *testing.T) {
	mockMeta := mockplugin.NewPluginMeta(t)
	mockMeta.On("RootPkgPath").Return("github.com/apache/devlake/plugins/redmine_go")
	mockMeta.On("Name").Return("redmine_go").Maybe()
	require.NoError(t, plugin.RegisterPlugin("redmine_go", mockMeta))
	idGen := didgen.NewDomainIdGenerator(&models.RedmineGoVersion{})
	created := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	due := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	version := &models.RedmineGoVersion{Id: 21, Name: "RGO 1.0", Status: "open", DueDate: &due, RedmineCreatedOn: &created}

	sprint := sprintRow(1, "https://redmine.example.org", "board:6", version, idGen)
	assert.Equal(t, "redmine_go:RedmineGoVersion:1:21", sprint.Id)
	assert.Equal(t, "https://redmine.example.org/versions/21", sprint.Url)
	assert.Equal(t, "ACTIVE", sprint.Status)
	assert.Equal(t, &created, sprint.StartedDate)
	assert.Equal(t, &due, sprint.EndedDate)
	assert.Nil(t, sprint.CompletedDate, "an open version is not completed")
	assert.Equal(t, "board:6", sprint.OriginalBoardID)

	version.Status = "closed"
	sprint = sprintRow(1, "https://redmine.example.org", "board:6", version, idGen)
	assert.Equal(t, "CLOSED", sprint.Status)
	assert.Equal(t, &due, sprint.CompletedDate, "a closed version completes at its due date")

	version.DueDate = nil
	assert.Nil(t, sprintRow(1, "", "", version, idGen).CompletedDate, "no due date, no completion date")
}

func TestSweepQueryAsksForRelationsWithoutATimeBound(t *testing.T) {
	query := sweepQuery(6, 200, 100)
	assert.Equal(t, "relations", query.Get("include"))
	assert.Equal(t, "6", query.Get("project_id"))
	assert.Equal(t, "!*", query.Get("subproject_id"), "a subproject's issues never count for its parent")
	assert.Equal(t, "*", query.Get("status_id"), "closed issues are part of the sweep")
	assert.Equal(t, "id", query.Get("sort"))
	assert.Equal(t, "200", query.Get("offset"))
	assert.False(t, query.Has("updated_on"), "the sweep exists because updated_on does not move")
}

func TestParseSweepPageKeepsIdsAndRelationsOnly(t *testing.T) {
	body := `{"issues":[
		{"id":164,"subject":"long","description":"a long description","relations":[{"id":18,"issue_id":164,"issue_to_id":165,"relation_type":"precedes","delay":2}]},
		{"id":165,"subject":"other"}],"total_count":2}`
	rows, err := parseSweepPage(&http.Response{Body: io.NopCloser(strings.NewReader(body))})
	require.Nil(t, err)
	require.Len(t, rows, 2)
	assert.JSONEq(t, `{"id":164,"relations":[{"id":18,"issue_id":164,"issue_to_id":165,"relation_type":"precedes","delay":2}]}`, string(rows[0]))
	assert.JSONEq(t, `{"id":165,"relations":null}`, string(rows[1]))
	var decoded sweepIssue
	require.NoError(t, json.Unmarshal(rows[1], &decoded))
	assert.Empty(t, decoded.Relations)

	_, err = parseSweepPage(&http.Response{Body: io.NopCloser(strings.NewReader("<html>login</html>"))})
	assert.NotNil(t, err, "a page that is not the issue list must fail the sweep, never read as an empty project")
}

func TestListedEveryIssueNeedsAFullUnboundedNonEmptyList(t *testing.T) {
	bound := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name        string
		incremental bool
		since       *time.Time
		rows        int64
		want        bool
	}{
		{"first run, no time bound", false, nil, 50000, true},
		{"full sync, no time bound", false, nil, 1, true},
		{"an incremental run lists only what changed", true, &bound, 100, false},
		{"an incremental run never proves a deletion", true, nil, 100, false},
		{"a full run bounded by timeAfter stops at the bound", false, &bound, 50000, false},
		{"an empty list leaves stored issues unreplaced", false, nil, 0, false},
	} {
		assert.Equal(t, tc.want, listedEveryIssue(tc.incremental, tc.since, tc.rows), tc.name)
	}
}

func TestTimeEntryQueryUsesTheNumericProjectAndExcludesSubprojects(t *testing.T) {
	query := timeEntryQuery(6, 100, 100)
	assert.Equal(t, "6", query.Get("project_id"))
	assert.Equal(t, "!*", query.Get("subproject_id"))
	assert.Equal(t, "id", query.Get("sort"))
	assert.Equal(t, "100", query.Get("offset"))
	assert.Equal(t, "100", query.Get("limit"))
	assert.False(t, query.Has("updated_on"), "Redmine ignores updated_on on time entries")
}

func TestSubtaskTogglesNameRealSubtasksAndRealToggles(t *testing.T) {
	assert.Equal(t, models.ToggleReconcileSweep, SubtaskToggles["collectIssueSweep"])
	assert.Equal(t, models.ToggleReconcileSweep, SubtaskToggles["extractIssueSweep"])
	assert.Equal(t, models.ToggleCollectTimeEntries, SubtaskToggles["collectTimeEntries"])
	assert.Equal(t, models.ToggleCollectTimeEntries, SubtaskToggles["extractTimeEntries"])
	assert.Equal(t, models.ToggleCollectTimeEntries, SubtaskToggles["convertWorklogs"])
	assert.Equal(t, models.ToggleCollectChangesets, SubtaskToggles["extractIssueChangesets"])
	assert.Equal(t, models.ToggleCollectChangesets, SubtaskToggles["convertIssueCommits"])
	assert.Equal(t, models.ToggleCollectComments, SubtaskToggles["convertIssueComments"])
	assert.Equal(t, models.ToggleVersionsAsSprints, SubtaskToggles["convertSprints"])
	for name := range SubtaskToggles {
		assert.NotEqual(t, models.ToggleIncludePrivateNotes, SubtaskToggles[name], "%s: includePrivateNotes is read by the comment convertor, it gates no subtask", name)
	}
}
