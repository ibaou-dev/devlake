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

// Package archived holds frozen snapshots of the tool-layer models as they
// existed at each migration. The live models in plugins/redmine_go/models may
// evolve; these snapshots keep historical migrations stable. They are built
// on core/models/migrationscripts/archived helpers and never import the live
// models package.
//
// The archived helpers are not the live embeds: archived.ScopeConfig lacks
// ConnectionId and Name, and there is no archived.Scope at all, so both are
// unrolled by hand below. A hand-added column that goes missing surfaces only
// as a TestMigrationSchemaMatchesModels failure.
package archived

import (
	"github.com/apache/devlake/core/models/migrationscripts/archived"
)

// RedmineGoConnection unrolls BaseConnection, RestConnection and the API key
// into plain fields; Token keeps the encdec serializer.
type RedmineGoConnection struct {
	Name string `gorm:"type:varchar(100);uniqueIndex" json:"name"`
	archived.Model
	Endpoint         string `mapstructure:"endpoint" json:"endpoint"`
	Proxy            string `mapstructure:"proxy" json:"proxy"`
	RateLimitPerHour int    `json:"rateLimitPerHour"`
	Token            string `mapstructure:"token" json:"token" gorm:"serializer:encdec"`
}

// TableName returns the connection table name.
func (RedmineGoConnection) TableName() string { return "_tool_redmine_go_connections" }

// RedmineGoProject unrolls common.Scope by hand: archived.NoPKModel, the
// ConnectionId primary key and ScopeConfigId.
type RedmineGoProject struct {
	archived.NoPKModel
	ConnectionId  uint64 `json:"connectionId" gorm:"primaryKey"`
	ScopeConfigId uint64 `json:"scopeConfigId,omitempty"`
	Id            uint64 `json:"id" gorm:"primaryKey;autoIncrement:false"`
	Identifier    string `json:"identifier" gorm:"type:varchar(100);index"`
	ParentId      uint64 `json:"parentId"`
	Name          string `json:"name" gorm:"type:varchar(255)"`
	Description   string `json:"description"`
	Url           string `json:"url" gorm:"type:varchar(512)"`
}

// TableName returns the project table name.
func (RedmineGoProject) TableName() string { return "_tool_redmine_go_projects" }

// RedmineGoScopeConfig hand-adds ConnectionId and Name, which the live
// common.ScopeConfig carries but archived.ScopeConfig does not. The nested
// rule types are stored as JSON, so the snapshot keeps them as generic JSON values.
type RedmineGoScopeConfig struct {
	archived.ScopeConfig
	ConnectionId   uint64                   `json:"connectionId" gorm:"index"`
	Name           string                   `json:"name" gorm:"type:varchar(255);uniqueIndex"`
	TypeRules      []map[string]interface{} `json:"typeRules" gorm:"type:json;serializer:json"`
	StatusMappings []map[string]interface{} `json:"statusMappings" gorm:"type:json;serializer:json"`
	Severity       []map[string]interface{} `json:"severity" gorm:"type:json;serializer:json"`
	Environment    []map[string]interface{} `json:"environment" gorm:"type:json;serializer:json"`
	Component      map[string]interface{}   `json:"component" gorm:"type:json;serializer:json"`

	ReconcileSweep      *bool `json:"reconcileSweep"`
	CollectTimeEntries  *bool `json:"collectTimeEntries"`
	CollectChangesets   *bool `json:"collectChangesets"`
	CollectComments     *bool `json:"collectComments"`
	IncludePrivateNotes *bool `json:"includePrivateNotes"`
	VersionsAsSprints   *bool `json:"versionsAsSprints"`
}

// TableName returns the scope-config table name.
func (RedmineGoScopeConfig) TableName() string { return "_tool_redmine_go_scope_configs" }
