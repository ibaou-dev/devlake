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

// RedmineGoTracker is the tracker table as created by the M2 migration.
type RedmineGoTracker struct {
	ConnectionId      uint64 `gorm:"primaryKey"`
	Id                uint64 `gorm:"primaryKey;autoIncrement:false"`
	Name              string `gorm:"type:varchar(255)"`
	DefaultStatusId   uint64
	DefaultStatusName string `gorm:"type:varchar(255)"`
	archived.NoPKModel
}

// TableName returns the tracker table name.
func (RedmineGoTracker) TableName() string { return "_tool_redmine_go_trackers" }

// RedmineGoStatus is the status table as created by the M2 migration.
type RedmineGoStatus struct {
	ConnectionId uint64 `gorm:"primaryKey"`
	Id           uint64 `gorm:"primaryKey;autoIncrement:false"`
	Name         string `gorm:"type:varchar(255)"`
	IsClosed     bool
	archived.NoPKModel
}

// TableName returns the status table name.
func (RedmineGoStatus) TableName() string { return "_tool_redmine_go_statuses" }

// RedmineGoPriority is the priority table as created by the M2 migration.
type RedmineGoPriority struct {
	ConnectionId uint64 `gorm:"primaryKey"`
	Id           uint64 `gorm:"primaryKey;autoIncrement:false"`
	Name         string `gorm:"type:varchar(255)"`
	IsDefault    bool
	Active       bool
	archived.NoPKModel
}

// TableName returns the priority table name.
func (RedmineGoPriority) TableName() string { return "_tool_redmine_go_priorities" }

// RedmineGoCustomField is the custom-field table as created by the M2 migration.
type RedmineGoCustomField struct {
	ConnectionId   uint64 `gorm:"primaryKey"`
	Id             uint64 `gorm:"primaryKey;autoIncrement:false"`
	Name           string `gorm:"type:varchar(255)"`
	CustomizedType string `gorm:"type:varchar(100)"`
	FieldFormat    string `gorm:"type:varchar(100)"`
	Multiple       bool
	PossibleValues string `gorm:"type:text"`
	Trackers       string `gorm:"type:text"`
	archived.NoPKModel
}

// TableName returns the custom-field table name.
func (RedmineGoCustomField) TableName() string { return "_tool_redmine_go_custom_fields" }

// RedmineGoVersion is the version table as created by the M2 migration.
type RedmineGoVersion struct {
	ConnectionId     uint64 `gorm:"primaryKey"`
	Id               uint64 `gorm:"primaryKey;autoIncrement:false"`
	ProjectId        uint64 `gorm:"index"`
	Name             string `gorm:"type:varchar(255)"`
	Description      string
	Status           string `gorm:"type:varchar(32)"`
	Sharing          string `gorm:"type:varchar(32)"`
	DueDate          *time.Time
	RedmineCreatedOn *time.Time
	RedmineUpdatedOn *time.Time
	archived.NoPKModel
}

// TableName returns the version table name.
func (RedmineGoVersion) TableName() string { return "_tool_redmine_go_versions" }

// RedmineGoIssueCategory is the issue-category table as created by the M2 migration.
type RedmineGoIssueCategory struct {
	ConnectionId   uint64 `gorm:"primaryKey"`
	Id             uint64 `gorm:"primaryKey;autoIncrement:false"`
	ProjectId      uint64 `gorm:"index"`
	Name           string `gorm:"type:varchar(255)"`
	AssignedToId   uint64
	AssignedToName string `gorm:"type:varchar(255)"`
	archived.NoPKModel
}

// TableName returns the issue-category table name.
func (RedmineGoIssueCategory) TableName() string { return "_tool_redmine_go_issue_categories" }

// RedmineGoAccount is the account table as created by the M2 migration.
type RedmineGoAccount struct {
	ConnectionId uint64 `gorm:"primaryKey"`
	Id           uint64 `gorm:"primaryKey;autoIncrement:false"`
	Name         string `gorm:"type:varchar(255)"`
	archived.NoPKModel
}

