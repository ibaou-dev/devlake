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

import type { RedmineGoRef, RedmineGoStatusMapping, RedmineGoStatusStd } from '@/api/plugin/redmine_go';

import { fallbackStatus, trackerDefaultStatusIds } from './defaults';
import type { StatusOption, TrackerOption } from './options';

export const STATUS_STDS: RedmineGoStatusStd[] = ['TODO', 'IN_PROGRESS', 'DONE', 'OTHER'];

export type StatusRow = {
  status: RedmineGoRef;
  // the stored standard status, absent when the config has no row for it
  std?: RedmineGoStatusStd;
  // what the plugin applies when std is absent
  fallback: RedmineGoStatusStd;
  // true when no known status has this id any more
  stale: boolean;
};

// buildStatusRows lists every known status with its stored mapping, then the
// stored mappings whose status is unknown (stale), so nothing the config holds
// is hidden. A status the config does not mention carries its built-in
// fallback, which is what the backend applies to it.
export const buildStatusRows = (
  statuses: StatusOption[],
  trackers: TrackerOption[],
  mappings: RedmineGoStatusMapping[],
): StatusRow[] => {
  const defaultIds = trackerDefaultStatusIds(trackers);
  const stored = new Map(mappings.map((mapping) => [mapping.status.id, mapping]));
  const rows: StatusRow[] = statuses.map((status) => ({
    status: { id: status.id, name: status.name },
    std: stored.get(status.id)?.std,
    fallback: fallbackStatus(status.isClosed, defaultIds.has(status.id)),
    stale: false,
  }));
  const known = new Set(statuses.map((status) => status.id));
  mappings
    .filter((mapping) => !known.has(mapping.status.id))
    .forEach((mapping) =>
      rows.push({ status: mapping.status, std: mapping.std, fallback: 'IN_PROGRESS', stale: true }),
    );
  return rows;
};

// setStatusStd sets, or with `std` undefined removes, the mapping of a
// status. A new mapping is appended; an existing one keeps its position.
export const setStatusStd = (
  mappings: RedmineGoStatusMapping[],
  status: RedmineGoRef,
  std?: RedmineGoStatusStd,
): RedmineGoStatusMapping[] => {
  const exists = mappings.some((mapping) => mapping.status.id === status.id);
  if (!std) {
    return mappings.filter((mapping) => mapping.status.id !== status.id);
  }
  if (!exists) {
    return [...mappings, { status: { id: status.id, name: status.name }, std }];
  }
  return mappings.map((mapping) => (mapping.status.id === status.id ? { ...mapping, std } : mapping));
};
