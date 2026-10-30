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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/apache/devlake/core/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnectionSanitize(t *testing.T) {
	connection := RedmineGoConnection{}
	connection.Token = "23a70a05f3fab12f3c818bdea373935c969cf199"
	sanitized := connection.Sanitize()
	assert.NotEqual(t, connection.Token, sanitized.Token)
	assert.NotContains(t, sanitized.Token, "f3fab12f3c818bdea373935c")
	assert.Equal(t, "23a70a05f3fab12f3c818bdea373935c969cf199", connection.Token, "Sanitize works on a copy")
}

func TestConnectionMergeFromRequest(t *testing.T) {
	const storedToken = "23a70a05f3fab12f3c818bdea373935c969cf199"
	stored := func() *RedmineGoConnection {
		c := &RedmineGoConnection{}
		c.Name = "redmine"
		c.Endpoint = "https://redmine.example.org"
		c.Token = storedToken
		return c
	}
	cases := []struct {
		name      string
		body      map[string]interface{}
		wantToken string
		wantName  string
	}{
		{"sanitized token keeps the stored one", map[string]interface{}{"name": "renamed", "token": utils.SanitizeString(storedToken)}, storedToken, "renamed"},
		{"empty token keeps the stored one", map[string]interface{}{"token": ""}, storedToken, "redmine"},
		{"missing token keeps the stored one", map[string]interface{}{"name": "again"}, storedToken, "again"},
		{"a new token replaces the stored one", map[string]interface{}{"token": "0123456789abcdef0123456789abcdef01234567"}, "0123456789abcdef0123456789abcdef01234567", "redmine"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			connection := stored()
			require.NoError(t, connection.MergeFromRequest(connection, tc.body))
			assert.Equal(t, tc.wantToken, connection.Token)
			assert.Equal(t, tc.wantName, connection.Name)
			assert.Equal(t, "https://redmine.example.org", connection.Endpoint)
		})
	}
}

func TestSetupAuthenticationUsesHeaderOnly(t *testing.T) {
	connection := &RedmineGoConn{Token: "secret-key"}
	req, err := http.NewRequest(http.MethodGet, "https://redmine.example.org/issues.json", nil)
	require.NoError(t, err)
	require.Nil(t, connection.SetupAuthentication(req))
	assert.Equal(t, "secret-key", req.Header.Get(ApiKeyHeader))
	assert.Empty(t, req.URL.Query().Get("key"), "the key never goes into the query string")
	assert.Empty(t, req.Header.Get("Authorization"))
}

// A redirect to another host must not carry the API key; a redirect inside the
// same host (http to https, trailing slash, login page) keeps it.
func TestRedirectPolicyDropsApiKeyForForeignHost(t *testing.T) {
	var foreignKey string
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		foreignKey = r.Header.Get(ApiKeyHeader)
	}))
	defer foreign.Close()
	var sameHostKey string
	var origin *httptest.Server
	origin = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/away":
			http.Redirect(w, r, foreign.URL+"/landing", http.StatusFound)
		case "/local":
			http.Redirect(w, r, origin.URL+"/landing", http.StatusFound)
		default:
			sameHostKey = r.Header.Get(ApiKeyHeader)
		}
	}))
	defer origin.Close()

	client := &http.Client{CheckRedirect: dropApiKeyOnForeignRedirect}
	get := func(url string) {
		req, err := http.NewRequest(http.MethodGet, url, nil)
		require.NoError(t, err)
		req.Header.Set(ApiKeyHeader, "secret-key")
		res, err := client.Do(req)
		require.NoError(t, err)
		_ = res.Body.Close()
	}
	get(origin.URL + "/away")
	assert.Empty(t, foreignKey, "the key must not reach a foreign host")
	get(origin.URL + "/local")
	assert.Equal(t, "secret-key", sameHostKey, "same-host redirects keep the key")
}

func TestRedirectPolicyStopsAfterTenRedirects(t *testing.T) {
	var loop *httptest.Server
	loop = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, loop.URL+"/again", http.StatusFound)
	}))
	defer loop.Close()
	client := &http.Client{CheckRedirect: dropApiKeyOnForeignRedirect}
	res, err := client.Get(loop.URL)
	if res != nil {
		_ = res.Body.Close()
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stopped after 10 redirects")
}
