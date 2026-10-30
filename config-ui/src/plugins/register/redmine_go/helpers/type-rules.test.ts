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

import { moveItem } from './reorder';
import {
  coversTracker,
  newTypeRule,
  previewTrackerFallbacks,
  validateTypeRule,
  withoutCondition,
  withTracker,
} from './type-rules';
import { mergeTrackers } from './options';
import { defaultTrackers } from '../test-utils';

const bug = { id: 1, name: 'Bug' };

describe('type rule editing', () => {
  it('reorders rules, since the first match wins', () => {
    const rules = [
      {
        tracker: bug,
        type: 'INCIDENT' as const,
        when: { field: { id: 21, name: 'Env' }, op: 'in' as const, values: [{ value: 'PROD' }] },
      },
      { tracker: bug, type: 'BUG' as const },
    ];
    expect(moveItem(rules, 1, -1).map((r) => r.type)).toEqual(['BUG', 'INCIDENT']);
  });

  it('sets and clears the tracker and the condition', () => {
    const rule = {
      tracker: bug,
      type: 'BUG' as const,
      when: { field: { id: 21, name: 'Env' }, op: 'in' as const, values: [{ value: 'PROD' }] },
    };
    expect(withTracker(rule, undefined)).not.toHaveProperty('tracker');
    expect(withTracker(newTypeRule(), bug)).toEqual({ type: 'TASK', tracker: bug });
    expect(withoutCondition(rule)).toEqual({ tracker: bug, type: 'BUG' });
  });
});

describe('validateTypeRule', () => {
  it('requires a tracker or a condition', () => {
    expect(validateTypeRule(newTypeRule())).toEqual(['needs a tracker or a condition']);
    expect(validateTypeRule({ tracker: bug, type: 'BUG' })).toEqual([]);
  });

  it('requires the condition to have a field and a value', () => {
    expect(validateTypeRule({ type: 'BUG', when: { field: { id: 0, name: '' }, op: 'in', values: [] } })).toEqual([
      'condition needs a custom field',
      'condition needs at least one value',
    ]);
    expect(
      validateTypeRule({
        type: 'BUG',
        when: { field: { id: 21, name: 'Env' }, op: 'notIn', values: [{ value: 'PROD' }] },
      }),
    ).toEqual([]);
  });
});

describe('coversTracker', () => {
  it('is true for an unconditional rule on the tracker, by id or by name when the id is gone', () => {
    expect(coversTracker({ tracker: bug, type: 'BUG' }, bug)).toBe(true);
    expect(coversTracker({ tracker: { id: 99, name: 'bug' }, type: 'BUG' }, bug)).toBe(true);
    expect(coversTracker({ tracker: { id: 2, name: 'Feature' }, type: 'BUG' }, bug)).toBe(false);
  });

  it('is false for a conditional rule, which covers only some issues', () => {
    expect(
      coversTracker(
        {
          tracker: bug,
          type: 'INCIDENT',
          when: { field: { id: 21, name: 'Env' }, op: 'in', values: [{ value: 'PROD' }] },
        },
        bug,
      ),
    ).toBe(false);
  });
});

describe('previewTrackerFallbacks', () => {
  it('lists the trackers no rule covers with the built-in type they fall back to', () => {
    const trackers = mergeTrackers(
      [
        { id: 1, name: 'Bug' },
        { id: 2, name: 'Feature' },
        { id: 3, name: 'Support' },
        { id: 4, name: 'Change Request' },
      ],
      [],
    );
    const preview = previewTrackerFallbacks([{ tracker: bug, type: 'INCIDENT' }], trackers);
    expect(preview.map((p) => [p.tracker.name, p.type, p.byDefaultName])).toEqual([
      ['Feature', 'REQUIREMENT', true],
      ['Support', 'TASK', true],
      ['Change Request', 'CHANGE REQUEST', false],
    ]);
  });

  it('is empty when every tracker has a rule and complete without rules shows all', () => {
    const trackers = defaultTrackers();
    const rules = trackers.map((t) => ({ tracker: { id: t.id, name: t.name }, type: 'TASK' as const }));
    expect(previewTrackerFallbacks(rules, trackers)).toEqual([]);
    expect(previewTrackerFallbacks([], trackers)).toHaveLength(3);
  });
});
