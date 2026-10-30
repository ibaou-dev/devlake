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
	"fmt"
	"net/http"

	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	"github.com/apache/devlake/core/utils"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
)

// ApiKeyHeader is the only channel the Redmine API key travels in. A query
// parameter (`?key=`) would put the secret into access logs and proxies.
const ApiKeyHeader = "X-Redmine-API-Key"

// DefaultRateLimitPerHour is the request budget of a connection that sets no
// RateLimitPerHour. Redmine has no rate limit of its own; 36000 requests an
// hour (ten a second) is gentle enough for a small self-hosted instance
// while keeping large collections practical.
const DefaultRateLimitPerHour = 36000

// maxRedirects mirrors net/http's own limit for the default redirect policy.
const maxRedirects = 10

// RedmineGoConn holds the essential information to connect to the Redmine
// REST API: an endpoint, an optional proxy, a rate limit and an API key. The
// key is encrypted at rest and sent in ApiKeyHeader.
type RedmineGoConn struct {
	helper.RestConnection `mapstructure:",squash"`
	Token                 string `mapstructure:"token" validate:"required" json:"token" gorm:"serializer:encdec"`
}

// SetupAuthentication puts the API key into ApiKeyHeader.
func (c *RedmineGoConn) SetupAuthentication(req *http.Request) errors.Error {
	req.Header.Set(ApiKeyHeader, c.Token)
	return nil
}

// PrepareApiClient installs a redirect policy that never forwards the API key
// to another host: Go copies custom headers across redirects, so a Redmine
// that redirects to a foreign host would otherwise receive the secret.
func (c *RedmineGoConn) PrepareApiClient(apiClient plugin.ApiClient) errors.Error {
	if client, ok := apiClient.(*helper.ApiClient); ok {
		client.GetClient().CheckRedirect = dropApiKeyOnForeignRedirect
	}
	return nil
}

func dropApiKeyOnForeignRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	if req.URL.Host != via[0].URL.Host {
		req.Header.Del(ApiKeyHeader)
	}
	return nil
}

// Sanitize masks the API key so the connection can be returned to clients.
func (c *RedmineGoConn) Sanitize() RedmineGoConn {
	c.Token = utils.SanitizeString(c.Token)
	return *c
}

// RedmineGoConnection is RedmineGoConn plus the ID and name used for storage.
type RedmineGoConnection struct {
	helper.BaseConnection `mapstructure:",squash"`
	RedmineGoConn         `mapstructure:",squash"`
}

// Sanitize returns a copy of the connection with the API key masked.
func (connection RedmineGoConnection) Sanitize() RedmineGoConnection {
	connection.RedmineGoConn = connection.RedmineGoConn.Sanitize()
	return connection
}

// MergeFromRequest merges a PATCH body into the stored connection and keeps
// the stored API key when the body carries an empty or the sanitized value,
// so renaming a connection never wipes the secret.
func (connection *RedmineGoConnection) MergeFromRequest(target *RedmineGoConnection, body map[string]interface{}) error {
	token := target.Token
	if err := helper.DecodeMapStruct(body, target, true); err != nil {
		return err
	}
	modifiedToken := target.Token
	if modifiedToken == "" || modifiedToken == utils.SanitizeString(token) {
		target.Token = token
	}
	return nil
}

// TableName returns the connection table name.
func (RedmineGoConnection) TableName() string {
	return "_tool_redmine_go_connections"
}
