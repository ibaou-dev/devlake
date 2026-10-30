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

import type { RedmineGoRef, RedmineGoTransformationData } from '@/api/plugin/redmine_go';

import type { CustomFieldOption, StatusOption, TrackerOption } from './options';

export type KnownObjects = {
  trackers: TrackerOption[];
  statuses: StatusOption[];
  customFields: CustomFieldOption[];
};

// isStale tells whether a stored reference points at an object that neither
// Redmine nor the collected data knows any more. The id is authoritative, so
// a renamed object is not stale.
export const isStale = (ref: RedmineGoRef, known: Array<{ id: number }>): boolean =>
  !known.some((item) => item.id === ref.id);

export type StaleReference = {
  // where the reference sits, for example `typeRules[2].tracker`
  path: string;
  kind: 'tracker' | 'status' | 'customField';
  ref: RedmineGoRef;
};

// findStaleReferences walks a transformation and lists every reference whose
// id is unknown. Stale references stay in the config, because a config copied
// to another Redmine resolves them by name; the form only tags them.
export const findStaleReferences = (
  transformation: RedmineGoTransformationData,
  known: KnownObjects,
): StaleReference[] => {
  const stale: StaleReference[] = [];
  const check = (
    path: string,
    kind: StaleReference['kind'],
    ref: RedmineGoRef | undefined,
    list: Array<{ id: number }>,
  ) => {
    if (ref && isStale(ref, list)) {
      stale.push({ path, kind, ref });
    }
  };
  (transformation.typeRules ?? []).forEach((rule, i) => {
    check(`typeRules[${i}].tracker`, 'tracker', rule.tracker, known.trackers);
    check(`typeRules[${i}].when.field`, 'customField', rule.when?.field, known.customFields);
  });
  (transformation.statusMappings ?? []).forEach((mapping, i) => {
    check(`statusMappings[${i}].status`, 'status', mapping.status, known.statuses);
  });
  (transformation.severity ?? []).forEach((entry, i) => {
    check(`severity[${i}].field`, 'customField', entry.field, known.customFields);
  });
  (transformation.environment ?? []).forEach((entry, i) => {
    check(`environment[${i}].field`, 'customField', entry.field, known.customFields);
  });
  check('component.field', 'customField', transformation.component?.field, known.customFields);
  return stale;
};
