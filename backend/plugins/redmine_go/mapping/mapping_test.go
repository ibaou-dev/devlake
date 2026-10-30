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

package mapping

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/apache/devlake/core/models/domainlayer/ticket"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The scope-config doc's stack example (Redmine on port 23010).
const stackConfigJSON = `{
  "typeRules": [
    { "tracker": { "id": 1, "name": "Bug" }, "type": "BUG" },
    { "tracker": { "id": 2, "name": "Feature" }, "type": "REQUIREMENT" },
    { "tracker": { "id": 3, "name": "Support" }, "type": "INCIDENT" }
  ],
  "statusMappings": [
    { "status": { "id": 2, "name": "In Progress" }, "std": "IN_PROGRESS" },
    { "status": { "id": 3, "name": "Resolved" }, "std": "IN_PROGRESS" },
    { "status": { "id": 4, "name": "Feedback" }, "std": "IN_PROGRESS" },
    { "status": { "id": 7, "name": "Awaiting client" }, "std": "OTHER" },
    { "status": { "id": 8, "name": "Rejected by QA" }, "std": "IN_PROGRESS" }
  ],
  "component": { "source": "category" }
}`

// The scope-config doc's dev example (Redmine on port 4002).
const devConfigJSON = `{
  "typeRules": [
    { "tracker": { "id": 1, "name": "Bug" },
      "when": { "field": { "id": 21, "name": "Discovery Environment" }, "op": "in",
                "values": [ { "value": "PROD", "label": "PROD" } ] },
      "type": "INCIDENT" },
    { "tracker": { "id": 1, "name": "Bug" }, "type": "BUG" },
    { "tracker": { "id": 12, "name": "Support Ticket" },
      "when": { "field": { "id": 43, "name": "Ticket Severity" }, "op": "in",
                "values": [ { "value": "1", "label": "Critical" } ] },
      "type": "INCIDENT" },
    { "tracker": { "id": 12, "name": "Support Ticket" }, "type": "TASK" },
    { "tracker": { "id": 4, "name": "Change Request" }, "type": "REQUIREMENT" },
    { "tracker": { "id": 5, "name": "Feature" }, "type": "REQUIREMENT" },
    { "tracker": { "id": 8, "name": "Analysis Task" }, "type": "SUBTASK" },
    { "tracker": { "id": 9, "name": "Implementation Task" }, "type": "SUBTASK" },
    { "tracker": { "id": 6, "name": "Task" }, "type": "TASK" },
    { "tracker": { "id": 7, "name": "Deployment Request" }, "type": "TASK" },
    { "tracker": { "id": 11, "name": "Log Request" }, "type": "TASK" }
  ],
  "statusMappings": [
    { "status": { "id": 7, "name": "Under Analysis" }, "std": "IN_PROGRESS" },
    { "status": { "id": 21, "name": "Under Technical Review" }, "std": "IN_PROGRESS" },
    { "status": { "id": 22, "name": "Technically Approved" }, "std": "TODO" },
    { "status": { "id": 9, "name": "Ready for Development" }, "std": "TODO" },
    { "status": { "id": 10, "name": "In Progress" }, "std": "IN_PROGRESS" },
    { "status": { "id": 11, "name": "In Review" }, "std": "IN_PROGRESS" },
    { "status": { "id": 3, "name": "Resolved" }, "std": "IN_PROGRESS" },
    { "status": { "id": 12, "name": "In Testing" }, "std": "IN_PROGRESS" },
    { "status": { "id": 14, "name": "Reopened" }, "std": "IN_PROGRESS" },
    { "status": { "id": 24, "name": "UAT Rejected" }, "std": "IN_PROGRESS" },
    { "status": { "id": 13, "name": "Verified" }, "std": "DONE" },
    { "status": { "id": 25, "name": "Accepted" }, "std": "DONE" },
    { "status": { "id": 18, "name": "Deployed" }, "std": "DONE" },
    { "status": { "id": 37, "name": "Fix Delivered" }, "std": "DONE" },
    { "status": { "id": 8, "name": "Awaiting Sign-off" }, "std": "OTHER" },
    { "status": { "id": 15, "name": "Awaiting Client Sign-off" }, "std": "OTHER" },
    { "status": { "id": 23, "name": "In UAT" }, "std": "OTHER" },
    { "status": { "id": 36, "name": "Awaiting Client Reply" }, "std": "OTHER" },
    { "status": { "id": 17, "name": "On Hold" }, "std": "OTHER" },
    { "status": { "id": 34, "name": "Triaged" }, "std": "IN_PROGRESS" },
    { "status": { "id": 35, "name": "Escalated" }, "std": "IN_PROGRESS" }
  ],
  "severity": [
    { "field": { "id": 18, "name": "Bug Severity" }, "map": [] },
    { "field": { "id": 43, "name": "Ticket Severity" },
      "map": [ { "value": "1", "label": "Critical", "to": "Critical" }, { "value": "2", "label": "Normal", "to": "Normal" } ] }
  ],
  "environment": [
    { "field": { "id": 21, "name": "Discovery Environment" },
      "map": [
        { "value": "PROD", "to": "PRODUCTION" }, { "value": "ACC", "to": "STAGING" },
        { "value": "UAT", "to": "TESTING" }, { "value": "DEV", "to": "DEVELOPMENT" },
        { "value": "env:prod-client", "label": "observed registry key", "to": "PRODUCTION" }
      ] }
  ],
  "component": { "source": "customField", "field": { "id": 31, "name": "Component" } },
  "collectComments": true,
  "includePrivateNotes": false
}`

