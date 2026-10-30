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

	"github.com/apache/devlake/core/models/domainlayer/crossdomain"
	"github.com/apache/devlake/core/models/domainlayer/ticket"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stackConfig is the stack Redmine's reference configuration of
// redmine-go-scope-config.md: Support tickets are incidents, five statuses
// are mapped, and the component is the issue category.
const stackConfig = `{
  "typeRules": [
    { "tracker": { "id": 1, "name": "Bug" }, "type": "BUG" },
    { "tracker": { "id": 2, "name": "Feature" }, "type": "REQUIREMENT" },
    { "tracker": { "id": 3, "name": "Support" }, "type": "INCIDENT" }
  ],
  "statusMappings": [
    { "status": { "id": 2, "name": "In Progress" }, "std": "IN_PROGRESS" },
    { "status": { "id": 3, "name": "Resolved" }, "std": "IN_PROGRESS" },
    { "status": { "id": 4, "name": "Feedback" }, "std": "IN_PROGRESS" },
    { "status": { "id": 7, "name": "Awaiting client" }, "std": "OTHER" },
    { "status": { "id": 8, "name": "Rejected by QA" }, "std": "IN_PROGRESS" }
  ],
  "component": { "source": "category" }
}`

// lane5StatusChanges is the number of status changes of the Lane 5 projects
// (Citizen Portal and Business Registry): the count of dm.journal_detail rows
// with property attr and name status_id, the PoC's own SQL view over the
// Python plugin's raw journals, on 2026-10-30.
const lane5StatusChanges = 481

// Lane 5 is the PoC's seeded Redmine data: 124 issues of the default trackers
// in two projects, 481 status changes, relations, parent and child links, and
// version and custom-field changes. Replaying a capture of it checks the
// plugin against a data set that did not come from its own fixtures.
func TestLane5Replay(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "lane5/")
	extractIssues(tester, 1, 2)

	assert.Equal(t, int64(65), count(t, tester, &models.RedmineGoIssue{}, "project_id = ?", 1))
	assert.Equal(t, int64(59), count(t, tester, &models.RedmineGoIssue{}, "project_id = ?", 2))
	assert.Equal(t, int64(lane5StatusChanges), count(t, tester, &models.RedmineGoJournalDetail{}, "property = ? AND name = ?", "attr", "status_id"),
		"as many status changes as the PoC's own journal view")
	assertNoDuplicateDetails(t, tester)

	// Out of the box: Redmine's default trackers map to the standard types,
	// statuses by is_closed and the trackers' default status.
	convertIssues(tester, zeroConfig(), 1, 2)
	verifyDomainTable(tester, &ticket.Issue{}, "issues_lane5")
	verifyDomainTable(tester, &ticket.IssueChangelogs{}, "issue_changelogs_lane5")

	typeCount := func(original, std string) int64 {
		return count(t, tester, &ticket.Issue{}, "original_type = ? AND type = ?", original, std)
	}
	assert.Equal(t, int64(35), typeCount("Bug", ticket.BUG))
	assert.Equal(t, int64(64), typeCount("Feature", ticket.REQUIREMENT))
	assert.Equal(t, int64(25), typeCount("Support", ticket.TASK), "a support request is a task, not an incident")
	assert.Zero(t, count(t, tester, &ticket.Issue{}, "type = ?", ticket.INCIDENT))
	assert.Equal(t, int64(124), count(t, tester, &ticket.Issue{}, ""))

	statusCount := func(original, std string) int64 {
		return count(t, tester, &ticket.Issue{}, "original_status = ? AND status = ?", original, std)
	}
	assert.Equal(t, int64(4), statusCount("New", ticket.TODO), "the trackers' default status is TODO")
	assert.Equal(t, int64(1), statusCount("In Progress", ticket.IN_PROGRESS))
	assert.Equal(t, int64(6), statusCount("Resolved", ticket.IN_PROGRESS))
	assert.Equal(t, int64(1), statusCount("Rejected by QA", ticket.IN_PROGRESS))
	assert.Equal(t, int64(112), statusCount("Closed", ticket.DONE), "a closed status is DONE")

	// The status history is complete: the same number of rows as the PoC, with
	// the standard status on each side and the status names as originals.
	statusRows := count(t, tester, &ticket.IssueChangelogs{}, "field_name = ?", "status")
	assert.Equal(t, int64(lane5StatusChanges), statusRows)
	assert.Zero(t, count(t, tester, &ticket.IssueChangelogs{}, "field_name = ? AND to_value NOT IN (?, ?, ?, ?)", "status", ticket.TODO, ticket.IN_PROGRESS, ticket.DONE, ticket.OTHER))
	assert.Zero(t, count(t, tester, &ticket.IssueChangelogs{}, "field_name = ? AND original_to_value = ''", "status"))
	assert.Positive(t, count(t, tester, &ticket.IssueChangelogs{}, "field_name = ? AND from_value = ? AND to_value = ?", "status", ticket.TODO, ticket.DONE))
	// Each closed issue has a resolution date, and a lead time when it
	// resolved after it was created.
	assert.Zero(t, count(t, tester, &ticket.Issue{}, "status = ? AND resolution_date IS NULL", ticket.DONE))

	// Relations, parent and child links, version and custom-field changes are
	// all kept as changelog rows.
	for field, minimum := range map[string]int64{
		"fix_version": 95, "Rejection reason": 53, "relation_blocks": 4, "relation_blocked": 4, "parent": 2, "child": 2,
	} {
		assert.GreaterOrEqual(t, count(t, tester, &ticket.IssueChangelogs{}, "field_name = ?", field), minimum, field)
	}
	assert.Equal(t, int64(0), count(t, tester, &crossdomain.Account{}, "email <> ''"))

	// A different mapping changes the output without touching the tool layer:
	// with the stack reference configuration, support tickets are incidents.
	toolIssues := count(t, tester, &models.RedmineGoIssue{}, "")
	toolDetails := count(t, tester, &models.RedmineGoJournalDetail{}, "")
	convertIssues(tester, transformationFrom(t, stackConfig), 1, 2)
	assert.Equal(t, int64(25), typeCount("Support", ticket.INCIDENT))
	assert.Zero(t, typeCount("Support", ticket.TASK))
	assert.Equal(t, int64(35), typeCount("Bug", ticket.BUG))
	assert.Equal(t, int64(124), count(t, tester, &ticket.Issue{}, ""), "re-conversion neither duplicates nor drops issues")
	assert.Equal(t, int64(lane5StatusChanges), count(t, tester, &ticket.IssueChangelogs{}, "field_name = ?", "status"))
	assert.Equal(t, toolIssues, count(t, tester, &models.RedmineGoIssue{}, ""))
	assert.Equal(t, toolDetails, count(t, tester, &models.RedmineGoJournalDetail{}, ""))
	require.Equal(t, int64(1), statusCount("In Progress", ticket.IN_PROGRESS))
}
