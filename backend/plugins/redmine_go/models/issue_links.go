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

package models

import (
	"time"

	"github.com/apache/devlake/core/models/common"
)

// RedmineGoIssueRelation is one Redmine issue relation, owned by the project
// of its source issue (`issue_id`): a relation appears in the lists of both
// of its issues, and this keeps one row per relation. RelationType is
// Redmine's own, read from the source issue to the target (`blocks`,
// `precedes`, `relates`, ...). The target may lie in a project that is not a
// scope.
type RedmineGoIssueRelation struct {
	ConnectionId uint64 `gorm:"primaryKey"`
	Id           uint64 `gorm:"primaryKey;autoIncrement:false"`
	ProjectId    uint64 `gorm:"index"`
	IssueId      uint64 `gorm:"index"`
	IssueToId    uint64
	RelationType string `gorm:"type:varchar(32)"`
	Delay        *int
	common.NoPKModel
}

// TableName returns the issue-relation table name.
func (RedmineGoIssueRelation) TableName() string {
	return "_tool_redmine_go_issue_relations"
}

// RedmineGoIssueChangeset is a repository changeset linked to an issue. The
// revision is what `issue_commits` keys on; the message and time stay here.
type RedmineGoIssueChangeset struct {
	ConnectionId uint64 `gorm:"primaryKey"`
	IssueId      uint64 `gorm:"primaryKey;autoIncrement:false"`
	Revision     string `gorm:"primaryKey;type:varchar(255)"`
	UserId       uint64
	UserName     string `gorm:"type:varchar(255)"`
	Comments     string
	CommittedOn  *time.Time
	common.NoPKModel
}

// TableName returns the issue-changeset table name.
func (RedmineGoIssueChangeset) TableName() string {
	return "_tool_redmine_go_issue_changesets"
}

// RedmineGoTimeEntry is a Redmine time entry of a project. IssueId is 0 for
// an entry logged on the project itself; such entries are not stored. The
// activity name stays here, as `issue_worklogs` has no column for it. Hours
// are Redmine's decimal hours and SpentOn is the day the time was spent.
type RedmineGoTimeEntry struct {
	ConnectionId     uint64 `gorm:"primaryKey"`
	Id               uint64 `gorm:"primaryKey;autoIncrement:false"`
	ProjectId        uint64 `gorm:"index"`
	IssueId          uint64 `gorm:"index"`
	UserId           uint64
	UserName         string `gorm:"type:varchar(255)"`
	ActivityId       uint64
	ActivityName     string `gorm:"type:varchar(255)"`
	Hours            float64
	Comments         string
	SpentOn          time.Time
	RedmineCreatedOn *time.Time
	RedmineUpdatedOn *time.Time
	common.NoPKModel
}

// TableName returns the time-entry table name.
func (RedmineGoTimeEntry) TableName() string {
	return "_tool_redmine_go_time_entries"
}