// Helpers.

func decodeTransformation(t *testing.T, raw string) *models.Transformation {
	t.Helper()
	var tr models.Transformation
	require.NoError(t, json.Unmarshal([]byte(raw), &tr))
	require.Nil(t, tr.Validate())
	return &tr
}

func newIssue(trackerID int, trackerName string, fields ...FieldValues) *Issue {
	return &Issue{TrackerID: trackerID, TrackerName: trackerName, Fields: fields}
}

func field(id int, name string, values ...string) FieldValues {
	return FieldValues{ID: id, Name: name, Values: values}
}

func tracker(id int, name string, defaultStatusID int) models.TrackerInfo {
	return models.TrackerInfo{ID: id, Name: name, DefaultStatus: &models.Ref{ID: defaultStatusID, Name: "default"}}
}

func status(id int, name string, closed bool) models.StatusInfo {
	return models.StatusInfo{ID: id, Name: name, IsClosed: closed}
}

func ref(id int, name string) models.Ref { return models.Ref{ID: id, Name: name} }

func refPtr(id int, name string) *models.Ref { return new(ref(id, name)) }

func hour(h int) time.Time { return time.Date(2026, 1, 1, h, 0, 0, 0, time.UTC) }

func hourPtr(h int) *time.Time { return new(hour(h)) }

// defaultInstance is Redmine's default data, plus the German-named tracker.
func defaultInstance() Instance {
	return Instance{
		Trackers: []models.TrackerInfo{
			tracker(1, "Bug", 1), tracker(2, "Feature", 1), tracker(3, "Support", 1), tracker(4, "Fehler", 1),
		},
		Statuses: []models.StatusInfo{
			status(1, "New", false), status(2, "In Progress", false), status(3, "Resolved", false),
			status(4, "Feedback", false), status(5, "Closed", true), status(6, "Rejected", true),
		},
	}
}

// stackInstance is the stack Redmine: three trackers and eight statuses, no
// custom field metadata.
func stackInstance() Instance {
	return Instance{
		Trackers: []models.TrackerInfo{tracker(1, "Bug", 1), tracker(2, "Feature", 1), tracker(3, "Support", 1)},
		Statuses: []models.StatusInfo{
			status(1, "New", false), status(2, "In Progress", false), status(3, "Resolved", false),
			status(4, "Feedback", false), status(5, "Closed", true), status(6, "Rejected", true),
			status(7, "Awaiting client", false), status(8, "Rejected by QA", false),
		},
	}
}

// devInstance is a subset of the dev Redmine's real ids.
func devInstance() Instance {
	return Instance{
		Trackers: []models.TrackerInfo{
			tracker(1, "Bug", 1), tracker(12, "Support Ticket", 1), tracker(4, "Change Request", 1),
			tracker(5, "Feature", 1), tracker(8, "Analysis Task", 1), tracker(9, "Implementation Task", 1),
			tracker(6, "Task", 1), tracker(7, "Deployment Request", 1), tracker(11, "Log Request", 1),
		},
		Statuses: []models.StatusInfo{
			status(1, "New", false), status(3, "Resolved", false), status(5, "Closed", true),
			status(6, "Rejected", true), status(8, "Awaiting Sign-off", false), status(13, "Verified", false),
			status(18, "Deployed", false), status(26, "Custom", false),
		},
		Fields: []FieldInfo{
			{ID: 18, Name: "Bug Severity", Labels: map[string]string{"1": "Blocker", "3": "Major"}},
			{ID: 21, Name: "Discovery Environment"},
			{ID: 31, Name: "Component", Labels: map[string]string{"x": "Ex"}},
			{ID: 43, Name: "Ticket Severity", Labels: map[string]string{"1": "Critical", "2": "Normal"}},
		},
	}
}

// 1. The FR example and the upper-cased-name fallback.

func TestIssueTypeFRExample(t *testing.T) {
	// Only Bug is configured. "Epic" is neither covered by a rule nor an
	// English default name, so it falls back to its upper-cased name. (With an
	// empty config, "Feature" would be REQUIREMENT instead.)
	tr := &models.Transformation{TypeRules: []models.TypeRule{
		{
			Tracker: refPtr(1, "Bug"),
			When: &models.Condition{
				Field: ref(21, "Discovery Environment"), Op: models.ConditionIn,
				Values: []models.Value{{Value: "PROD"}},
			},
			Type: ticket.INCIDENT,
		},
		{Tracker: refPtr(1, "Bug"), Type: ticket.BUG},
	}}
	inst := Instance{
		Trackers: []models.TrackerInfo{tracker(1, "Bug", 1), tracker(20, "Epic", 1)},
		Fields:   []FieldInfo{{ID: 21, Name: "Discovery Environment"}},
	}
	m := NewMapper(tr, inst)
	cases := []struct {
		name  string
		issue *Issue
		want  string
	}{
		{"bug discovered in PROD", newIssue(1, "Bug", field(21, "Discovery Environment", "PROD")), ticket.INCIDENT},
		{"bug discovered in DEV", newIssue(1, "Bug", field(21, "Discovery Environment", "DEV")), ticket.BUG},
		{"bug without the field", newIssue(1, "Bug"), ticket.BUG},
		{"unmapped tracker", newIssue(20, "Epic"), "EPIC"},
		{"unmapped tracker is trimmed", newIssue(20, "  Epic "), "EPIC"},
		{"empty tracker name", newIssue(0, ""), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, m.IssueType(tc.issue))
		})
	}
}

