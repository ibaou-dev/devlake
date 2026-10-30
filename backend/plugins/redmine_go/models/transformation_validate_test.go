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

package models

import (
	"encoding/json"
	"testing"

	"github.com/apache/devlake/core/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// devExample is the dev-Redmine reference configuration of the scope-config
// contract, trimmed to one entry of each kind where the rest repeats a shape.
const devExample = `{
  "typeRules": [
    { "tracker": { "id": 1, "name": "Bug" },
      "when": { "field": { "id": 21, "name": "Discovery Environment" }, "op": "in",
                "values": [ { "value": "PROD", "label": "PROD" } ] },
      "type": "INCIDENT" },
    { "tracker": { "id": 1, "name": "Bug" }, "type": "BUG" },
    { "when": { "field": { "id": 43, "name": "Ticket Severity" }, "op": "notIn",
                "values": [ { "value": "2" } ] }, "type": "TASK" },
    { "tracker": { "id": 8, "name": "Analysis Task" }, "type": "SUBTASK" }
  ],
  "statusMappings": [
    { "status": { "id": 7, "name": "Under Analysis" }, "std": "IN_PROGRESS" },
    { "status": { "id": 17, "name": "On Hold" }, "std": "OTHER" }
  ],
  "severity": [
    { "field": { "id": 18, "name": "Bug Severity" }, "map": [] },
    { "field": { "id": 43, "name": "Ticket Severity" },
      "map": [ { "value": "1", "label": "Critical", "to": "Critical" } ] }
  ],
  "environment": [
    { "field": { "id": 21, "name": "Discovery Environment" },
      "map": [ { "value": "PROD", "to": "PRODUCTION" }, { "value": "env:prod-client", "label": "observed", "to": "PRODUCTION" } ] }
  ],
  "component": { "source": "customField", "field": { "id": 31, "name": "Component" } },
  "collectComments": true,
  "includePrivateNotes": false
}`

func decodeTransformation(t *testing.T, raw string) *Transformation {
	t.Helper()
	body := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(raw), &body))
	sc := &RedmineGoScopeConfig{}
	require.NoError(t, sc.MergeFromRequest(sc, body))
	return &sc.Transformation
}

func TestValidateAcceptsReferenceConfigurations(t *testing.T) {
	assert.Nil(t, (&Transformation{}).Validate(), "an empty transformation is valid: everything falls back")
	tr := decodeTransformation(t, devExample)
	assert.Len(t, tr.TypeRules, 4)
	assert.Equal(t, "customField", tr.Component.Source)
	assert.True(t, tr.ToggleEnabled(ToggleCollectComments))
	assert.False(t, tr.ToggleEnabled(ToggleIncludePrivateNotes))
	assert.True(t, tr.ToggleEnabled(ToggleReconcileSweep), "unset toggle takes its default")
	assert.False(t, tr.ToggleEnabled(ToggleVersionsAsSprints), "unset toggle takes its default")
}

