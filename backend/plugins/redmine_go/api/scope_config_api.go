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
	"net/http"

	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// defaultEntities is the domain-type pair every scope config collects: issues
// (TICKET) and the records derived from their changelog (CROSS). The config UI
// always sends both; the default keeps raw API callers from persisting a scope
// config that collects nothing.
var defaultEntities = []string{plugin.DOMAIN_TYPE_TICKET, plugin.DOMAIN_TYPE_CROSS}

// defaultScopeConfigEntities fills in the default entity selection when the
// request body leaves it unset. An explicit selection, even a single domain
// type, is never overridden. Over the wire the body is decoded into
// map[string]interface{}, so entities arrives as []interface{}; the []string
// shape only occurs in-process.
func defaultScopeConfigEntities(body map[string]interface{}) {
	switch entities := body["entities"].(type) {
	case []interface{}:
		if len(entities) == 0 {
			body["entities"] = defaultEntities
		}
	case []string:
		if len(entities) == 0 {
			body["entities"] = defaultEntities
		}
	default:
		body["entities"] = defaultEntities
	}
}

// PostScopeConfig create scope config for Redmine
// @Summary create scope config for Redmine
// @Description Create a scope config. The transformation is validated against the scope-config contract and rejected with 400 and a message naming each violation; entities default to TICKET and CROSS
// @Tags plugins/redmine_go
// @Accept application/json
// @Param connectionId path int true "connection ID"
// @Param scopeConfig body models.RedmineGoScopeConfig true "scope config"
// @Success 200  {object} models.RedmineGoScopeConfig
// @Failure 400  {object} shared.ApiBody "Bad Request"
// @Failure 500  {object} shared.ApiBody "Internal Error"
// @Router /plugins/redmine_go/connections/{connectionId}/scope-configs [POST]
func PostScopeConfig(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	candidate := &models.RedmineGoScopeConfig{}
	if err := candidate.MergeFromRequest(candidate, input.Body); err != nil {
		return nil, errors.Convert(err)
	}
	defaultScopeConfigEntities(input.Body)
	return dsHelper.ScopeConfigApi.Post(input)
}

// PatchScopeConfig update scope config for Redmine
// @Summary update scope config for Redmine
// @Description Update a scope config; the merged result is validated against the scope-config contract and rejected with 400 when it breaks it
// @Tags plugins/redmine_go
// @Accept application/json
// @Param connectionId path int true "connection ID"
// @Param scopeConfigId path int true "scopeConfigId"
// @Param scopeConfig body models.RedmineGoScopeConfig true "scope config"
// @Success 200  {object} models.RedmineGoScopeConfig
// @Failure 400  {object} shared.ApiBody "Bad Request"
// @Failure 500  {object} shared.ApiBody "Internal Error"
// @Router /plugins/redmine_go/connections/{connectionId}/scope-configs/{scopeConfigId} [PATCH]
func PatchScopeConfig(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	return dsHelper.ScopeConfigApi.Patch(input)
}

// GetScopeConfig return one scope config
// @Summary return one scope config
// @Description return one scope config
// @Tags plugins/redmine_go
// @Param connectionId path int true "connection ID"
// @Param scopeConfigId path int true "scopeConfigId"
// @Success 200  {object} models.RedmineGoScopeConfig
// @Failure 400  {object} shared.ApiBody "Bad Request"
// @Failure 500  {object} shared.ApiBody "Internal Error"
// @Router /plugins/redmine_go/connections/{connectionId}/scope-configs/{scopeConfigId} [GET]
func GetScopeConfig(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	return dsHelper.ScopeConfigApi.GetDetail(input)
}

