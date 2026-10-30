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

	"github.com/apache/devlake/core/dal"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fieldByName(t *testing.T, values api.ObservedValues, name string) api.ObservedField {
	t.Helper()
	for _, field := range values.CustomFields {
		if field.Name == name {
			return field
		}
	}
	require.Failf(t, "field missing", "no observed custom field %q", name)
	return api.ObservedField{}
}

func TestObservedValuesAreEmptyBeforeTheFirstCollectionAndFilledAfter(t *testing.T) {
	tester := newDataFlowTester(t)
	for _, table := range []dal.Tabler{
		&models.RedmineGoTracker{}, &models.RedmineGoStatus{}, &models.RedmineGoPriority{}, &models.RedmineGoCustomField{},
		&models.RedmineGoIssueCategory{}, &models.RedmineGoObservedField{}, &models.RedmineGoIssue{},
	} {
		tester.FlushTabler(table)
	}
	empty, err := api.LoadObservedValues(tester.Dal, connectionId)
	require.NoError(t, err)
	encoded, jsonErr := json.Marshal(empty)
	require.NoError(t, jsonErr)
	assert.JSONEq(t, `{"trackers":[],"statuses":[],"customFields":[],"categories":[]}`, string(encoded))

	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")
	extractIssues(tester, 6, 7, 8)
	values, err := api.LoadObservedValues(tester.Dal, connectionId)
	require.NoError(t, err)

	assert.Contains(t, values.Trackers, api.ObservedRef{ID: 5, Name: "RGO Fixture"})
	assert.Contains(t, values.Statuses, api.ObservedStatus{ID: 10, Name: "Archived (rgo)", IsClosed: true})
	assert.Contains(t, values.Statuses, api.ObservedStatus{ID: 1, Name: "New", IsClosed: false})
	assert.Equal(t, []api.ObservedRef{{ID: 1, Name: "RGO Backend"}, {ID: 2, Name: "RGO Frontend"}}, values.Categories)

	severity := fieldByName(t, values, "RGO Severity")
	assert.Equal(t, uint64(12), severity.ID)
	assert.Equal(t, "enumeration", severity.FieldFormat)
	assert.Equal(t, []api.ObservedValue{{Value: "7", Label: "Minor"}, {Value: "8", Label: "Major"}, {Value: "9", Label: "Critical"}}, severity.Values,
		"an enumeration's values keep their labels, in the definition's order")
	environment := fieldByName(t, values, "RGO Environment")
	assert.True(t, environment.Multiple)
	assert.Contains(t, environment.Values, api.ObservedValue{Value: "12", Label: "Prod"})
	assert.Len(t, values.CustomFields, 10, "the instance's issue custom fields: four of the Lane 5 data and the six RGO fixtures")

	// A user-format field has no possible values in its definition: what its
	// issues carry is the only source, as a user id with no label.
	reviewer := fieldByName(t, values, "RGO Reviewer")
	for _, value := range reviewer.Values {
		assert.Equal(t, value.Value, value.Label)
	}
}

func TestObservedValuesNameFieldsFromIssuesWhenTheKeyCannotReadDefinitions(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")
	extractIssues(tester, 6, 7, 8)
	// A non-admin key never collects custom_fields.json.
	tester.FlushTabler(&models.RedmineGoCustomField{})

	values, err := api.LoadObservedValues(tester.Dal, connectionId)
	require.NoError(t, err)
	require.Len(t, values.CustomFields, 6, "observed fields still name every field")
	severity := fieldByName(t, values, "RGO Severity")
	assert.Equal(t, "", severity.FieldFormat)
	for _, value := range severity.Values {
		assert.Equal(t, value.Value, value.Label, "without the definition a value is its own label")
	}
	assert.NotEmpty(t, severity.Values, "the values issues carry are still offered")
}

func TestDeletingAConnectionRemovesItsMetadataAndNothingElse(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")
	extractIssues(tester, 6, 7, 8)

	// Rows of another connection must survive.
	require.NoError(t, tester.Db.Create(&models.RedmineGoTracker{ConnectionId: 2, Id: 1, Name: "Bug"}).Error)
	require.NoError(t, tester.Db.Create(&models.RedmineGoAccount{ConnectionId: 2, Id: 1, Name: "Someone"}).Error)
	require.NoError(t, tester.Db.Table("_raw_redmine_go_trackers").Create(&helper.RawData{
		ID: nextRawId(t, tester, "_raw_redmine_go_trackers"), Params: `{"ConnectionId":2}`,
		Data: []byte(`{"id":1,"name":"Bug"}`), Url: "trackers.json", Input: []byte("null"),
	}).Error)

	connectionTables := []interface{}{
		&models.RedmineGoTracker{}, &models.RedmineGoStatus{}, &models.RedmineGoPriority{},
		&models.RedmineGoCustomField{}, &models.RedmineGoAccount{}, &models.RedmineGoObservedField{},
	}
	for _, table := range connectionTables {
		assert.Positive(t, count(t, tester, table, "connection_id = ?", connectionId), "%T before", table)
	}
	rawRows := func(table, params string) int64 {
		var n int64
		require.NoError(t, tester.Db.Table(table).Where("params = ?", params).Count(&n).Error)
		return n
	}
	for _, table := range []string{"_raw_redmine_go_trackers", "_raw_redmine_go_statuses", "_raw_redmine_go_priorities", "_raw_redmine_go_custom_fields"} {
		assert.Positive(t, rawRows(table, `{"ConnectionId":1}`), table)
	}
	issues := count(t, tester, &models.RedmineGoIssue{}, "")
	versions := count(t, tester, &models.RedmineGoVersion{}, "")

	require.Nil(t, api.DeleteConnectionMetadata(tester.Dal, connectionId))

	for _, table := range connectionTables {
		assert.Zero(t, count(t, tester, table, "connection_id = ?", connectionId), "%T after", table)
	}
	for _, table := range []string{"_raw_redmine_go_trackers", "_raw_redmine_go_statuses", "_raw_redmine_go_priorities", "_raw_redmine_go_custom_fields"} {
		assert.Zero(t, rawRows(table, `{"ConnectionId":1}`), table)
	}
	assert.Equal(t, int64(1), count(t, tester, &models.RedmineGoTracker{}, "connection_id = ?", 2), "another connection keeps its metadata")
	assert.Equal(t, int64(1), count(t, tester, &models.RedmineGoAccount{}, "connection_id = ?", 2))
	assert.Equal(t, int64(1), rawRows("_raw_redmine_go_trackers", `{"ConnectionId":2}`))
	assert.Equal(t, issues, count(t, tester, &models.RedmineGoIssue{}, ""), "scope data is the scope deletion's job")
	assert.Equal(t, versions, count(t, tester, &models.RedmineGoVersion{}, ""))

	// Deleting twice, or a connection that never collected, is not an error.
	require.Nil(t, api.DeleteConnectionMetadata(tester.Dal, connectionId))
	require.Nil(t, api.DeleteConnectionMetadata(tester.Dal, 99))
}
