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
	"github.com/apache/devlake/core/errors"
	coreModels "github.com/apache/devlake/core/models"
	"github.com/apache/devlake/core/models/domainlayer/didgen"
	"github.com/apache/devlake/core/models/domainlayer/ticket"
	"github.com/apache/devlake/core/plugin"
	"github.com/apache/devlake/core/utils"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/helpers/srvhelper"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/apache/devlake/plugins/redmine_go/tasks"
)

// MakePipelinePlanV200 builds the blueprint plan and the domain scopes of the
// selected projects.
func MakePipelinePlanV200(
	subtaskMetas []plugin.SubTaskMeta,
	connectionId uint64,
	bpScopes []*coreModels.BlueprintScope,
) (coreModels.PipelinePlan, []plugin.Scope, errors.Error) {
	connection, err := dsHelper.ConnSrv.FindByPk(connectionId)
	if err != nil {
		return nil, nil, err
	}
	scopeDetails, err := dsHelper.ScopeSrv.MapScopeDetails(connectionId, bpScopes)
	if err != nil {
		return nil, nil, err
	}
	plan, err := makePipelinePlanV200(subtaskMetas, tasks.SubtaskToggles, basicRes.GetConfig, scopeDetails, connection)
	if err != nil {
		return nil, nil, err
	}
	scopes, err := makeScopesV200(scopeDetails, connection)
	return plan, scopes, err
}

// makePipelinePlanV200 plans one task per project, each in its own stage so
// projects are collected one after the other and share the connection's rate
// limit instead of competing for it. getenv reads the REDMINE_GO_* flags that
// force a toggle off for every scope.
func makePipelinePlanV200(
	subtaskMetas []plugin.SubTaskMeta,
	subtaskToggles map[string]models.Toggle,
	getenv func(name string) string,
	scopeDetails []*srvhelper.ScopeDetail[models.RedmineGoProject, models.RedmineGoScopeConfig],
	connection *models.RedmineGoConnection,
) (coreModels.PipelinePlan, errors.Error) {
	plan := make(coreModels.PipelinePlan, len(scopeDetails))
	for i, scopeDetail := range scopeDetails {
		scope, scopeConfig := scopeDetail.Scope, scopeDetail.ScopeConfig
		transformation := scopeConfig.Transformation
		transformation.ApplyEnvFlags(getenv)
		task, err := helper.MakePipelinePlanTask(
			"redmine_go",
			EnabledSubtasks(subtaskMetas, subtaskToggles, &transformation),
			scopeConfig.Entities,
			tasks.RedmineGoOptions{
				ConnectionId:  connection.ID,
				ProjectId:     scope.Id,
				ScopeConfigId: scope.ScopeConfigId,
			},
		)
		if err != nil {
			return nil, err
		}
		plan[i] = coreModels.PipelineStage{task}
	}
	return plan, nil
}

// EnabledSubtasks drops the subtasks whose gating toggle is switched off.
// Entity filtering happens afterwards, in MakePipelinePlanTask.
func EnabledSubtasks(
	metas []plugin.SubTaskMeta,
	subtaskToggles map[string]models.Toggle,
	transformation *models.Transformation,
) []plugin.SubTaskMeta {
	enabled := make([]plugin.SubTaskMeta, 0, len(metas))
	for _, meta := range metas {
		if toggle, gated := subtaskToggles[meta.Name]; gated && !transformation.ToggleEnabled(toggle) {
			continue
		}
		enabled = append(enabled, meta)
	}
	return enabled
}

// makeScopesV200 maps each selected project to its domain scope, a
// ticket.Board. The board id is built from the numeric project id, so it
// survives renames of the project's name and identifier.
func makeScopesV200(
	scopeDetails []*srvhelper.ScopeDetail[models.RedmineGoProject, models.RedmineGoScopeConfig],
	connection *models.RedmineGoConnection,
) ([]plugin.Scope, errors.Error) {
	scopes := make([]plugin.Scope, 0, len(scopeDetails))
	idgen := didgen.NewDomainIdGenerator(&models.RedmineGoProject{})
	for _, scopeDetail := range scopeDetails {
		scope, scopeConfig := scopeDetail.Scope, scopeDetail.ScopeConfig
		id := idgen.Generate(connection.ID, scope.Id)
		if utils.StringsContains(scopeConfig.Entities, plugin.DOMAIN_TYPE_TICKET) {
			scopes = append(scopes, ticket.NewBoard(id, scope.Name))
		}
	}
	return scopes, nil
}
