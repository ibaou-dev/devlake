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

import API from '@/api';
import { emptyObservedValues } from '@/api/plugin/redmine_go';

import {
  mergeCustomFields,
  mergeStatuses,
  mergeTrackers,
  type CustomFieldOption,
  type StatusOption,
  type TrackerOption,
} from './options';

export type RedmineOptions = {
  trackers: TrackerOption[];
  statuses: StatusOption[];
  customFields: CustomFieldOption[];
  // true when custom_fields.json could not be read, which is what a
  // non-administrator key gets (403): the custom-field lists then come from
  // collected data only
  customFieldsFromCollectedData: boolean;
  // true when the connection has no collected custom field yet, so a
  // non-admin key has nothing to offer until the first collection ran
  nothingCollected: boolean;
  // messages of the live reads of trackers and statuses that failed
  liveErrors: string[];
};

type Attempt<T> = { ok: true; value: T } | { ok: false; error: unknown };

const attempt = async <T>(call: () => Promise<T>): Promise<Attempt<T>> => {
  try {
    return { ok: true, value: await call() };
  } catch (error) {
    return { ok: false, error };
  }
};

const describe = (error: unknown): string => {
  const err = error as { response?: { data?: { message?: string } }; message?: string };
  return err?.response?.data?.message ?? err?.message ?? 'request failed';
};

// loadRedmineOptions reads the instance's trackers, statuses and custom fields
// live through the connection's proxy and merges them with the values seen in
// collected data. Each read fails on its own: a non-admin key cannot read
// custom_fields.json (the observed values then feed the lists), and an
// unreachable Redmine still leaves whatever was collected.
export const loadRedmineOptions = async (prefix: string, connectionId: ID): Promise<RedmineOptions> => {
  const [trackers, statuses, customFields, observed] = await Promise.all([
    attempt(() => API.plugin.redmine_go.trackers(prefix)),
    attempt(() => API.plugin.redmine_go.issueStatuses(prefix)),
    attempt(() => API.plugin.redmine_go.customFields(prefix)),
    attempt(() => API.plugin.redmine_go.observedValues(connectionId)),
  ]);
  const seen = observed.ok ? observed.value : emptyObservedValues();
  const liveErrors: string[] = [];
  if (!trackers.ok) {
    liveErrors.push(`Trackers could not be loaded from Redmine: ${describe(trackers.error)}`);
  }
  if (!statuses.ok) {
    liveErrors.push(`Statuses could not be loaded from Redmine: ${describe(statuses.error)}`);
  }
  return {
    trackers: mergeTrackers(trackers.ok ? trackers.value : null, seen.trackers),
    statuses: mergeStatuses(statuses.ok ? statuses.value : null, seen.statuses),
    customFields: mergeCustomFields(customFields.ok ? customFields.value : null, seen.customFields),
    customFieldsFromCollectedData: !customFields.ok,
    nothingCollected: !customFields.ok && seen.customFields.length === 0,
    liveErrors,
  };
};
