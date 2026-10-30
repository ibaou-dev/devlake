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

import {
  customFields,
  emptyObservedValues,
  issueStatuses,
  observedValues,
  scopeConfigDefaults,
  trackers,
} from './index';

const request = vi.fn();

vi.mock('@/utils', () => ({
  request: (...args: any[]) => request(...args),
}));

describe('redmine_go API wrappers', () => {
  beforeEach(() => vi.clearAllMocks());

  it('unwraps the Redmine collections the proxy forwards', async () => {
    request.mockResolvedValueOnce({ trackers: [{ id: 1, name: 'Bug' }] });
    request.mockResolvedValueOnce({ issue_statuses: [{ id: 1, name: 'New', is_closed: false }] });
    request.mockResolvedValueOnce({
      custom_fields: [{ id: 21, name: 'Env', customized_type: 'issue', field_format: 'list' }],
    });

    expect(await trackers('/p')).toEqual([{ id: 1, name: 'Bug' }]);
    expect(await issueStatuses('/p')).toHaveLength(1);
    expect(await customFields('/p')).toHaveLength(1);
    expect(request.mock.calls.map((call) => call[0])).toEqual([
      '/p/trackers.json',
      '/p/issue_statuses.json',
      '/p/custom_fields.json',
    ]);
  });

  it('returns empty lists for an empty proxy answer', async () => {
    request.mockResolvedValue({});
    expect(await trackers('/p')).toEqual([]);
    expect(await issueStatuses('/p')).toEqual([]);
    expect(await customFields('/p')).toEqual([]);
  });

  it('reads observed values and fills the arrays the backend left out', async () => {
    request.mockResolvedValue({ trackers: [{ id: 1, name: 'Bug' }] });
    expect(await observedValues(5)).toEqual({ ...emptyObservedValues(), trackers: [{ id: 1, name: 'Bug' }] });
    expect(request).toHaveBeenCalledWith('/plugins/redmine_go/connections/5/observed-values');
  });

  it('treats a 404 from observed-values as nothing observed yet, and rethrows anything else', async () => {
    request.mockRejectedValueOnce({ response: { status: 404 } });
    expect(await observedValues(5)).toEqual(emptyObservedValues());

    const failure = { response: { status: 500 } };
    request.mockRejectedValueOnce(failure);
    await expect(observedValues(5)).rejects.toBe(failure);
  });

  it('asks the backend for the scope-config defaults of a connection', async () => {
    request.mockResolvedValue({ typeRules: [] });
    expect(await scopeConfigDefaults(5)).toEqual({ typeRules: [] });
    expect(request).toHaveBeenCalledWith('/plugins/redmine_go/connections/5/scope-config-defaults');
  });
});
