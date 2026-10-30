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

	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProxyPathAllowed(t *testing.T) {
	for path, want := range map[string]bool{
		"projects.json":                               true,
		"projects/6.json":                             true,
		"projects/rgo-fixtures.json":                  true,
		"projects/6/versions.json":                    true,
		"projects/rgo-fixtures/issue_categories.json": true,
		"trackers.json":                               true,
		"issue_statuses.json":                         true,
		"enumerations/issue_priorities.json":          true,
		"enumerations/time_entry_activities.json":     true,
		"custom_fields.json":                          true,

		"":                                    false,
		"users/current.json":                  false,
		"users/1.json":                        false,
		"users.json":                          false,
		"my/account.json":                     false,
		"issues.json":                         false,
		"issues/1.json":                       false,
		"time_entries.json":                   false,
		"projects/6/memberships.json":         false,
		"projects/6/versions":                 false,
		"projects/6/versions.xml":             false,
		"trackers.json/../users/current.json": false,
		"projects/../users/current.json":      false,
		"projects/6/../../users/current.json": false,
		"projects//6.json":                    false,
		"enumerations/../users/current.json":  false,
		"enumerations/a/b.json":               false,
		"/trackers.json":                      false, // callers strip the leading slash first
		"trackers.json/":                      false,
	} {
		assert.Equal(t, want, proxyPathAllowed(path), "path %q", path)
	}
}

// A refused path is answered with 403 and a message, before any connection is
// looked up or any request leaves the server.
func TestProxyRefusesPathsOutsideTheAllowlist(t *testing.T) {
	for _, path := range []string{"users/current.json", "/users/current.json", "issues.json", "../users/current.json"} {
		out, err := Proxy(&plugin.ApiResourceInput{Params: map[string]string{"connectionId": "1", "path": path}})
		require.Error(t, err, "path %q", path)
		assert.Nil(t, out)
		assert.Equal(t, errors.Forbidden, err.GetType(), "path %q", path)
		assert.Equal(t, 403, err.GetType().GetHttpCode())
		assert.Contains(t, err.Error(), "only forwards Redmine metadata paths")
	}
}
