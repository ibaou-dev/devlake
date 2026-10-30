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

// RedmineGoIssue is a Redmine issue exactly as collected: standard fields flat,
// the custom fields as a JSON array. It carries no mapped values: types,
// statuses and the like are derived at conversion, so a scope-config change
// never needs a re-extraction. Source timestamps carry the redmine_ prefix so
// they never shadow the reserved created_at and updated_at columns. Id fields
// are 0 and names empty when the issue has no such value.
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
	// CustomFieldsJson is the issue's custom_fields array, verbatim.
	CustomFieldsJson string `gorm:"type:text"`
	RedmineCreatedOn time.Time
	RedmineUpdatedOn time.Time `gorm:"index"`
	RedmineClosedOn  *time.Time
	common.NoPKModel
}

// TableName returns the issue table name.
func (RedmineGoIssue) TableName() string {
	return "_tool_redmine_go_issues"
}

// RedmineGoJournal is one Redmine journal (a history entry) of an issue: its
// author, time and free-text note. The notes feed the comment conversion;
// the field changes of the same entry are in RedmineGoJournalDetail.
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
	common.NoPKModel
}

// TableName returns the journal table name.
func (RedmineGoJournal) TableName() string {
	return "_tool_redmine_go_journals"
}

// RedmineGoJournalDetail is one field change of a journal. DetailKey
// identifies the change inside its journal: a hash of its content plus the
// count of identical changes before it. It does not depend on the position in
// Redmine's answer, so re-extraction reproduces the same keys, and the
// several rows a multi-value custom field journals under one name stay apart.
// Property is Redmine's own (attr, cf, relation, attachment); Name is the
// attribute name, custom field id or relation type. FieldName is the
// normalised name changelog consumers see; OldLabel and NewLabel are the
// values translated through the metadata tables, or the raw value when no
// name is known.
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
	common.NoPKModel
}

// TableName returns the journal-detail table name.
func (RedmineGoJournalDetail) TableName() string {
	return "_tool_redmine_go_journal_details"
}
