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
	"strings"

	"github.com/apache/devlake/core/models/domainlayer/ticket"
)

// TrackerInfo holds the fields of a Redmine tracker (`/trackers.json`) that
// the built-in defaults read.
type TrackerInfo struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	DefaultStatus *Ref   `json:"default_status"`
}

// StatusInfo holds the fields of a Redmine issue status
// (`/issue_statuses.json`) that the built-in defaults read.
type StatusInfo struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	IsClosed bool   `json:"is_closed"`
}

// defaultTrackerTypes maps the lower-cased English names of Redmine's default
// trackers to a standard issue type. Support is a TASK, not an INCIDENT: a
// support request is no evidence of a failed change, and counting each one
// would distort DORA's change failure rate.
var defaultTrackerTypes = map[string]string{
	"bug":     ticket.BUG,
	"feature": ticket.REQUIREMENT,
	"support": ticket.TASK,
}

// DefaultIssueType returns the standard type of a tracker that carries one of
// Redmine's English default names, compared case-insensitively. Any other
// tracker has no built-in type and reports false.
func DefaultIssueType(trackerName string) (string, bool) {
	issueType, ok := defaultTrackerTypes[strings.ToLower(strings.TrimSpace(trackerName))]
	return issueType, ok
}

// DefaultStatusStd returns the built-in standard status of a Redmine status:
// DONE when Redmine reports it closed, TODO when it is the default status of
// any tracker, IN_PROGRESS otherwise. It depends on language-independent
// facts only, because Redmine's default data is localized.
func DefaultStatusStd(isClosed, isTrackerDefault bool) string {
	switch {
	case isClosed:
		return ticket.DONE
	case isTrackerDefault:
		return ticket.TODO
	default:
		return ticket.IN_PROGRESS
	}
}

// DefaultTransformation resolves the out-of-the-box mapping against the
// instance's real trackers and statuses, so that a new scope config can be
// pre-filled with the mapping the plugin would apply anyway. Every status gets
// an explicit row; only trackers with an English default name get a rule, the
// others stay unmapped and fall back to their upper-cased name. Component
// comes from the issue category and every toggle carries its default.
func DefaultTransformation(trackers []TrackerInfo, statuses []StatusInfo) Transformation {
	trackerDefaults := make(map[int]bool, len(trackers))
	for _, tracker := range trackers {
		if tracker.DefaultStatus != nil {
			trackerDefaults[tracker.DefaultStatus.ID] = true
		}
	}
	t := Transformation{
		TypeRules:      []TypeRule{},
		StatusMappings: make([]StatusMapping, 0, len(statuses)),
		Severity:       []ValueMapField{},
		Environment:    []ValueMapField{},
		Component:      &ComponentMapping{Source: ComponentSourceCategory},
	}
	for _, tracker := range trackers {
		if issueType, ok := DefaultIssueType(tracker.Name); ok {
			t.TypeRules = append(t.TypeRules, TypeRule{Tracker: &Ref{ID: tracker.ID, Name: tracker.Name}, Type: issueType})
		}
	}
	for _, status := range statuses {
		t.StatusMappings = append(t.StatusMappings, StatusMapping{
			Status: Ref{ID: status.ID, Name: status.Name},
			Std:    DefaultStatusStd(status.IsClosed, trackerDefaults[status.ID]),
		})
	}
	for _, toggle := range []struct {
		field  **bool
		toggle Toggle
	}{
		{&t.ReconcileSweep, ToggleReconcileSweep},
		{&t.CollectTimeEntries, ToggleCollectTimeEntries},
		{&t.CollectChangesets, ToggleCollectChangesets},
		{&t.CollectComments, ToggleCollectComments},
		{&t.IncludePrivateNotes, ToggleIncludePrivateNotes},
		{&t.VersionsAsSprints, ToggleVersionsAsSprints},
	} {
		value := ToggleDefault(toggle.toggle)
		*toggle.field = &value
	}
	return t
}
