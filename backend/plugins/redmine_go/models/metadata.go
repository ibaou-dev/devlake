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

// RedmineGoTracker is a Redmine tracker. DefaultStatusId is the status new
// issues of the tracker start in; the built-in status mapping reads it.
type RedmineGoTracker struct {
	ConnectionId      uint64 `gorm:"primaryKey"`
	Id                uint64 `gorm:"primaryKey;autoIncrement:false"`
	Name              string `gorm:"type:varchar(255)"`
	DefaultStatusId   uint64
	DefaultStatusName string `gorm:"type:varchar(255)"`
	common.NoPKModel
}

// TableName returns the tracker table name.
func (RedmineGoTracker) TableName() string {
	return "_tool_redmine_go_trackers"
}

// RedmineGoStatus is a Redmine issue status; IsClosed drives the built-in
// DONE fallback and the resolution-date fallback.
type RedmineGoStatus struct {
	ConnectionId uint64 `gorm:"primaryKey"`
	Id           uint64 `gorm:"primaryKey;autoIncrement:false"`
	Name         string `gorm:"type:varchar(255)"`
	IsClosed     bool
	common.NoPKModel
}

// TableName returns the status table name.
func (RedmineGoStatus) TableName() string {
	return "_tool_redmine_go_statuses"
}

// RedmineGoPriority is a Redmine issue priority.
type RedmineGoPriority struct {
	ConnectionId uint64 `gorm:"primaryKey"`
	Id           uint64 `gorm:"primaryKey;autoIncrement:false"`
	Name         string `gorm:"type:varchar(255)"`
	IsDefault    bool
	Active       bool
	common.NoPKModel
}

// TableName returns the priority table name.
func (RedmineGoPriority) TableName() string {
	return "_tool_redmine_go_priorities"
}

// RedmineGoCustomField is a Redmine custom field. Redmine only lists them for
// administrators, so the table stays empty for other keys. PossibleValues is a
// JSON array of {"value","label"} and Trackers a JSON array of {"id","name"}.
type RedmineGoCustomField struct {
	ConnectionId   uint64 `gorm:"primaryKey"`
	Id             uint64 `gorm:"primaryKey;autoIncrement:false"`
	Name           string `gorm:"type:varchar(255)"`
	CustomizedType string `gorm:"type:varchar(100)"`
	FieldFormat    string `gorm:"type:varchar(100)"`
	Multiple       bool
	PossibleValues string `gorm:"type:text"`
	Trackers       string `gorm:"type:text"`
	common.NoPKModel
}

// TableName returns the custom-field table name.
func (RedmineGoCustomField) TableName() string {
	return "_tool_redmine_go_custom_fields"
}

// RedmineGoVersion is a Redmine version (a release or milestone). ProjectId is
// the owning project; a version shared with other projects appears in each of
// their lists but is stored once.
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
	common.NoPKModel
}

// TableName returns the version table name.
func (RedmineGoVersion) TableName() string {
	return "_tool_redmine_go_versions"
}

// RedmineGoIssueCategory is a Redmine issue category of one project.
type RedmineGoIssueCategory struct {
	ConnectionId   uint64 `gorm:"primaryKey"`
	Id             uint64 `gorm:"primaryKey;autoIncrement:false"`
	ProjectId      uint64 `gorm:"index"`
	Name           string `gorm:"type:varchar(255)"`
	AssignedToId   uint64
	AssignedToName string `gorm:"type:varchar(255)"`
	common.NoPKModel
}

// TableName returns the issue-category table name.
func (RedmineGoIssueCategory) TableName() string {
	return "_tool_redmine_go_issue_categories"
}

// RedmineGoAccount is a Redmine user as it appears embedded in issues and
// journals: an id and a display name. Redmine's `/users/:id.json` is never
// called, so no e-mail address or other profile data is stored.
type RedmineGoAccount struct {
	ConnectionId uint64 `gorm:"primaryKey"`
	Id           uint64 `gorm:"primaryKey;autoIncrement:false"`
	Name         string `gorm:"type:varchar(255)"`
	common.NoPKModel
}

// TableName returns the account table name.
func (RedmineGoAccount) TableName() string {
	return "_tool_redmine_go_accounts"
}

// RedmineGoObservedField is a custom field seen embedded in a collected issue.
// It names the fields a non-admin key cannot list, so journal rows and the
// mapping UI can still show names.
type RedmineGoObservedField struct {
	ConnectionId uint64 `gorm:"primaryKey"`
	Id           uint64 `gorm:"primaryKey;autoIncrement:false"`
	Name         string `gorm:"type:varchar(255)"`
	Multiple     bool
	common.NoPKModel
}

// TableName returns the observed-field table name.
func (RedmineGoObservedField) TableName() string {
	return "_tool_redmine_go_observed_fields"
}