// 2. The stack example.

func TestStackExample(t *testing.T) {
	m := NewMapper(decodeTransformation(t, stackConfigJSON), stackInstance())

	t.Run("type rules", func(t *testing.T) {
		assert.Equal(t, ticket.BUG, m.IssueType(newIssue(1, "Bug")))
		assert.Equal(t, ticket.REQUIREMENT, m.IssueType(newIssue(2, "Feature")))
		assert.Equal(t, ticket.INCIDENT, m.IssueType(newIssue(3, "Support")))
	})

	statuses := []struct {
		name string
		id   int
		want string
	}{
		{"New falls back to TODO (tracker default)", 1, ticket.TODO},
		{"In Progress explicit", 2, ticket.IN_PROGRESS},
		{"Resolved explicit", 3, ticket.IN_PROGRESS},
		{"Feedback explicit", 4, ticket.IN_PROGRESS},
		{"Closed falls back to DONE", 5, ticket.DONE},
		{"Rejected falls back to DONE", 6, ticket.DONE},
		{"Awaiting client explicit OTHER", 7, ticket.OTHER},
		{"Rejected by QA explicit", 8, ticket.IN_PROGRESS},
	}
	names := map[int]string{
		1: "New", 2: "In Progress", 3: "Resolved", 4: "Feedback",
		5: "Closed", 6: "Rejected", 7: "Awaiting client", 8: "Rejected by QA",
	}
	for _, tc := range statuses {
		t.Run(tc.name, func(t *testing.T) {
			// The closed flag passed in is deliberately wrong: the instance wins.
			assert.Equal(t, tc.want, m.Status(tc.id, names[tc.id], false))
		})
	}

	t.Run("component is the category", func(t *testing.T) {
		assert.Equal(t, "Backend", m.Component(&Issue{CategoryName: "Backend"}))
		assert.Equal(t, "", m.Component(&Issue{}))
	})
	t.Run("no severity or environment", func(t *testing.T) {
		assert.Equal(t, "", m.Severity(newIssue(1, "Bug", field(18, "Bug Severity", "3"))))
		assert.Empty(t, m.EnvironmentClasses(newIssue(1, "Bug", field(21, "Discovery Environment", "PROD"))))
	})
}

// 3. The dev example.

func TestDevExampleIssueType(t *testing.T) {
	m := NewMapper(decodeTransformation(t, devConfigJSON), devInstance())
	env := func(v ...string) FieldValues { return field(21, "Discovery Environment", v...) }
	sev := func(v ...string) FieldValues { return field(43, "Ticket Severity", v...) }
	cases := []struct {
		name  string
		issue *Issue
		want  string
	}{
		{"bug in PROD", newIssue(1, "Bug", env("PROD")), ticket.INCIDENT},
		{"bug in DEV", newIssue(1, "Bug", env("DEV")), ticket.BUG},
		{"bug without environment", newIssue(1, "Bug"), ticket.BUG},
		{"bug with several environments incl. PROD", newIssue(1, "Bug", env("DEV", "PROD")), ticket.INCIDENT},
		{"critical support ticket", newIssue(12, "Support Ticket", sev("1")), ticket.INCIDENT},
		{"normal support ticket", newIssue(12, "Support Ticket", sev("2")), ticket.TASK},
		{"support ticket without severity", newIssue(12, "Support Ticket"), ticket.TASK},
		{"change request", newIssue(4, "Change Request"), ticket.REQUIREMENT},
		{"feature", newIssue(5, "Feature"), ticket.REQUIREMENT},
		{"analysis task", newIssue(8, "Analysis Task"), ticket.SUBTASK},
		{"implementation task", newIssue(9, "Implementation Task"), ticket.SUBTASK},
		{"task", newIssue(6, "Task"), ticket.TASK},
		{"deployment request", newIssue(7, "Deployment Request"), ticket.TASK},
		{"log request", newIssue(11, "Log Request"), ticket.TASK},
		{"unconfigured tracker", newIssue(99, "Epic"), "EPIC"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, m.IssueType(tc.issue))
		})
	}
}

