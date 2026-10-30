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

package api

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	"github.com/apache/devlake/plugins/redmine_go/mapping"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// maxObservedValuesPerField bounds the values seen in issues that one custom
// field reports, most frequent first, so a field that holds free text cannot
// bloat the answer. Possible values from the field definition are never cut.
const maxObservedValuesPerField = 200

// freeTextFormats are the custom field formats whose values are not a choice:
// listing what happened to be typed into them helps nobody map anything.
var freeTextFormats = map[string]bool{
	"string": true, "text": true, "int": true, "float": true, "date": true, "link": true,
}

// ObservedRef is an id and a name.
type ObservedRef struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

// ObservedStatus is an issue status with its closed flag.
type ObservedStatus struct {
	ID       uint64 `json:"id"`
	Name     string `json:"name"`
	IsClosed bool   `json:"isClosed"`
}

// ObservedValue is one value of a custom field as Redmine stores it, with the
// label to show for it: the raw value itself when no label is known.
type ObservedValue struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// ObservedField is a custom field with the values collected for it.
type ObservedField struct {
	ID          uint64          `json:"id"`
	Name        string          `json:"name"`
	FieldFormat string          `json:"fieldFormat"`
	Multiple    bool            `json:"multiple"`
	Values      []ObservedValue `json:"values"`
}

// ObservedValues is what the collected data of a connection offers the mapping
// form. Every array is present, empty before the first collection.
type ObservedValues struct {
	Trackers     []ObservedRef    `json:"trackers"`
	Statuses     []ObservedStatus `json:"statuses"`
	CustomFields []ObservedField  `json:"customFields"`
	Categories   []ObservedRef    `json:"categories"`
}

// observedSources are the tool rows an ObservedValues is built from.
type observedSources struct {
	trackers   []models.RedmineGoTracker
	statuses   []models.RedmineGoStatus
	fields     []models.RedmineGoCustomField
	observed   []models.RedmineGoObservedField
	categories []models.RedmineGoIssueCategory
	// seen counts, per custom field id, the values found on collected issues.
	seen map[uint64]map[string]int
}

