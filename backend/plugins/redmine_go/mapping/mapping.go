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

// Package mapping evaluates a redmine_go scope-config transformation against
// the facts of one Redmine issue. It is pure: no database, no HTTP.
package mapping

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/apache/devlake/core/models/domainlayer/ticket"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// FieldInfo describes one issue custom field known to the instance.
type FieldInfo struct {
	ID     int
	Name   string
	Labels map[string]string // raw value -> human label (from possible_values); may be nil
}

// Instance carries the metadata facts the mapping rules are evaluated against.
// Any slice may be empty: non-admin keys have no custom field metadata.
type Instance struct {
	Trackers []models.TrackerInfo
	Statuses []models.StatusInfo
	Fields   []FieldInfo
}

// FieldValues is one custom field of an issue with its raw values.
type FieldValues struct {
	ID     int
	Name   string
	Values []string // empty = field has no value
}

// Issue is the mapping input: the facts of one issue.
type Issue struct {
	TrackerID      int
	TrackerName    string
	StatusID       int
	StatusName     string
	StatusIsClosed bool
	CategoryName   string
	Fields         []FieldValues
}

// StatusChange is one status transition of an issue. The names are only used
// to match a status the instance does not know by id.
type StatusChange struct {
	At             time.Time
	FromStatusID   int
	FromStatusName string
	ToStatusID     int
	ToStatusName   string
}

// ParseFieldValues decodes a Redmine custom_fields[].value (JSON null, string,
// or array of strings; numbers and booleans are tolerated) into its non-empty
// string values.
func ParseFieldValues(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v interface{}
	if err := dec.Decode(&v); err != nil {
		return nil
	}
	return appendScalars(nil, v)
}

func appendScalars(out []string, v interface{}) []string {
	switch x := v.(type) {
	case string:
		if strings.TrimSpace(x) != "" {
			out = append(out, x)
		}
	case json.Number:
		out = append(out, x.String())
	case bool:
		out = append(out, strconv.FormatBool(x))
	case []interface{}:
		for _, e := range x {
			out = appendScalars(out, e)
		}
	}
	return out
}

// valueMap is a ValueMapField with its entries indexed by raw value.
type valueMap struct {
	field models.Ref
	to    map[string]string
}

// Mapper evaluates one transformation against one instance's metadata. It is
// immutable after construction and safe for concurrent use.
type Mapper struct {
	typeRules      []models.TypeRule
	statusMappings []models.StatusMapping
	severity       []valueMap
	environment    []valueMap
	component      models.ComponentMapping

	trackerIDs      map[int]struct{}
	statusIDs       map[int]struct{}
	fieldIDs        map[int]struct{}
	statusClosed    map[int]bool
	defaultStatuses map[int]struct{}
	fieldLabels     map[int]map[string]string
}

// NewMapper builds a Mapper. A nil transformation is the empty one: every
// mapping takes its built-in fallback.
func NewMapper(t *models.Transformation, inst Instance) *Mapper {
	if t == nil {
		t = &models.Transformation{}
	}
	m := &Mapper{
		typeRules:       t.TypeRules,
		statusMappings:  t.StatusMappings,
		severity:        newValueMaps(t.Severity),
		environment:     newValueMaps(t.Environment),
		component:       models.ComponentMapping{Source: models.ComponentSourceCategory},
		trackerIDs:      make(map[int]struct{}, len(inst.Trackers)),
		statusIDs:       make(map[int]struct{}, len(inst.Statuses)),
		fieldIDs:        make(map[int]struct{}, len(inst.Fields)),
		statusClosed:    make(map[int]bool, len(inst.Statuses)),
		defaultStatuses: make(map[int]struct{}, len(inst.Trackers)),
		fieldLabels:     make(map[int]map[string]string, len(inst.Fields)),
	}
	if t.Component != nil {
		m.component = *t.Component
	}
	for _, tr := range inst.Trackers {
		m.trackerIDs[tr.ID] = struct{}{}
		if tr.DefaultStatus != nil {
			m.defaultStatuses[tr.DefaultStatus.ID] = struct{}{}
		}
	}
	for _, st := range inst.Statuses {
		m.statusIDs[st.ID] = struct{}{}
		m.statusClosed[st.ID] = st.IsClosed
	}
	for _, f := range inst.Fields {
		m.fieldIDs[f.ID] = struct{}{}
		if len(f.Labels) > 0 {
			m.fieldLabels[f.ID] = f.Labels
		}
	}
	return m
}

