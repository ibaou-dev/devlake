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
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustUnmarshal(t *testing.T, text string, target interface{}) {
	t.Helper()
	require.NoError(t, json.Unmarshal([]byte(text), target))
}

func response(body string) *http.Response {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}
}

func TestFlexString(t *testing.T) {
	var detail apiJournalDetail
	for body, want := range map[string]string{
		`{"old_value": null, "new_value": null}`:            "",
		`{"old_value": "7", "new_value": "text"}`:           "7",
		`{"old_value": 12, "new_value": 1.5}`:               "12",
		`{"old_value": true, "new_value": false}`:           "true",
		`{"old_value": "with \"quotes\"", "new_value": ""}`: `with "quotes"`,
	} {
		detail = apiJournalDetail{}
		mustUnmarshal(t, body, &detail)
		assert.Equal(t, want, string(detail.OldValue), body)
	}
}

func TestParseDateAndTime(t *testing.T) {
	assert.Nil(t, parseDate(""))
	assert.Nil(t, parseDate("not a date"))
	assert.Equal(t, time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC), *parseDate("2026-10-30"))
	assert.Nil(t, parseTime(""))
	assert.Nil(t, parseTime("yesterday"))
	assert.Equal(t, time.Date(2026, 10, 30, 0, 40, 36, 0, time.UTC), *parseTime("2026-10-30T00:40:36Z"))
	assert.Equal(t, time.Date(2026, 10, 30, 0, 40, 36, 0, time.UTC), *parseTime("2026-10-30T02:40:36+02:00"), "normalised to UTC")
}

func TestListResponseParserReturnsOneRowPerItem(t *testing.T) {
	parser := listResponseParser("issues")
	items, err := parser(response(`{"issues": [{"id": 1}, {"id": 2}], "total_count": 2}`))
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.JSONEq(t, `{"id": 2}`, string(items[1]))

	items, err = parser(response(`{"issues": [], "total_count": 0}`))
	require.NoError(t, err)
	assert.Empty(t, items)

	items, err = parser(response(`{"total_count": 0}`))
	require.NoError(t, err)
	assert.Empty(t, items, "an absent list is an empty one")

	_, err = parser(response(`not json`))
	assert.Error(t, err)
	_, err = parser(response(`{"issues": "nope"}`))
	assert.Error(t, err)
}

func TestTotalPagesFromCount(t *testing.T) {
	args := &helper.ApiCollectorArgs{PageSize: 100}
	for count, want := range map[int]int{0: 0, 1: 1, 100: 1, 101: 2, 250: 3, 300: 3} {
		pages, err := totalPagesFromCount(response(`{"total_count": `+itoa(count)+`}`), args)
		require.NoError(t, err)
		assert.Equal(t, want, pages, "total_count %d", count)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestChangedIssuesSince(t *testing.T) {
	since := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	assert.Nil(t, changedIssuesSince(false, &since), "a full run walks every issue")
	assert.Nil(t, changedIssuesSince(true, nil), "no bookmark walks every issue")
	got := changedIssuesSince(true, &since)
	require.NotNil(t, got)
	assert.Equal(t, since.Add(-incrementalOverlap), *got)
}

// The issue list asks for exactly what the plan specifies: the project alone
// (subproject_id=!*), every status, a stable id order, 100 per page, and an
// explicit-UTC lower bound.
func TestIssueQuery(t *testing.T) {
	since := time.Date(2026, 10, 30, 11, 0, 0, 0, time.FixedZone("x", 3600))
	query := issueQuery(6, &since, 200, 100)
	assert.Equal(t, "6", query.Get("project_id"))
	assert.Equal(t, "!*", query.Get("subproject_id"))
	assert.Equal(t, "*", query.Get("status_id"))
	assert.Equal(t, "id", query.Get("sort"))
	assert.Equal(t, "100", query.Get("limit"))
	assert.Equal(t, "200", query.Get("offset"))
	assert.Equal(t, ">=2026-10-30T10:00:00Z", query.Get("updated_on"))
	assert.Contains(t, query.Encode(), "subproject_id=%21%2A")

	assert.Empty(t, issueQuery(6, nil, 0, 100).Get("updated_on"), "no bound without a bookmark")
}

func TestParseIssueDetailUnwrapsTheIssue(t *testing.T) {
	items, err := parseIssueDetail(response(`{"issue": {"id": 172, "journals": []}}`))
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.JSONEq(t, `{"id": 172, "journals": []}`, string(items[0]))

	items, err = parseIssueDetail(response(`{}`))
	require.NoError(t, err)
	assert.Empty(t, items)

	_, err = parseIssueDetail(response(`<html>`))
	assert.Error(t, err)
}