func TestDevExampleStatus(t *testing.T) {
	m := NewMapper(decodeTransformation(t, devConfigJSON), devInstance())
	cases := []struct {
		name     string
		id       int
		stName   string
		isClosed bool
		want     string
	}{
		{"Verified is DONE although open", 13, "Verified", false, ticket.DONE},
		{"Awaiting Sign-off is OTHER", 8, "Awaiting Sign-off", false, ticket.OTHER},
		{"Resolved explicit", 3, "Resolved", false, ticket.IN_PROGRESS},
		{"Deployed explicit", 18, "Deployed", false, ticket.DONE},
		{"New falls back to TODO", 1, "New", false, ticket.TODO},
		{"Closed falls back to DONE", 5, "Closed", true, ticket.DONE},
		{"Rejected falls back to DONE", 6, "Rejected", true, ticket.DONE},
		{"unmapped open status is IN_PROGRESS", 26, "Custom", false, ticket.IN_PROGRESS},
		{"status unknown to the instance uses the flag", 99, "Ghost", true, ticket.DONE},
		{"status unknown, not closed", 99, "Ghost", false, ticket.IN_PROGRESS},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, m.Status(tc.id, tc.stName, tc.isClosed))
		})
	}
}

func TestDevExampleSeverity(t *testing.T) {
	m := NewMapper(decodeTransformation(t, devConfigJSON), devInstance())
	bug := func(v ...string) FieldValues { return field(18, "Bug Severity", v...) }
	tkt := func(v ...string) FieldValues { return field(43, "Ticket Severity", v...) }
	cases := []struct {
		name  string
		issue *Issue
		want  string
	}{
		{"bug severity with a label", newIssue(1, "Bug", bug("3")), "Major"},
		{"bug severity without a label is raw", newIssue(1, "Bug", bug("7")), "7"},
		{"ticket severity mapped", newIssue(12, "Support Ticket", tkt("1")), "Critical"},
		{"ticket severity mapped 2", newIssue(12, "Support Ticket", tkt("2")), "Normal"},
		{"ticket severity unmapped and unlabelled is raw", newIssue(12, "Support Ticket", tkt("3")), "3"},
		{"first non-empty field wins", newIssue(1, "Bug", tkt("1"), bug("1")), "Blocker"},
		{"empty first field falls to the next", newIssue(1, "Bug", bug(), tkt("2")), "Normal"},
		{"blank value counts as empty", newIssue(1, "Bug", bug(""), tkt("2")), "Normal"},
		{"only the first value is used", newIssue(1, "Bug", bug("3", "1")), "Major"},
		{"no field has a value", newIssue(1, "Bug", bug(), tkt()), ""},
		{"no fields at all", newIssue(1, "Bug"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, m.Severity(tc.issue))
		})
	}
}

