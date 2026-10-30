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
	"testing"

	"github.com/apache/devlake/plugins/redmine_go/mapping"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHoursToMinutes(t *testing.T) {
	assert.Nil(t, hoursToMinutes(nil))
	for hours, want := range map[float64]int64{0: 0, 1.5: 90, 0.25: 15, 2.333: 140, 0.01: 1} {
		hours := hours
		got := hoursToMinutes(&hours)
		require.NotNil(t, got)
		assert.Equal(t, want, *got, "%v h", hours)
	}
}

func TestChangelogHelpers(t *testing.T) {
	assert.Equal(t, "status_id", changelogFieldId(detailOf("attr", "status_id")))
	assert.Equal(t, "cf_12", changelogFieldId(detailOf("cf", "12")))
	assert.Equal(t, "relation_blocks", changelogFieldId(detailOf("relation", "blocks")))
	assert.Equal(t, "attachment_5", changelogFieldId(detailOf("attachment", "5")))
}

func detailOf(property, name string) *models.RedmineGoJournalDetail {
	return &models.RedmineGoJournalDetail{Property: property, Name: name}
}

func TestStandardStatusFollowsTheMapper(t *testing.T) {
	instance := mapping.Instance{
		Trackers: []models.TrackerInfo{{ID: 1, Name: "Bug", DefaultStatus: &models.Ref{ID: 1, Name: "New"}}},
		Statuses: []models.StatusInfo{{ID: 1, Name: "New"}, {ID: 2, Name: "In Progress"}, {ID: 5, Name: "Closed", IsClosed: true}},
	}
	mapper := mapping.NewMapper(nil, instance)
	assert.Equal(t, "TODO", standardStatus(mapper, "1", "New"))
	assert.Equal(t, "IN_PROGRESS", standardStatus(mapper, "2", "In Progress"))
	assert.Equal(t, "DONE", standardStatus(mapper, "5", "Closed"))
	assert.Equal(t, "", standardStatus(mapper, "", ""), "a change with no status on that side stays empty")
	assert.Equal(t, "", standardStatus(mapper, "oops", "oops"))
}
