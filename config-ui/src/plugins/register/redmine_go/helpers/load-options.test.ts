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

import { describe, it, expect, vi, beforeEach } from 'vitest';

import { loadRedmineOptions } from './load-options';
import { DEFAULT_STATUSES, DEFAULT_TRACKERS, DEV_CUSTOM_FIELDS, EMPTY_OBSERVED } from '../test-utils';

const trackers = vi.fn();
const issueStatuses = vi.fn();
const customFields = vi.fn();
const observedValues = vi.fn();

vi.mock('@/api', () => ({
  default: {
    plugin: {
      redmine_go: {
        trackers: (...args: any[]) => trackers(...args),
        issueStatuses: (...args: any[]) => issueStatuses(...args),
        customFields: (...args: any[]) => customFields(...args),
        observedValues: (...args: any[]) => observedValues(...args),
      },
    },
  },
}));

const forbidden = Object.assign(new Error('Request failed with status code 403'), { response: { status: 403 } });

describe('loadRedmineOptions', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    trackers.mockResolvedValue(DEFAULT_TRACKERS);
    issueStatuses.mockResolvedValue(DEFAULT_STATUSES);
    observedValues.mockResolvedValue(EMPTY_OBSERVED);
  });

  it('reads live metadata for an administrator key', async () => {
    customFields.mockResolvedValue(DEV_CUSTOM_FIELDS);
    const options = await loadRedmineOptions('/proxy', 1);

    expect(options.trackers.map((t) => t.name)).toEqual(['Bug', 'Feature', 'Support']);
    expect(options.statuses).toHaveLength(6);
    expect(options.customFields.map((f) => f.name)).toEqual([
      'Discovery Environment',
      'Ticket Severity',
      'Bug Severity',
      'Component',
    ]);
    expect(options.customFieldsFromCollectedData).toBe(false);
    expect(options.nothingCollected).toBe(false);
    expect(options.liveErrors).toEqual([]);
    expect(trackers).toHaveBeenCalledWith('/proxy');
    expect(observedValues).toHaveBeenCalledWith(1);
  });

  it('falls back to observed custom fields when custom_fields.json is forbidden (non-admin key)', async () => {
    customFields.mockRejectedValue(forbidden);
    observedValues.mockResolvedValue({
      ...EMPTY_OBSERVED,
      customFields: [
        {
          id: 43,
          name: 'Ticket Severity',
          fieldFormat: 'list',
          multiple: false,
          values: [{ value: '1', label: 'Critical' }],
        },
      ],
    });
    const options = await loadRedmineOptions('/proxy', 1);

    expect(options.customFieldsFromCollectedData).toBe(true);
    expect(options.nothingCollected).toBe(false);
    expect(options.customFields).toMatchObject([{ id: 43, name: 'Ticket Severity', source: 'observed' }]);
    expect(options.trackers).toHaveLength(3);
  });

  it('says nothing was collected yet for a non-admin key on a fresh connection', async () => {
    customFields.mockRejectedValue(forbidden);
    const options = await loadRedmineOptions('/proxy', 1);

    expect(options.customFields).toEqual([]);
    expect(options.nothingCollected).toBe(true);
  });

  it('keeps going on the observed trackers and statuses when Redmine is unreachable, and reports it', async () => {
    trackers.mockRejectedValue(new Error('connect ECONNREFUSED'));
    issueStatuses.mockRejectedValue({ response: { data: { message: 'bad gateway' } } });
    customFields.mockRejectedValue(new Error('connect ECONNREFUSED'));
    observedValues.mockResolvedValue({
      ...EMPTY_OBSERVED,
      trackers: [{ id: 1, name: 'Bug' }],
      statuses: [{ id: 1, name: 'New', isClosed: false }],
    });
    const options = await loadRedmineOptions('/proxy', 1);

    expect(options.trackers.map((t) => t.source)).toEqual(['observed']);
    expect(options.statuses.map((s) => s.source)).toEqual(['observed']);
    expect(options.liveErrors).toEqual([
      'Trackers could not be loaded from Redmine: connect ECONNREFUSED',
      'Statuses could not be loaded from Redmine: bad gateway',
    ]);
  });

  it('treats a failing observed-values call like an empty one', async () => {
    customFields.mockResolvedValue(DEV_CUSTOM_FIELDS);
    observedValues.mockRejectedValue(new Error('500'));
    const options = await loadRedmineOptions('/proxy', 1);

    expect(options.customFields).toHaveLength(4);
    expect(options.liveErrors).toEqual([]);
  });
});