// GetScopeConfigList return all scope configs
// @Summary return all scope configs
// @Description return all scope configs of a connection
// @Tags plugins/redmine_go
// @Param connectionId path int true "connection ID"
// @Param pageSize query int false "page size, default 50"
// @Param page query int false "page size, default 1"
// @Success 200  {object} []models.RedmineGoScopeConfig
// @Failure 400  {object} shared.ApiBody "Bad Request"
// @Failure 500  {object} shared.ApiBody "Internal Error"
// @Router /plugins/redmine_go/connections/{connectionId}/scope-configs [GET]
func GetScopeConfigList(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	return dsHelper.ScopeConfigApi.GetAll(input)
}

// DeleteScopeConfig delete a scope config
// @Summary delete a scope config
// @Description delete a scope config
// @Tags plugins/redmine_go
// @Param scopeConfigId path int true "scopeConfigId"
// @Param connectionId path int true "connectionId"
// @Success 200
// @Failure 400  {object} shared.ApiBody "Bad Request"
// @Failure 500  {object} shared.ApiBody "Internal Error"
// @Router /plugins/redmine_go/connections/{connectionId}/scope-configs/{scopeConfigId} [DELETE]
func DeleteScopeConfig(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	return dsHelper.ScopeConfigApi.Delete(input)
}

// GetProjectsByScopeConfig return projects details related by scope config
// @Summary return all related projects
// @Description return all related projects
// @Tags plugins/redmine_go
// @Param scopeConfigId path int true "scopeConfigId"
// @Success 200  {object} models.ProjectScopeOutput
// @Failure 400  {object} shared.ApiBody "Bad Request"
// @Failure 500  {object} shared.ApiBody "Internal Error"
// @Router /plugins/redmine_go/scope-config/{scopeConfigId}/projects [GET]
func GetProjectsByScopeConfig(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	return dsHelper.ScopeConfigApi.GetProjectsByScopeConfig(input)
}

// loadDefaultTransformation resolves the built-in mapping against the
// instance's live trackers and statuses. Both lists are readable with a
// non-administrator key.
func loadDefaultTransformation(apiClient plugin.ApiClient) (*models.Transformation, errors.Error) {
	var trackers struct {
		Trackers []models.TrackerInfo `json:"trackers"`
	}
	if err := getJson(apiClient, "trackers.json", nil, &trackers); err != nil {
		return nil, err
	}
	var statuses struct {
		IssueStatuses []models.StatusInfo `json:"issue_statuses"`
	}
	if err := getJson(apiClient, "issue_statuses.json", nil, &statuses); err != nil {
		return nil, err
	}
	defaults := models.DefaultTransformation(trackers.Trackers, statuses.IssueStatuses)
	return &defaults, nil
}

// GetScopeConfigDefaults return the out-of-the-box mapping of a connection
// @Summary return the out-of-the-box mapping resolved against the live Redmine
// @Description Returns the transformation a scope config gets when nothing is configured, resolved against the trackers and statuses of the connected Redmine: Bug, Feature and Support trackers map to BUG, REQUIREMENT and TASK, statuses map by is_closed and tracker default status, the component is the issue category, and every toggle carries its default. The config UI pre-fills new scope configs with it.
// @Tags plugins/redmine_go
// @Param connectionId path int true "connection ID"
// @Success 200  {object} models.Transformation
// @Failure 400  {object} shared.ApiBody "Bad Request"
// @Failure 500  {object} shared.ApiBody "Internal Error"
// @Router /plugins/redmine_go/connections/{connectionId}/scope-config-defaults [GET]
func GetScopeConfigDefaults(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	connection, err := dsHelper.ConnApi.FindByPk(input)
	if err != nil {
		return nil, err
	}
	apiClient, err := helper.NewApiClientFromConnection(context.TODO(), basicRes, connection)
	if err != nil {
		return nil, errors.BadInput.Wrap(err, "cannot reach Redmine")
	}
	defaults, err := loadDefaultTransformation(apiClient)
	if err != nil {
		return nil, err
	}
	return &plugin.ApiResourceOutput{Body: defaults, Status: http.StatusOK}, nil
}
