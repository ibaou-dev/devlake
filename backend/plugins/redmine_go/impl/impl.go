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
	"fmt"
	"time"

	"github.com/apache/devlake/core/context"
	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	coreModels "github.com/apache/devlake/core/models"
	"github.com/apache/devlake/core/plugin"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/apache/devlake/plugins/redmine_go/models/migrationscripts"
	"github.com/apache/devlake/plugins/redmine_go/tasks"
)

var _ interface {
	plugin.PluginMeta
	plugin.PluginInit
	plugin.PluginTask
	plugin.PluginApi
	plugin.PluginModel
	plugin.PluginSource
	plugin.PluginMigration
	plugin.CloseablePluginTask
	plugin.DataSourcePluginBlueprintV200
} = (*RedmineGo)(nil)

// RedmineGo is the Redmine data source plugin.
type RedmineGo struct{}

// Init wires the API package to the framework.
func (p RedmineGo) Init(basicRes context.BasicRes) errors.Error {
	api.Init(basicRes, p)
	return nil
}

// Description describes the plugin in the plugin list.
func (p RedmineGo) Description() string {
	return "To collect and enrich data from Redmine"
}

// Name returns the plugin key, which prefixes its API routes and tables.
func (p RedmineGo) Name() string {
	return "redmine_go"
}

// RootPkgPath returns the plugin's Go package path.
func (p RedmineGo) RootPkgPath() string {
	return "github.com/apache/devlake/plugins/redmine_go"
}

// Connection returns the connection model.
func (p RedmineGo) Connection() dal.Tabler {
	return &models.RedmineGoConnection{}
}

// Scope returns the scope model: one Redmine project.
func (p RedmineGo) Scope() plugin.ToolLayerScope {
	return &models.RedmineGoProject{}
}

// ScopeConfig returns the scope-config model.
func (p RedmineGo) ScopeConfig() dal.Tabler {
	return &models.RedmineGoScopeConfig{}
}

// MigrationScripts returns the plugin's schema migrations.
func (p RedmineGo) MigrationScripts() []plugin.MigrationScript {
	return migrationscripts.All()
}

// GetTablesInfo lists every tool-layer table of the plugin.
func (p RedmineGo) GetTablesInfo() []dal.Tabler {
	return []dal.Tabler{
		&models.RedmineGoConnection{},
		&models.RedmineGoProject{},
		&models.RedmineGoScopeConfig{},
		&models.RedmineGoTracker{},
		&models.RedmineGoStatus{},
		&models.RedmineGoPriority{},
		&models.RedmineGoCustomField{},
		&models.RedmineGoVersion{},
		&models.RedmineGoIssueCategory{},
		&models.RedmineGoAccount{},
		&models.RedmineGoObservedField{},
		&models.RedmineGoIssue{},
		&models.RedmineGoJournal{},
		&models.RedmineGoJournalDetail{},
	}
}

// SubTaskMetas declares every subtask in execution order: metadata, then
// issues and their journals, then the conversion to the domain layer. The
// list's order is the contract: no Dependencies fields anywhere in the plugin.
func (p RedmineGo) SubTaskMetas() []plugin.SubTaskMeta {
	return []plugin.SubTaskMeta{
		tasks.CollectTrackersMeta,
		tasks.ExtractTrackersMeta,
		tasks.CollectStatusesMeta,
		tasks.ExtractStatusesMeta,
		tasks.CollectPrioritiesMeta,
		tasks.ExtractPrioritiesMeta,
		tasks.CollectCustomFieldsMeta,
		tasks.ExtractCustomFieldsMeta,
		tasks.CollectVersionsMeta,
		tasks.ExtractVersionsMeta,
		tasks.CollectIssueCategoriesMeta,
		tasks.ExtractIssueCategoriesMeta,
		tasks.CollectIssuesMeta,
		tasks.ExtractIssuesMeta,
		tasks.CollectIssueDetailsMeta,
		tasks.ExtractIssueJournalsMeta,
		tasks.ConvertProjectsMeta,
		tasks.ConvertAccountsMeta,
		tasks.ConvertIssuesMeta,
		tasks.ConvertIssueChangelogsMeta,
	}
}

