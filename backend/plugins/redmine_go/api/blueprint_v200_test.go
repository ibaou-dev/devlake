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
	"testing"

	"github.com/apache/devlake/core/models/common"
	"github.com/apache/devlake/core/plugin"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/helpers/srvhelper"
	mockplugin "github.com/apache/devlake/mocks/core/plugin"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mockRedmineGoPlugin(t *testing.T) {
	mockMeta := mockplugin.NewPluginMeta(t)
	mockMeta.On("RootPkgPath").Return("github.com/apache/devlake/plugins/redmine_go")
	mockMeta.On("Name").Return("redmine_go").Maybe()
	_ = plugin.RegisterPlugin("redmine_go", mockMeta)
}

type scopeDetail = srvhelper.ScopeDetail[models.RedmineGoProject, models.RedmineGoScopeConfig]

func newScopeDetail(id uint64, scopeConfigId uint64, config *models.RedmineGoScopeConfig) *scopeDetail {
	return &scopeDetail{
		Scope: models.RedmineGoProject{
			Scope:      common.Scope{ConnectionId: 1, ScopeConfigId: scopeConfigId},
			Id:         id,
			Identifier: "rgo",
			Name:       "RGO Fixtures",
		},
		ScopeConfig: config,
	}
}

func newScopeConfig(entities ...string) *models.RedmineGoScopeConfig {
	return &models.RedmineGoScopeConfig{ScopeConfig: common.ScopeConfig{Entities: entities}}
}

func testConnectionModel() *models.RedmineGoConnection {
	return &models.RedmineGoConnection{BaseConnection: helper.BaseConnection{Model: common.Model{ID: 1}}}
}

func TestMakeScopesV200(t *testing.T) {
	mockRedmineGoPlugin(t)

	scopes, err := makeScopesV200(
		[]*scopeDetail{newScopeDetail(6, 0, newScopeConfig(plugin.DOMAIN_TYPE_TICKET))},
		testConnectionModel(),
	)
	require.Nil(t, err)
	require.Len(t, scopes, 1)
	assert.Equal(t, "redmine_go:RedmineGoProject:1:6", scopes[0].ScopeId(), "the board id is built from the numeric project id")
	assert.Equal(t, "RGO Fixtures", scopes[0].ScopeName())
}

func TestMakeScopesV200WithoutTicketEntity(t *testing.T) {
	mockRedmineGoPlugin(t)

	scopes, err := makeScopesV200(
		[]*scopeDetail{newScopeDetail(6, 0, newScopeConfig(plugin.DOMAIN_TYPE_CROSS))},
		testConnectionModel(),
	)
	require.Nil(t, err)
	assert.Empty(t, scopes, "no ticket entity, no board")
}

var testSubtaskMetas = []plugin.SubTaskMeta{
	{Name: "collectIssues", EnabledByDefault: true, DomainTypes: []string{plugin.DOMAIN_TYPE_TICKET}},
	{Name: "collectTimeEntries", EnabledByDefault: true, DomainTypes: []string{plugin.DOMAIN_TYPE_TICKET}},
	{Name: "convertAccounts", EnabledByDefault: true, DomainTypes: []string{plugin.DOMAIN_TYPE_CROSS}},
	{Name: "convertVersionsAsSprints", EnabledByDefault: true, DomainTypes: []string{plugin.DOMAIN_TYPE_TICKET}},
	{Name: "disabledByDefault", EnabledByDefault: false, DomainTypes: []string{plugin.DOMAIN_TYPE_TICKET}},
}

var testSubtaskToggles = map[string]models.Toggle{
	"collectTimeEntries":       models.ToggleCollectTimeEntries,
	"convertVersionsAsSprints": models.ToggleVersionsAsSprints,
}

