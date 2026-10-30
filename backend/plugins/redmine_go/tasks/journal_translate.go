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
	"fmt"
	"strconv"
	"strings"
)

// journalLookup resolves the ids journals mention to names. Every map is keyed
// by the Redmine id; a missing key means the name is unknown, and the raw id is
// kept. Accounts is excluded from the digest because users appear as soon as
// they act, which must not force a re-extraction of every journal.
type journalLookup struct {
	Trackers   map[uint64]string    `json:"trackers"`
	Statuses   map[uint64]string    `json:"statuses"`
	Priorities map[uint64]string    `json:"priorities"`
	Versions   map[uint64]string    `json:"versions"`
	Categories map[uint64]string    `json:"categories"`
	Projects   map[uint64]string    `json:"projects"`
	Fields     map[uint64]fieldMeta `json:"fields"`
	Accounts   map[uint64]string    `json:"-"`
}

// fieldMeta is what a journal needs to know of a custom field: its name, its
// format, and the label of each stored value when the key could list them.
type fieldMeta struct {
	Name   string            `json:"name"`
	Format string            `json:"format"`
	Labels map[string]string `json:"labels"`
}

// journalChange is one journal detail as Redmine reports it.
type journalChange struct {
	Property string
	Name     string
	OldValue string
	NewValue string
}

// translatedChange is a journal detail in terms a changelog reader knows: the
// normalised field name and the old and new values as labels.
type translatedChange struct {
	FieldName string
	OldLabel  string
	NewLabel  string
}

// attrFieldNames normalises the attribute names of `attr` details. An
// attribute not listed keeps its Redmine name without a trailing `_id`.
var attrFieldNames = map[string]string{
	"status_id":        "status",
	"assigned_to_id":   "assignee",
	"tracker_id":       "tracker",
	"priority_id":      "priority",
	"fixed_version_id": "fix_version",
	"project_id":       "project",
	"parent_id":        "parent",
	"child_id":         "child",
	"category_id":      "category",
	"author_id":        "author",
	"subject":          "summary",
}

// translate names the field of a journal detail and labels its old and new
// values. It is pure: the same change and lookup always give the same result.
func (lk *journalLookup) translate(change journalChange) translatedChange {
	switch change.Property {
	case "attr":
		return lk.translateAttr(change)
	case "cf":
		return lk.translateCustomField(change)
	case "relation":
		return translatedChange{
			FieldName: "relation_" + change.Name,
			OldLabel:  issueRef(change.OldValue),
			NewLabel:  issueRef(change.NewValue),
		}
	case "attachment":
		return translatedChange{FieldName: "attachment", OldLabel: change.OldValue, NewLabel: change.NewValue}
	default:
		name := change.Name
		if name == "" {
			name = change.Property
		}
		return translatedChange{FieldName: name, OldLabel: change.OldValue, NewLabel: change.NewValue}
	}
}

func (lk *journalLookup) translateAttr(change journalChange) translatedChange {
	fieldName, ok := attrFieldNames[change.Name]
	if !ok {
		fieldName = strings.TrimSuffix(change.Name, "_id")
	}
	var names map[uint64]string
	switch change.Name {
	case "status_id":
		names = lk.Statuses
	case "assigned_to_id", "author_id":
		names = lk.Accounts
	case "tracker_id":
		names = lk.Trackers
	case "priority_id":
		names = lk.Priorities
	case "fixed_version_id":
		names = lk.Versions
	case "project_id":
		names = lk.Projects
	case "category_id":
		names = lk.Categories
	case "parent_id", "child_id":
		return translatedChange{FieldName: fieldName, OldLabel: issueRef(change.OldValue), NewLabel: issueRef(change.NewValue)}
	default:
		return translatedChange{FieldName: fieldName, OldLabel: change.OldValue, NewLabel: change.NewValue}
	}
	return translatedChange{
		FieldName: fieldName,
		OldLabel:  nameOf(names, change.OldValue),
		NewLabel:  nameOf(names, change.NewValue),
	}
}

func (lk *journalLookup) translateCustomField(change journalChange) translatedChange {
	id, err := strconv.ParseUint(change.Name, 10, 64)
	if err != nil {
		return translatedChange{FieldName: "cf_" + change.Name, OldLabel: change.OldValue, NewLabel: change.NewValue}
	}
	meta, known := lk.Fields[id]
	fieldName := meta.Name
	if !known || fieldName == "" {
		fieldName = "cf_" + change.Name
	}
	label := func(value string) string {
		if value == "" || !known {
			return value
		}
		if text, ok := meta.Labels[value]; ok && text != "" {
			return text
		}
		switch meta.Format {
		case "user":
			return nameOf(lk.Accounts, value)
		case "version":
			return nameOf(lk.Versions, value)
		}
		return value
	}
	return translatedChange{FieldName: fieldName, OldLabel: label(change.OldValue), NewLabel: label(change.NewValue)}
}

// nameOf returns the name behind a numeric id value, or the value itself when
// it is empty, not numeric, or unknown.
func nameOf(names map[uint64]string, value string) string {
	if value == "" {
		return ""
	}
	id, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return value
	}
	if name := names[id]; name != "" {
		return name
	}
	return value
}

// issueRef renders an issue id as `#123`; an empty or non-numeric value is
// returned unchanged.
func issueRef(value string) string {
	if value == "" {
		return ""
	}
	if _, err := strconv.ParseUint(value, 10, 64); err != nil {
		return value
	}
	return "#" + value
}

// detailKeys fingerprints each change of one journal, in order. The key is a
// hash of the change's content plus the number of identical changes before
// it, so it does not depend on the order Redmine lists the details in, and a
// journal that adds several values to one multi-value custom field (the same
// property and name, different values) gets distinct keys.
func detailKeys(changes []journalChange) []string {
	seen := make(map[string]int, len(changes))
	keys := make([]string, len(changes))
	for i, change := range changes {
		sum := sha256.Sum256([]byte(strings.Join([]string{change.Property, change.Name, change.OldValue, change.NewValue}, "\x00")))
		digest := hex.EncodeToString(sum[:8])
		keys[i] = fmt.Sprintf("%s-%d", digest, seen[digest])
		seen[digest]++
	}
	return keys
}
