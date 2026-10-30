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
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/models/common"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
)

// RedmineGoScopeConfig carries the mapping rules and toggles of a scope. A
// scope without a scope config still works: every mapping has a built-in
// fallback and every toggle a default (see DefaultTransformation).
type RedmineGoScopeConfig struct {
	common.ScopeConfig `mapstructure:",squash" json:",inline" gorm:"embedded"`
	Transformation     `mapstructure:",squash" json:",inline" gorm:"embedded"`
}

// TableName returns the scope-config table name.
func (RedmineGoScopeConfig) TableName() string {
	return "_tool_redmine_go_scope_configs"
}

// SetConnectionId binds the scope config to a connection.
func (sc *RedmineGoScopeConfig) SetConnectionId(c *RedmineGoScopeConfig, connectionId uint64) {
	c.ConnectionId = connectionId
	c.ScopeConfig.ConnectionId = connectionId
}

// MergeFromRequest applies a request body to the scope config, a stored one
// for PATCH or a new one for POST, and rejects the result when it breaks the
// transformation contract, so a request can never leave an invalid config
// behind.
func (sc *RedmineGoScopeConfig) MergeFromRequest(target *RedmineGoScopeConfig, body map[string]interface{}) error {
	if err := CheckTransformationShape(body); err != nil {
		return err
	}
	if err := helper.DecodeMapStruct(body, target, true); err != nil {
		return errors.BadInput.Wrap(err, "invalid scope config")
	}
	if err := target.Validate(); err != nil {
		return err
	}
	return nil
}
