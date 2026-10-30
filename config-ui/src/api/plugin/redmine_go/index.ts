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

import { request } from '@/utils';

// RedmineGoRef points at a Redmine tracker, status or custom field. The id is
// authoritative; the name is for display and the fallback when the id is gone.
export type RedmineGoRef = {
  id: number;
  name: string;
};

// RedmineGoValue is a custom-field value as Redmine stores it plus an
// optional display label.
export type RedmineGoValue = {
  value: string;
  label?: string;
};

export type RedmineGoCondition = {
  field: RedmineGoRef;
  op: 'in' | 'notIn';
  values: RedmineGoValue[];
};

export type RedmineGoIssueType = 'REQUIREMENT' | 'BUG' | 'INCIDENT' | 'TASK' | 'SUBTASK';

export type RedmineGoStatusStd = 'TODO' | 'IN_PROGRESS' | 'DONE' | 'OTHER';

export type RedmineGoEnvironmentClass = 'PRODUCTION' | 'STAGING' | 'TESTING' | 'DEVELOPMENT';

export type RedmineGoTypeRule = {
  tracker?: RedmineGoRef;
  when?: RedmineGoCondition;
  type: RedmineGoIssueType;
};

export type RedmineGoStatusMapping = {
  status: RedmineGoRef;
  std: RedmineGoStatusStd;
};

export type RedmineGoValueMapEntry = {
  value: string;
  label?: string;
  to: string;
};

export type RedmineGoValueMapField = {
  field: RedmineGoRef;
  map: RedmineGoValueMapEntry[];
};

export type RedmineGoComponentSource = 'none' | 'category' | 'customField';

export type RedmineGoComponent = {
  source: RedmineGoComponentSource;
  field?: RedmineGoRef;
};

export type RedmineGoToggleKey =
  | 'reconcileSweep'
  | 'collectTimeEntries'
  | 'collectChangesets'
  | 'collectComments'
  | 'includePrivateNotes'
  | 'versionsAsSprints';

// RedmineGoTransformationData is the mapping part of a scope config. Toggles are
// tri-state: absent or null means the plugin's default.
export type RedmineGoTransformationData = {
  typeRules?: RedmineGoTypeRule[] | null;
  statusMappings?: RedmineGoStatusMapping[] | null;
  severity?: RedmineGoValueMapField[] | null;
  environment?: RedmineGoValueMapField[] | null;
  component?: RedmineGoComponent | null;
} & Partial<Record<RedmineGoToggleKey, boolean | null>>;

export type RedmineGoTestResult = {
  success: boolean;
  message: string;
  user?: { id: number; login: string; name: string };
  // true when the API key belongs to a Redmine administrator, the only key
  // that can read custom field definitions
  isAdmin: boolean;
};

// Redmine REST shapes read through the connection's proxy route.
export type RedmineTracker = {
  id: number;
  name: string;
  default_status?: RedmineGoRef;
};

export type RedmineIssueStatus = {
  id: number;
  name: string;
  is_closed?: boolean;
};

export type RedmineCustomField = {
  id: number;
  name: string;
  customized_type: string;
  field_format: string;
  multiple?: boolean;
  possible_values?: Array<{ value: string; label: string }>;
};

export type RedmineGoObservedValues = {
  trackers: RedmineGoRef[];
  statuses: Array<RedmineGoRef & { isClosed: boolean }>;
  customFields: Array<{
    id: number;
    name: string;
    fieldFormat: string;
    multiple: boolean;
    values: Array<{ value: string; label: string }>;
  }>;
  categories: RedmineGoRef[];
};

export const emptyObservedValues = (): RedmineGoObservedValues => ({
  trackers: [],
  statuses: [],
  customFields: [],
  categories: [],
});

export const trackers = (prefix: string): Promise<RedmineTracker[]> =>
  request(`${prefix}/trackers.json`).then((res: { trackers?: RedmineTracker[] }) => res?.trackers ?? []);

export const issueStatuses = (prefix: string): Promise<RedmineIssueStatus[]> =>
  request(`${prefix}/issue_statuses.json`).then(
    (res: { issue_statuses?: RedmineIssueStatus[] }) => res?.issue_statuses ?? [],
  );

// customFields needs an administrator key: Redmine answers 403 otherwise and
// the caller falls back to observedValues.
export const customFields = (prefix: string): Promise<RedmineCustomField[]> =>
  request(`${prefix}/custom_fields.json`).then(
    (res: { custom_fields?: RedmineCustomField[] }) => res?.custom_fields ?? [],
  );

// observedValues reads what the collected tool tables hold. A 404 means the
// backend has nothing for the connection yet and is the same as empty arrays.
export const observedValues = async (connectionId: ID): Promise<RedmineGoObservedValues> => {
  try {
    const res: Partial<RedmineGoObservedValues> | null = await request(
      `/plugins/redmine_go/connections/${connectionId}/observed-values`,
    );
    return { ...emptyObservedValues(), ...(res ?? {}) };
  } catch (err: any) {
    if (err?.response?.status === 404) {
      return emptyObservedValues();
    }
    throw err;
  }
};

// scopeConfigDefaults returns the out-of-the-box mapping resolved against the
// connected Redmine's trackers and statuses.
export const scopeConfigDefaults = (connectionId: ID): Promise<RedmineGoTransformationData> =>
  request(`/plugins/redmine_go/connections/${connectionId}/scope-config-defaults`);