func TestDevExampleEnvironment(t *testing.T) {
	m := NewMapper(decodeTransformation(t, devConfigJSON), devInstance())
	env := func(v ...string) *Issue { return newIssue(1, "Bug", field(21, "Discovery Environment", v...)) }
	cases := []struct {
		name  string
		issue *Issue
		want  []string
	}{
		{"PROD", env("PROD"), []string{models.EnvironmentProduction}},
		{"ACC", env("ACC"), []string{models.EnvironmentStaging}},
		{"UAT", env("UAT"), []string{models.EnvironmentTesting}},
		{"DEV", env("DEV"), []string{models.EnvironmentDevelopment}},
		{"registry key", env("env:prod-client"), []string{models.EnvironmentProduction}},
		{"unmapped value yields no class", env("QA"), nil},
		{"multi-value keeps order", env("DEV", "ACC"), []string{models.EnvironmentDevelopment, models.EnvironmentStaging}},
		{"multi-value dedupes", env("PROD", "env:prod-client", "UAT"), []string{models.EnvironmentProduction, models.EnvironmentTesting}},
		{"unmapped values are skipped in a multi-value", env("QA", "DEV"), []string{models.EnvironmentDevelopment}},
		{"empty field", env(), nil},
		{"absent field", newIssue(1, "Bug"), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := m.EnvironmentClasses(tc.issue)
			if tc.want == nil {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestEnvironmentDoesNotFallThroughOnceAFieldHasValues(t *testing.T) {
	tr := &models.Transformation{Environment: []models.ValueMapField{
		{Field: ref(21, "First"), Map: []models.ValueMapEntry{{Value: "PROD", To: models.EnvironmentProduction}}},
		{Field: ref(22, "Second"), Map: []models.ValueMapEntry{{Value: "DEV", To: models.EnvironmentDevelopment}}},
	}}
	m := NewMapper(tr, Instance{})
	// First has an unmapped value, so it is chosen and yields nothing.
	assert.Empty(t, m.EnvironmentClasses(newIssue(1, "Bug", field(21, "First", "nope"), field(22, "Second", "DEV"))))
	// First is empty, so Second is consulted.
	assert.Equal(t, []string{models.EnvironmentDevelopment},
		m.EnvironmentClasses(newIssue(1, "Bug", field(21, "First"), field(22, "Second", "DEV"))))
}

func TestDevExampleComponent(t *testing.T) {
	m := NewMapper(decodeTransformation(t, devConfigJSON), devInstance())
	comp := func(v ...string) *Issue {
		return &Issue{CategoryName: "ignored", Fields: []FieldValues{field(31, "Component", v...)}}
	}
	assert.Equal(t, "a,b", m.Component(comp("a", "b")))
	assert.Equal(t, "a", m.Component(comp("a")))
	assert.Equal(t, "Ex,b", m.Component(comp("x", "b")), "labels apply")
	assert.Equal(t, "", m.Component(comp()))
	assert.Equal(t, "", m.Component(&Issue{CategoryName: "ignored"}), "missing field")
}

func TestComponentSources(t *testing.T) {
	issue := &Issue{CategoryName: "Backend", Fields: []FieldValues{field(31, "Component", "a")}}
	cases := []struct {
		name string
		comp *models.ComponentMapping
		want string
	}{
		{"nil behaves as category", nil, "Backend"},
		{"category", &models.ComponentMapping{Source: models.ComponentSourceCategory}, "Backend"},
		{"none", &models.ComponentMapping{Source: models.ComponentSourceNone}, ""},
		{"customField", &models.ComponentMapping{Source: models.ComponentSourceCustomField, Field: refPtr(31, "Component")}, "a"},
		{"customField without a ref", &models.ComponentMapping{Source: models.ComponentSourceCustomField}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewMapper(&models.Transformation{Component: tc.comp}, Instance{})
			assert.Equal(t, tc.want, m.Component(issue))
		})
	}
}

// 4. Out-of-the-box defaults.

func TestOutOfTheBoxDefaults(t *testing.T) {
	for name, tr := range map[string]*models.Transformation{
		"nil transformation":   nil,
		"empty transformation": {},
	} {
		t.Run(name, func(t *testing.T) {
			m := NewMapper(tr, defaultInstance())

			types := []struct {
				tracker string
				want    string
			}{
				{"Bug", ticket.BUG},
				{"Feature", ticket.REQUIREMENT},
				{"Support", ticket.TASK},
				{"support", ticket.TASK},
				{"Fehler", "FEHLER"},
			}
			for _, tc := range types {
				assert.Equal(t, tc.want, m.IssueType(newIssue(0, tc.tracker)), tc.tracker)
			}

			statuses := []struct {
				id   int
				name string
				want string
			}{
				{1, "New", ticket.TODO},
				{2, "In Progress", ticket.IN_PROGRESS},
				{3, "Resolved", ticket.IN_PROGRESS},
				{4, "Feedback", ticket.IN_PROGRESS},
				{5, "Closed", ticket.DONE},
				{6, "Rejected", ticket.DONE},
			}
			for _, tc := range statuses {
				assert.Equal(t, tc.want, m.Status(tc.id, tc.name, false), tc.name)
			}

			assert.Equal(t, "Docs", m.Component(&Issue{CategoryName: "Docs"}))
			assert.Equal(t, "", m.Severity(newIssue(1, "Bug", field(18, "Bug Severity", "3"))))
			assert.Empty(t, m.EnvironmentClasses(newIssue(1, "Bug", field(21, "Env", "PROD"))))
		})
	}
}

// 5. Reference matching.

func TestTrackerRefFallsBackToNameOnlyWhenIDUnknown(t *testing.T) {
	inst := Instance{Trackers: []models.TrackerInfo{tracker(1, "Bug", 1), tracker(2, "Feature", 1)}}

	t.Run("unknown id matches by name, ignoring case and spaces", func(t *testing.T) {
		tr := &models.Transformation{TypeRules: []models.TypeRule{{Tracker: refPtr(99, " bUg "), Type: ticket.INCIDENT}}}
		assert.Equal(t, ticket.INCIDENT, NewMapper(tr, inst).IssueType(newIssue(1, "Bug")))
	})
	t.Run("unknown id and different name does not match", func(t *testing.T) {
		tr := &models.Transformation{TypeRules: []models.TypeRule{{Tracker: refPtr(99, "Story"), Type: ticket.INCIDENT}}}
		assert.Equal(t, ticket.BUG, NewMapper(tr, inst).IssueType(newIssue(1, "Bug")))
	})
	t.Run("known id with the same name on another tracker does not match", func(t *testing.T) {
		// Id 2 exists (Feature), so a ref {2, "Bug"} designates Feature only.
		tr := &models.Transformation{TypeRules: []models.TypeRule{{Tracker: refPtr(2, "Bug"), Type: ticket.INCIDENT}}}
		m := NewMapper(tr, inst)
		assert.Equal(t, ticket.BUG, m.IssueType(newIssue(1, "Bug")))
		assert.Equal(t, ticket.INCIDENT, m.IssueType(newIssue(2, "Feature")))
	})
	t.Run("empty instance knows no ids, so names match", func(t *testing.T) {
		tr := &models.Transformation{TypeRules: []models.TypeRule{{Tracker: refPtr(2, "Bug"), Type: ticket.INCIDENT}}}
		assert.Equal(t, ticket.INCIDENT, NewMapper(tr, Instance{}).IssueType(newIssue(1, "Bug")))
	})
}

func TestStatusRefFallsBackToNameOnlyWhenIDUnknown(t *testing.T) {
	inst := Instance{
		Trackers: []models.TrackerInfo{tracker(1, "Bug", 1)},
		Statuses: []models.StatusInfo{status(1, "New", false), status(2, "Done", true)},
	}

	t.Run("unknown id matches by name", func(t *testing.T) {
		tr := &models.Transformation{StatusMappings: []models.StatusMapping{{Status: ref(99, "done"), Std: ticket.OTHER}}}
		assert.Equal(t, ticket.OTHER, NewMapper(tr, inst).Status(2, "Done", true))
	})
	t.Run("known id with the same name does not match another status", func(t *testing.T) {
		tr := &models.Transformation{StatusMappings: []models.StatusMapping{{Status: ref(1, "Done"), Std: ticket.OTHER}}}
		m := NewMapper(tr, inst)
		assert.Equal(t, ticket.DONE, m.Status(2, "Done", true), "falls back to is_closed")
		assert.Equal(t, ticket.OTHER, m.Status(1, "New", false), "the id still matches")
	})
	t.Run("first explicit mapping wins", func(t *testing.T) {
		tr := &models.Transformation{StatusMappings: []models.StatusMapping{
			{Status: ref(2, "Done"), Std: ticket.OTHER},
			{Status: ref(2, "Done"), Std: ticket.TODO},
		}}
		assert.Equal(t, ticket.OTHER, NewMapper(tr, inst).Status(2, "Done", true))
	})
}

func TestFieldRefFallsBackToNameOnlyWhenIDUnknown(t *testing.T) {
	inType := func(f models.Ref) *models.Transformation {
		return &models.Transformation{TypeRules: []models.TypeRule{{
			Tracker: refPtr(1, "Bug"),
			When:    &models.Condition{Field: f, Op: models.ConditionIn, Values: []models.Value{{Value: "x"}}},
			Type:    ticket.INCIDENT,
		}}}
	}
	inst := Instance{
		Trackers: []models.TrackerInfo{tracker(1, "Bug", 1)},
		Fields:   []FieldInfo{{ID: 21, Name: "Env"}, {ID: 22, Name: "Other"}},
	}

	t.Run("id match", func(t *testing.T) {
		m := NewMapper(inType(ref(21, "Env")), inst)
		assert.Equal(t, ticket.INCIDENT, m.IssueType(newIssue(1, "Bug", field(21, "Env", "x"))))
	})
	t.Run("unknown id matches by name", func(t *testing.T) {
		m := NewMapper(inType(ref(99, "env")), inst)
		assert.Equal(t, ticket.INCIDENT, m.IssueType(newIssue(1, "Bug", field(21, "Env", "x"))))
	})
	t.Run("known id with the name of another field does not match", func(t *testing.T) {
		m := NewMapper(inType(ref(21, "Other")), inst)
		assert.Equal(t, ticket.BUG, m.IssueType(newIssue(1, "Bug", field(22, "Other", "x"))))
	})
	t.Run("severity and environment and component use the same rule", func(t *testing.T) {
		tr := &models.Transformation{
			Severity:    []models.ValueMapField{{Field: ref(99, "Env"), Map: []models.ValueMapEntry{{Value: "x", To: "Sev"}}}},
			Environment: []models.ValueMapField{{Field: ref(99, "Env"), Map: []models.ValueMapEntry{{Value: "x", To: models.EnvironmentTesting}}}},
			Component:   &models.ComponentMapping{Source: models.ComponentSourceCustomField, Field: refPtr(99, "Env")},
		}
		m := NewMapper(tr, inst)
		issue := newIssue(1, "Bug", field(21, "Env", "x"))
		assert.Equal(t, "Sev", m.Severity(issue))
		assert.Equal(t, []string{models.EnvironmentTesting}, m.EnvironmentClasses(issue))
		assert.Equal(t, "x", m.Component(issue))
	})
}

func TestConditions(t *testing.T) {
	cond := func(op string, values ...string) *models.Transformation {
		vs := make([]models.Value, len(values))
		for i, v := range values {
			vs[i] = models.Value{Value: v}
		}
		return &models.Transformation{TypeRules: []models.TypeRule{{
			Tracker: refPtr(1, "Bug"),
			When:    &models.Condition{Field: ref(21, "Env"), Op: op, Values: vs},
			Type:    ticket.INCIDENT,
		}}}
	}
	inst := Instance{Trackers: []models.TrackerInfo{tracker(1, "Bug", 1)}, Fields: []FieldInfo{{ID: 21, Name: "Env"}}}
	env := func(v ...string) *Issue { return newIssue(1, "Bug", field(21, "Env", v...)) }

	cases := []struct {
		name  string
		tr    *models.Transformation
		issue *Issue
		want  string
	}{
		{"in holds when any value is listed", cond(models.ConditionIn, "PROD"), env("DEV", "PROD"), ticket.INCIDENT},
		{"in with several listed values", cond(models.ConditionIn, "PROD", "ACC"), env("ACC"), ticket.INCIDENT},
		{"in fails when none is listed", cond(models.ConditionIn, "PROD"), env("DEV", "UAT"), ticket.BUG},
		{"in fails on an empty field", cond(models.ConditionIn, "PROD"), env(), ticket.BUG},
		{"in fails on an absent field", cond(models.ConditionIn, "PROD"), newIssue(1, "Bug"), ticket.BUG},
		{"in matches the raw value exactly", cond(models.ConditionIn, "PROD"), env("prod"), ticket.BUG},
		{"notIn holds on an empty field", cond(models.ConditionNotIn, "PROD"), env(), ticket.INCIDENT},
		{"notIn holds on an absent field", cond(models.ConditionNotIn, "PROD"), newIssue(1, "Bug"), ticket.INCIDENT},
		{"notIn holds when none is listed", cond(models.ConditionNotIn, "PROD"), env("DEV"), ticket.INCIDENT},
		{"notIn fails when any value is listed", cond(models.ConditionNotIn, "PROD"), env("DEV", "PROD"), ticket.BUG},
		{"blank values count as empty", cond(models.ConditionIn, ""), env(""), ticket.BUG},
		{"unknown operator never holds", cond("equals", "PROD"), env("PROD"), ticket.BUG},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, NewMapper(tc.tr, inst).IssueType(tc.issue))
		})
	}
}

