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
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func testLookup() *journalLookup {
	return &journalLookup{
		Trackers:   map[uint64]string{1: "Bug", 2: "Feature"},
		Statuses:   map[uint64]string{1: "New", 5: "Closed"},
		Priorities: map[uint64]string{2: "Normal", 4: "Urgent"},
		Versions:   map[uint64]string{21: "RGO 1.0", 22: "RGO 2.0"},
		Categories: map[uint64]string{3: "Backend"},
		Projects:   map[uint64]string{6: "RGO Fixtures"},
		Accounts:   map[uint64]string{15: "Fixture A", 16: "Fixture B"},
		Fields: map[uint64]fieldMeta{
			12: {Name: "RGO Severity", Format: "enumeration", Labels: map[string]string{"7": "Minor", "9": "Critical"}},
			13: {Name: "RGO Environment", Format: "enumeration", Labels: map[string]string{"10": "Dev", "12": "Prod"}},
			14: {Name: "RGO Components", Format: "list", Labels: map[string]string{"API": "API"}},
			15: {Name: "RGO Reviewer", Format: "user"},
			16: {Name: "RGO Target release", Format: "version"},
			20: {Name: "Seen only on issues"},
		},
	}
}

func TestTranslateEveryPropertyKind(t *testing.T) {
	lk := testLookup()
	for _, tc := range []struct {
		name   string
		change journalChange
		want   translatedChange
	}{
		{"status", journalChange{"attr", "status_id", "1", "5"}, translatedChange{"status", "New", "Closed"}},
		{"status unknown id keeps the raw id", journalChange{"attr", "status_id", "1", "99"}, translatedChange{"status", "New", "99"}},
		{"tracker", journalChange{"attr", "tracker_id", "1", "2"}, translatedChange{"tracker", "Bug", "Feature"}},
		{"priority", journalChange{"attr", "priority_id", "2", "4"}, translatedChange{"priority", "Normal", "Urgent"}},
		{"assignee set from nobody", journalChange{"attr", "assigned_to_id", "", "15"}, translatedChange{"assignee", "", "Fixture A"}},
		{"assignee unknown user keeps the id", journalChange{"attr", "assigned_to_id", "15", "77"}, translatedChange{"assignee", "Fixture A", "77"}},
		{"fix version", journalChange{"attr", "fixed_version_id", "21", "22"}, translatedChange{"fix_version", "RGO 1.0", "RGO 2.0"}},
		{"fix version removed", journalChange{"attr", "fixed_version_id", "21", ""}, translatedChange{"fix_version", "RGO 1.0", ""}},
		{"project move", journalChange{"attr", "project_id", "6", "8"}, translatedChange{"project", "RGO Fixtures", "8"}},
		{"category", journalChange{"attr", "category_id", "", "3"}, translatedChange{"category", "", "Backend"}},
		{"parent is an issue reference", journalChange{"attr", "parent_id", "", "155"}, translatedChange{"parent", "", "#155"}},
		{"child is an issue reference", journalChange{"attr", "child_id", "156", ""}, translatedChange{"child", "#156", ""}},
		{"estimated hours stay raw", journalChange{"attr", "estimated_hours", "1.5", "3.0"}, translatedChange{"estimated_hours", "1.5", "3.0"}},
		{"subject is the summary", journalChange{"attr", "subject", "old", "new"}, translatedChange{"summary", "old", "new"}},
		{"unlisted attribute loses _id", journalChange{"attr", "reviewer_id", "1", "2"}, translatedChange{"reviewer", "1", "2"}},
		{"enumeration custom field", journalChange{"cf", "12", "7", "9"}, translatedChange{"RGO Severity", "Minor", "Critical"}},
		{"multi-value row: one value added", journalChange{"cf", "13", "", "12"}, translatedChange{"RGO Environment", "", "Prod"}},
		{"multi-value row: one value removed", journalChange{"cf", "13", "10", ""}, translatedChange{"RGO Environment", "Dev", ""}},
		{"list custom field", journalChange{"cf", "14", "API", ""}, translatedChange{"RGO Components", "API", ""}},
		{"user custom field resolves through accounts", journalChange{"cf", "15", "15", "16"}, translatedChange{"RGO Reviewer", "Fixture A", "Fixture B"}},
		{"version custom field resolves through versions", journalChange{"cf", "16", "21", "22"}, translatedChange{"RGO Target release", "RGO 1.0", "RGO 2.0"}},
		{"enumeration value without a label keeps the id", journalChange{"cf", "12", "7", "8"}, translatedChange{"RGO Severity", "Minor", "8"}},
		{"field known only from issues keeps raw values", journalChange{"cf", "20", "a", "b"}, translatedChange{"Seen only on issues", "a", "b"}},
		{"unknown custom field is named by its id", journalChange{"cf", "99", "x", "y"}, translatedChange{"cf_99", "x", "y"}},
		{"non-numeric custom field name", journalChange{"cf", "oops", "x", "y"}, translatedChange{"cf_oops", "x", "y"}},
		{"relation added", journalChange{"relation", "blocks", "", "163"}, translatedChange{"relation_blocks", "", "#163"}},
		{"relation removed", journalChange{"relation", "duplicated", "160", ""}, translatedChange{"relation_duplicated", "#160", ""}},
		{"attachment", journalChange{"attachment", "5", "", "spec.pdf"}, translatedChange{"attachment", "", "spec.pdf"}},
		{"unknown property is named by its name", journalChange{"plugin", "score", "1", "2"}, translatedChange{"score", "1", "2"}},
		{"unknown property without a name", journalChange{"plugin", "", "1", "2"}, translatedChange{"plugin", "1", "2"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, lk.translate(tc.change))
		})
	}
}