// TableName returns the account table name.
func (RedmineGoAccount) TableName() string { return "_tool_redmine_go_accounts" }

// RedmineGoObservedField is the observed-field table as created by the M2 migration.
type RedmineGoObservedField struct {
	ConnectionId uint64 `gorm:"primaryKey"`
	Id           uint64 `gorm:"primaryKey;autoIncrement:false"`
	Name         string `gorm:"type:varchar(255)"`
	Multiple     bool
	archived.NoPKModel
}

// TableName returns the observed-field table name.
func (RedmineGoObservedField) TableName() string { return "_tool_redmine_go_observed_fields" }

// RedmineGoIssue is the issue table as created by the M2 migration.
type RedmineGoIssue struct {
	ConnectionId        uint64 `gorm:"primaryKey"`
	Id                  uint64 `gorm:"primaryKey;autoIncrement:false"`
	ProjectId           uint64 `gorm:"index"`
	ProjectName         string `gorm:"type:varchar(255)"`
	TrackerId           uint64
	TrackerName         string `gorm:"type:varchar(255)"`
	StatusId            uint64
	StatusName          string `gorm:"type:varchar(255)"`
	StatusIsClosed      bool
	PriorityId          uint64
	PriorityName        string `gorm:"type:varchar(255)"`
	AuthorId            uint64
	AuthorName          string `gorm:"type:varchar(255)"`
	AssigneeId          uint64
	AssigneeName        string `gorm:"type:varchar(255)"`
	CategoryId          uint64
	CategoryName        string `gorm:"type:varchar(255)"`
	FixedVersionId      uint64
	FixedVersionName    string `gorm:"type:varchar(255)"`
	ParentId            uint64
	Subject             string `gorm:"type:varchar(1024)"`
	Description         string
	StartDate           *time.Time
	DueDate             *time.Time
	DoneRatio           int
	IsPrivate           bool
	EstimatedHours      *float64
	TotalEstimatedHours *float64
	SpentHours          *float64
	TotalSpentHours     *float64
	CustomFieldsJson    string `gorm:"type:text"`
	RedmineCreatedOn    time.Time
	RedmineUpdatedOn    time.Time `gorm:"index"`
	RedmineClosedOn     *time.Time
	archived.NoPKModel
}

// TableName returns the issue table name.
func (RedmineGoIssue) TableName() string { return "_tool_redmine_go_issues" }

// RedmineGoJournal is the journal table as created by the M2 migration.
type RedmineGoJournal struct {
	ConnectionId     uint64 `gorm:"primaryKey"`
	Id               uint64 `gorm:"primaryKey;autoIncrement:false"`
	IssueId          uint64 `gorm:"index"`
	UserId           uint64
	UserName         string `gorm:"type:varchar(255)"`
	Notes            string
	PrivateNotes     bool
	RedmineCreatedOn time.Time
	RedmineUpdatedOn *time.Time
	archived.NoPKModel
}

// TableName returns the journal table name.
func (RedmineGoJournal) TableName() string { return "_tool_redmine_go_journals" }

// RedmineGoJournalDetail is the journal-detail table as created by the M2 migration.
type RedmineGoJournalDetail struct {
	ConnectionId     uint64 `gorm:"primaryKey"`
	JournalId        uint64 `gorm:"primaryKey;autoIncrement:false"`
	DetailKey        string `gorm:"primaryKey;type:varchar(64)"`
	IssueId          uint64 `gorm:"index"`
	Position         int
	UserId           uint64
	UserName         string `gorm:"type:varchar(255)"`
	RedmineCreatedOn time.Time
	Property         string `gorm:"type:varchar(64)"`
	Name             string `gorm:"type:varchar(255)"`
	FieldName        string `gorm:"type:varchar(255)"`
	OldValue         string
	NewValue         string
	OldLabel         string
	NewLabel         string
	archived.NoPKModel
}

// TableName returns the journal-detail table name.
func (RedmineGoJournalDetail) TableName() string { return "_tool_redmine_go_journal_details" }
