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

package api

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObservedValuesAreEmptyArraysBeforeTheFirstCollection(t *testing.T) {
	encoded, err := json.Marshal((&observedSources{}).build())
	require.NoError(t, err)
	assert.JSONEq(t, `{"trackers":[],"statuses":[],"customFields":[],"categories":[]}`, string(encoded),
		"the UI reads every array: none may be null")
}

func TestObservedValuesMatchTheContractShape(t *testing.T) {
	sources := &observedSources{
		trackers:   []models.RedmineGoTracker{{Id: 2, Name: "Feature"}, {Id: 1, Name: "Bug"}},
		statuses:   []models.RedmineGoStatus{{Id: 5, Name: "Closed", IsClosed: true}, {Id: 1, Name: "New"}},
		categories: []models.RedmineGoIssueCategory{{Id: 9, ProjectId: 6, Name: "Backend"}, {Id: 9, ProjectId: 6, Name: "Backend"}, {Id: 3, ProjectId: 8, Name: "Backend"}},
		fields: []models.RedmineGoCustomField{{
			Id: 12, Name: "Severity", FieldFormat: "enumeration",
			PossibleValues: `[{"value":"7","label":"Minor"},{"value":"9","label":"Critical"}]`,
		}},
		observed: []models.RedmineGoObservedField{{Id: 12, Name: "Severity"}, {Id: 13, Name: "Environment", Multiple: true}},
	}
	sources.addIssueFields(`[{"id":12,"name":"Severity","value":"9"},{"id":13,"name":"Environment","multiple":true,"value":["env:prod","env:prod"]}]`)
	sources.addIssueFields(`[{"id":12,"name":"Severity","value":null},{"id":13,"name":"Environment","multiple":true,"value":["env:prod","env:test"]}]`)
	sources.addIssueFields(`[{"id":12,"name":"Severity","value":"99"}]`)

	encoded, err := json.Marshal(sources.build())
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"trackers": [{"id":1,"name":"Bug"},{"id":2,"name":"Feature"}],
		"statuses": [{"id":1,"name":"New","isClosed":false},{"id":5,"name":"Closed","isClosed":true}],
		"customFields": [
			{"id":12,"name":"Severity","fieldFormat":"enumeration","multiple":false,"values":[
				{"value":"7","label":"Minor"},{"value":"9","label":"Critical"},{"value":"99","label":"99"}]},
			{"id":13,"name":"Environment","fieldFormat":"","multiple":true,"values":[
				{"value":"env:prod","label":"env:prod"},{"value":"env:test","label":"env:test"}]}
		],
		"categories": [{"id":3,"name":"Backend"},{"id":9,"name":"Backend"}]
	}`, string(encoded))
}

func TestObservedValuesKnowAFieldFromDataAloneForANonAdminKey(t *testing.T) {
	sources := &observedSources{observed: []models.RedmineGoObservedField{{Id: 21, Name: "Discovery Environment"}}}
	sources.addIssueFields(`[{"id":21,"name":"Discovery Environment","value":"PROD"}]`)
	sources.addIssueFields(`[{"id":21,"name":"Discovery Environment","value":"PROD"}]`)
	sources.addIssueFields(`[{"id":21,"name":"Discovery Environment","value":"env:prod-client"}]`)

	fields := sources.build().CustomFields
	require.Len(t, fields, 1)
	assert.Equal(t, "Discovery Environment", fields[0].Name)
	assert.Equal(t, []ObservedValue{{Value: "PROD", Label: "PROD"}, {Value: "env:prod-client", Label: "env:prod-client"}}, fields[0].Values,
		"most frequent first; a raw value is its own label")
}

func TestObservedValuesSkipFreeTextFieldsAndCapTheRest(t *testing.T) {
	sources := &observedSources{
		fields: []models.RedmineGoCustomField{
			{Id: 1, Name: "Summary", FieldFormat: "string"},
			{Id: 2, Name: "Reviewer", FieldFormat: "user"},
		},
	}
	for i := range maxObservedValuesPerField + 50 {
		sources.addIssueFields(fmt.Sprintf(`[{"id":1,"value":"free text %d"},{"id":2,"value":"%d"}]`, i, i))
	}
	fields := sources.build().CustomFields
	require.Len(t, fields, 2)
	assert.Empty(t, fields[0].Values, "typed text is not a choice")
	assert.Len(t, fields[1].Values, maxObservedValuesPerField, "values seen are capped")
}

func TestObservedValuesNeverCutADefinitionsPossibleValues(t *testing.T) {
	possible := "["
	for i := range maxObservedValuesPerField + 10 {
		if i > 0 {
			possible += ","
		}
		possible += fmt.Sprintf(`{"value":"v%d","label":"Value %d"}`, i, i)
	}
	possible += "]"
	sources := &observedSources{fields: []models.RedmineGoCustomField{{Id: 3, Name: "Big list", FieldFormat: "list", PossibleValues: possible}}}
	assert.Len(t, sources.build().CustomFields[0].Values, maxObservedValuesPerField+10)
}

func TestObservedValuesIgnoreUnreadableDocuments(t *testing.T) {
	sources := &observedSources{observed: []models.RedmineGoObservedField{{Id: 1, Name: "A"}}}
	sources.addIssueFields("")
	sources.addIssueFields("not json")
	sources.addIssueFields(`[{"id":1,"value":{"nested":true}}]`)
	require.Len(t, sources.build().CustomFields, 1)
	assert.Empty(t, sources.build().CustomFields[0].Values)
}