func newValueMaps(fields []models.ValueMapField) []valueMap {
	out := make([]valueMap, len(fields))
	for i, f := range fields {
		to := make(map[string]string, len(f.Map))
		for _, e := range f.Map {
			if _, dup := to[e.Value]; !dup {
				to[e.Value] = e.To
			}
		}
		out[i] = valueMap{field: f.Field, to: to}
	}
	return out
}

// refMatches reports whether a ref designates the object (id, name): the ids
// are equal, or the ref's id is unknown to the instance and the names are
// equal ignoring case and surrounding spaces.
func refMatches(known map[int]struct{}, ref models.Ref, id int, name string) bool {
	if ref.ID == id {
		return true
	}
	if _, ok := known[ref.ID]; ok {
		return false
	}
	refName := strings.TrimSpace(ref.Name)
	return refName != "" && strings.EqualFold(refName, strings.TrimSpace(name))
}

// nonEmpty returns values without blank entries; it allocates only when one
// has to be dropped.
func nonEmpty(values []string) []string {
	for i, v := range values {
		if strings.TrimSpace(v) != "" {
			continue
		}
		out := append([]string(nil), values[:i]...)
		for _, w := range values[i+1:] {
			if strings.TrimSpace(w) != "" {
				out = append(out, w)
			}
		}
		return out
	}
	return values
}

// fieldOf finds the issue's custom field designated by ref and returns its id
// and non-empty values; a missing field has no values.
func (m *Mapper) fieldOf(issue *Issue, ref models.Ref) (int, []string) {
	for i := range issue.Fields {
		f := &issue.Fields[i]
		if refMatches(m.fieldIDs, ref, f.ID, f.Name) {
			return f.ID, nonEmpty(f.Values)
		}
	}
	return 0, nil
}

// firstValued returns the first value map whose field has values on the issue.
func (m *Mapper) firstValued(issue *Issue, maps []valueMap) (valueMap, int, []string) {
	for _, vm := range maps {
		if id, values := m.fieldOf(issue, vm.field); len(values) > 0 {
			return vm, id, values
		}
	}
	return valueMap{}, 0, nil
}

// IssueType returns the standard issue type: the first matching type rule, else
// the built-in English tracker default, else the upper-cased tracker name.
func (m *Mapper) IssueType(issue *Issue) string {
	for i := range m.typeRules {
		rule := &m.typeRules[i]
		if rule.Tracker != nil && !refMatches(m.trackerIDs, *rule.Tracker, issue.TrackerID, issue.TrackerName) {
			continue
		}
		if rule.When != nil && !m.conditionHolds(issue, rule.When) {
			continue
		}
		return rule.Type
	}
	if t, ok := models.DefaultIssueType(issue.TrackerName); ok {
		return t
	}
	return strings.ToUpper(strings.TrimSpace(issue.TrackerName))
}

func (m *Mapper) conditionHolds(issue *Issue, c *models.Condition) bool {
	_, values := m.fieldOf(issue, c.Field)
	listed := false
	for _, v := range values {
		for _, want := range c.Values {
			if v == want.Value {
				listed = true
				break
			}
		}
		if listed {
			break
		}
	}
	switch c.Op {
	case models.ConditionIn:
		return listed
	case models.ConditionNotIn:
		return !listed
	default:
		return false
	}
}

// Status returns the standard status: the first matching status mapping, else
// the built-in default. isClosed is the issue's own flag, used only when the
// instance does not know the status.
func (m *Mapper) Status(statusID int, statusName string, isClosed bool) string {
	for i := range m.statusMappings {
		sm := &m.statusMappings[i]
		if refMatches(m.statusIDs, sm.Status, statusID, statusName) {
			return sm.Std
		}
	}
	_, isDefault := m.defaultStatuses[statusID]
	return models.DefaultStatusStd(m.StatusIsClosed(statusID, isClosed), isDefault)
}

