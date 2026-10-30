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
	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/models/domainlayer/crossdomain"
	"github.com/apache/devlake/core/models/domainlayer/didgen"
	"github.com/apache/devlake/core/models/domainlayer/ticket"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// deleteChunk bounds the length of an IN list.
const deleteChunk = 500

// deleteByIds deletes the rows of a model whose column is one of ids, in
// chunks, and further restricted by the clauses. The column name is always a
// literal of the caller.
func deleteByIds[T uint64 | string](db dal.Dal, model interface{}, column string, ids []T, clauses ...dal.Clause) errors.Error {
	for start := 0; start < len(ids); start += deleteChunk {
		chunk := ids[start:min(start+deleteChunk, len(ids))]
		where := append([]dal.Clause{dal.Where(column+" IN ?", chunk)}, clauses...)
		if err := db.Delete(model, where...); err != nil {
			return err
		}
	}
	return nil
}

// removeIssues deletes issues, and everything the plugin derived from them,
// from the tool and domain layers. It is used for issues that were deleted in
// Redmine or left a scope by moving to another project.
//
// DORA's incident generator only deletes incidents of issues that still
// exist, so an incident of a removed issue would stay forever unless it is
// deleted here. Relationships that point at a removed issue from another
// project's issues are left to that project's sweep, which sees Redmine
// drop them.
func removeIssues(db dal.Dal, connectionId uint64, issueIds []uint64) errors.Error {
	if len(issueIds) == 0 {
		return nil
	}
	connection := dal.Where("connection_id = ?", connectionId)
	for _, model := range []interface{}{
		&models.RedmineGoJournalDetail{}, &models.RedmineGoJournal{}, &models.RedmineGoIssueRelation{},
		&models.RedmineGoIssueChangeset{}, &models.RedmineGoTimeEntry{},
	} {
		if err := deleteByIds(db, model, "issue_id", issueIds, connection); err != nil {
			return err
		}
	}
	if err := deleteByIds(db, &models.RedmineGoIssue{}, "id", issueIds, connection); err != nil {
		return err
	}

	issueIdGen := didgen.NewDomainIdGenerator(&models.RedmineGoIssue{})
	domainIds := make([]string, len(issueIds))
	for i, issueId := range issueIds {
		domainIds[i] = issueIdGen.Generate(connectionId, issueId)
	}
	for _, target := range []struct {
		model  interface{}
		column string
	}{
		{&ticket.Issue{}, "id"},
		{&ticket.BoardIssue{}, "issue_id"},
		{&ticket.IssueChangelogs{}, "issue_id"},
		{&ticket.IssueWorklog{}, "issue_id"},
		{&ticket.IssueComment{}, "issue_id"},
		{&ticket.IssueRelationship{}, "source_issue_id"},
		{&ticket.IssueAssignee{}, "issue_id"},
		{&ticket.IssueCustomArrayField{}, "issue_id"},
		{&ticket.SprintIssue{}, "issue_id"},
		{&crossdomain.IssueCommit{}, "issue_id"},
		{&ticket.Incident{}, "id"},
		{&ticket.IncidentAssignee{}, "incident_id"},
	} {
		if err := deleteByIds(db, target.model, target.column, domainIds); err != nil {
			return err
		}
	}
	return nil
}
