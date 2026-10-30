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

import type { RedmineGoToggleKey, RedmineGoTransformationData } from '@/api/plugin/redmine_go';

export type ToggleDef = {
  key: RedmineGoToggleKey;
  label: string;
  // the value the plugin uses while the scope config leaves the toggle unset
  defaultValue: boolean;
  help: string;
};

export const TOGGLES: ToggleDef[] = [
  {
    key: 'reconcileSweep',
    label: 'Reconciliation sweep',
    defaultValue: true,
    help: 'Re-read each project’s full issue list to pick up removed relations and deleted issues, which Redmine does not report as changes.',
  },
  {
    key: 'collectTimeEntries',
    label: 'Collect time entries',
    defaultValue: true,
    help: 'Collect logged time as issue worklogs; it is pulled in full on every run because Redmine ignores updated_on for time entries.',
  },
  {
    key: 'collectChangesets',
    label: 'Collect changesets',
    defaultValue: true,
    help: 'Link repository commits that reference an issue, from the changesets Redmine has indexed.',
  },
  {
    key: 'collectComments',
    label: 'Collect comments',
    defaultValue: true,
    help: 'Store journal notes as issue comments.',
  },
  {
    key: 'includePrivateNotes',
    label: 'Include private notes',
    defaultValue: false,
    help: 'Also collect private journal notes; they are skipped unless this is on.',
  },
  {
    key: 'versionsAsSprints',
    label: 'Versions as sprints',
    defaultValue: false,
    help: 'Emit each Redmine version as a sprint on the project’s board; fix versions are always stored on the issue.',
  },
];

// toggleValue returns the effective value of a toggle: the stored boolean, or
// the default when it is absent or null. Toggles are tri-state on purpose, so
// a blueprint without a scope config still collects what is on by default.
export const toggleValue = (transformation: RedmineGoTransformationData, def: ToggleDef): boolean => {
  const stored = transformation[def.key];
  return typeof stored === 'boolean' ? stored : def.defaultValue;
};

// isToggleSet tells whether the scope config holds an explicit value.
export const isToggleSet = (transformation: RedmineGoTransformationData, key: RedmineGoToggleKey): boolean =>
  typeof transformation[key] === 'boolean';
