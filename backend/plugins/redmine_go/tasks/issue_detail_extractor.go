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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// apiJournal is one journal of an issue detail.
type apiJournal struct {
	Id           uint64             `json:"id"`
	User         *apiRef            `json:"user"`
	Notes        string             `json:"notes"`
	CreatedOn    time.Time          `json:"created_on"`
	UpdatedOn    *time.Time         `json:"updated_on"`
	PrivateNotes bool               `json:"private_notes"`
	Details      []apiJournalDetail `json:"details"`
}

// apiJournalDetail is one field change of a journal.
type apiJournalDetail struct {
	Property string     `json:"property"`
	Name     string     `json:"name"`
	OldValue flexString `json:"old_value"`
	NewValue flexString `json:"new_value"`
}

// journalExtractorConfig is the subtask config of the journal extractor.
// LookupDigest fingerprints the metadata journal values are translated
// through, so a renamed status or a new version re-extracts every journal
// from the raw rows, with no new API call.
type journalExtractorConfig struct {
	Version      int
	LookupDigest string
}

// ExtractIssueJournalsMeta describes the journal extractor.
var ExtractIssueJournalsMeta = plugin.SubTaskMeta{
	Name:             "extractIssueJournals",
	EntryPoint:       ExtractIssueJournals,
	EnabledByDefault: true,
	Description:      "Extract journals into _tool_redmine_go_journals and one row per field change into _tool_redmine_go_journal_details, with ids translated to names",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET, plugin.DOMAIN_TYPE_CROSS},
	DependencyTables: []string{rawTableName(RAW_ISSUE_DETAILS_TABLE)},
	ProductTables: []string{
		models.RedmineGoJournal{}.TableName(),
		models.RedmineGoJournalDetail{}.TableName(),
		models.RedmineGoAccount{}.TableName(),
	},
}

var _ plugin.SubTaskEntryPoint = ExtractIssueJournals

// ExtractIssueJournals extracts the journals of the collected issue details.
// Journals are core data, not a toggled option.
//
// An issue's journals are replaced as a whole each time it is extracted, so
// an edited note or a re-collected issue never leaves duplicates, and only the
// newest raw row of an issue in the window expands: an older version would
// bring back rows the newer one no longer has.
func ExtractIssueJournals(taskCtx plugin.SubTaskContext) errors.Error {
	data := taskCtx.GetData().(*RedmineGoTaskData)
	connectionId := data.Options.ConnectionId

	lookup, digest, err := loadJournalLookup(taskCtx.GetDal(), connectionId)
	if err != nil {
		return err
	}
	return extractIssueDetails(taskCtx, detailExtraction{
		Config:   journalExtractorConfig{Version: extractorVersion, LookupDigest: digest},
		Replaced: []interface{}{&models.RedmineGoJournalDetail{}, &models.RedmineGoJournal{}},
		Rows: func(item *apiIssue) []interface{} {
			return journalRows(connectionId, item, lookup)
		},
	})
}

// journalRows maps the journals of an issue detail to tool rows: the journal,
// one row per field change, and the author as an account.
func journalRows(connectionId uint64, item *apiIssue, lookup *journalLookup) []interface{} {
	var results []interface{}
	for _, journal := range item.Journals {
		row := &models.RedmineGoJournal{
			ConnectionId:     connectionId,
			Id:               journal.Id,
			IssueId:          item.Id,
			Notes:            journal.Notes,
			PrivateNotes:     journal.PrivateNotes,
			RedmineCreatedOn: journal.CreatedOn.UTC(),
		}
		if journal.UpdatedOn != nil {
			updated := journal.UpdatedOn.UTC()
			row.RedmineUpdatedOn = &updated
		}
		if journal.User != nil {
			row.UserId, row.UserName = journal.User.Id, journal.User.Name
		}
		results = append(results, row)
		if account := accountRow(connectionId, journal.User); account != nil {
			results = append(results, account)
		}

		changes := make([]journalChange, len(journal.Details))
		for i, detail := range journal.Details {
			changes[i] = journalChange{
				Property: detail.Property, Name: detail.Name,
				OldValue: string(detail.OldValue), NewValue: string(detail.NewValue),
			}
		}
		keys := detailKeys(changes)
		for i, change := range changes {
			translated := lookup.translate(change)
			results = append(results, &models.RedmineGoJournalDetail{
				ConnectionId:     connectionId,
				JournalId:        journal.Id,
				DetailKey:        keys[i],
				IssueId:          item.Id,
				Position:         i,
				UserId:           row.UserId,
				UserName:         row.UserName,
				RedmineCreatedOn: row.RedmineCreatedOn,
				Property:         change.Property,
				Name:             change.Name,
				FieldName:        translated.FieldName,
				OldValue:         change.OldValue,
				NewValue:         change.NewValue,
				OldLabel:         translated.OldLabel,
				NewLabel:         translated.NewLabel,
			})
		}
	}
	return results
}

