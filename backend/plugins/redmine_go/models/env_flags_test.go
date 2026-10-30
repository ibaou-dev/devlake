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
)

func envOf(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

var allToggles = []Toggle{
	ToggleReconcileSweep, ToggleCollectTimeEntries, ToggleCollectChangesets,
	ToggleCollectComments, ToggleIncludePrivateNotes, ToggleVersionsAsSprints,
}

func TestEveryToggleHasAnEnvFlag(t *testing.T) {
	assert.Equal(t, map[Toggle]string{
		ToggleReconcileSweep:      "REDMINE_GO_RECONCILE_SWEEP",
		ToggleCollectTimeEntries:  "REDMINE_GO_COLLECT_TIME_ENTRIES",
		ToggleCollectChangesets:   "REDMINE_GO_COLLECT_CHANGESETS",
		ToggleCollectComments:     "REDMINE_GO_COLLECT_COMMENTS",
		ToggleIncludePrivateNotes: "REDMINE_GO_INCLUDE_PRIVATE_NOTES",
		ToggleVersionsAsSprints:   "REDMINE_GO_VERSIONS_AS_SPRINTS",
	}, EnvFlags)
	for _, toggle := range allToggles {
		assert.Contains(t, EnvFlags, toggle)
	}
}

func TestEnvFlagFalseForcesEveryToggleOff(t *testing.T) {
	for _, toggle := range allToggles {
		for _, scopeValue := range []*bool{nil, new(true), new(false)} {
			t.Run(string(toggle), func(t *testing.T) {
				transformation := Transformation{}
				setToggle(&transformation, toggle, scopeValue)
				transformation.ApplyEnvFlags(envOf(map[string]string{EnvFlags[toggle]: "false"}))
				assert.False(t, transformation.ToggleEnabled(toggle))
			})
		}
	}
}

func TestEnvFlagNeverForcesAToggleOn(t *testing.T) {
	for _, toggle := range allToggles {
		for _, flag := range []string{"true", "TRUE", "1", "", "yes", "enabled", "garbage"} {
			off := Transformation{}
			setToggle(&off, toggle, new(false))
			off.ApplyEnvFlags(envOf(map[string]string{EnvFlags[toggle]: flag}))
			assert.False(t, off.ToggleEnabled(toggle), "%s=%q must not enable a toggle the scope config switched off", EnvFlags[toggle], flag)

			unset := Transformation{}
			unset.ApplyEnvFlags(envOf(map[string]string{EnvFlags[toggle]: flag}))
			assert.Equal(t, ToggleDefault(toggle), unset.ToggleEnabled(toggle), "%s=%q leaves the default alone", EnvFlags[toggle], flag)

			on := Transformation{}
			setToggle(&on, toggle, new(true))
			on.ApplyEnvFlags(envOf(map[string]string{EnvFlags[toggle]: flag}))
			assert.True(t, on.ToggleEnabled(toggle), "%s=%q does not override a scope config that is on", EnvFlags[toggle], flag)
		}
	}
}

func TestEnvFlagAcceptsTheBooleanSpellingsOfFalse(t *testing.T) {
	for _, flag := range []string{"false", "FALSE", "False", "0", "f", " false "} {
		transformation := Transformation{}
		transformation.ApplyEnvFlags(envOf(map[string]string{"REDMINE_GO_COLLECT_COMMENTS": flag}))
		assert.False(t, transformation.ToggleEnabled(ToggleCollectComments), flag)
	}
}

func TestEnvFlagsAreIndependent(t *testing.T) {
	transformation := Transformation{}
	transformation.ApplyEnvFlags(envOf(map[string]string{"REDMINE_GO_COLLECT_TIME_ENTRIES": "false"}))
	assert.False(t, transformation.ToggleEnabled(ToggleCollectTimeEntries))
	for _, toggle := range allToggles {
		if toggle != ToggleCollectTimeEntries {
			assert.Equal(t, ToggleDefault(toggle), transformation.ToggleEnabled(toggle), string(toggle))
		}
	}
}

func TestApplyEnvFlagsDoesNotChangeACopysSharedToggles(t *testing.T) {
	stored := Transformation{CollectTimeEntries: new(true)}
	planned := stored
	planned.ApplyEnvFlags(envOf(map[string]string{"REDMINE_GO_COLLECT_TIME_ENTRIES": "false"}))
	assert.False(t, planned.ToggleEnabled(ToggleCollectTimeEntries))
	assert.True(t, *stored.CollectTimeEntries, "the planner's copy must not rewrite the stored config")
}

func setToggle(t *Transformation, toggle Toggle, value *bool) {
	switch toggle {
	case ToggleReconcileSweep:
		t.ReconcileSweep = value
	case ToggleCollectTimeEntries:
		t.CollectTimeEntries = value
	case ToggleCollectChangesets:
		t.CollectChangesets = value
	case ToggleCollectComments:
		t.CollectComments = value
	case ToggleIncludePrivateNotes:
		t.IncludePrivateNotes = value
	case ToggleVersionsAsSprints:
		t.VersionsAsSprints = value
	}
}
