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

package models

import (
	"strconv"

	"github.com/apache/devlake/core/models/common"
	"github.com/apache/devlake/core/plugin"
)

var _ plugin.ToolLayerScope = (*RedmineGoProject)(nil)

// RedmineGoProject is the data-source scope of the plugin: one Redmine
// project, which maps to a domain-layer ticket.Board. The primary key is the
// numeric project id, which survives identifier and name changes; Identifier
// is the human-readable slug used in URLs.
type RedmineGoProject struct {
	common.Scope `mapstructure:",squash"`
	Id           uint64 `json:"id" mapstructure:"id" gorm:"primaryKey;autoIncrement:false"`
	Identifier   string `json:"identifier" mapstructure:"identifier" gorm:"type:varchar(100);index"`
	// ParentId is the id of the parent project, or 0 for a top-level project.
	ParentId    uint64 `json:"parentId" mapstructure:"parentId"`
	Name        string `json:"name" mapstructure:"name" gorm:"type:varchar(255)"`
	Description string `json:"description" mapstructure:"description"`
	// Url is the project's web page in Redmine.
	Url string `json:"url" mapstructure:"url" gorm:"type:varchar(512)"`
}

// ScopeId returns the project id as the string the scope API addresses it by.
func (p RedmineGoProject) ScopeId() string {
	return strconv.FormatUint(p.Id, 10)
}

// ScopeName returns the project name.
func (p RedmineGoProject) ScopeName() string {
	return p.Name
}

// ScopeFullName returns the project name; Redmine has no namespace above it.
func (p RedmineGoProject) ScopeFullName() string {
	return p.Name
}

// ScopeParams returns the identity stored in the params column of raw rows.
func (p RedmineGoProject) ScopeParams() interface{} {
	return &RedmineGoApiParams{
		ConnectionId: p.ConnectionId,
		ProjectId:    p.Id,
	}
}

// TableName returns the project table name.
func (RedmineGoProject) TableName() string {
	return "_tool_redmine_go_projects"
}

// RedmineGoApiParams identifies the scope a raw row belongs to. It is stored
// in the params column of every _raw_redmine_go_* table.
type RedmineGoApiParams struct {
	ConnectionId uint64
	ProjectId    uint64
}

// GetParams implements helper.TaskOptions: the raw-data identity is the
// params struct itself.
func (p RedmineGoApiParams) GetParams() any {
	return p
}
