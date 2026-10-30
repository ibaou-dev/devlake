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
  RedmineGoComponent,
  RedmineGoIssueType,
  RedmineGoStatusMapping,
  RedmineGoStatusStd,
  RedmineGoTransformationData,
  RedmineGoTypeRule,
  RedmineGoValueMapField,
} from '@/api/plugin/redmine_go';

import type { StatusOption, TrackerOption } from './options';

// Redmine's default trackers by their lower-cased English names. Support is a
// TASK, not an INCIDENT: a support request is no evidence of a failed change.
const DEFAULT_TRACKER_TYPES: Record<string, RedmineGoIssueType> = {
  bug: 'BUG',
  feature: 'REQUIREMENT',
  support: 'TASK',
};

export type TrackerFallback = {
  type: string;
  // true when the type comes from Redmine's English default name, false when
  // it is the upper-cased tracker name
  byDefaultName: boolean;
};

// fallbackTracker mirrors the backend's built-in tracker mapping: the English
// default name (case-insensitive) or else the upper-cased name.
export const fallbackTracker = (trackerName: string): TrackerFallback => {
  const name = trackerName.trim();
  const type = DEFAULT_TRACKER_TYPES[name.toLowerCase()];
  return type ? { type, byDefaultName: true } : { type: name.toUpperCase(), byDefaultName: false };
};

// fallbackStatus mirrors the backend's built-in status mapping: DONE when
// Redmine reports the status closed, TODO when it is a tracker's default
// status, IN_PROGRESS otherwise.
export const fallbackStatus = (isClosed: boolean, isTrackerDefault: boolean): RedmineGoStatusStd => {
  if (isClosed) {
    return 'DONE';
  }
  return isTrackerDefault ? 'TODO' : 'IN_PROGRESS';
};

export const trackerDefaultStatusIds = (trackers: TrackerOption[]): Set<number> =>
  new Set(trackers.flatMap((tracker) => (tracker.defaultStatusId ? [tracker.defaultStatusId] : [])));

// seedTransformation builds the mapping the plugin applies anyway, with the
// live ids, for the form to show and edit: a type rule per tracker with an
// English default name, a status row per status, the issue category as
// component. Toggles are left out on purpose, so they stay absent (default)
// until the user changes one. It is the client-side twin of the backend's
// scope-config-defaults, used when that endpoint cannot be reached.
export type SeededMapping = {
  typeRules: RedmineGoTypeRule[];
  statusMappings: RedmineGoStatusMapping[];
  severity: RedmineGoValueMapField[];
  environment: RedmineGoValueMapField[];
  component: RedmineGoComponent;
};

export const seedTransformation = (trackers: TrackerOption[], statuses: StatusOption[]): SeededMapping => {
  const defaultIds = trackerDefaultStatusIds(trackers);
  return {
    typeRules: trackers.flatMap((tracker) => {
      const fallback = fallbackTracker(tracker.name);
      return fallback.byDefaultName
        ? [{ tracker: { id: tracker.id, name: tracker.name }, type: fallback.type as RedmineGoIssueType }]
        : [];
    }),
    statusMappings: statuses.map((status) => ({
      status: { id: status.id, name: status.name },
      std: fallbackStatus(status.isClosed, defaultIds.has(status.id)),
    })),
    severity: [],
    environment: [],
    component: { source: 'category' },
  };
};

const MAPPING_KEYS = ['typeRules', 'statusMappings', 'severity', 'environment', 'component'] as const;

// needsSeed tells whether a transformation has never held mapping data: the
// form's initial state of a new scope config. A config that was seeded, saved
// or loaded always carries at least one of these keys, so a seeded-and-edited
// form is never overwritten, even when the user emptied every list.
export const needsSeed = (transformation: RedmineGoTransformationData): boolean =>
  MAPPING_KEYS.every((key) => transformation[key] === undefined);

// applySeed merges seed values into a transformation without touching a key
// that is already defined, and keeps toggles absent.
export const applySeed = (
  transformation: RedmineGoTransformationData,
  seed: RedmineGoTransformationData,
): RedmineGoTransformationData => {
  const next: RedmineGoTransformationData = { ...transformation };
  MAPPING_KEYS.forEach((key) => {
    if (next[key] === undefined && seed[key] != null) {
      (next as Record<string, unknown>)[key] = seed[key];
    }
  });
  return next;
};
