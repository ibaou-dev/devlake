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
	"testing"
	"time"

	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	mockdal "github.com/apache/devlake/mocks/core/dal"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// sliceStream serves details from memory and counts how far it was read.
type sliceStream struct {
	rows []*models.RedmineGoJournalDetail
	read int
	err  errors.Error
}

func (s *sliceStream) next() (*models.RedmineGoJournalDetail, errors.Error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.read == len(s.rows) {
		return nil, nil
	}
	s.read++
	return s.rows[s.read-1], nil
}

func statusDetail(issue uint64, hour int, from, to string) *models.RedmineGoJournalDetail {
	return &models.RedmineGoJournalDetail{
		IssueId: issue, OldValue: from, NewValue: to, OldLabel: "from " + from, NewLabel: "to " + to,
		RedmineCreatedOn: time.Date(2026, 1, 1, hour, 0, 0, 0, time.UTC),
	}
}

func TestStatusHistoryServesEachIssueItsOwnChangesInOrder(t *testing.T) {
	stream := &sliceStream{rows: []*models.RedmineGoJournalDetail{
		statusDetail(1, 1, "1", "2"),
		statusDetail(3, 1, "1", "2"),
		statusDetail(3, 5, "2", "5"),
		statusDetail(7, 2, "1", "5"),
	}}
	history := &statusHistory{stream: stream}

	none, err := history.changesOf(0)
	require.NoError(t, err)
	assert.Empty(t, none, "an issue below the first change has no history")
	assert.Equal(t, 1, stream.read, "only one row is read ahead")

	// Issue 1 is skipped by the caller (not DONE): its change is dropped.
	three, err := history.changesOf(3)
	require.NoError(t, err)
	require.Len(t, three, 2)
	assert.Equal(t, 1, three[0].FromStatusID)
	assert.Equal(t, 2, three[0].ToStatusID)
	assert.Equal(t, "to 5", three[1].ToStatusName)
	assert.Equal(t, 5, three[1].ToStatusID)
	assert.True(t, three[0].At.Before(three[1].At))

	four, err := history.changesOf(4)
	require.NoError(t, err)
	assert.Empty(t, four)

	seven, err := history.changesOf(7)
	require.NoError(t, err)
	require.Len(t, seven, 1)

	late, err := history.changesOf(9)
	require.NoError(t, err)
	assert.Empty(t, late)
	again, err := history.changesOf(10)
	require.NoError(t, err)
	assert.Empty(t, again, "an exhausted stream stays empty")
	assert.Equal(t, 4, stream.read, "every row is read exactly once")
}

func TestStatusHistoryKeepsWhatIsKnownOfUnreadableIds(t *testing.T) {
	history := &statusHistory{stream: &sliceStream{rows: []*models.RedmineGoJournalDetail{statusDetail(2, 1, "", "oops")}}}
	changes, err := history.changesOf(2)
	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Zero(t, changes[0].FromStatusID)
	assert.Zero(t, changes[0].ToStatusID)
	assert.Equal(t, "to oops", changes[0].ToStatusName)
}

func TestStatusHistoryReportsAStreamFailure(t *testing.T) {
	history := &statusHistory{stream: &sliceStream{err: errors.Default.New("boom")}}
	_, err := history.changesOf(1)
	assert.Error(t, err)
}

// recordingContext is a subtask context that records the progress it is given.
type recordingContext struct {
	plugin.SubTaskContext
	current    int
	total      int
	progressed bool
}

func (c *recordingContext) SetProgress(current, total int) {
	c.current, c.total, c.progressed = current, total, true
}
func (c *recordingContext) GetName() string { return "subtask" }

func TestWithTotalReplacesTheUnknownTotal(t *testing.T) {
	inner := &recordingContext{}
	wrapped := withTotal(inner, 120)
	wrapped.SetProgress(0, -1)
	assert.True(t, inner.progressed)
	assert.Equal(t, 120, inner.total)

	untouched := withTotal(inner, 0)
	assert.Same(t, plugin.SubTaskContext(inner), untouched, "nothing to throttle without rows")
}

func TestDeleteIssueRowsInBatchesCoversEveryIssueOncePerTable(t *testing.T) {
	ids := make([]uint64, deleteBatchSize*2+1)
	for i := range ids {
		ids[i] = uint64(i + 1)
	}
	db := new(mockdal.Dal)
	db.On("Delete", mock.Anything, mock.Anything).Return(nil)
	require.NoError(t, deleteIssueRowsInBatches(db, []interface{}{&models.RedmineGoJournalDetail{}, &models.RedmineGoJournal{}}, 1, ids))
	db.AssertNumberOfCalls(t, "Delete", 6)

	db = new(mockdal.Dal)
	require.NoError(t, deleteIssueRowsInBatches(db, []interface{}{&models.RedmineGoJournal{}}, 1, nil))
	db.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}
