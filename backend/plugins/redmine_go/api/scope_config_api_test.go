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
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	"github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultScopeConfigEntities(t *testing.T) {
	want := []string{plugin.DOMAIN_TYPE_TICKET, plugin.DOMAIN_TYPE_CROSS}
	for name, body := range map[string]map[string]interface{}{
		"missing":      {},
		"empty":        {"entities": []interface{}{}},
		"empty string": {"entities": []string{}},
	} {
		defaultScopeConfigEntities(body)
		assert.Equal(t, want, body["entities"], name)
	}
	explicit := map[string]interface{}{"entities": []interface{}{"TICKET"}}
	defaultScopeConfigEntities(explicit)
	assert.Equal(t, []interface{}{"TICKET"}, explicit["entities"], "an explicit selection is never overridden")
}

func decodeBody(t *testing.T, raw string) map[string]interface{} {
	t.Helper()
	body := map[string]interface{}{}
	require.NoError(t, json.Unmarshal([]byte(raw), &body))
	return body
}

// An invalid scope config is rejected with 400 and a message naming the
// violation, before anything is stored.
func TestPostScopeConfigRejectsInvalidTransformations(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{"unknown issue type",
			`{"name":"x","typeRules":[{"tracker":{"id":1,"name":"Bug"},"type":"STORY"}]}`,
			`typeRules[0].type "STORY" must be one of REQUIREMENT, BUG, INCIDENT, TASK, SUBTASK`},
		{"rule that matches nothing",
			`{"name":"x","typeRules":[{"type":"BUG"}]}`,
			"typeRules[0] needs a tracker or a when condition"},
		{"unknown standard status",
			`{"name":"x","statusMappings":[{"status":{"id":2,"name":"In Progress"},"std":"DOING"}]}`,
			`statusMappings[0].std "DOING"`},
		{"environment class outside the four",
			`{"name":"x","environment":[{"field":{"id":13,"name":"Env"},"map":[{"value":"p","to":"LIVE"}]}]}`,
			`environment[0].map[0].to "LIVE"`},
		{"customField component without a field",
			`{"name":"x","component":{"source":"customField"}}`,
			`component.field is required when component.source is "customField"`},
		{"misspelt nested field",
			`{"name":"x","typeRules":[{"trackr":{"id":1,"name":"Bug"},"type":"BUG"}]}`,
			`unknown field "trackr"`},
		{"toggle of the wrong type",
			`{"name":"x","collectComments":"sometimes"}`,
			"collectComments"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := PostScopeConfig(&plugin.ApiResourceInput{Body: decodeBody(t, tc.body)})
			require.NotNil(t, err)
			assert.Equal(t, errors.BadInput, err.GetType())
			assert.Equal(t, http.StatusBadRequest, err.GetType().GetHttpCode())
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

const defaultTrackersBody = `{"trackers":[
  {"id":1,"name":"Bug","default_status":{"id":1,"name":"New"},"description":null,"enabled_standard_fields":["assigned_to_id"]},
  {"id":2,"name":"Feature","default_status":{"id":1,"name":"New"},"description":null,"enabled_standard_fields":[]},
  {"id":3,"name":"Support","default_status":{"id":1,"name":"New"},"description":null,"enabled_standard_fields":[]}]}`

const defaultStatusesBody = `{"issue_statuses":[
  {"id":1,"name":"New","is_closed":false,"description":null},
  {"id":2,"name":"In Progress","is_closed":false,"description":null},
  {"id":3,"name":"Resolved","is_closed":false,"description":null},
  {"id":4,"name":"Feedback","is_closed":false,"description":null},
  {"id":5,"name":"Closed","is_closed":true,"description":null},
  {"id":6,"name":"Rejected","is_closed":true,"description":null}]}`

func TestLoadDefaultTransformation(t *testing.T) {
	setupTestGlobals()
	var key string
	status := http.StatusOK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key = r.Header.Get(models.ApiKeyHeader)
		if status != http.StatusOK {
			w.WriteHeader(status)
			return
		}
		switch r.URL.Path {
		case "/trackers.json":
			_, _ = w.Write([]byte(defaultTrackersBody))
		case "/issue_statuses.json":
			_, _ = w.Write([]byte(defaultStatusesBody))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	connection := &models.RedmineGoConnection{}
	connection.Endpoint = server.URL
	connection.Token = "non-admin-key"
	apiClient, err := api.NewApiClientFromConnection(context.TODO(), basicRes, connection)
	require.Nil(t, err)

	t.Run("default Redmine data resolves to Bug, Feature, Support and the status fallbacks", func(t *testing.T) {
		got, err := loadDefaultTransformation(apiClient)
		require.Nil(t, err)
		assert.Equal(t, "non-admin-key", key)
		assert.Equal(t, []models.TypeRule{
			{Tracker: &models.Ref{ID: 1, Name: "Bug"}, Type: "BUG"},
			{Tracker: &models.Ref{ID: 2, Name: "Feature"}, Type: "REQUIREMENT"},
			{Tracker: &models.Ref{ID: 3, Name: "Support"}, Type: "TASK"},
		}, got.TypeRules)
		std := map[string]string{}
		for _, m := range got.StatusMappings {
			std[m.Status.Name] = m.Std
		}
		assert.Equal(t, map[string]string{
			"New": "TODO", "In Progress": "IN_PROGRESS", "Resolved": "IN_PROGRESS",
			"Feedback": "IN_PROGRESS", "Closed": "DONE", "Rejected": "DONE",
		}, std)
		assert.Equal(t, &models.ComponentMapping{Source: "category"}, got.Component)
		encoded, jsonErr := json.Marshal(got)
		require.NoError(t, jsonErr)
		assert.Contains(t, string(encoded), `"collectComments":true`)
		assert.Contains(t, string(encoded), `"includePrivateNotes":false`)
		assert.Nil(t, got.Validate(), "what the endpoint returns is a valid scope config")
	})

	t.Run("a Redmine failure is reported, not swallowed", func(t *testing.T) {
		status = http.StatusForbidden
		_, err := loadDefaultTransformation(apiClient)
		require.NotNil(t, err)
		assert.Contains(t, err.Error(), "answered 403 for trackers.json")
	})
}
