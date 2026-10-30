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

import type { RedmineGoValue } from '@/api/plugin/redmine_go';

import type { FieldValueOption } from './options';

// toConditionValues turns the raw strings a multi-select holds into stored
// values. The label of a value comes from the existing entry, else from the
// known option; a value neither knows (free entry) is stored without one.
// Matching always uses `value`, the label only makes the form readable.
export const toConditionValues = (
  selected: string[],
  current: RedmineGoValue[],
  options: FieldValueOption[],
): RedmineGoValue[] =>
  [...new Set(selected.map((value) => value.trim()).filter(Boolean))].map((value) => {
    const label = current.find((it) => it.value === value)?.label ?? options.find((it) => it.value === value)?.label;
    return label ? { value, label } : { value };
  });

// conditionOptions builds the select options of a condition's values: the
// known values of the field, plus the ones the rule already holds that are
// neither listed nor observed, so a saved free-entry value still shows.
export const conditionOptions = (
  options: FieldValueOption[],
  current: RedmineGoValue[],
): Array<{ value: string; label: string; unseen?: boolean }> => {
  const known = new Set(options.map((option) => option.value));
  return [
    ...options.map((option) => ({
      value: option.value,
      label: option.label !== option.value ? `${option.label} (${option.value})` : option.value,
    })),
    ...current
      .filter((value) => !known.has(value.value))
      .map((value) => ({ value: value.value, label: value.label ?? value.value, unseen: true })),
  ];
};
