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

import { findStaleReferences, isStale } from './stale';
import { defaultStatuses, defaultTrackers, devCustomFields } from '../test-utils';

describe('isStale', () => {
  it('goes by id, so a renamed object is not stale', () => {
    expect(isStale({ id: 1, name: 'Renamed' }, [{ id: 1 }])).toBe(false);
    expect(isStale({ id: 9, name: 'Bug' }, [{ id: 1 }])).toBe(true);
  });
});

describe('findStaleReferences', () => {
  const known = { trackers: defaultTrackers(), statuses: defaultStatuses(), customFields: devCustomFields() };

  it('reports nothing for a config that matches the instance', () => {
    expect(
      findStaleReferences(
        {
          typeRules: [{ tracker: { id: 1, name: 'Bug' }, type: 'BUG' }],
          statusMappings: [{ status: { id: 1, name: 'New' }, std: 'TODO' }],
          severity: [{ field: { id: 43, name: 'Ticket Severity' }, map: [] }],
          component: { source: 'customField', field: { id: 31, name: 'Component' } },
        },
        known,
      ),
    ).toEqual([]);
  });

  it('reports every reference whose id is unknown, with its path', () => {
    const stale = findStaleReferences(
      {
        typeRules: [
          {
            tracker: { id: 77, name: 'Gone tracker' },
            type: 'BUG',
            when: { field: { id: 78, name: 'Gone field' }, op: 'in', values: [{ value: 'x' }] },
          },
        ],
        statusMappings: [{ status: { id: 79, name: 'Gone status' }, std: 'DONE' }],
        environment: [{ field: { id: 80, name: 'Gone env' }, map: [] }],
        component: { source: 'customField', field: { id: 81, name: 'Gone component' } },
      },
      known,
    );
    expect(stale.map((s) => [s.path, s.kind, s.ref.name])).toEqual([
      ['typeRules[0].tracker', 'tracker', 'Gone tracker'],
      ['typeRules[0].when.field', 'customField', 'Gone field'],
      ['statusMappings[0].status', 'status', 'Gone status'],
      ['environment[0].field', 'customField', 'Gone env'],
      ['component.field', 'customField', 'Gone component'],
    ]);
  });

  it('tolerates the nulls the API returns for unset keys', () => {
    expect(
      findStaleReferences({ typeRules: null, statusMappings: null, severity: null, component: null }, known),
    ).toEqual([]);
  });
});