func TestValidateRejects(t *testing.T) {
	bug := &Ref{ID: 1, Name: "Bug"}
	field := Ref{ID: 21, Name: "Env"}
	cases := []struct {
		name string
		t    Transformation
		want string
	}{
		{"unknown issue type", Transformation{TypeRules: []TypeRule{{Tracker: bug, Type: "STORY"}}},
			`typeRules[0].type "STORY" must be one of REQUIREMENT, BUG, INCIDENT, TASK, SUBTASK`},
		{"rule without tracker and condition", Transformation{TypeRules: []TypeRule{{Type: "BUG"}}},
			"typeRules[0] needs a tracker or a when condition"},
		{"tracker ref without id", Transformation{TypeRules: []TypeRule{{Tracker: &Ref{Name: "Bug"}, Type: "BUG"}}},
			"typeRules[0].tracker.id must be a positive integer"},
		{"tracker ref without name", Transformation{TypeRules: []TypeRule{{Tracker: &Ref{ID: 1}, Type: "BUG"}}},
			"typeRules[0].tracker.name must not be empty"},
		{"bad condition operator", Transformation{TypeRules: []TypeRule{{When: &Condition{Field: field, Op: "eq", Values: []Value{{Value: "x"}}}, Type: "BUG"}}},
			`typeRules[0].when.op "eq" must be one of in, notIn`},
		{"condition without values", Transformation{TypeRules: []TypeRule{{When: &Condition{Field: field, Op: ConditionIn}, Type: "BUG"}}},
			"typeRules[0].when.values must list at least one value"},
		{"unknown standard status", Transformation{StatusMappings: []StatusMapping{{Status: Ref{ID: 2, Name: "In Progress"}, Std: "DOING"}}},
			`statusMappings[0].std "DOING" must be one of TODO, IN_PROGRESS, DONE, OTHER`},
		{"status ref without id", Transformation{StatusMappings: []StatusMapping{{Status: Ref{Name: "x"}, Std: "DONE"}}},
			"statusMappings[0].status.id must be a positive integer"},
		{"severity target empty", Transformation{Severity: []ValueMapField{{Field: field, Map: []ValueMapEntry{{Value: "1"}}}}},
			"severity[0].map[0].to must not be empty"},
		{"environment class unknown", Transformation{Environment: []ValueMapField{{Field: field, Map: []ValueMapEntry{{Value: "PROD", To: "LIVE"}}}}},
			`environment[0].map[0].to "LIVE" must be one of PRODUCTION, STAGING, TESTING, DEVELOPMENT`},
		{"component source unknown", Transformation{Component: &ComponentMapping{Source: "tracker"}},
			`component.source "tracker" must be one of none, category, customField`},
		{"component customField without field", Transformation{Component: &ComponentMapping{Source: ComponentSourceCustomField}},
			`component.field is required when component.source is "customField"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.t.Validate()
			require.NotNil(t, err)
			assert.Equal(t, errors.BadInput, err.GetType())
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestValidateReportsEveryViolation(t *testing.T) {
	tr := Transformation{
		TypeRules:      []TypeRule{{Type: "STORY"}},
		StatusMappings: []StatusMapping{{Status: Ref{ID: 1, Name: "New"}, Std: "NOPE"}},
	}
	err := tr.Validate()
	require.NotNil(t, err)
	for _, want := range []string{"typeRules[0] needs a tracker", "typeRules[0].type", "statusMappings[0].std"} {
		assert.Contains(t, err.Error(), want)
	}
}

func TestMergeFromRequestRejectsInvalidPatch(t *testing.T) {
	sc := &RedmineGoScopeConfig{}
	err := sc.MergeFromRequest(sc, map[string]interface{}{
		"typeRules": []interface{}{map[string]interface{}{"type": "BUG"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "typeRules[0] needs a tracker")
}

func TestMergeFromRequestKeepsUntouchedFieldsAndToggles(t *testing.T) {
	sc := decodeTransformationConfig(t, `{"collectComments": false, "component": {"source": "none"}}`)
	require.NoError(t, sc.MergeFromRequest(sc, map[string]interface{}{"name": "renamed"}))
	assert.Equal(t, "renamed", sc.Name)
	assert.False(t, sc.ToggleEnabled(ToggleCollectComments))
	assert.Equal(t, ComponentSourceNone, sc.Component.Source)
	// a null toggle returns to its default
	require.NoError(t, sc.MergeFromRequest(sc, map[string]interface{}{"collectComments": nil}))
	assert.True(t, sc.ToggleEnabled(ToggleCollectComments))
}

func decodeTransformationConfig(t *testing.T, raw string) *RedmineGoScopeConfig {
	t.Helper()
	body := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(raw), &body))
	sc := &RedmineGoScopeConfig{}
	require.NoError(t, sc.MergeFromRequest(sc, body))
	return sc
}
