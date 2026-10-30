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

package tasks

import (
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// SubtaskToggles names the scope-config toggle that gates each optional
// subtask, keyed by subtask name. The blueprint leaves a gated subtask out of
// the plan when its toggle is off, whether the scope config or a REDMINE_GO_*
// environment flag turned it off; subtasks absent from the map always run.
// includePrivateNotes gates no subtask: the comment convertor reads it.
var SubtaskToggles = map[string]models.Toggle{
	CollectIssueSweepMeta.Name:      models.ToggleReconcileSweep,
	ExtractIssueSweepMeta.Name:      models.ToggleReconcileSweep,
	CollectTimeEntriesMeta.Name:     models.ToggleCollectTimeEntries,
	ExtractTimeEntriesMeta.Name:     models.ToggleCollectTimeEntries,
	ConvertWorklogsMeta.Name:        models.ToggleCollectTimeEntries,
	ExtractIssueChangesetsMeta.Name: models.ToggleCollectChangesets,
	ConvertIssueCommitsMeta.Name:    models.ToggleCollectChangesets,
	ConvertIssueCommentsMeta.Name:   models.ToggleCollectComments,
	ConvertSprintsMeta.Name:         models.ToggleVersionsAsSprints,
}