func TestTranslateWithoutAnyMetadataKeepsRawIds(t *testing.T) {
	lk := &journalLookup{}
	assert.Equal(t, translatedChange{"status", "1", "5"}, lk.translate(journalChange{"attr", "status_id", "1", "5"}))
	assert.Equal(t, translatedChange{"cf_12", "7", "9"}, lk.translate(journalChange{"cf", "12", "7", "9"}))
}

func TestDetailKeysAreStableAndDistinct(t *testing.T) {
	multiValue := []journalChange{
		{"cf", "13", "10", ""},
		{"cf", "13", "", "11"},
		{"cf", "13", "", "12"},
		{"attr", "status_id", "1", "2"},
	}
	keys := detailKeys(multiValue)
	assert.Len(t, uniqueStrings(keys), len(keys), "rows of one multi-value field keep distinct keys")

	// The keys follow the content, not Redmine's listing order.
	reversed := []journalChange{multiValue[3], multiValue[2], multiValue[1], multiValue[0]}
	reversedKeys := detailKeys(reversed)
	for i := range multiValue {
		assert.Equal(t, keys[i], reversedKeys[len(multiValue)-1-i], "change %d", i)
	}
	assert.Equal(t, keys, detailKeys(multiValue), "repeatable")

	// Identical changes are told apart by their occurrence.
	twins := detailKeys([]journalChange{{"cf", "13", "", "11"}, {"cf", "13", "", "11"}})
	assert.NotEqual(t, twins[0], twins[1])

	// Any difference in content changes the key.
	a := detailKeys([]journalChange{{"attr", "status_id", "1", "2"}})[0]
	b := detailKeys([]journalChange{{"attr", "status_id", "1", "3"}})[0]
	assert.NotEqual(t, a, b)
	assert.Regexp(t, `^[0-9a-f]{16}-0$`, a)
}

func uniqueStrings(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func TestJournalRowsKeepNotesPrivacyAndAuthors(t *testing.T) {
	var item apiIssue
	mustUnmarshal(t, `{"id": 175, "journals": [
		{"id": 10, "user": {"id": 1, "name": "Redmine Admin"}, "notes": "public note", "created_on": "2026-10-30T00:40:46Z",
		 "updated_on": "2026-10-30T01:00:00Z", "private_notes": false,
		 "details": [{"property": "attr", "name": "status_id", "old_value": "1", "new_value": "5"}]},
		{"id": 11, "user": {"id": 15, "name": "Fixture A"}, "notes": "secret", "created_on": "2026-10-30T02:00:00Z",
		 "private_notes": true, "details": []}
	]}`, &item)

	rows := journalRows(1, &item, testLookup())
	var journals, details, accounts int
	for _, row := range rows {
		switch r := row.(type) {
		case interface{ TableName() string }:
			switch r.TableName() {
			case "_tool_redmine_go_journals":
				journals++
			case "_tool_redmine_go_journal_details":
				details++
			case "_tool_redmine_go_accounts":
				accounts++
			}
		}
	}
	assert.Equal(t, 2, journals, "a journal without details still keeps its note")
	assert.Equal(t, 1, details)
	assert.Equal(t, 2, accounts, "journal authors become accounts")
	assert.Equal(t, fmt.Sprintf("%T", rows[0]), "*models.RedmineGoJournal")
}
