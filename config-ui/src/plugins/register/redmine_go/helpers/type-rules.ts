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

import type { RedmineGoIssueType, RedmineGoRef, RedmineGoTypeRule } from '@/api/plugin/redmine_go';

import { fallbackTracker } from './defaults';
import type { TrackerOption } from './options';

export const ISSUE_TYPES: RedmineGoIssueType[] = ['REQUIREMENT', 'BUG', 'INCIDENT', 'TASK', 'SUBTASK'];

// newTypeRule is the blank rule the editor appends: it has no tracker and no
// condition yet, which validateTypeRule reports until the user picks one.
export const newTypeRule = (): RedmineGoTypeRule => ({ type: 'TASK' });

// withTracker sets or clears the tracker of a rule.
export const withTracker = (rule: RedmineGoTypeRule, tracker?: RedmineGoRef): RedmineGoTypeRule => {
  const { tracker: _removed, ...rest } = rule;
  return tracker ? { ...rest, tracker } : rest;
};

// withoutCondition drops the condition of a rule.
export const withoutCondition = (rule: RedmineGoTypeRule): RedmineGoTypeRule => {
  const { when: _removed, ...rest } = rule;
  return rest;
};

// validateTypeRule lists what the backend would reject in a rule: it needs a
// tracker or a condition, and a condition needs a field and at least one
// value.
export const validateTypeRule = (rule: RedmineGoTypeRule): string[] => {
  const problems: string[] = [];
  if (!rule.tracker && !rule.when) {
    problems.push('needs a tracker or a condition');
  }
  if (rule.when) {
    if (!rule.when.field?.id) {
      problems.push('condition needs a custom field');
    }
    if (!rule.when.values?.length) {
      problems.push('condition needs at least one value');
    }
  }
  return problems;
};

// coversTracker tells whether a rule applies to every issue of the tracker:
// it names the tracker (by id, or by name when the id no longer exists) and
// has no condition. Conditional rules only cover some issues.
export const coversTracker = (rule: RedmineGoTypeRule, tracker: RedmineGoRef): boolean =>
  !rule.when &&
  !!rule.tracker &&
  (rule.tracker.id === tracker.id || rule.tracker.name.toLowerCase() === tracker.name.toLowerCase());

export type TrackerFallbackPreview = {
  tracker: TrackerOption;
  type: string;
  byDefaultName: boolean;
};

// previewTrackerFallbacks lists, for each tracker that no unconditional rule
// covers, the type its issues get from the built-in fallback: Redmine's
// English default names map to a standard type, any other tracker to its
// upper-cased name.
export const previewTrackerFallbacks = (
  rules: RedmineGoTypeRule[],
  trackers: TrackerOption[],
): TrackerFallbackPreview[] =>
  trackers
    .filter((tracker) => !rules.some((rule) => coversTracker(rule, tracker)))
    .map((tracker) => ({ tracker, ...fallbackTracker(tracker.name) }));
