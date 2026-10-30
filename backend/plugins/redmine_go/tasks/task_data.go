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

package tasks

import (
	"time"

	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// RedmineGoOptions are the per-scope options of a pipeline task.
type RedmineGoOptions struct {
	ConnectionId  uint64 `json:"connectionId" mapstructure:"connectionId,omitempty"`
	ProjectId     uint64 `json:"projectId" mapstructure:"projectId,omitempty"`
	ScopeConfigId uint64 `json:"scopeConfigId" mapstructure:"scopeConfigId,omitempty"`
	// TimeAfter limits collection to data updated after this time (RFC 3339).
	TimeAfter string `json:"timeAfter" mapstructure:"timeAfter,omitempty"`
}

// RedmineGoTaskData is the shared context handed to every subtask.
type RedmineGoTaskData struct {
	Options    *RedmineGoOptions
	Connection *models.RedmineGoConnection
	TimeAfter  *time.Time
	// ApiClient is the rate-limited REST client. It is nil in e2e tests, which
	// import raw CSV rows instead of collecting.
	ApiClient *helper.ApiAsyncClient
	// Project is the scope being collected.
	Project *models.RedmineGoProject
	// ScopeConfig is never nil: PrepareTaskData substitutes an empty config,
	// whose unset mappings and toggles resolve to the built-in defaults.
	ScopeConfig *models.RedmineGoScopeConfig
}
