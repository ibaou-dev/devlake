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
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/utils"
)

// Validate checks the transformation against the scope-config contract and
// returns errors.BadInput naming every violation by its JSON path, so a client
// can fix them all in one round trip. It is a pure function of the receiver.
func (t *Transformation) Validate() errors.Error {
	v := &violations{}
	for i, rule := range t.TypeRules {
		v.typeRule(fmt.Sprintf("typeRules[%d]", i), rule)
	}
	for i, mapping := range t.StatusMappings {
		path := fmt.Sprintf("statusMappings[%d]", i)
		v.ref(path+".status", mapping.Status)
		v.oneOf(path+".std", mapping.Std, StatusStds)
	}
	for i, field := range t.Severity {
		v.valueMapField(fmt.Sprintf("severity[%d]", i), field, nil)
	}
	for i, field := range t.Environment {
		v.valueMapField(fmt.Sprintf("environment[%d]", i), field, EnvironmentClasses)
	}
	if t.Component != nil {
		v.component("component", *t.Component)
	}
	if len(v.list) == 0 {
		return nil
	}
	return errors.BadInput.New("invalid scope config: " + strings.Join(v.list, "; "))
}

type violations struct {
	list []string
}

func (v *violations) add(format string, args ...interface{}) {
	v.list = append(v.list, fmt.Sprintf(format, args...))
}

func (v *violations) oneOf(path, value string, allowed []string) {
	if !utils.StringsContains(allowed, value) {
		v.add("%s %q must be one of %s", path, value, strings.Join(allowed, ", "))
	}
}

func (v *violations) ref(path string, ref Ref) {
	if ref.ID < 1 {
		v.add("%s.id must be a positive integer", path)
	}
	if strings.TrimSpace(ref.Name) == "" {
		v.add("%s.name must not be empty", path)
	}
}

func (v *violations) typeRule(path string, rule TypeRule) {
	if rule.Tracker == nil && rule.When == nil {
		v.add("%s needs a tracker or a when condition", path)
	}
	if rule.Tracker != nil {
		v.ref(path+".tracker", *rule.Tracker)
	}
	if rule.When != nil {
		v.condition(path+".when", *rule.When)
	}
	v.oneOf(path+".type", rule.Type, IssueTypes)
}

func (v *violations) condition(path string, c Condition) {
	v.ref(path+".field", c.Field)
	v.oneOf(path+".op", c.Op, ConditionOps)
	if len(c.Values) == 0 {
		v.add("%s.values must list at least one value", path)
	}
}

// valueMapField checks a severity or environment entry; classes restricts the
// mapped targets when it is not nil.
func (v *violations) valueMapField(path string, f ValueMapField, classes []string) {
	v.ref(path+".field", f.Field)
	for i, entry := range f.Map {
		entryPath := fmt.Sprintf("%s.map[%d]", path, i)
		if entry.To == "" {
			v.add("%s.to must not be empty", entryPath)
		} else if classes != nil {
			v.oneOf(entryPath+".to", entry.To, classes)
		}
	}
}

func (v *violations) component(path string, c ComponentMapping) {
	v.oneOf(path+".source", c.Source, ComponentSources)
	if c.Source == ComponentSourceCustomField {
		if c.Field == nil {
			v.add("%s.field is required when %s.source is %q", path, path, ComponentSourceCustomField)
		} else {
			v.ref(path+".field", *c.Field)
		}
	}
}

// transformationKeys are the request-body keys that belong to Transformation.
var transformationKeys = []string{
	"typeRules", "statusMappings", "severity", "environment", "component",
	"reconcileSweep", "collectTimeEntries", "collectChangesets", "collectComments",
	"includePrivateNotes", "versionsAsSprints",
}

// CheckTransformationShape rejects a request body whose transformation keys
// carry unknown nested fields or values of the wrong JSON type. The lenient
// map decoding used to fill the model would silently drop a misspelt field,
// leaving a rule that does not mean what its author wrote.
func CheckTransformationShape(body map[string]interface{}) errors.Error {
	subset := make(map[string]interface{}, len(transformationKeys))
	for _, key := range transformationKeys {
		if value, ok := body[key]; ok {
			subset[key] = value
		}
	}
	raw, err := json.Marshal(subset)
	if err != nil {
		return errors.BadInput.Wrap(err, "invalid scope config")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&Transformation{}); err != nil {
		return errors.BadInput.New("invalid scope config: " + err.Error())
	}
	return nil
}
