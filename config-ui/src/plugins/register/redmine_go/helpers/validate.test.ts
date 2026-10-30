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

import { validateTransformation } from './validate';

describe('validateTransformation', () => {
  it('accepts an empty and a seeded transformation', () => {
    expect(validateTransformation({})).toEqual([]);
    expect(
      validateTransformation({
        typeRules: [{ tracker: { id: 1, name: 'Bug' }, type: 'BUG' }],
        component: { source: 'category' },
      }),
    ).toEqual([]);
  });

  it('names each problem the API would reject', () => {
    expect(
      validateTransformation({
        typeRules: [{ type: 'TASK' }, { tracker: { id: 1, name: 'Bug' }, type: 'BUG' }],
        environment: [{ field: { id: 21, name: 'Discovery Environment' }, map: [{ value: 'PROD', to: '' }] }],
        component: { source: 'customField' },
      }),
    ).toEqual([
      'Type rule 1 needs a tracker or a condition',
      'Environment value “PROD” of Discovery Environment needs a class',
      'Component needs a custom field',
    ]);
  });
});
