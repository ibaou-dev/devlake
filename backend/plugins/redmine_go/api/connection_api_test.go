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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/apache/devlake/core/config"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	"github.com/apache/devlake/helpers/unithelper"
	mockdal "github.com/apache/devlake/mocks/core/dal"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/go-playground/validator/v10"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupTestGlobals points the package globals at throwaway instances, the way
// Init does in production.
func setupTestGlobals() {
	mockRes := unithelper.DummyBasicRes(func(mockDal *mockdal.Dal) {})
	mockRes.On("GetConfigReader").Return(config.GetConfig())
	basicRes = mockRes
	vld = validator.New()
}

func TestNormalizeEndpoint(t *testing.T) {
	cases := []struct {
		name     string
		endpoint string
		want     string
		wantErr  string
	}{
		{"https host", "https://redmine.example.org", "https://redmine.example.org", ""},
		{"trailing slash is dropped", "https://redmine.example.org/", "https://redmine.example.org", ""},
		{"several trailing slashes", "https://redmine.example.org///", "https://redmine.example.org", ""},
		{"surrounding whitespace", "  http://redmine:3000/ ", "http://redmine:3000", ""},
		{"sub-URI is kept", "https://example.org/redmine/", "https://example.org/redmine", ""},
		{"empty", "", "", "is not an absolute URL"},
		{"no scheme", "redmine.example.org", "", "is not an absolute URL"},
		{"relative path", "/redmine", "", "is not an absolute URL"},
		{"ftp scheme", "ftp://redmine.example.org", "", `uses scheme "ftp"`},
		{"file scheme", "file:///etc/passwd", "", "is not an absolute URL"},
		{"credentials", "https://admin:secret@redmine.example.org", "", "must not contain credentials"},
		{"query", "https://redmine.example.org/?key=abc", "", "must not contain a query"},
		{"fragment", "https://redmine.example.org/#top", "", "must not contain a query or fragment"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeEndpoint(tc.endpoint)
			if tc.wantErr != "" {
				require.NotNil(t, err)
				assert.Equal(t, errors.BadInput, err.GetType())
				assert.Contains(t, err.Error(), tc.wantErr)
				assert.Contains(t, err.Error(), endpointExample, "the rejection shows the expected shape")
				return
			}
			require.Nil(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// fakeRedmine answers users/current.json under a base path and records what
// the request carried.
type fakeRedmine struct {
	server  *httptest.Server
	mux     *http.ServeMux
	status  int
	body    string
	ctype   string
	lastKey string
	lastURL string
}

const adminBody = `{"user":{"id":1,"login":"admin","admin":true,"firstname":"Redmine","lastname":"Admin",` +
	`"mail":"admin@example.org","api_key":"23a70a05f3fab12f3c818bdea373935c969cf199","status":1}}`
const memberBody = `{"user":{"id":9,"login":"ops1","firstname":"Ops","lastname":"One","status":1}}`

func newFakeRedmine(t *testing.T, basePath string) *fakeRedmine {
	f := &fakeRedmine{mux: http.NewServeMux(), status: http.StatusOK, body: adminBody, ctype: "application/json"}
	f.mux.HandleFunc(basePath+"/users/current.json", func(w http.ResponseWriter, r *http.Request) {
		f.lastKey = r.Header.Get(models.ApiKeyHeader)
		f.lastURL = r.URL.String()
		w.Header().Set("Content-Type", f.ctype)
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(f.body))
	})
	f.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	f.server = httptest.NewServer(f.mux)
	t.Cleanup(f.server.Close)
	return f
}

func testConn(endpoint, token string) models.RedmineGoConn {
	connection := models.RedmineGoConn{Token: token}
	connection.Endpoint = endpoint
	return connection
}

func TestTestConnection(t *testing.T) {
	setupTestGlobals()

	t.Run("administrator key", func(t *testing.T) {
		fake := newFakeRedmine(t, "")
		result, err := testConnection(context.TODO(), testConn(fake.server.URL+"/", "admin-key-0123456789"))
		require.Nil(t, err)
		assert.True(t, result.Success)
		assert.True(t, result.IsAdmin)
		assert.Equal(t, &TestUser{Id: 1, Login: "admin", Name: "Redmine Admin"}, result.User)
		assert.Contains(t, result.Message, "authenticated as admin")
		assert.Contains(t, result.Message, "administrator key")
		assert.Equal(t, "admin-key-0123456789", fake.lastKey, "the key goes out in the header")
		assert.Equal(t, "/users/current.json", fake.lastURL, "never in the query string")
		assert.NotContains(t, result.Connection.Token, "admin-key-0123456789", "the echoed connection is sanitized")
		encoded, marshalErr := json.Marshal(result)
		require.NoError(t, marshalErr)
		assert.NotContains(t, string(encoded), "23a70a05f3fab12f3c818bdea373935c969cf199", "the user's api_key is never echoed")
		assert.NotContains(t, string(encoded), "admin@example.org", "nor the e-mail address")
	})

	t.Run("non-administrator key", func(t *testing.T) {
		fake := newFakeRedmine(t, "")
		fake.body = memberBody
		result, err := testConnection(context.TODO(), testConn(fake.server.URL, "member-key-0123456789"))
		require.Nil(t, err)
		assert.True(t, result.Success)
		assert.False(t, result.IsAdmin)
		assert.Equal(t, &TestUser{Id: 9, Login: "ops1", Name: "Ops One"}, result.User)
		assert.Contains(t, result.Message, "non-administrator key")
	})

	t.Run("Redmine under a sub-URI", func(t *testing.T) {
		fake := newFakeRedmine(t, "/redmine")
		result, err := testConnection(context.TODO(), testConn(fake.server.URL+"/redmine/", "k"))
		require.Nil(t, err)
		assert.Equal(t, 1, result.User.Id)
		assert.Equal(t, "/redmine/users/current.json", fake.lastURL)
	})

	t.Run("wrong key", func(t *testing.T) {
		fake := newFakeRedmine(t, "")
		fake.status, fake.body = http.StatusUnauthorized, ""
		_, err := testConnection(context.TODO(), testConn(fake.server.URL, "wrong"))
		require.NotNil(t, err)
		assert.Equal(t, http.StatusBadRequest, err.GetType().GetHttpCode(), "never 401, which would pop up a browser dialog")
		assert.Contains(t, err.Error(), "rejected the API key")
	})

	t.Run("REST API disabled answers 403", func(t *testing.T) {
		fake := newFakeRedmine(t, "")
		fake.status, fake.body = http.StatusForbidden, ""
		_, err := testConnection(context.TODO(), testConn(fake.server.URL, "k"))
		require.NotNil(t, err)
		assert.Contains(t, err.Error(), "REST web service is disabled")
	})

	t.Run("HTML login page instead of JSON", func(t *testing.T) {
		fake := newFakeRedmine(t, "")
		fake.ctype = "text/html; charset=utf-8"
		fake.body = "<html><head><title>Redmine</title></head><body><form action=\"/login\">Login</form></body></html>"
		_, err := testConnection(context.TODO(), testConn(fake.server.URL, "k"))
		require.NotNil(t, err)
		assert.Equal(t, http.StatusBadRequest, err.GetType().GetHttpCode())
		assert.Contains(t, err.Error(), "did not answer with a Redmine user")
		assert.Contains(t, err.Error(), "REST web service is disabled")
		assert.NotContains(t, err.Error(), "<html", "the page itself is not echoed")
	})

	t.Run("JSON that is not a user", func(t *testing.T) {
		fake := newFakeRedmine(t, "")
		fake.body = `{"status":"ok"}`
		_, err := testConnection(context.TODO(), testConn(fake.server.URL, "k"))
		require.NotNil(t, err)
		assert.Contains(t, err.Error(), "did not answer with a Redmine user")
	})

	t.Run("wrong base path", func(t *testing.T) {
		fake := newFakeRedmine(t, "/redmine")
		_, err := testConnection(context.TODO(), testConn(fake.server.URL, "k"))
		require.NotNil(t, err)
		assert.Contains(t, err.Error(), "not found (404)")
		assert.Contains(t, err.Error(), "sub-URI")
	})

	t.Run("unexpected status", func(t *testing.T) {
		fake := newFakeRedmine(t, "")
		fake.status = http.StatusTeapot
		_, err := testConnection(context.TODO(), testConn(fake.server.URL, "k"))
		require.NotNil(t, err)
		assert.Contains(t, err.Error(), "unexpected status code 418")
	})

	t.Run("connection refused", func(t *testing.T) {
		fake := newFakeRedmine(t, "")
		endpoint := fake.server.URL
		fake.server.Close()
		_, err := testConnection(context.TODO(), testConn(endpoint, "k"))
		require.NotNil(t, err)
		assert.Equal(t, http.StatusBadRequest, err.GetType().GetHttpCode())
		assert.Contains(t, err.Error(), "cannot reach Redmine at "+endpoint+": the connection was refused")
		assert.NotContains(t, err.Error(), "Wraps", "the error chain stays out of the message")
	})

	t.Run("host name does not resolve", func(t *testing.T) {
		_, err := testConnection(context.TODO(), testConn("http://nowhere.invalid:3000", "k"))
		require.NotNil(t, err)
		assert.Contains(t, err.Error(), "the host name does not resolve")
		assert.NotContains(t, err.Error(), "Wraps")
	})

	t.Run("bad URL", func(t *testing.T) {
		_, err := testConnection(context.TODO(), testConn("ftp://redmine.example.org", "k"))
		require.NotNil(t, err)
		assert.Equal(t, errors.BadInput, err.GetType())
		assert.Contains(t, err.Error(), `uses scheme "ftp"`)
	})

	t.Run("missing key", func(t *testing.T) {
		_, err := testConnection(context.TODO(), testConn("https://redmine.example.org", ""))
		require.NotNil(t, err)
		assert.Contains(t, err.Error(), "invalid connection")
	})
}

func TestPostAndPatchConnectionRejectBadInputBeforeStoring(t *testing.T) {
	setupTestGlobals()

	_, err := PostConnections(&plugin.ApiResourceInput{Body: map[string]interface{}{
		"name": "r", "endpoint": "ftp://redmine.example.org", "token": "k",
	}})
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), `uses scheme "ftp"`)

	_, err = PostConnections(&plugin.ApiResourceInput{Body: map[string]interface{}{
		"name": "r", "endpoint": "https://redmine.example.org", "token": "  ",
	}})
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "token is required")

	_, err = PatchConnection(&plugin.ApiResourceInput{Body: map[string]interface{}{"endpoint": "not a url"}})
	require.NotNil(t, err)
	assert.Equal(t, errors.BadInput, err.GetType())

	_, err = PatchConnection(&plugin.ApiResourceInput{Body: map[string]interface{}{"endpoint": 42}})
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "endpoint must be a string")
}