func plannedSubtasks(t *testing.T, config *models.RedmineGoScopeConfig) []string {
	t.Helper()
	plan, err := makePipelinePlanV200(
		testSubtaskMetas, testSubtaskToggles,
		[]*scopeDetail{newScopeDetail(6, 42, config)},
		testConnectionModel(),
	)
	require.Nil(t, err)
	require.Len(t, plan, 1)
	require.Len(t, plan[0], 1)
	return plan[0][0].Subtasks
}

func TestMakePipelinePlanV200ThreadsScopeParamsIntoTaskOptions(t *testing.T) {
	plan, err := makePipelinePlanV200(
		testSubtaskMetas, testSubtaskToggles,
		[]*scopeDetail{newScopeDetail(6, 42, newScopeConfig(plugin.DOMAIN_TYPE_TICKET))},
		testConnectionModel(),
	)
	require.Nil(t, err)
	require.Len(t, plan, 1)
	task := plan[0][0]
	assert.Equal(t, "redmine_go", task.Plugin)
	assert.EqualValues(t, 6, task.Options["projectId"])
	assert.EqualValues(t, 42, task.Options["scopeConfigId"])
	assert.EqualValues(t, 1, task.Options["connectionId"])
}

func TestMakePipelinePlanV200OneStagePerProject(t *testing.T) {
	plan, err := makePipelinePlanV200(
		testSubtaskMetas, testSubtaskToggles,
		[]*scopeDetail{
			newScopeDetail(6, 0, newScopeConfig(plugin.DOMAIN_TYPE_TICKET)),
			newScopeDetail(7, 0, newScopeConfig(plugin.DOMAIN_TYPE_TICKET)),
		},
		testConnectionModel(),
	)
	require.Nil(t, err)
	require.Len(t, plan, 2, "projects run one after the other")
	assert.EqualValues(t, 6, plan[0][0].Options["projectId"])
	assert.EqualValues(t, 7, plan[1][0].Options["projectId"])
}

func TestMakePipelinePlanV200FiltersSubtasksByEntities(t *testing.T) {
	assert.Equal(t,
		[]string{"collectIssues", "collectTimeEntries"},
		plannedSubtasks(t, newScopeConfig(plugin.DOMAIN_TYPE_TICKET)),
		"TICKET alone leaves out the CROSS subtasks and the one disabled by default")
	assert.Equal(t,
		[]string{"collectIssues", "collectTimeEntries", "convertAccounts"},
		plannedSubtasks(t, newScopeConfig(plugin.DOMAIN_TYPE_TICKET, plugin.DOMAIN_TYPE_CROSS)))
}

func TestMakePipelinePlanV200FiltersSubtasksByToggles(t *testing.T) {
	off, on := false, true
	t.Run("an unset config takes the toggle defaults: time entries on, sprints off", func(t *testing.T) {
		assert.Equal(t, []string{"collectIssues", "collectTimeEntries"},
			plannedSubtasks(t, newScopeConfig(plugin.DOMAIN_TYPE_TICKET)))
	})
	t.Run("a toggle switched off removes its subtask", func(t *testing.T) {
		config := newScopeConfig(plugin.DOMAIN_TYPE_TICKET)
		config.CollectTimeEntries = &off
		assert.Equal(t, []string{"collectIssues"}, plannedSubtasks(t, config))
	})
	t.Run("a toggle switched on adds its subtask", func(t *testing.T) {
		config := newScopeConfig(plugin.DOMAIN_TYPE_TICKET)
		config.VersionsAsSprints = &on
		assert.Equal(t, []string{"collectIssues", "collectTimeEntries", "convertVersionsAsSprints"}, plannedSubtasks(t, config))
	})
	t.Run("ungated subtasks ignore every toggle", func(t *testing.T) {
		config := newScopeConfig(plugin.DOMAIN_TYPE_TICKET, plugin.DOMAIN_TYPE_CROSS)
		config.CollectTimeEntries, config.VersionsAsSprints = &off, &off
		assert.Equal(t, []string{"collectIssues", "convertAccounts"}, plannedSubtasks(t, config))
	})
}