// StatusIsClosed returns the instance's is_closed flag of a status, or
// fallback when the instance does not know the status.
func (m *Mapper) StatusIsClosed(statusID int, fallback bool) bool {
	if closed, ok := m.statusClosed[statusID]; ok {
		return closed
	}
	return fallback
}

// Severity returns issues.severity: the first value of the first configured
// field that has a value, translated by its map, else its label, else raw.
func (m *Mapper) Severity(issue *Issue) string {
	vm, fieldID, values := m.firstValued(issue, m.severity)
	if len(values) == 0 {
		return ""
	}
	if to, ok := vm.to[values[0]]; ok {
		return to
	}
	return m.FieldLabel(fieldID, values[0])
}

// EnvironmentClasses returns the deduplicated environment classes of the issue.
// Only the first configured field that has a value is consulted, and only its
// values that have a map entry produce a class.
func (m *Mapper) EnvironmentClasses(issue *Issue) []string {
	vm, _, values := m.firstValued(issue, m.environment)
	var out []string
	for _, v := range values {
		to, ok := vm.to[v]
		if !ok || containsString(out, to) {
			continue
		}
		out = append(out, to)
	}
	return out
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Component returns issues.component from the configured source.
func (m *Mapper) Component(issue *Issue) string {
	switch m.component.Source {
	case models.ComponentSourceCategory:
		return issue.CategoryName
	case models.ComponentSourceCustomField:
		if m.component.Field == nil {
			return ""
		}
		id, values := m.fieldOf(issue, *m.component.Field)
		labels := make([]string, len(values))
		for i, v := range values {
			labels[i] = m.FieldLabel(id, v)
		}
		return strings.Join(labels, ",")
	default:
		return ""
	}
}

// FieldLabel returns the instance's label of a custom field value, or the
// value itself when no label is known.
func (m *Mapper) FieldLabel(fieldID int, value string) string {
	if label, ok := m.fieldLabels[fieldID][value]; ok && label != "" {
		return label
	}
	return value
}

// IsDone reports whether a status maps to DONE under the transformation.
func (m *Mapper) IsDone(statusID int, statusName string, isClosed bool) bool {
	return m.Status(statusID, statusName, isClosed) == ticket.DONE
}

// ResolutionDate returns when the issue entered its current uninterrupted run
// of statuses that map to DONE, so a reopened issue is resolved at its latest
// closing and an open status mapped to DONE (for example "Verified") resolves
// too. It is nil while the issue is not DONE.
//
// The start comes from the status changes: walking back from the latest, a
// change into DONE from a non-DONE status is the start, and a move between two
// DONE statuses extends the run further back. When the history cannot place the
// start (no change into DONE, an issue created in a DONE status, a gap in the
// journals), closedOn is the fallback. Changes may come in any order.
func (m *Mapper) ResolutionDate(issue *Issue, closedOn *time.Time, changes []StatusChange) *time.Time {
	if !m.IsDone(issue.StatusID, issue.StatusName, issue.StatusIsClosed) {
		return nil
	}
	ordered := append([]StatusChange(nil), changes...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].At.Before(ordered[j].At) })
	for i := len(ordered) - 1; i >= 0; i-- {
		c := &ordered[i]
		if !m.IsDone(c.ToStatusID, c.ToStatusName, false) {
			break
		}
		if !m.IsDone(c.FromStatusID, c.FromStatusName, false) {
			at := c.At
			return &at
		}
	}
	if closedOn == nil {
		return nil
	}
	at := *closedOn
	return &at
}

// LeadTimeMinutes returns whole minutes between created and resolution, nil
// when resolution is nil or before created.
func LeadTimeMinutes(created time.Time, resolution *time.Time) *uint {
	if resolution == nil || resolution.Before(created) {
		return nil
	}
	minutes := uint(resolution.Sub(created) / time.Minute)
	return &minutes
}