// PrepareTaskData loads the connection, scope and scope config of a task and
// builds the rate-limited API client shared by its subtasks.
func (p RedmineGo) PrepareTaskData(taskCtx plugin.TaskContext, options map[string]interface{}) (interface{}, errors.Error) {
	var op tasks.RedmineGoOptions
	if err := helper.Decode(options, &op, nil); err != nil {
		return nil, errors.Default.Wrap(err, "could not decode Redmine options")
	}
	if op.ConnectionId == 0 {
		return nil, errors.BadInput.New("redmine_go connectionId is invalid")
	}
	if op.ProjectId == 0 {
		return nil, errors.BadInput.New("redmine_go projectId is required")
	}

	connection := &models.RedmineGoConnection{}
	connectionHelper := helper.NewConnectionHelper(taskCtx, nil, p.Name())
	if err := connectionHelper.FirstById(connection, op.ConnectionId); err != nil {
		return nil, errors.Default.Wrap(err, "error getting connection for Redmine plugin")
	}

	// A scope without a scope config is valid: every mapping has a built-in
	// fallback and every toggle a default, so an empty config stands in for it.
	scopeConfig := &models.RedmineGoScopeConfig{}
	if op.ScopeConfigId != 0 {
		if err := taskCtx.GetDal().First(scopeConfig, dal.Where("id = ?", op.ScopeConfigId)); err != nil {
			return nil, errors.Default.Wrap(err, "error getting scope config for Redmine plugin")
		}
	}
	// An empty selection means all domain types, exactly as the blueprint
	// planner reads it; otherwise the planner would schedule CROSS subtasks
	// while the extractors, seeing no CROSS here, derive nothing for them.
	if len(scopeConfig.Entities) == 0 {
		scopeConfig.Entities = plugin.DOMAIN_TYPES
	}
	// REDMINE_GO_* environment flags can only force a toggle off, and are read
	// here so no subtask has to look at the environment.
	scopeConfig.Transformation.ApplyEnvFlags(taskCtx.GetConfig)

	project := &models.RedmineGoProject{}
	if err := taskCtx.GetDal().First(project, dal.Where("connection_id = ? AND id = ?", op.ConnectionId, op.ProjectId)); err != nil {
		return nil, errors.Default.Wrap(err, "error getting project scope for Redmine plugin")
	}

	apiClient, err := tasks.NewRedmineGoApiClient(taskCtx, connection)
	if err != nil {
		return nil, err
	}

	taskData := &tasks.RedmineGoTaskData{
		Options:     &op,
		Connection:  connection,
		ScopeConfig: scopeConfig,
		Project:     project,
		ApiClient:   apiClient,
	}
	if op.TimeAfter != "" {
		timeAfter, errConv := errors.Convert01(time.Parse(time.RFC3339, op.TimeAfter))
		if errConv != nil {
			return nil, errors.BadInput.Wrap(errConv, "invalid timeAfter")
		}
		taskData.TimeAfter = &timeAfter
	}
	return taskData, nil
}

// Close releases the API client's scheduler once all subtasks are done. A task
// whose preparation failed has no data and nothing to release.
func (p RedmineGo) Close(taskCtx plugin.TaskContext) errors.Error {
	rawData := taskCtx.GetData()
	if rawData == nil {
		return nil
	}
	data, ok := rawData.(*tasks.RedmineGoTaskData)
	if !ok {
		return errors.Default.New(fmt.Sprintf("GetData failed when try to close %+v", taskCtx))
	}
	if data.ApiClient != nil {
		data.ApiClient.Release()
	}
	return nil
}

// ApiResources lists the plugin's REST routes.
func (p RedmineGo) ApiResources() map[string]map[string]plugin.ApiResourceHandler {
	return map[string]map[string]plugin.ApiResourceHandler{
		"test": {
			"POST": api.TestConnection,
		},
		"connections": {
			"POST": api.PostConnections,
			"GET":  api.ListConnections,
		},
		"connections/:connectionId": {
			"PATCH":  api.PatchConnection,
			"DELETE": api.DeleteConnection,
			"GET":    api.GetConnection,
		},
		"connections/:connectionId/test": {
			"POST": api.TestExistingConnection,
		},
		"connections/:connectionId/remote-scopes": {
			"GET": api.RemoteScopes,
		},
		"connections/:connectionId/search-remote-scopes": {
			"GET": api.SearchRemoteScopes,
		},
		"connections/:connectionId/proxy/rest/*path": {
			"GET": api.Proxy,
		},
		"connections/:connectionId/scope-configs": {
			"POST": api.PostScopeConfig,
			"GET":  api.GetScopeConfigList,
		},
		"connections/:connectionId/scope-configs/:scopeConfigId": {
			"PATCH":  api.PatchScopeConfig,
			"GET":    api.GetScopeConfig,
			"DELETE": api.DeleteScopeConfig,
		},
		"connections/:connectionId/scope-config-defaults": {
			"GET": api.GetScopeConfigDefaults,
		},
		"connections/:connectionId/observed-values": {
			"GET": api.GetObservedValues,
		},
		"connections/:connectionId/scopes": {
			"GET": api.GetScopeList,
			"PUT": api.PutScopes,
		},
		"connections/:connectionId/scopes/:scopeId": {
			"GET":    api.GetScope,
			"PATCH":  api.PatchScope,
			"DELETE": api.DeleteScope,
		},
		// The config UI looks up the projects attached to a scope config through
		// the singular `scope-config`, which is not nested under a connection.
		"scope-config/:scopeConfigId/projects": {
			"GET": api.GetProjectsByScopeConfig,
		},
	}
}

// MakeDataSourcePipelinePlanV200 plans the blueprint tasks of the selected
// projects and returns their domain scopes.
func (p RedmineGo) MakeDataSourcePipelinePlanV200(
	connectionId uint64,
	scopes []*coreModels.BlueprintScope,
) (coreModels.PipelinePlan, []plugin.Scope, errors.Error) {
	return api.MakePipelinePlanV200(p.SubTaskMetas(), connectionId, scopes)
}
