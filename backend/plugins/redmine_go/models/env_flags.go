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
	"strconv"
	"strings"
)

// EnvFlags names the environment variable that can force each toggle off for
// every scope of the deployment.
var EnvFlags = map[Toggle]string{
	ToggleReconcileSweep:      "REDMINE_GO_RECONCILE_SWEEP",
	ToggleCollectTimeEntries:  "REDMINE_GO_COLLECT_TIME_ENTRIES",
	ToggleCollectChangesets:   "REDMINE_GO_COLLECT_CHANGESETS",
	ToggleCollectComments:     "REDMINE_GO_COLLECT_COMMENTS",
	ToggleIncludePrivateNotes: "REDMINE_GO_INCLUDE_PRIVATE_NOTES",
	ToggleVersionsAsSprints:   "REDMINE_GO_VERSIONS_AS_SPRINTS",
}

// envForcesOff reports whether an environment value switches a behaviour off:
// it is a boolean that parses to false ("false", "0", "f", any case). An unset,
// empty, true or unreadable value forces nothing.
func envForcesOff(value string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && !parsed
}

// ApplyEnvFlags lets the environment override the toggles: a flag set to false
// forces its toggle off over whatever the scope config says, and no value can
// force a toggle on, so an operator can disable a costly or sensitive
// behaviour for every scope but never enable one the scope config left off.
// getenv looks up one variable and returns "" when it is unset. The
// transformation is changed in place; the toggles it replaces are fresh
// values, so a copy of the struct is unaffected.
func (t *Transformation) ApplyEnvFlags(getenv func(name string) string) {
	off := false
	for toggle, name := range EnvFlags {
		if !envForcesOff(getenv(name)) {
			continue
		}
		switch toggle {
		case ToggleReconcileSweep:
			t.ReconcileSweep = &off
		case ToggleCollectTimeEntries:
			t.CollectTimeEntries = &off
		case ToggleCollectChangesets:
			t.CollectChangesets = &off
		case ToggleCollectComments:
			t.CollectComments = &off
		case ToggleIncludePrivateNotes:
			t.IncludePrivateNotes = &off
		case ToggleVersionsAsSprints:
			t.VersionsAsSprints = &off
		}
	}
}
