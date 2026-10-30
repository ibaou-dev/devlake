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

package e2e

import (
	"testing"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/models/common"
	"github.com/apache/devlake/core/models/domainlayer/crossdomain"
	"github.com/apache/devlake/core/models/domainlayer/ticket"
	"github.com/apache/devlake/helpers/e2ehelper"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixturesConfig maps the RGO fixture projects of the stack Redmine: a
// critical fixture is an incident, the rest are tasks; severity, environment
// and component come from the fixture custom fields (ids 12, 13 and 14).
const fixturesConfig = `{
  "typeRules": [
    { "tracker": { "id": 5, "name": "RGO Fixture" },
      "when": { "field": { "id": 12, "name": "RGO Severity" }, "op": "in", "values": [ { "value": "9", "label": "Critical" } ] },
      "type": "INCIDENT" },
    { "tracker": { "id": 5, "name": "RGO Fixture" }, "type": "TASK" }
  ],
  "statusMappings": [ { "status": { "id": 10, "name": "Archived (rgo)" }, "std": "OTHER" } ],
  "severity": [ { "field": { "id": 12, "name": "RGO Severity" }, "map": [ { "value": "9", "label": "Critical", "to": "Critical" } ] } ],
  "environment": [ { "field": { "id": 13, "name": "RGO Environment" },
    "map": [ { "value": "12", "to": "PRODUCTION" }, { "value": "11", "to": "TESTING" }, { "value": "10", "to": "DEVELOPMENT" } ] } ],
  "component": { "source": "customField", "field": { "id": 14, "name": "RGO Components" } }
}`

// verifyDomainTable compares a domain table with a snapshot file.
func verifyDomainTable(tester *e2ehelper.DataFlowTester, table dal.Tabler, file string) {
	tester.VerifyTableWithOptions(table, e2ehelper.TableOptions{
		CSVRelPath:  "./snapshot_tables/" + file + ".csv",
		IgnoreTypes: []interface{}{common.NoPKModel{}},
	})
}

// issueByKey loads the domain issue of a Redmine issue id.
func issueByKey(t *testing.T, tester *e2ehelper.DataFlowTester, key string) ticket.Issue {
	t.Helper()
	var issue ticket.Issue
	require.NoError(t, tester.Db.First(&issue, "issue_key = ?", key).Error, "issue %s", key)
	return issue
}

// The RGO fixtures are a capture of the stack Redmine's fixture projects: 26
// issues in three projects (one of them a subproject) with every journal
// kind the fixtures seed.
func TestIssueExtraction(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")
	extractIssues(tester, 6, 7, 8)

	for _, table := range []dal.Tabler{
		&models.RedmineGoIssue{}, &models.RedmineGoJournal{}, &models.RedmineGoJournalDetail{},
		&models.RedmineGoAccount{}, &models.RedmineGoObservedField{},
	} {
		verifyToolTable(tester, table)
	}

	assert.Equal(t, int64(26), count(t, tester, &models.RedmineGoIssue{}, ""))
	assert.Equal(t, int64(24), count(t, tester, &models.RedmineGoIssue{}, "project_id = ?", 6))
	assert.Equal(t, int64(1), count(t, tester, &models.RedmineGoIssue{}, "project_id = ?", 7), "the subproject's issue is its own")
	assert.Equal(t, int64(1), count(t, tester, &models.RedmineGoIssue{}, "project_id = ?", 8))

	toolIssue := func(id uint64) models.RedmineGoIssue {
		var row models.RedmineGoIssue
		require.NoError(t, tester.Db.First(&row, "connection_id = ? AND id = ?", connectionId, id).Error, "issue %d", id)
		return row
	}
	assert.Equal(t, uint64(155), toolIssue(156).ParentId, "the parent issue is kept")
	assert.True(t, toolIssue(157).IsPrivate)

	issue := toolIssue(152)
	assert.Equal(t, "RGO Frontend", issue.CategoryName)
	assert.Equal(t, "RGO 0.9 (closed)", issue.FixedVersionName)
	assert.Equal(t, "New", issue.StatusName)
	assert.NotZero(t, issue.RedmineCreatedOn)
	assert.NotZero(t, issue.RedmineUpdatedOn)

	// Closed issues keep closed_on, and the custom fields stay verbatim as JSON.
	issue = toolIssue(172)
	assert.True(t, issue.StatusIsClosed)
	require.NotNil(t, issue.RedmineClosedOn)
	assert.JSONEq(t, `[
		{"id":12,"name":"RGO Severity","value":null},
		{"id":13,"name":"RGO Environment","multiple":true,"value":[]},
		{"id":14,"name":"RGO Components","multiple":true,"value":[]},
		{"id":15,"name":"RGO Reviewer","value":null},
		{"id":16,"name":"RGO Target release","value":null},
		{"id":17,"name":"RGO Regulated","value":null}]`, issue.CustomFieldsJson)

	// Users come from the embedded {id, name} objects only; no e-mail address
	// is stored anywhere.
	assert.Positive(t, count(t, tester, &models.RedmineGoAccount{}, ""))
	assert.Equal(t, int64(6), count(t, tester, &models.RedmineGoObservedField{}, ""), "the six fixture fields seen on issues")
}

func TestIssueConversionWithoutScopeConfig(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")
	extractIssues(tester, 6, 7, 8)
	convertIssues(tester, zeroConfig(), 6, 7, 8)

	verifyDomainTable(tester, &ticket.Board{}, "boards")
	verifyDomainTable(tester, &ticket.Issue{}, "issues_zero_config")
	verifyDomainTable(tester, &ticket.BoardIssue{}, "board_issues")
	verifyDomainTable(tester, &ticket.IssueAssignee{}, "issue_assignees")
	verifyDomainTable(tester, &crossdomain.Account{}, "accounts")
	verifyDomainTable(tester, &ticket.IssueChangelogs{}, "issue_changelogs")

	assert.Equal(t, int64(3), count(t, tester, &ticket.Board{}, ""), "one board per converted scope")
	assert.Equal(t, int64(26), count(t, tester, &ticket.Issue{}, ""), "every issue converts, once")
	assert.Equal(t, int64(26), count(t, tester, &ticket.BoardIssue{}, ""))
	assert.Zero(t, count(t, tester, &ticket.IssueCustomArrayField{}, ""), "no environment mapping, no class rows")

	// The fixtures' tracker has no English default name, so it falls back to
	// its upper-cased name; statuses fall back to is_closed and the trackers'
	// default status.
	assert.Equal(t, int64(26), count(t, tester, &ticket.Issue{}, "type = ? AND original_type = ?", "RGO FIXTURE", "RGO Fixture"))
	var statuses []struct {
		Status         string
		OriginalStatus string
	}
	require.NoError(t, tester.Db.Model(&ticket.Issue{}).Select("status, original_status").Group("status, original_status").Scan(&statuses).Error)
	got := map[string]string{}
	for _, row := range statuses {
		got[row.OriginalStatus] = row.Status
	}
	assert.Equal(t, "TODO", got["New"])
	assert.Equal(t, "DONE", got["Closed"])
	assert.Equal(t, "DONE", got["Archived (rgo)"], "a closed status is DONE")
	// An open status that no tracker starts in (IN_PROGRESS) is covered by the Lane 5 replay.

	// URLs, key, scope, parent and dates.
	issue := issueByKey(t, tester, "156")
	assert.Equal(t, "https://redmine.example.org/issues/156", issue.Url)
	assert.Equal(t, "rgo-fixtures", issue.OriginalProject)
	assert.Equal(t, "redmine_go:RedmineGoIssue:1:155", issue.ParentIssueId)
	assert.Equal(t, "rgo-fixtures-sub", issueByKey(t, tester, "154").OriginalProject, "a subproject issue belongs to the subproject")
	assert.Equal(t, "RGO Frontend", issueByKey(t, tester, "152").Component, "the default component is the category")
	assert.Equal(t, "RGO 0.9 (closed)", issueByKey(t, tester, "152").FixVersions)

	// A closed issue has a resolution date and a lead time.
	closed := issueByKey(t, tester, "172")
	assert.Equal(t, "DONE", closed.Status)
	require.NotNil(t, closed.ResolutionDate)
	require.NotNil(t, closed.LeadTimeMinutes)
	// An open one has neither.
	open := issueByKey(t, tester, "155")
	assert.Nil(t, open.ResolutionDate)
	assert.Nil(t, open.LeadTimeMinutes)

	// No e-mail address reaches the domain layer.
	assert.Zero(t, count(t, tester, &crossdomain.Account{}, "email <> ''"))
	assert.Positive(t, count(t, tester, &crossdomain.Account{}, "full_name <> ''"))
}

func TestIssueConversionWithMappingConfig(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")
	extractIssues(tester, 6, 7, 8)
	mapping := transformationFrom(t, fixturesConfig)
	convertIssues(tester, mapping, 6, 7, 8)

	verifyDomainTable(tester, &ticket.Issue{}, "issues_mapped")
	verifyDomainTable(tester, &ticket.IssueCustomArrayField{}, "issue_custom_array_fields")
	verifyDomainTable(tester, &ticket.IssueChangelogs{}, "issue_changelogs_mapped")

	// Issue 174 has severity Critical, environments Test and Prod and the
	// component Reports: an incident in PRODUCTION and TESTING.
	issue := issueByKey(t, tester, "174")
	assert.Equal(t, ticket.INCIDENT, issue.Type)
	assert.Equal(t, "Critical", issue.Severity)
	assert.Equal(t, "UI,Reports", issue.Component, "all values of a multi-value field, joined")
	var classes []string
	require.NoError(t, tester.Db.Model(&ticket.IssueCustomArrayField{}).
		Where("issue_id = ? AND field_id = ?", issue.Id, "environment_class").Order("field_value").Pluck("field_value", &classes).Error)
	assert.Equal(t, []string{"PRODUCTION", "TESTING"}, classes)

	// The other fixtures are tasks, with no severity.
	assert.Equal(t, ticket.TASK, issueByKey(t, tester, "155").Type)
	assert.Empty(t, issueByKey(t, tester, "155").Severity)
	assert.Equal(t, int64(1), count(t, tester, &ticket.Issue{}, "type = ?", ticket.INCIDENT))
	assert.Equal(t, int64(25), count(t, tester, &ticket.Issue{}, "type = ?", ticket.TASK))

	// The archived status is mapped explicitly, overriding is_closed.
	assert.Equal(t, ticket.OTHER, issueByKey(t, tester, "173").Status)
}

// Domain rows that no longer have a source disappear even when the run emits
// no row of their table: the divider replaces a table only once it sees a row.
func TestConversionReplacesStaleDomainRows(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")
	extractIssues(tester, 6, 7, 8)

	convertIssues(tester, transformationFrom(t, fixturesConfig), 6, 7, 8)
	require.Positive(t, count(t, tester, &ticket.IssueCustomArrayField{}, ""))
	require.Positive(t, count(t, tester, &ticket.IssueAssignee{}, ""))
	changelogs := count(t, tester, &ticket.IssueChangelogs{}, "")
	require.Positive(t, changelogs)

	// The mapping loses its environment rule, and nobody is assigned any more.
	require.NoError(t, tester.Db.Model(&models.RedmineGoIssue{}).Where("1 = 1").Updates(map[string]interface{}{
		"assignee_id": 0, "assignee_name": "",
	}).Error)
	convertIssues(tester, zeroConfig(), 6, 7, 8)
	assert.Zero(t, count(t, tester, &ticket.IssueCustomArrayField{}, ""), "environment classes follow the mapping")
	assert.Zero(t, count(t, tester, &ticket.IssueAssignee{}, ""), "assignees follow the issues")
	assert.Equal(t, int64(26), count(t, tester, &ticket.Issue{}, ""))
	assert.Equal(t, changelogs, count(t, tester, &ticket.IssueChangelogs{}, ""), "re-conversion neither duplicates nor drops changelogs")

	// Every journal is gone: so are the changelogs.
	require.NoError(t, tester.Db.Where("1 = 1").Delete(&models.RedmineGoJournalDetail{}).Error)
	convertIssues(tester, zeroConfig(), 6, 7, 8)
	assert.Zero(t, count(t, tester, &ticket.IssueChangelogs{}, ""))
}