// addIssueFields counts the values of one issue's custom_fields JSON. An
// unreadable document counts nothing.
func (s *observedSources) addIssueFields(customFieldsJson string) {
	if customFieldsJson == "" {
		return
	}
	var fields []struct {
		ID    uint64          `json:"id"`
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal([]byte(customFieldsJson), &fields); err != nil {
		return
	}
	for _, field := range fields {
		for _, value := range mapping.ParseFieldValues(field.Value) {
			if s.seen == nil {
				s.seen = map[uint64]map[string]int{}
			}
			if s.seen[field.ID] == nil {
				s.seen[field.ID] = map[string]int{}
			}
			s.seen[field.ID][value]++
		}
	}
}

// build assembles the answer. A custom field is known from its definition
// (administrator keys) and from the issues it was seen on, which also names it
// for any other key. Its values are the definition's possible values, labelled,
// followed by the values seen on issues that the definition does not list, such
// as user ids or registry keys, most frequent first. Fields with a free-text
// format list no seen values.
func (s *observedSources) build() ObservedValues {
	out := ObservedValues{
		Trackers:     make([]ObservedRef, 0, len(s.trackers)),
		Statuses:     make([]ObservedStatus, 0, len(s.statuses)),
		CustomFields: []ObservedField{},
		Categories:   []ObservedRef{},
	}
	for _, tracker := range s.trackers {
		out.Trackers = append(out.Trackers, ObservedRef{ID: tracker.Id, Name: tracker.Name})
	}
	sort.Slice(out.Trackers, func(i, j int) bool { return out.Trackers[i].ID < out.Trackers[j].ID })
	for _, status := range s.statuses {
		out.Statuses = append(out.Statuses, ObservedStatus{ID: status.Id, Name: status.Name, IsClosed: status.IsClosed})
	}
	sort.Slice(out.Statuses, func(i, j int) bool { return out.Statuses[i].ID < out.Statuses[j].ID })

	categories := map[uint64]string{}
	for _, category := range s.categories {
		categories[category.Id] = category.Name
	}
	for id, name := range categories {
		out.Categories = append(out.Categories, ObservedRef{ID: id, Name: name})
	}
	sort.Slice(out.Categories, func(i, j int) bool {
		if out.Categories[i].Name != out.Categories[j].Name {
			return out.Categories[i].Name < out.Categories[j].Name
		}
		return out.Categories[i].ID < out.Categories[j].ID
	})

	fields := map[uint64]*ObservedField{}
	for _, field := range s.observed {
		fields[field.Id] = &ObservedField{ID: field.Id, Name: field.Name, Multiple: field.Multiple, Values: []ObservedValue{}}
	}
	possible := map[uint64]map[string]bool{}
	for _, def := range s.fields {
		field := fields[def.Id]
		if field == nil {
			field = &ObservedField{ID: def.Id, Values: []ObservedValue{}}
			fields[def.Id] = field
		}
		field.Name, field.FieldFormat = def.Name, def.FieldFormat
		field.Multiple = field.Multiple || def.Multiple
		possible[def.Id] = map[string]bool{}
		for _, value := range parsePossibleValues(def.PossibleValues) {
			possible[def.Id][value.Value] = true
			field.Values = append(field.Values, value)
		}
	}
	for id, counts := range s.seen {
		field := fields[id]
		if field == nil {
			field = &ObservedField{ID: id, Values: []ObservedValue{}}
			fields[id] = field
		}
		if freeTextFormats[field.FieldFormat] {
			continue
		}
		field.Values = append(field.Values, mostSeen(counts, possible[id])...)
	}
	for _, field := range fields {
		out.CustomFields = append(out.CustomFields, *field)
	}
	sort.Slice(out.CustomFields, func(i, j int) bool { return out.CustomFields[i].ID < out.CustomFields[j].ID })
	return out
}

// parsePossibleValues reads a field's possible_values JSON, an array of
// {"value","label"}; a missing label is the value itself.
func parsePossibleValues(raw string) []ObservedValue {
	if raw == "" {
		return nil
	}
	var values []ObservedValue
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	for i := range values {
		if values[i].Label == "" {
			values[i].Label = values[i].Value
		}
	}
	return values
}

// mostSeen returns the most frequent values of counts that are not in known,
// at most maxObservedValuesPerField, ties broken by value.
func mostSeen(counts map[string]int, known map[string]bool) []ObservedValue {
	values := make([]string, 0, len(counts))
	for value := range counts {
		if !known[value] {
			values = append(values, value)
		}
	}
	sort.Slice(values, func(i, j int) bool {
		if counts[values[i]] != counts[values[j]] {
			return counts[values[i]] > counts[values[j]]
		}
		return values[i] < values[j]
	})
	if len(values) > maxObservedValuesPerField {
		values = values[:maxObservedValuesPerField]
	}
	out := make([]ObservedValue, len(values))
	for i, value := range values {
		out[i] = ObservedValue{Value: value, Label: value}
	}
	return out
}

// LoadObservedValues reads what a connection's collected tool tables offer
// the mapping form. The issues are streamed, so a large instance costs time
// but no memory.
func LoadObservedValues(db dal.Dal, connectionId uint64) (ObservedValues, errors.Error) {
	var sources observedSources
	where := dal.Where("connection_id = ?", connectionId)
	for _, target := range []interface{}{
		&sources.trackers, &sources.statuses, &sources.fields, &sources.observed, &sources.categories,
	} {
		if err := db.All(target, where); err != nil {
			return ObservedValues{}, err
		}
	}
	cursor, err := db.Cursor(
		dal.Select("custom_fields_json"),
		dal.From(&models.RedmineGoIssue{}),
		where,
	)
	if err != nil {
		return ObservedValues{}, err
	}
	defer cursor.Close()
	for cursor.Next() {
		var row struct{ CustomFieldsJson string }
		if err := db.Fetch(cursor, &row); err != nil {
			return ObservedValues{}, errors.Default.Wrap(err, "error reading issue custom fields")
		}
		sources.addIssueFields(row.CustomFieldsJson)
	}
	return sources.build(), nil
}

// GetObservedValues return the values collected for a connection
// @Summary return the trackers, statuses, custom field values and categories collected for a connection
// @Description Built from the connection's collected tool tables: the trackers, statuses and categories, and for each custom field its possible values plus the values seen on collected issues (labelled when the field definition is known; user ids and registry keys appear only here). It is the config UI's source of options when the Redmine key cannot read custom field definitions. Every array is empty before the first collection.
// @Tags plugins/redmine_go
// @Param connectionId path int true "connection ID"
// @Success 200  {object} ObservedValues
// @Failure 400  {object} shared.ApiBody "Bad Request"
// @Failure 404  {object} shared.ApiBody "Connection not found"
// @Failure 500  {object} shared.ApiBody "Internal Error"
// @Router /plugins/redmine_go/connections/{connectionId}/observed-values [GET]
func GetObservedValues(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	connection, err := dsHelper.ConnApi.FindByPk(input)
	if err != nil {
		return nil, err
	}
	values, err := LoadObservedValues(basicRes.GetDal(), connection.ID)
	if err != nil {
		return nil, err
	}
	return &plugin.ApiResourceOutput{Body: values, Status: http.StatusOK}, nil
}
