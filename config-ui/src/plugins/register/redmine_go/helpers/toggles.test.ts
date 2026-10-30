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

import { isToggleSet, toggleValue, TOGGLES } from './toggles';

const def = (key: string) => TOGGLES.find((t) => t.key === key)!;

describe('toggles are tri-state', () => {
  it('uses the default while the toggle is absent or null', () => {
    expect(toggleValue({}, def('collectComments'))).toBe(true);
    expect(toggleValue({ collectComments: null }, def('collectComments'))).toBe(true);
    expect(toggleValue({}, def('includePrivateNotes'))).toBe(false);
    expect(isToggleSet({}, 'collectComments')).toBe(false);
    expect(isToggleSet({ collectComments: null }, 'collectComments')).toBe(false);
  });

  it('uses the stored boolean, including an explicit false', () => {
    expect(toggleValue({ collectComments: false }, def('collectComments'))).toBe(false);
    expect(toggleValue({ includePrivateNotes: true }, def('includePrivateNotes'))).toBe(true);
    expect(isToggleSet({ collectComments: false }, 'collectComments')).toBe(true);
  });

  it('carries the defaults of the schema: four on, two off', () => {
    expect(TOGGLES.filter((t) => t.defaultValue).map((t) => t.key)).toEqual([
      'reconcileSweep',
      'collectTimeEntries',
      'collectChangesets',
      'collectComments',
    ]);
    expect(TOGGLES.filter((t) => !t.defaultValue).map((t) => t.key)).toEqual([
      'includePrivateNotes',
      'versionsAsSprints',
    ]);
  });
});