// latestDetailVersions maps issue id to the newest raw row of it over the
// window the extractor is about to walk. Raw rows arrive in id order, so the
// newest has the highest id. The issue id is read from the row's input column,
// which holds just `{"id": n}`, so the issue bodies are not loaded.
func latestDetailVersions(db dal.Dal, extractor *helper.StatefulApiExtractor[apiIssue]) (map[uint64]uint64, errors.Error) {
	latest := map[uint64]uint64{}
	table := extractor.GetRawDataTable()
	if !db.HasTable(table) {
		return latest, nil
	}
	clauses := []dal.Clause{
		dal.Select("id, input"),
		dal.From(table),
		dal.Where("params = ?", extractor.GetRawDataParams()),
		dal.Orderby("id ASC"),
	}
	if extractor.IsIncremental() {
		if since := extractor.GetSince(); since != nil {
			clauses = append(clauses, dal.Where("created_at >= ?", *since))
		}
	}
	clauses = append(clauses, dal.Where("created_at < ?", *extractor.GetUntil()))
	cursor, err := db.Cursor(clauses...)
	if err != nil {
		return nil, err
	}
	defer cursor.Close()
	for cursor.Next() {
		row := &helper.RawData{}
		if err := db.Fetch(cursor, row); err != nil {
			return nil, errors.Default.Wrap(err, "error scanning raw issue detail versions")
		}
		var input issueDetailInput
		if err := json.Unmarshal(row.Input, &input); err != nil || input.Id == 0 {
			// Left out of the map, Extract keeps such a row on the safe path:
			// it expands like any row.
			continue
		}
		latest[input.Id] = row.ID
	}
	return latest, nil
}

// loadJournalLookup reads the metadata and account tables journals are
// translated through, and returns it with a digest of the part whose change
// must re-extract the journals.
func loadJournalLookup(db dal.Dal, connectionId uint64) (*journalLookup, string, errors.Error) {
	lookup := &journalLookup{}
	var err errors.Error
	for _, source := range []struct {
		target *map[uint64]string
		model  interface{}
	}{
		{&lookup.Trackers, &models.RedmineGoTracker{}},
		{&lookup.Statuses, &models.RedmineGoStatus{}},
		{&lookup.Priorities, &models.RedmineGoPriority{}},
		{&lookup.Versions, &models.RedmineGoVersion{}},
		{&lookup.Categories, &models.RedmineGoIssueCategory{}},
		{&lookup.Accounts, &models.RedmineGoAccount{}},
	} {
		if *source.target, err = idNames(db, "id, name", source.model, connectionId); err != nil {
			return nil, "", err
		}
	}
	// Scope names first; the project names collected issues carry cover the
	// projects issues were moved from or to.
	if lookup.Projects, err = idNames(db, "id, name", &models.RedmineGoProject{}, connectionId); err != nil {
		return nil, "", err
	}
	issueProjects, err := idNames(db, "DISTINCT project_id AS id, project_name AS name", &models.RedmineGoIssue{}, connectionId)
	if err != nil {
		return nil, "", err
	}
	for id, name := range issueProjects {
		if lookup.Projects[id] == "" {
			lookup.Projects[id] = name
		}
	}

	lookup.Fields = map[uint64]fieldMeta{}
	var observed []models.RedmineGoObservedField
	if err = db.All(&observed, dal.Where("connection_id = ?", connectionId)); err != nil {
		return nil, "", err
	}
	for _, field := range observed {
		lookup.Fields[field.Id] = fieldMeta{Name: field.Name}
	}
	var fields []models.RedmineGoCustomField
	if err = db.All(&fields, dal.Where("connection_id = ?", connectionId)); err != nil {
		return nil, "", err
	}
	for _, field := range fields {
		lookup.Fields[field.Id] = fieldMeta{Name: field.Name, Format: field.FieldFormat, Labels: possibleValueLabels(field.PossibleValues)}
	}

	encoded, convErr := json.Marshal(lookup)
	if convErr != nil {
		return nil, "", errors.Default.Wrap(convErr, "could not fingerprint the journal lookup")
	}
	sum := sha256.Sum256(encoded)
	return lookup, hex.EncodeToString(sum[:]), nil
}

// idNames reads an id-to-name map of one connection from a tool table.
func idNames(db dal.Dal, columns string, model interface{}, connectionId uint64) (map[uint64]string, errors.Error) {
	cursor, err := db.Cursor(dal.Select(columns), dal.From(model), dal.Where("connection_id = ?", connectionId))
	if err != nil {
		return nil, err
	}
	defer cursor.Close()
	names := map[uint64]string{}
	for cursor.Next() {
		var row struct {
			Id   uint64
			Name string
		}
		if err := db.Fetch(cursor, &row); err != nil {
			return nil, errors.Default.Wrap(err, "error reading names")
		}
		names[row.Id] = row.Name
	}
	return names, nil
}

// possibleValueLabels reads a custom field's possible_values JSON, an array of
// {"value","label"}, into a value-to-label map; anything unreadable gives nil.
func possibleValueLabels(raw string) map[string]string {
	if raw == "" {
		return nil
	}
	var values []struct {
		Value string `json:"value"`
		Label string `json:"label"`
	}
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil
	}
	labels := make(map[string]string, len(values))
	for _, value := range values {
		if value.Label != "" {
			labels[value.Value] = value.Label
		}
	}
	return labels
}
