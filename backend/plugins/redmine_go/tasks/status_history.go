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
	"strconv"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/plugins/redmine_go/mapping"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// detailStream yields journal details one at a time, nil once exhausted.
type detailStream interface {
	next() (*models.RedmineGoJournalDetail, errors.Error)
}

// statusHistory pairs issues with their status changes from a single pass
// over the changes, so the issue convertor needs no query per issue. The
// stream is ordered by issue id, oldest change first, and callers ask for
// issues in ascending id order: the stream is only ever read forward, in
// memory proportional to one issue's history.
type statusHistory struct {
	stream  detailStream
	pending *models.RedmineGoJournalDetail
	done    bool
}

// changesOf returns the status transitions of the issue, oldest first, and
// moves the stream past them. A change whose status ids are unreadable keeps
// what is known: an empty side stands for no status. Changes of issues the
// caller skipped, and of issues below the requested id, are dropped; an id
// lower than the one an earlier call asked for finds nothing.
func (h *statusHistory) changesOf(issueId uint64) ([]mapping.StatusChange, errors.Error) {
	var changes []mapping.StatusChange
	for {
		if h.pending == nil {
			if h.done {
				return changes, nil
			}
			detail, err := h.stream.next()
			if err != nil {
				return nil, err
			}
			if detail == nil {
				h.done = true
				return changes, nil
			}
			h.pending = detail
		}
		switch {
		case h.pending.IssueId < issueId:
			h.pending = nil
		case h.pending.IssueId == issueId:
			changes = append(changes, statusChangeOf(h.pending))
			h.pending = nil
		default:
			return changes, nil
		}
	}
}

// statusChangeOf reads a status_id journal detail as a transition.
func statusChangeOf(detail *models.RedmineGoJournalDetail) mapping.StatusChange {
	change := mapping.StatusChange{At: detail.RedmineCreatedOn, FromStatusName: detail.OldLabel, ToStatusName: detail.NewLabel}
	if id, err := strconv.Atoi(detail.OldValue); err == nil {
		change.FromStatusID = id
	}
	if id, err := strconv.Atoi(detail.NewValue); err == nil {
		change.ToStatusID = id
	}
	return change
}

// cursorStream reads journal details from a database cursor.
type cursorStream struct {
	db     dal.Dal
	cursor dal.Rows
}

func (s *cursorStream) next() (*models.RedmineGoJournalDetail, errors.Error) {
	if !s.cursor.Next() {
		return nil, nil
	}
	detail := &models.RedmineGoJournalDetail{}
	if err := s.db.Fetch(s.cursor, detail); err != nil {
		return nil, errors.Default.Wrap(err, "error reading status changes")
	}
	return detail, nil
}

// openStatusHistory opens the status changes of the project's issues, ordered
// by issue id and then by when they happened. Close the returned cursor when
// done.
func openStatusHistory(db dal.Dal, connectionId, projectId uint64) (*statusHistory, dal.Rows, errors.Error) {
	cursor, err := db.Cursor(
		dal.Select("d.*"),
		dal.From("_tool_redmine_go_journal_details d"),
		dal.Join("INNER JOIN _tool_redmine_go_issues i ON (i.connection_id = d.connection_id AND i.id = d.issue_id)"),
		dal.Where("d.connection_id = ? AND i.project_id = ? AND d.property = ? AND d.name = ?", connectionId, projectId, "attr", "status_id"),
		dal.Orderby("d.issue_id ASC, d.redmine_created_on ASC, d.journal_id ASC, d.position ASC"),
	)
	if err != nil {
		return nil, nil, err
	}
	return &statusHistory{stream: &cursorStream{db: db, cursor: cursor}}, cursor, nil
}
