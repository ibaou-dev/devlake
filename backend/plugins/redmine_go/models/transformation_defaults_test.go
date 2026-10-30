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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultIssueType(t *testing.T) {
	cases := []struct {
		name string
		want string
		ok   bool
	}{
		{"Bug", "BUG", true},
		{"bug", "BUG", true},
		{" FEATURE ", "REQUIREMENT", true},
		{"Support", "TASK", true},
		{"Fehler", "", false},
		{"Change Request", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := DefaultIssueType(tc.name)
		assert.Equal(t, tc.want, got, tc.name)
		assert.Equal(t, tc.ok, ok, tc.name)
	}
}

func TestDefaultStatusStd(t *testing.T) {
	cases := []struct {
		name                string
		closed, trackerBase bool
		want                string
	}{
		{"closed wins over tracker default", true, true, "DONE"},
		{"closed", true, false, "DONE"},
		{"tracker default", false, true, "TODO"},
		{"anything else", false, false, "IN_PROGRESS"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, DefaultStatusStd(tc.closed, tc.trackerBase), tc.name)
	}
}

func TestDefaultTransformationOnRedmineDefaultData(t *testing.T) {
	newStatus := &Ref{ID: 1, Name: "New"}
	trackers := []TrackerInfo{
		{ID: 1, Name: "Bug", DefaultStatus: newStatus},
		{ID: 2, Name: "Feature", DefaultStatus: newStatus},
		{ID: 3, Name: "Support", DefaultStatus: newStatus},
	}
	statuses := []StatusInfo{
		{ID: 1, Name: "New"}, {ID: 2, Name: "In Progress"}, {ID: 3, Name: "Resolved"},
		{ID: 4, Name: "Feedback"}, {ID: 5, Name: "Closed", IsClosed: true}, {ID: 6, Name: "Rejected", IsClosed: true},
	}
	got := DefaultTransformation(trackers, statuses)

	assert.Equal(t, []TypeRule{
		{Tracker: &Ref{ID: 1, Name: "Bug"}, Type: "BUG"},
		{Tracker: &Ref{ID: 2, Name: "Feature"}, Type: "REQUIREMENT"},
		{Tracker: &Ref{ID: 3, Name: "Support"}, Type: "TASK"},
	}, got.TypeRules)
	std := map[string]string{}
	for _, m := range got.StatusMappings {
		std[m.Status.Name] = m.Std
	}
	assert.Equal(t, map[string]string{
		"New": "TODO", "In Progress": "IN_PROGRESS", "Resolved": "IN_PROGRESS",
		"Feedback": "IN_PROGRESS", "Closed": "DONE", "Rejected": "DONE",
	}, std)
	assert.Equal(t, &ComponentMapping{Source: ComponentSourceCategory}, got.Component)
	assert.NotNil(t, got.Severity)
	assert.NotNil(t, got.Environment)
	for toggle, want := range map[Toggle]bool{
		ToggleReconcileSweep: true, ToggleCollectTimeEntries: true, ToggleCollectChangesets: true,
		ToggleCollectComments: true, ToggleIncludePrivateNotes: false, ToggleVersionsAsSprints: false,
	} {
		assert.Equal(t, want, got.ToggleEnabled(toggle), string(toggle))
	}
	require.Nil(t, got.Validate(), "the defaults must satisfy the contract")
}

func TestDefaultTransformationOnLocalizedAndCustomisedInstance(t *testing.T) {
	neu := &Ref{ID: 10, Name: "Neu"}
	trackers := []TrackerInfo{
		{ID: 1, Name: "Fehler", DefaultStatus: neu},
		{ID: 2, Name: "BUG", DefaultStatus: neu},
		{ID: 4, Name: "Change Request", DefaultStatus: &Ref{ID: 11, Name: "Triaged"}},
		{ID: 5, Name: "No default status"},
	}
	statuses := []StatusInfo{
		{ID: 10, Name: "Neu"}, {ID: 11, Name: "Triaged"}, {ID: 12, Name: "Erledigt", IsClosed: true}, {ID: 13, Name: "Waiting"},
	}
	got := DefaultTransformation(trackers, statuses)
	assert.Equal(t, []TypeRule{{Tracker: &Ref{ID: 2, Name: "BUG"}, Type: "BUG"}}, got.TypeRules,
		"only English default names are mapped, case-insensitively; others stay unmapped")
	std := []string{}
	for _, m := range got.StatusMappings {
		std = append(std, m.Std)
	}
	assert.Equal(t, []string{"TODO", "TODO", "DONE", "IN_PROGRESS"}, std,
		"a status that is the default of any tracker is TODO")
}

func TestDefaultTransformationWithoutMetadata(t *testing.T) {
	got := DefaultTransformation(nil, nil)
	assert.Empty(t, got.TypeRules)
	assert.NotNil(t, got.TypeRules, "arrays serialise as [] not null")
	assert.NotNil(t, got.StatusMappings)
	require.Nil(t, got.Validate())
}