func TestTypeRuleOrderAndShape(t *testing.T) {
	inst := Instance{
		Trackers: []models.TrackerInfo{tracker(1, "Bug", 1), tracker(2, "Task", 1)},
		Fields:   []FieldInfo{{ID: 21, Name: "Env"}},
	}
	when := &models.Condition{Field: ref(21, "Env"), Op: models.ConditionIn, Values: []models.Value{{Value: "PROD"}}}

	t.Run("first matching rule wins", func(t *testing.T) {
		tr := &models.Transformation{TypeRules: []models.TypeRule{
			{Tracker: refPtr(1, "Bug"), Type: ticket.TASK},
			{Tracker: refPtr(1, "Bug"), Type: ticket.INCIDENT},
		}}
		assert.Equal(t, ticket.TASK, NewMapper(tr, inst).IssueType(newIssue(1, "Bug")))
	})
	t.Run("a more specific rule first shadows the general one", func(t *testing.T) {
		tr := &models.Transformation{TypeRules: []models.TypeRule{
			{Tracker: refPtr(1, "Bug"), Type: ticket.BUG},
			{Tracker: refPtr(1, "Bug"), When: when, Type: ticket.INCIDENT},
		}}
		assert.Equal(t, ticket.BUG, NewMapper(tr, inst).IssueType(newIssue(1, "Bug", field(21, "Env", "PROD"))))
	})
	t.Run("a rule with only when applies to every tracker", func(t *testing.T) {
		tr := &models.Transformation{TypeRules: []models.TypeRule{{When: when, Type: ticket.INCIDENT}}}
		m := NewMapper(tr, inst)
		assert.Equal(t, ticket.INCIDENT, m.IssueType(newIssue(1, "Bug", field(21, "Env", "PROD"))))
		assert.Equal(t, ticket.INCIDENT, m.IssueType(newIssue(2, "Task", field(21, "Env", "PROD"))))
		assert.Equal(t, ticket.BUG, m.IssueType(newIssue(1, "Bug")), "condition fails, fallback")
		assert.Equal(t, "TASK", m.IssueType(newIssue(2, "Task")), "condition fails, upper-cased name")
	})
	t.Run("a rule for another tracker is skipped", func(t *testing.T) {
		tr := &models.Transformation{TypeRules: []models.TypeRule{{Tracker: refPtr(2, "Task"), Type: ticket.SUBTASK}}}
		assert.Equal(t, ticket.BUG, NewMapper(tr, inst).IssueType(newIssue(1, "Bug")))
	})
}

