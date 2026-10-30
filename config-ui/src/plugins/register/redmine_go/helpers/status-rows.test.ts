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

import { describe, it, expect } from 'vitest';

import { buildStatusRows, setStatusStd } from './status-rows';
import { defaultStatuses, defaultTrackers } from '../test-utils';

describe('buildStatusRows', () => {
  it('lists every live status; unmapped ones carry the built-in fallback', () => {
    const rows = buildStatusRows(defaultStatuses(), defaultTrackers(), [
      { status: { id: 3, name: 'Resolved' }, std: 'DONE' },
    ]);
    expect(rows.map((r) => [r.status.name, r.std, r.fallback])).toEqual([
      ['New', undefined, 'TODO'],
      ['In Progress', undefined, 'IN_PROGRESS'],
      ['Resolved', 'DONE', 'IN_PROGRESS'],
      ['Feedback', undefined, 'IN_PROGRESS'],
      ['Closed', undefined, 'DONE'],
      ['Rejected', undefined, 'DONE'],
    ]);
    expect(rows.every((r) => !r.stale)).toBe(true);
  });

  it('appends mappings of statuses that no longer exist as stale rows, keeping their value', () => {
    const rows = buildStatusRows(defaultStatuses(), defaultTrackers(), [
      { status: { id: 55, name: 'Awaiting client' }, std: 'OTHER' },
    ]);
    expect(rows).toHaveLength(7);
    expect(rows[6]).toMatchObject({ status: { id: 55, name: 'Awaiting client' }, std: 'OTHER', stale: true });
  });
});

describe('setStatusStd', () => {
  const base = [
    { status: { id: 1, name: 'New' }, std: 'TODO' as const },
    { status: { id: 2, name: 'In Progress' }, std: 'IN_PROGRESS' as const },
  ];

  it('updates in place, appends a new row, and removes on undefined', () => {
    expect(setStatusStd(base, { id: 2, name: 'In Progress' }, 'OTHER')[1].std).toBe('OTHER');
    expect(setStatusStd(base, { id: 4, name: 'Feedback' }, 'DONE')[2]).toEqual({
      status: { id: 4, name: 'Feedback' },
      std: 'DONE',
    });
    expect(setStatusStd(base, { id: 1, name: 'New' }, undefined)).toEqual([base[1]]);
  });
});
