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
	"strconv"
	"strings"
	"time"

	"github.com/apache/devlake/core/errors"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
)

// Redmine's list endpoints return at most this many items per page.
const redminePageSize = 100

// incrementalOverlap widens the lower bound of the incremental window.
// Redmine stamps updated_on when a transaction writes, but the row becomes
// visible only at commit, and the DevLake clock may differ from Redmine's by
// seconds or minutes. Re-collecting the last hour is harmless because
// extraction upserts by id and replaces child rows per issue.
const incrementalOverlap = time.Hour

// changedIssuesSince returns the lower bound for the issues a child collector
// walks, or nil for every issue: the same window the issue collector queried,
// so a child bookmark that lags the issue bookmark (an earlier failed run)
// still covers every issue that was re-collected.
func changedIssuesSince(isIncremental bool, since *time.Time) *time.Time {
	if !isIncremental || since == nil {
		return nil
	}
	adjusted := since.Add(-incrementalOverlap)
	return &adjusted
}

// apiRef is the {id, name} pair Redmine embeds for referenced objects.
type apiRef struct {
	Id   uint64 `json:"id"`
	Name string `json:"name"`
}

// apiStatusRef is an embedded issue status; Redmine 5 and later add is_closed.
type apiStatusRef struct {
	Id       uint64 `json:"id"`
	Name     string `json:"name"`
	IsClosed bool   `json:"is_closed"`
}

// flexString decodes a JSON string, number or boolean into its text, and null
// into "". Journal values are text in Redmine's database but nothing in the
// API contract stops a plugin from emitting a number.
type flexString string

// UnmarshalJSON implements json.Unmarshaler.
func (s *flexString) UnmarshalJSON(data []byte) error {
	text := strings.TrimSpace(string(data))
	if text == "null" {
		*s = ""
		return nil
	}
	if strings.HasPrefix(text, `"`) {
		var decoded string
		if err := json.Unmarshal(data, &decoded); err != nil {
			return err
		}
		*s = flexString(decoded)
		return nil
	}
	*s = flexString(text)
	return nil
}

// parseDate parses a Redmine date ("2026-10-30"); an absent or unparsable
// value is nil.
func parseDate(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil
	}
	return &parsed
}

// parseTime parses a Redmine timestamp; an absent or unparsable value is nil.
func parseTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil
	}
	utc := parsed.UTC()
	return &utc
}

// listResponseParser returns a ResponseParser for Redmine list answers shaped
// `{"<key>": [...], "total_count": n}`: one raw row per item.
func listResponseParser(key string) func(res *http.Response) ([]json.RawMessage, errors.Error) {
	return func(res *http.Response) ([]json.RawMessage, errors.Error) {
		body, err := io.ReadAll(res.Body)
		if err != nil {
			return nil, errors.Convert(err)
		}
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal(body, &envelope); err != nil {
			return nil, errors.Default.Wrap(err, "could not decode the Redmine list response")
		}
		var items []json.RawMessage
		if raw, ok := envelope[key]; ok {
			if err := json.Unmarshal(raw, &items); err != nil {
				return nil, errors.Default.Wrap(err, "could not decode the Redmine "+key+" list")
			}
		}
		return items, nil
	}
}

// totalPagesFromCount reads total_count from a list response and returns the
// number of pages of the fixed page size, so the collector can fetch the
// remaining pages in parallel.
func totalPagesFromCount(res *http.Response, args *helper.ApiCollectorArgs) (int, errors.Error) {
	var page struct {
		TotalCount int `json:"total_count"`
	}
	if err := helper.UnmarshalResponse(res, &page); err != nil {
		return 0, err
	}
	return (page.TotalCount + args.PageSize - 1) / args.PageSize, nil
}

// idString renders an id for use in text columns and journal values.
func idString(id uint64) string {
	return strconv.FormatUint(id, 10)
}
