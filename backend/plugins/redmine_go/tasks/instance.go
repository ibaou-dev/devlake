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
	"encoding/json"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/log"
	"github.com/apache/devlake/plugins/redmine_go/mapping"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// loadMappingInstance reads the metadata the mapping rules are evaluated
// against: trackers with their default status, statuses, and custom fields. A
// field the key could not list (non-admin) is known by the name and id seen on
// collected issues, without its possible values.
func loadMappingInstance(db dal.Dal, connectionId uint64) (mapping.Instance, errors.Error) {
	var instance mapping.Instance
	where := dal.Where("connection_id = ?", connectionId)

	var trackers []models.RedmineGoTracker
	if err := db.All(&trackers, where); err != nil {
		return instance, err
	}
	for _, tracker := range trackers {
		info := models.TrackerInfo{ID: int(tracker.Id), Name: tracker.Name}
		if tracker.DefaultStatusId != 0 {
			info.DefaultStatus = &models.Ref{ID: int(tracker.DefaultStatusId), Name: tracker.DefaultStatusName}
		}
		instance.Trackers = append(instance.Trackers, info)
	}

	var statuses []models.RedmineGoStatus
	if err := db.All(&statuses, where); err != nil {
		return instance, err
	}
	for _, status := range statuses {
		instance.Statuses = append(instance.Statuses, models.StatusInfo{ID: int(status.Id), Name: status.Name, IsClosed: status.IsClosed})
	}

	fields := map[uint64]mapping.FieldInfo{}
	var observed []models.RedmineGoObservedField
	if err := db.All(&observed, where); err != nil {
		return instance, err
	}
	for _, field := range observed {
		fields[field.Id] = mapping.FieldInfo{ID: int(field.Id), Name: field.Name}
	}
	var customFields []models.RedmineGoCustomField
	if err := db.All(&customFields, where); err != nil {
		return instance, err
	}
	for _, field := range customFields {
		fields[field.Id] = mapping.FieldInfo{ID: int(field.Id), Name: field.Name, Labels: possibleValueLabels(field.PossibleValues)}
	}
	for _, field := range fields {
		instance.Fields = append(instance.Fields, field)
	}
	return instance, nil
}

// warnUnresolvedFields logs one warning for each configured custom field the
// instance cannot resolve. It runs once where the mapper is prepared, not per
// issue, so a misconfigured field costs one line per run.
func warnUnresolvedFields(logger log.Logger, scopeConfig *models.RedmineGoScopeConfig, mapper *mapping.Mapper) {
	for _, field := range mapper.UnresolvedFields() {
		logger.Warn(nil, "scope config %q (id %d): custom field %q (id %d) is unknown to the Redmine metadata and to the fields seen on collected issues, so its mapping has no effect",
			scopeConfig.Name, scopeConfig.ID, field.Name, field.ID)
	}
}

// mappingIssue converts a tool issue to the mapping input. A custom_fields
// element that cannot be read is skipped: the issue then maps as if the field
// were empty.
func mappingIssue(issue *models.RedmineGoIssue) *mapping.Issue {
	input := &mapping.Issue{
		TrackerID:      int(issue.TrackerId),
		TrackerName:    issue.TrackerName,
		StatusID:       int(issue.StatusId),
		StatusName:     issue.StatusName,
		StatusIsClosed: issue.StatusIsClosed,
		CategoryName:   issue.CategoryName,
	}
	if issue.CustomFieldsJson == "" {
		return input
	}
	var fields []apiCustomFieldValue
	if err := json.Unmarshal([]byte(issue.CustomFieldsJson), &fields); err != nil {
		return input
	}
	for _, field := range fields {
		input.Fields = append(input.Fields, mapping.FieldValues{
			ID: int(field.Id), Name: field.Name, Values: mapping.ParseFieldValues(field.Value),
		})
	}
	return input
}