// 6. Resolution date and lead time.

func TestResolutionDate(t *testing.T) {
	inst := Instance{Statuses: []models.StatusInfo{
		status(1, "New", false), status(2, "In Progress", false), status(5, "Closed", true), status(6, "Rejected", true),
	}}
	m := NewMapper(nil, inst)
	cases := []struct {
		name     string
		closedOn *time.Time
		closed   bool
		changes  []StatusChange
		want     *time.Time
	}{
		{
			name: "closed_on wins", closedOn: hourPtr(9), closed: true,
			changes: []StatusChange{{At: hour(3), ToStatusID: 5}}, want: hourPtr(9),
		},
		{
			name: "closed_on wins even when the status is not closed", closedOn: hourPtr(9), closed: false,
			want: hourPtr(9),
		},
		{
			name:   "null closed_on takes the last transition into a closed status, unsorted",
			closed: true,
			changes: []StatusChange{
				{At: hour(4), ToStatusID: 5},
				{At: hour(2), ToStatusID: 2},
				{At: hour(6), ToStatusID: 6},
				{At: hour(1), ToStatusID: 1},
			},
			want: hourPtr(6),
		},
		{
			name: "a later transition to a non-closed status is ignored", closed: true,
			changes: []StatusChange{
				{At: hour(3), ToStatusID: 2},
				{At: hour(1), ToStatusID: 5},
				{At: hour(2), ToStatusID: 6},
			},
			want: hourPtr(2),
		},
		{
			name: "the last closed transition need not be the last change", closed: true,
			changes: []StatusChange{
				{At: hour(5), ToStatusID: 1},
				{At: hour(3), ToStatusID: 5},
				{At: hour(4), ToStatusID: 2},
			},
			want: hourPtr(3),
		},
		{
			name: "status not closed yields nil", closed: false,
			changes: []StatusChange{{At: hour(1), ToStatusID: 5}},
			want:    nil,
		},
		{
			name: "closed but no closed transition yields nil", closed: true,
			changes: []StatusChange{{At: hour(1), ToStatusID: 1}, {At: hour(2), ToStatusID: 2}},
			want:    nil,
		},
		{
			name: "a transition into a status unknown to the instance does not count", closed: true,
			changes: []StatusChange{{At: hour(1), ToStatusID: 99}},
			want:    nil,
		},
		{name: "closed without any change yields nil", closed: true, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := m.ResolutionDate(tc.closedOn, tc.closed, tc.changes)
			if tc.want == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.True(t, tc.want.Equal(*got), "want %s, got %s", tc.want, got)
		})
	}
}

