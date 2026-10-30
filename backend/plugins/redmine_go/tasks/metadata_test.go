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
	"net/http"
	"testing"

	lakeErrors "github.com/apache/devlake/core/errors"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/impls/logruslog"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCustomFieldsForbiddenWarnsAndContinues(t *testing.T) {
	var out bytes.Buffer
	base := logrus.New()
	base.SetOutput(&out)
	logger, err := logruslog.NewDefaultLogger(base)
	require.NoError(t, err)
	afterResponse := customFieldsList.afterResponse(logger)
	res := func(status int) *http.Response { return &http.Response{StatusCode: status} }

	assert.Equal(t, helper.ErrIgnoreAndContinue, afterResponse(res(http.StatusForbidden)))
	assert.Equal(t, 1, bytes.Count(out.Bytes(), []byte("level=warning")), "one warning for the one refused request")
	assert.Contains(t, out.String(), "custom_fields.json")
	assert.Contains(t, out.String(), "administrators only")

	unauthorized := afterResponse(res(http.StatusUnauthorized))
	require.NotNil(t, unauthorized)
	assert.Equal(t, lakeErrors.Unauthorized, unauthorized.GetType(), "a wrong key still fails the run")
	assert.Nil(t, afterResponse(res(http.StatusOK)))
	assert.Nil(t, afterResponse(res(http.StatusNotFound)), "only the forbidden answer is tolerated for custom fields")
}

func TestPerItemListsSkipMissingProjects(t *testing.T) {
	logger, err := logruslog.NewDefaultLogger(logrus.New())
	require.NoError(t, err)
	for _, list := range []metadataList{versionsList, categoriesList, trackersList} {
		afterResponse := list.afterResponse(logger)
		assert.Equal(t, helper.ErrIgnoreAndContinue, afterResponse(&http.Response{StatusCode: http.StatusNotFound}), list.rawTable)
		assert.Equal(t, helper.ErrIgnoreAndContinue, afterResponse(&http.Response{StatusCode: http.StatusForbidden}), list.rawTable)
	}
}

// Connection-wide lists share one raw identity across scopes; project lists
// are keyed by the scope's project.
func TestMetadataListOptions(t *testing.T) {
	data := &RedmineGoTaskData{Options: &RedmineGoOptions{ConnectionId: 3, ProjectId: 6}}
	assert.Equal(t, models.RedmineGoConnectionParams{ConnectionId: 3}, trackersList.options(data).GetParams())
	assert.Equal(t, models.RedmineGoConnectionParams{ConnectionId: 3}, customFieldsList.options(data).GetParams())
	assert.Equal(t, models.RedmineGoApiParams{ConnectionId: 3, ProjectId: 6}, versionsList.options(data).GetParams())
	assert.Equal(t, models.RedmineGoApiParams{ConnectionId: 3, ProjectId: 6}, categoriesList.options(data).GetParams())
}
