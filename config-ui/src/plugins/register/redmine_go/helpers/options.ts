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
  RedmineCustomField,
  RedmineGoObservedValues,
  RedmineGoRef,
  RedmineIssueStatus,
  RedmineTracker,
} from '@/api/plugin/redmine_go';

// OptionSource tells where an option came from. Live options are read from
// Redmine through the proxy; observed ones come from the data DevLake has
// collected and are the only source for a non-admin key's custom fields.
export type OptionSource = 'live' | 'observed';

export type TrackerOption = RedmineGoRef & {
  // id of the tracker's default status, known for live trackers only
  defaultStatusId?: number;
  source: OptionSource;
};

export type StatusOption = RedmineGoRef & {
  isClosed: boolean;
  source: OptionSource;
};

export type FieldValueOption = {
  value: string;
  label: string;
  source: OptionSource;
};

export type CustomFieldOption = RedmineGoRef & {
  fieldFormat: string;
  multiple: boolean;
  values: FieldValueOption[];
  source: OptionSource;
};

const byName = <T extends { name: string; id: number }>(a: T, b: T) => a.name.localeCompare(b.name) || a.id - b.id;

// mergeTrackers unions the live trackers with the observed ones. A live
// answer wins for an id present in both; observed-only trackers (renamed or
// deleted in Redmine since collection) follow in name order.
export const mergeTrackers = (live: RedmineTracker[] | null, observed: RedmineGoRef[]): TrackerOption[] => {
  const liveOptions: TrackerOption[] = (live ?? []).map((tracker) => ({
    id: tracker.id,
    name: tracker.name,
    defaultStatusId: tracker.default_status?.id,
    source: 'live',
  }));
  const seen = new Set(liveOptions.map((tracker) => tracker.id));
  const observedOnly: TrackerOption[] = observed
    .filter((tracker) => !seen.has(tracker.id))
    .map((tracker) => ({ id: tracker.id, name: tracker.name, source: 'observed' as const }))
    .sort(byName);
  return [...liveOptions, ...observedOnly];
};

// mergeStatuses unions live and observed statuses the same way as
// mergeTrackers.
export const mergeStatuses = (
  live: RedmineIssueStatus[] | null,
  observed: RedmineGoObservedValues['statuses'],
): StatusOption[] => {
  const liveOptions: StatusOption[] = (live ?? []).map((status) => ({
    id: status.id,
    name: status.name,
    isClosed: !!status.is_closed,
    source: 'live',
  }));
  const seen = new Set(liveOptions.map((status) => status.id));
  const observedOnly: StatusOption[] = observed
    .filter((status) => !seen.has(status.id))
    .map((status) => ({ id: status.id, name: status.name, isClosed: !!status.isClosed, source: 'observed' as const }))
    .sort(byName);
  return [...liveOptions, ...observedOnly];
};

// mergeValueOptions returns possible_values ∪ observed values, keyed by the
// raw value. Redmine's label wins when both sides know the value; values only
// seen in collected data (registry keys, user ids, retired list entries) are
// appended in the order they were observed.
export const mergeValueOptions = (
  possible: Array<{ value: string; label?: string }> = [],
  observed: Array<{ value: string; label?: string }> = [],
): FieldValueOption[] => {
  const options = new Map<string, FieldValueOption>();
  possible.forEach((item) => {
    if (!options.has(item.value)) {
      options.set(item.value, { value: item.value, label: item.label || item.value, source: 'live' });
    }
  });
  observed.forEach((item) => {
    if (!options.has(item.value)) {
      options.set(item.value, { value: item.value, label: item.label || item.value, source: 'observed' });
    }
  });
  return [...options.values()];
};

// mergeCustomFields unions the issue custom fields of a live
// `custom_fields.json` answer (null when the call failed, as it does for a
// non-admin key) with the observed ones. Values of a field present on both
// sides are merged with mergeValueOptions.
export const mergeCustomFields = (
  live: RedmineCustomField[] | null,
  observed: RedmineGoObservedValues['customFields'],
): CustomFieldOption[] => {
  const observedById = new Map(observed.map((field) => [field.id, field]));
  const liveFields = (live ?? []).filter((field) => field.customized_type === 'issue');
  const liveOptions: CustomFieldOption[] = liveFields.map((field) => ({
    id: field.id,
    name: field.name,
    fieldFormat: field.field_format,
    multiple: !!field.multiple,
    values: mergeValueOptions(field.possible_values, observedById.get(field.id)?.values),
    source: 'live',
  }));
  const seen = new Set(liveOptions.map((field) => field.id));
  const observedOnly: CustomFieldOption[] = observed
    .filter((field) => !seen.has(field.id))
    .map((field) => ({
      id: field.id,
      name: field.name,
      fieldFormat: field.fieldFormat,
      multiple: !!field.multiple,
      values: mergeValueOptions([], field.values),
      source: 'observed' as const,
    }))
    .sort(byName);
  return [...liveOptions, ...observedOnly];
};

// valueOptionLabel renders a value with its label, as `Critical (1)`, and as
// the bare value when the label adds nothing.
export const valueOptionLabel = (option: { value: string; label?: string }): string =>
  option.label && option.label !== option.value ? `${option.label} (${option.value})` : option.value;
