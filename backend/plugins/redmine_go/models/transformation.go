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
	"github.com/apache/devlake/core/models/domainlayer/ticket"
)

// Ref points at a Redmine object (tracker, status or custom field). The id is
// authoritative and survives renames; the name is for display and is the
// fallback when the id no longer exists, for example in a config copied to
// another Redmine.
type Ref struct {
	ID   int    `json:"id" mapstructure:"id"`
	Name string `json:"name" mapstructure:"name"`
}

// Value is a custom-field value. Matching always uses Value, exactly as
// Redmine stores it; Label only makes the UI readable.
type Value struct {
	Value string `json:"value" mapstructure:"value"`
	Label string `json:"label,omitempty" mapstructure:"label"`
}

// Condition is a custom-field predicate of a type rule.
type Condition struct {
	Field  Ref     `json:"field" mapstructure:"field"`
	Op     string  `json:"op" mapstructure:"op"`
	Values []Value `json:"values" mapstructure:"values"`
}

// TypeRule maps issues to a standard issue type. Rules are evaluated in
// order and the first match wins; it matches when its tracker matches (or it
// has none) and its condition holds (or it has none).
type TypeRule struct {
	Tracker *Ref       `json:"tracker,omitempty" mapstructure:"tracker"`
	When    *Condition `json:"when,omitempty" mapstructure:"when"`
	Type    string     `json:"type" mapstructure:"type"`
}

// StatusMapping maps one Redmine status to a standard status.
type StatusMapping struct {
	Status Ref    `json:"status" mapstructure:"status"`
	Std    string `json:"std" mapstructure:"std"`
}

// ValueMapEntry translates one raw custom-field value.
type ValueMapEntry struct {
	Value string `json:"value" mapstructure:"value"`
	Label string `json:"label,omitempty" mapstructure:"label"`
	To    string `json:"to" mapstructure:"to"`
}

// ValueMapField reads a custom field and translates its values. Severity and
// environment mappings are ordered lists of these: the first field with a
// non-empty value wins.
type ValueMapField struct {
	Field Ref             `json:"field" mapstructure:"field"`
	Map   []ValueMapEntry `json:"map" mapstructure:"map"`
}

// ComponentMapping selects where issues.component comes from.
type ComponentMapping struct {
	Source string `json:"source" mapstructure:"source"`
	Field  *Ref   `json:"field,omitempty" mapstructure:"field"`
}

// Values accepted by the enumerated transformation fields.
const (
	ConditionIn    = "in"
	ConditionNotIn = "notIn"

	ComponentSourceNone        = "none"
	ComponentSourceCategory    = "category"
	ComponentSourceCustomField = "customField"

	EnvironmentProduction  = "PRODUCTION"
	EnvironmentStaging     = "STAGING"
	EnvironmentTesting     = "TESTING"
	EnvironmentDevelopment = "DEVELOPMENT"
)

var (
	// IssueTypes lists the standard types a type rule may produce.
	IssueTypes = []string{ticket.REQUIREMENT, ticket.BUG, ticket.INCIDENT, ticket.TASK, ticket.SUBTASK}
	// StatusStds lists the standard statuses a status mapping may produce.
	StatusStds = []string{ticket.TODO, ticket.IN_PROGRESS, ticket.DONE, ticket.OTHER}
	// EnvironmentClasses lists the classes an environment mapping may produce.
	EnvironmentClasses = []string{EnvironmentProduction, EnvironmentStaging, EnvironmentTesting, EnvironmentDevelopment}
	// ConditionOps lists the operators a type-rule condition may use.
	ConditionOps = []string{ConditionIn, ConditionNotIn}
	// ComponentSources lists the sources of issues.component.
	ComponentSources = []string{ComponentSourceNone, ComponentSourceCategory, ComponentSourceCustomField}
)

// Toggle names one optional collection or emission behaviour.
type Toggle string

// The scope-config toggles. ReconcileSweep, CollectTimeEntries,
// CollectChangesets and CollectComments are on unless switched off;
// IncludePrivateNotes and VersionsAsSprints are off unless switched on.
const (
	ToggleReconcileSweep      Toggle = "reconcileSweep"
	ToggleCollectTimeEntries  Toggle = "collectTimeEntries"
	ToggleCollectChangesets   Toggle = "collectChangesets"
	ToggleCollectComments     Toggle = "collectComments"
	ToggleIncludePrivateNotes Toggle = "includePrivateNotes"
	ToggleVersionsAsSprints   Toggle = "versionsAsSprints"
)

// ToggleDefault returns the value a toggle has when the scope config leaves it
// unset.
func ToggleDefault(toggle Toggle) bool {
	switch toggle {
	case ToggleIncludePrivateNotes, ToggleVersionsAsSprints:
		return false
	default:
		return true
	}
}

// Transformation holds the mapping rules and toggles of a scope config, the
// contract described by the scope-config schema. Toggles are tri-state so a
// scope without a scope config, and a config that never mentions a toggle,
// both fall back to the toggle's default instead of Go's zero value.
type Transformation struct {
	TypeRules      []TypeRule      `json:"typeRules" mapstructure:"typeRules" gorm:"type:json;serializer:json"`
	StatusMappings []StatusMapping `json:"statusMappings" mapstructure:"statusMappings" gorm:"type:json;serializer:json"`
	Severity       []ValueMapField `json:"severity" mapstructure:"severity" gorm:"type:json;serializer:json"`
	Environment    []ValueMapField `json:"environment" mapstructure:"environment" gorm:"type:json;serializer:json"`
	// Component is nil when unset, which behaves as source "category".
	Component *ComponentMapping `json:"component" mapstructure:"component" gorm:"type:json;serializer:json"`

	ReconcileSweep      *bool `json:"reconcileSweep" mapstructure:"reconcileSweep"`
	CollectTimeEntries  *bool `json:"collectTimeEntries" mapstructure:"collectTimeEntries"`
	CollectChangesets   *bool `json:"collectChangesets" mapstructure:"collectChangesets"`
	CollectComments     *bool `json:"collectComments" mapstructure:"collectComments"`
	IncludePrivateNotes *bool `json:"includePrivateNotes" mapstructure:"includePrivateNotes"`
	VersionsAsSprints   *bool `json:"versionsAsSprints" mapstructure:"versionsAsSprints"`
}

// ToggleEnabled reports whether the toggle is on, using its default when the
// scope config leaves it unset.
func (t *Transformation) ToggleEnabled(toggle Toggle) bool {
	var value *bool
	switch toggle {
	case ToggleReconcileSweep:
		value = t.ReconcileSweep
	case ToggleCollectTimeEntries:
		value = t.CollectTimeEntries
	case ToggleCollectChangesets:
		value = t.CollectChangesets
	case ToggleCollectComments:
		value = t.CollectComments
	case ToggleIncludePrivateNotes:
		value = t.IncludePrivateNotes
	case ToggleVersionsAsSprints:
		value = t.VersionsAsSprints
	}
	if value == nil {
		return ToggleDefault(toggle)
	}
	return *value
}
