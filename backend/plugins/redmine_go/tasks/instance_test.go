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
	"testing"

	"github.com/apache/devlake/core/models/common"
	"github.com/apache/devlake/impls/logruslog"
	"github.com/apache/devlake/plugins/redmine_go/mapping"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWarnUnresolvedFieldsLogsOncePerField(t *testing.T) {
	var out bytes.Buffer
	base := logrus.New()
	base.SetOutput(&out)
	logger, err := logruslog.NewDefaultLogger(base)
	require.NoError(t, err)

	cond := func(id int, name string) *models.Condition {
		return &models.Condition{Field: models.Ref{ID: id, Name: name}, Op: models.ConditionIn, Values: []models.Value{{Value: "x"}}}
	}
	scopeConfig := &models.RedmineGoScopeConfig{
		ScopeConfig: common.ScopeConfig{Model: common.Model{ID: 7}, Name: "stack-example"},
		Transformation: models.Transformation{
			TypeRules: []models.TypeRule{
				{When: cond(90, "Gone"), Type: "INCIDENT"},
				{When: cond(90, "Gone"), Type: "BUG"},
				{When: cond(21, "Env"), Type: "TASK"},
			},
			Severity: []models.ValueMapField{{Field: models.Ref{ID: 91, Name: "Vanished"}}},
		},
	}
	mapper := mapping.NewMapper(&scopeConfig.Transformation, mapping.Instance{Fields: []mapping.FieldInfo{{ID: 21, Name: "Env"}}})

	warnUnresolvedFields(logger, scopeConfig, mapper)

	assert.Equal(t, 2, bytes.Count(out.Bytes(), []byte("level=warning")), "one warning per unresolvable field")
	for _, want := range []string{`scope config \"stack-example\" (id 7)`, `custom field \"Gone\" (id 90)`, `custom field \"Vanished\" (id 91)`} {
		assert.Contains(t, out.String(), want)
	}
	assert.NotContains(t, out.String(), `\"Env\"`)

	out.Reset()
	warnUnresolvedFields(logger, &models.RedmineGoScopeConfig{}, mapping.NewMapper(nil, mapping.Instance{}))
	assert.Empty(t, out.String(), "no configured field, no warning")
}
