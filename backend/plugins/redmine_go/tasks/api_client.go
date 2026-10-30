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
	"fmt"
	"net/http"

	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/log"
	"github.com/apache/devlake/core/plugin"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// NewRedmineGoApiClient builds the rate-limited async REST client for a
// Redmine connection.
//
// Retry policy has exactly one owner: the transport (retry_transport.go),
// which retries 429 and 5xx with backoff and honours Retry-After. The
// framework async client's own loop, which retries any status >= 400 on a
// fixed tick, is disabled with SetMaxRetry(0). Left active it would multiply
// the transport's attempts and retry pointlessly on Redmine's 4xx answers.
func NewRedmineGoApiClient(taskCtx plugin.TaskContext, connection *models.RedmineGoConnection) (*helper.ApiAsyncClient, errors.Error) {
	apiClient, err := helper.NewApiClientFromConnection(taskCtx.GetContext(), taskCtx, connection)
	if err != nil {
		return nil, err
	}
	apiClient.GetClient().Transport = newRetryTransport(apiClient.GetClient().Transport)
	rateLimitPerHour := connection.RateLimitPerHour
	if rateLimitPerHour <= 0 {
		rateLimitPerHour = models.DefaultRateLimitPerHour
	}
	asyncClient, err := helper.CreateAsyncApiClient(taskCtx, apiClient, &helper.ApiRateLimitCalculator{
		UserRateLimitPerHour: rateLimitPerHour,
	})
	if err != nil {
		return nil, err
	}
	asyncClient.SetMaxRetry(0)
	return asyncClient, nil
}

// IgnoreMissingItem returns the AfterResponse for collectors that fetch one
// Redmine object per request, such as one issue or one project. A 404 means
// the object was deleted since it was listed, and a 403 means the key lost
// access to it; both skip the object with a warning, so the subtask and its
// bookmark keep moving instead of failing every run. A 401 is never skipped:
// it means the key itself is wrong, and fails with the framework's error.
func IgnoreMissingItem(logger log.Logger) plugin.ApiClientAfterResponse {
	return func(res *http.Response) errors.Error {
		switch res.StatusCode {
		case http.StatusUnauthorized:
			return errors.Unauthorized.New("authentication failed, please check the Redmine API key")
		case http.StatusNotFound, http.StatusForbidden:
			logger.Warn(nil, "skipping %s: Redmine answered %d", requestPath(res), res.StatusCode)
			return helper.ErrIgnoreAndContinue
		}
		return nil
	}
}

func requestPath(res *http.Response) string {
	if res.Request == nil || res.Request.URL == nil {
		return "request"
	}
	return fmt.Sprintf("%s %s", res.Request.Method, res.Request.URL.Path)
}
