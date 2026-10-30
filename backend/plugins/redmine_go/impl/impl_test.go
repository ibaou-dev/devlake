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

package impl

import (
	"testing"

	"github.com/apache/devlake/core/errors"
	mockplugin "github.com/apache/devlake/mocks/core/plugin"
	"github.com/apache/devlake/plugins/redmine_go/tasks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginIdentity(t *testing.T) {
	assert.Equal(t, "redmine_go", RedmineGo{}.Name())
	assert.Equal(t, "github.com/apache/devlake/plugins/redmine_go", RedmineGo{}.RootPkgPath())
	for _, meta := range (RedmineGo{}).SubTaskMetas() {
		assert.Nil(t, meta.Dependencies, "%s must not declare Dependencies: list order is the contract", meta.Name)
	}
}

func TestTablesInfoNamesEveryTableWithThePluginPrefix(t *testing.T) {
	tables := (RedmineGo{}).GetTablesInfo()
	names := make([]string, 0, len(tables))
	for _, table := range tables {
		names = append(names, table.TableName())
	}
	assert.Equal(t, []string{
		"_tool_redmine_go_connections",
		"_tool_redmine_go_projects",
		"_tool_redmine_go_scope_configs",
		"_tool_redmine_go_trackers",
		"_tool_redmine_go_statuses",
		"_tool_redmine_go_priorities",
		"_tool_redmine_go_custom_fields",
		"_tool_redmine_go_versions",
		"_tool_redmine_go_issue_categories",
		"_tool_redmine_go_accounts",
		"_tool_redmine_go_observed_fields",
		"_tool_redmine_go_issues",
		"_tool_redmine_go_journals",
		"_tool_redmine_go_journal_details",
	}, names)
}

// The subtask list is the execution order: metadata before issues, issues
// before their details, and every conversion after every extraction.
func TestSubTaskMetasRunCollectExtractConvertInOrder(t *testing.T) {
	var names []string
	for _, meta := range (RedmineGo{}).SubTaskMetas() {
		assert.NotEmpty(t, meta.DomainTypes, "%s must declare its domain types", meta.Name)
		assert.NotEmpty(t, meta.Description, meta.Name)
		assert.True(t, meta.EnabledByDefault, meta.Name)
		names = append(names, meta.Name)
	}
	assert.Equal(t, []string{
		"collectTrackers", "extractTrackers",
		"collectStatuses", "extractStatuses",
		"collectPriorities", "extractPriorities",
		"collectCustomFields", "extractCustomFields",
		"collectVersions", "extractVersions",
		"collectIssueCategories", "extractIssueCategories",
		"collectIssues", "extractIssues",
		"collectIssueDetails", "extractIssueJournals",
		"convertProjects", "convertAccounts", "convertIssues", "convertIssueChangelogs",
	}, names)
	for name := range tasks.SubtaskToggles {
		assert.Contains(t, names, name, "a toggle must name a registered subtask")
	}
}

func TestApiResourcesExposeTheContractRoutes(t *testing.T) {
	routes := (RedmineGo{}).ApiResources()
	for path, methods := range map[string][]string{
		"test":                           {"POST"},
		"connections":                    {"POST", "GET"},
		"connections/:connectionId":      {"PATCH", "DELETE", "GET"},
		"connections/:connectionId/test": {"POST"},
		"connections/:connectionId/remote-scopes":                {"GET"},
		"connections/:connectionId/search-remote-scopes":         {"GET"},
		"connections/:connectionId/proxy/rest/*path":             {"GET"},
		"connections/:connectionId/scope-configs":                {"POST", "GET"},
		"connections/:connectionId/scope-configs/:scopeConfigId": {"PATCH", "GET", "DELETE"},
		"connections/:connectionId/scope-config-defaults":        {"GET"},
		"connections/:connectionId/scopes":                       {"GET", "PUT"},
		"connections/:connectionId/scopes/:scopeId":              {"GET", "PATCH", "DELETE"},
		"scope-config/:scopeConfigId/projects":                   {"GET"},
	} {
		require.Contains(t, routes, path)
		for _, method := range methods {
			assert.Contains(t, routes[path], method, "%s %s", method, path)
		}
	}
}

func TestPrepareTaskDataValidatesOptionsBeforeTouchingTheDatabase(t *testing.T) {
	taskCtx := new(mockplugin.TaskContext)
	for name, options := range map[string]map[string]interface{}{
		"no connection": {"projectId": 6},
		"no project":    {"connectionId": 1},
	} {
		_, err := RedmineGo{}.PrepareTaskData(taskCtx, options)
		require.NotNil(t, err, name)
		assert.Equal(t, errors.BadInput, err.GetType(), name)
	}
}

func TestCloseToleratesAFailedPreparation(t *testing.T) {
	failed := new(mockplugin.TaskContext)
	failed.On("GetData").Return(nil)
	assert.Nil(t, RedmineGo{}.Close(failed), "no data, nothing to release")

	noClient := new(mockplugin.TaskContext)
	noClient.On("GetData").Return(&tasks.RedmineGoTaskData{})
	assert.Nil(t, RedmineGo{}.Close(noClient), "e2e task data has no client")

	wrong := new(mockplugin.TaskContext)
	wrong.On("GetData").Return("not task data")
	assert.NotNil(t, RedmineGo{}.Close(wrong))
}
