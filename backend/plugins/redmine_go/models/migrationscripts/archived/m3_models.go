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

package archived

import (
	"time"

	"github.com/apache/devlake/core/models/migrationscripts/archived"
)

// RedmineGoIssueRelation is the relation table as created by the M3 migration.
type RedmineGoIssueRelation struct {
	ConnectionId uint64 `gorm:"primaryKey"`
	Id           uint64 `gorm:"primaryKey;autoIncrement:false"`
	ProjectId    uint64 `gorm:"index"`
	IssueId      uint64 `gorm:"index"`
	IssueToId    uint64
	RelationType string `gorm:"type:varchar(32)"`
	Delay        *int
	archived.NoPKModel
}

// TableName returns the issue-relation table name.
func (RedmineGoIssueRelation) TableName() string { return "_tool_redmine_go_issue_relations" }

// RedmineGoIssueChangeset is the changeset table as created by the M3 migration.
type RedmineGoIssueChangeset struct {
	ConnectionId uint64 `gorm:"primaryKey"`
	IssueId      uint64 `gorm:"primaryKey;autoIncrement:false"`
	Revision     string `gorm:"primaryKey;type:varchar(255)"`
	UserId       uint64
	UserName     string `gorm:"type:varchar(255)"`
	Comments     string
	CommittedOn  *time.Time
	archived.NoPKModel
}

// TableName returns the issue-changeset table name.
func (RedmineGoIssueChangeset) TableName() string { return "_tool_redmine_go_issue_changesets" }

// RedmineGoTimeEntry is the time-entry table as created by the M3 migration.
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
	archived.NoPKModel
}

// TableName returns the time-entry table name.
func (RedmineGoTimeEntry) TableName() string { return "_tool_redmine_go_time_entries" }
