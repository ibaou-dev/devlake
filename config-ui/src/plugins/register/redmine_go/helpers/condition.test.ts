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

import { conditionOptions, toConditionValues } from './condition';

const options = [
  { value: '1', label: 'Critical', source: 'live' as const },
  { value: '2', label: 'Normal', source: 'live' as const },
];

describe('toConditionValues', () => {
  it('takes labels from the existing entry, else from the known option; free entries have none', () => {
    expect(toConditionValues(['1', 'env:prod-client', '2'], [{ value: '2', label: 'Standard' }], options)).toEqual([
      { value: '1', label: 'Critical' },
      { value: 'env:prod-client' },
      { value: '2', label: 'Standard' },
    ]);
  });

  it('trims, drops blanks and duplicates', () => {
    expect(toConditionValues([' 1 ', '', '1'], [], options)).toEqual([{ value: '1', label: 'Critical' }]);
  });
});

describe('conditionOptions', () => {
  it('lists known values with their label, then saved values nobody lists', () => {
    expect(conditionOptions(options, [{ value: '1' }, { value: 'env:prod-client', label: 'registry key' }])).toEqual([
      { value: '1', label: 'Critical (1)' },
      { value: '2', label: 'Normal (2)' },
      { value: 'env:prod-client', label: 'registry key', unseen: true },
    ]);
  });
});
