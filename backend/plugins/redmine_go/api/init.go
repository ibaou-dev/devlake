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
	"github.com/apache/devlake/core/context"
	"github.com/apache/devlake/core/plugin"
	"github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/go-playground/validator/v10"
)

var vld *validator.Validate
var basicRes context.BasicRes
var dsHelper *api.DsHelper[models.RedmineGoConnection, models.RedmineGoProject, models.RedmineGoScopeConfig]
var raProxy *api.DsRemoteApiProxyHelper[models.RedmineGoConnection]
var raScopeList *api.DsRemoteApiScopeListHelper[models.RedmineGoConnection, models.RedmineGoProject, struct{}]
var raScopeSearch *api.DsRemoteApiScopeSearchHelper[models.RedmineGoConnection, models.RedmineGoProject]

// Init wires the API package to the framework. It runs once when the plugin
// is loaded.
func Init(br context.BasicRes, p plugin.PluginMeta) {
	basicRes = br
	vld = validator.New()
	dsHelper = api.NewDataSourceHelper[
		models.RedmineGoConnection, models.RedmineGoProject, models.RedmineGoScopeConfig,
	](
		br,
		p.Name(),
		[]string{"name", "identifier"},
		func(c models.RedmineGoConnection) models.RedmineGoConnection {
			return c.Sanitize()
		},
		nil,
		nil,
	)
	raProxy = api.NewDsRemoteApiProxyHelper[models.RedmineGoConnection](dsHelper.ConnApi.ModelApiHelper)
	raScopeList = api.NewDsRemoteApiScopeListHelper[models.RedmineGoConnection, models.RedmineGoProject, struct{}](raProxy, listRedmineRemoteScopes)
	raScopeSearch = api.NewDsRemoteApiScopeSearchHelper[models.RedmineGoConnection, models.RedmineGoProject](raProxy, searchRedmineRemoteScopes)
}
