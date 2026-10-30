/*
 * Licensed to the Apache Software Foundation (ASF) under one or more
 * contributor license agreements.  See the NOTICE file distributed with
 * this work for additional information regarding copyright ownership.
 * The ASF licenses this file to You under the Apache License, Version 2.0
 * (the "License"); you may not use this file except in compliance with
 * the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */

import type {
  RedmineGoEnvironmentClass,
  RedmineGoRef,
  RedmineGoValueMapEntry,
  RedmineGoValueMapField,
} from '@/api/plugin/redmine_go';

import type { FieldValueOption } from './options';

export const ENVIRONMENT_CLASSES: RedmineGoEnvironmentClass[] = ['PRODUCTION', 'STAGING', 'TESTING', 'DEVELOPMENT'];

// addValueMapField appends a field to a severity or environment list. The
// list is ordered and the first field with a value wins, so the new field
// goes last; a field already in the list is not added twice.
export const addValueMapField = (list: RedmineGoValueMapField[], field: RedmineGoRef): RedmineGoValueMapField[] =>
  list.some((entry) => entry.field.id === field.id)
    ? list
    : [...list, { field: { id: field.id, name: field.name }, map: [] }];

export type ValueMapRow = {
  value: string;
  label: string;
  // the mapped target, absent while the value has no entry
  to?: string;
  // false for an entry whose value is neither listed by Redmine nor observed
  known: boolean;
};

// buildValueMapRows lists every known value of a field (Redmine's
// possible_values ∪ observed values) with its entry, then the entries whose
// value is unknown, so a mapping for an unseen value stays visible.
export const buildValueMapRows = (entries: RedmineGoValueMapEntry[], options: FieldValueOption[]): ValueMapRow[] => {
  const stored = new Map(entries.map((entry) => [entry.value, entry]));
  const rows: ValueMapRow[] = options.map((option) => ({
    value: option.value,
    label: stored.get(option.value)?.label ?? option.label,
    to: stored.get(option.value)?.to,
    known: true,
  }));
  const known = new Set(options.map((option) => option.value));
  entries
    .filter((entry) => !known.has(entry.value))
    .forEach((entry) =>
      rows.push({ value: entry.value, label: entry.label ?? entry.value, to: entry.to, known: false }),
    );
  return rows;
};

// setValueMapEntry sets, or with an empty `to` removes, the entry of a value.
// The label is stored only when it differs from the raw value: it is for
// reading, the match always uses the value.
export const setValueMapEntry = (
  entries: RedmineGoValueMapEntry[],
  value: string,
  label: string | undefined,
  to: string | undefined,
): RedmineGoValueMapEntry[] => {
  const target = to?.trim();
  if (!target) {
    return entries.filter((entry) => entry.value !== value);
  }
  const entry: RedmineGoValueMapEntry = label && label !== value ? { value, label, to: target } : { value, to: target };
  return entries.some((it) => it.value === value)
    ? entries.map((it) => (it.value === value ? entry : it))
    : [...entries, entry];
};