func TestResolutionDateDoesNotAliasChanges(t *testing.T) {
	m := NewMapper(nil, Instance{Statuses: []models.StatusInfo{status(5, "Closed", true)}})
	changes := []StatusChange{{At: hour(3), ToStatusID: 5}}
	got := m.ResolutionDate(nil, true, changes)
	require.NotNil(t, got)
	*got = hour(20)
	assert.Equal(t, hour(3), changes[0].At)
}

func TestLeadTimeMinutes(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { return new(created.Add(d)) }
	cases := []struct {
		name       string
		resolution *time.Time
		want       *uint
	}{
		{"nil resolution", nil, nil},
		{"resolution before created", at(-time.Minute), nil},
		{"same instant is zero", at(0), new(uint(0))},
		{"ninety minutes", at(90 * time.Minute), new(uint(90))},
		{"truncates seconds", at(2*time.Minute + 59*time.Second), new(uint(2))},
		{"under a minute is zero", at(59 * time.Second), new(uint(0))},
		{"two days", at(48 * time.Hour), new(uint(2880))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := LeadTimeMinutes(created, tc.resolution)
			if tc.want == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.Equal(t, *tc.want, *got)
		})
	}
}

// 7. ParseFieldValues.

func TestParseFieldValues(t *testing.T) {
	cases := []struct {
		name string
		raw  json.RawMessage
		want []string
	}{
		{"absent", nil, nil},
		{"null", json.RawMessage(`null`), nil},
		{"empty string", json.RawMessage(`""`), nil},
		{"blank string", json.RawMessage(`"  "`), nil},
		{"string", json.RawMessage(`"x"`), []string{"x"}},
		{"array", json.RawMessage(`["a","b"]`), []string{"a", "b"}},
		{"array of empties", json.RawMessage(`["",""]`), nil},
		{"empty array", json.RawMessage(`[]`), nil},
		{"array drops empties", json.RawMessage(`["a","","b"]`), []string{"a", "b"}},
		{"integer", json.RawMessage(`42`), []string{"42"}},
		{"float", json.RawMessage(`1.5`), []string{"1.5"}},
		{"bool true", json.RawMessage(`true`), []string{"true"}},
		{"bool false", json.RawMessage(`false`), []string{"false"}},
		{"mixed array", json.RawMessage(`[1,"a",null,false]`), []string{"1", "a", "false"}},
		{"object is ignored", json.RawMessage(`{"a":1}`), nil},
		{"invalid json", json.RawMessage(`{`), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseFieldValues(tc.raw)
			if tc.want == nil {
				assert.Empty(t, got)
				return
			}
			assert.Equal(t, tc.want, got)
		})
	}
}

// 8. FieldLabel and StatusIsClosed.

func TestFieldLabel(t *testing.T) {
	m := NewMapper(nil, Instance{Fields: []FieldInfo{
		{ID: 1, Name: "Severity", Labels: map[string]string{"1": "Critical", "2": ""}},
		{ID: 2, Name: "No labels"},
	}})
	assert.Equal(t, "Critical", m.FieldLabel(1, "1"))
	assert.Equal(t, "3", m.FieldLabel(1, "3"), "unlabelled value")
	assert.Equal(t, "2", m.FieldLabel(1, "2"), "empty label")
	assert.Equal(t, "1", m.FieldLabel(2, "1"), "field without labels")
	assert.Equal(t, "1", m.FieldLabel(99, "1"), "unknown field")
}

func TestStatusIsClosed(t *testing.T) {
	m := NewMapper(nil, Instance{Statuses: []models.StatusInfo{status(1, "New", false), status(5, "Closed", true)}})
	assert.False(t, m.StatusIsClosed(1, true), "instance truth beats the fallback")
	assert.True(t, m.StatusIsClosed(5, false), "instance truth beats the fallback")
	assert.True(t, m.StatusIsClosed(99, true), "unknown status uses the fallback")
	assert.False(t, m.StatusIsClosed(99, false), "unknown status uses the fallback")
}
