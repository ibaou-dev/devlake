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

import { applySeed, fallbackStatus, fallbackTracker, needsSeed, seedTransformation } from './defaults';
import { mergeStatuses, mergeTrackers } from './options';
import { defaultStatuses, defaultTrackers } from '../test-utils';

describe('fallbackTracker', () => {
  it('maps the English default names case-insensitively and upper-cases any other name', () => {
    expect(fallbackTracker('Bug')).toEqual({ type: 'BUG', byDefaultName: true });
    expect(fallbackTracker(' FEATURE ')).toEqual({ type: 'REQUIREMENT', byDefaultName: true });
    expect(fallbackTracker('support')).toEqual({ type: 'TASK', byDefaultName: true });
    expect(fallbackTracker('Fehler')).toEqual({ type: 'FEHLER', byDefaultName: false });
  });
});

describe('fallbackStatus', () => {
  it('is DONE when closed, TODO for a tracker default, IN_PROGRESS otherwise', () => {
    expect(fallbackStatus(true, true)).toBe('DONE');
    expect(fallbackStatus(false, true)).toBe('TODO');
    expect(fallbackStatus(false, false)).toBe('IN_PROGRESS');
  });
});

describe('seedTransformation', () => {
  it('gives default Redmine data the documented mapping, without toggles', () => {
    const seed = seedTransformation(defaultTrackers(), defaultStatuses());
    expect(seed.typeRules).toEqual([
      { tracker: { id: 1, name: 'Bug' }, type: 'BUG' },
      { tracker: { id: 2, name: 'Feature' }, type: 'REQUIREMENT' },
      { tracker: { id: 3, name: 'Support' }, type: 'TASK' },
    ]);
    expect(Object.fromEntries(seed.statusMappings.map((m) => [m.status.name, m.std]))).toEqual({
      New: 'TODO',
      'In Progress': 'IN_PROGRESS',
      Resolved: 'IN_PROGRESS',
      Feedback: 'IN_PROGRESS',
      Closed: 'DONE',
      Rejected: 'DONE',
    });
    expect(seed.component).toEqual({ source: 'category' });
    expect(seed.severity).toEqual([]);
    expect(seed.environment).toEqual([]);
    expect(Object.keys(seed)).not.toContain('reconcileSweep');
  });

  it('leaves trackers without an English default name unmapped', () => {
    const trackers = mergeTrackers(
      [
        { id: 1, name: 'Fehler', default_status: { id: 1, name: 'Neu' } },
        { id: 2, name: 'Bug' },
      ],
      [],
    );
    const seed = seedTransformation(trackers, mergeStatuses([{ id: 1, name: 'Neu' }], []));
    expect(seed.typeRules).toEqual([{ tracker: { id: 2, name: 'Bug' }, type: 'BUG' }]);
    expect(seed.statusMappings).toEqual([{ status: { id: 1, name: 'Neu' }, std: 'TODO' }]);
  });

  it('cannot tell a tracker default status from observed trackers alone', () => {
    const seed = seedTransformation(
      mergeTrackers(null, [{ id: 1, name: 'Bug' }]),
      mergeStatuses(null, [{ id: 1, name: 'New', isClosed: false }]),
    );
    expect(seed.statusMappings[0].std).toBe('IN_PROGRESS');
  });
});

describe('needsSeed and applySeed', () => {
  it('needs a seed until any mapping key is defined, even an emptied list', () => {
    expect(needsSeed({})).toBe(true);
    expect(needsSeed({ reconcileSweep: false })).toBe(true);
    expect(needsSeed({ typeRules: [] })).toBe(false);
    expect(needsSeed({ component: null })).toBe(false);
  });

  it('fills only undefined keys and never copies the toggles of the seed', () => {
    const seed = {
      typeRules: [{ tracker: { id: 1, name: 'Bug' }, type: 'BUG' as const }],
      statusMappings: [],
      component: { source: 'category' as const },
      reconcileSweep: true,
    };
    const next = applySeed({ statusMappings: [{ status: { id: 9, name: 'X' }, std: 'OTHER' }] }, seed);
    expect(next.typeRules).toEqual(seed.typeRules);
    expect(next.statusMappings).toEqual([{ status: { id: 9, name: 'X' }, std: 'OTHER' }]);
    expect(next.component).toEqual({ source: 'category' });
    expect('reconcileSweep' in next).toBe(false);
  });
});
