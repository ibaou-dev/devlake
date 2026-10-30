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
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	lakeErrors "github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/helpers/unithelper"
	"github.com/apache/devlake/impls/logruslog"
	mockplugin "github.com/apache/devlake/mocks/core/plugin"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestIgnoreMissingItem(t *testing.T) {
	var out bytes.Buffer
	base := logrus.New()
	base.SetOutput(&out)
	logger, err := logruslog.NewDefaultLogger(base)
	require.NoError(t, err)
	afterResponse := IgnoreMissingItem(logger)
	response := func(status int) *http.Response {
		req, reqErr := http.NewRequest(http.MethodGet, "https://redmine.example.org/issues/42.json", nil)
		require.NoError(t, reqErr)
		return &http.Response{StatusCode: status, Request: req}
	}

	for _, status := range []int{http.StatusNotFound, http.StatusForbidden} {
		out.Reset()
		assert.Equal(t, api.ErrIgnoreAndContinue, afterResponse(response(status)), "status %d", status)
		assert.Contains(t, out.String(), "skipping GET /issues/42.json", "a warning names the skipped item")
	}

	unauthorized := afterResponse(response(http.StatusUnauthorized))
	require.NotNil(t, unauthorized)
	assert.NotEqual(t, api.ErrIgnoreAndContinue, unauthorized)
	assert.Equal(t, lakeErrors.Unauthorized, unauthorized.GetType())

	for _, status := range []int{http.StatusOK, http.StatusBadRequest, http.StatusInternalServerError} {
		out.Reset()
		assert.Nil(t, afterResponse(response(status)), "status %d passes through to the framework", status)
		assert.Empty(t, out.String())
	}
}

func newTestTaskContext() *mockplugin.TaskContext {
	taskCtx := new(mockplugin.TaskContext)
	taskCtx.On("GetContext").Return(context.Background())
	taskCtx.On("GetConfig", mock.Anything).Return("")
	taskCtx.On("GetConfigReader").Return(viper.New())
	taskCtx.On("GetLogger").Return(unithelper.DummyLogger())
	return taskCtx
}

// The assembled client authenticates with the header only, and a per-item 404
// or 403 is skipped while the other items of the batch are still handled.
func TestAssembledClientAuthenticatesByHeaderAndSkipsMissingItems(t *testing.T) {
	var keyed atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(models.ApiKeyHeader) == "dummy-key" && r.URL.Query().Get("key") == "" {
			keyed.Add(1)
		}
		switch r.URL.Path {
		case "/issues/404.json":
			w.WriteHeader(http.StatusNotFound)
		case "/issues/403.json":
			w.WriteHeader(http.StatusForbidden)
		default:
			_, _ = w.Write([]byte("{}"))
		}
	}))
	defer server.Close()

	connection := &models.RedmineGoConnection{}
	connection.Endpoint = server.URL
	connection.Token = "dummy-key"
	client, err := NewRedmineGoApiClient(newTestTaskContext(), connection)
	require.NoError(t, err)
	defer client.Release()
	var warnings bytes.Buffer
	base := logrus.New()
	base.SetOutput(&warnings)
	logger, loggerErr := logruslog.NewDefaultLogger(base)
	require.NoError(t, loggerErr)
	client.SetAfterFunction(IgnoreMissingItem(logger))

	var mu sync.Mutex
	var handled []string
	for _, path := range []string{"issues/1.json", "issues/404.json", "issues/403.json", "issues/2.json"} {
		path := path
		client.DoGetAsync(path, nil, nil, func(res *http.Response) lakeErrors.Error {
			mu.Lock()
			defer mu.Unlock()
			handled = append(handled, res.Request.URL.Path)
			return nil
		})
	}
	require.NoError(t, client.WaitAsync())
	assert.ElementsMatch(t, []string{"/issues/1.json", "/issues/2.json"}, handled,
		"404 and 403 items are skipped, the rest are handled")
	assert.Contains(t, warnings.String(), "skipping GET /issues/404.json")
	assert.Contains(t, warnings.String(), "skipping GET /issues/403.json")
	assert.EqualValues(t, 4, keyed.Load(), "every request carries the key in the header and never in the query")
}
