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
	"time"

	"github.com/apache/devlake/core/models/domainlayer/ticket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Issue 172 of the fixtures is the reopen cycle: New, In Progress, Closed,
// In Progress again, Closed again, one second apart. Issue 175 is In Progress,
// open in Redmine, after a change into In Progress at 17:12:11.
func TestResolutionDateFollowsTheDoneMapping(t *testing.T) {
	tester := newDataFlowTester(t)
	importMetadata(t, tester)
	loadIssueRaw(t, tester, "")
	extractIssues(tester, 6, 7, 8)
	at := func(clock string) time.Time {
		parsed, err := time.Parse(time.RFC3339, "2026-10-30T"+clock+"Z")
		require.NoError(t, err)
		return parsed
	}

	// By default only closed statuses are DONE. The reopened issue resolves
	// when it was closed again, and an open issue has no resolution.
	convertIssues(tester, zeroConfig(), 6)
	closed := issueByKey(t, tester, "172")
	require.NotNil(t, closed.ResolutionDate)
	assert.True(t, at("00:40:49").Equal(*closed.ResolutionDate), "the latest closing, got %s", closed.ResolutionDate)
	assert.Nil(t, issueByKey(t, tester, "175").ResolutionDate)
	assert.Nil(t, issueByKey(t, tester, "175").LeadTimeMinutes)
	archived := issueByKey(t, tester, "173")
	require.NotNil(t, archived.ResolutionDate, "a closed status DONE with no closed_on resolves by its status change")
	assert.True(t, at("00:40:50").Equal(*archived.ResolutionDate))

	// Mapping In Progress to DONE changes both issues, and nothing else is
	// touched: the open status resolves from its own history.
	mapped := transformationFrom(t, `{"statusMappings":[{"status":{"id":2,"name":"In Progress"},"std":"DONE"}]}`)
	convertIssues(tester, mapped, 6)

	cycle := issueByKey(t, tester, "172")
	assert.Equal(t, ticket.DONE, cycle.Status)
	require.NotNil(t, cycle.ResolutionDate)
	assert.True(t, at("00:40:46").Equal(*cycle.ResolutionDate),
		"In Progress and Closed are both DONE, so the run began at the first change into In Progress, got %s", cycle.ResolutionDate)

	progress := issueByKey(t, tester, "175")
	assert.Equal(t, ticket.DONE, progress.Status, "an open status mapped to DONE")
	require.NotNil(t, progress.ResolutionDate)
	assert.True(t, at("17:12:11").Equal(*progress.ResolutionDate), "got %s", progress.ResolutionDate)
	require.NotNil(t, progress.LeadTimeMinutes, "and has a lead time")
	assert.EqualValues(t, 991, *progress.LeadTimeMinutes, "from 00:40:50 to 17:12:11")

	// An issue that never changed status stays open under either mapping.
	assert.Nil(t, issueByKey(t, tester, "155").ResolutionDate)
}
