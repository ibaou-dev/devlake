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

import { describe, it, expect, beforeAll } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

import { Harness, stubBrowserApis } from '../test-utils';

import { Toggles } from './toggles';

beforeAll(stubBrowserApis);

let latest: any;

const renderToggles = (initial: any) =>
  render(
    <Harness initial={initial} onState={(state) => (latest = state)}>
      {({ transformation, setTransformation }) => (
        <Toggles
          transformation={transformation}
          onChange={(key, value) => setTransformation({ ...transformation, [key]: value })}
        />
      )}
    </Harness>,
  );

const toggle = (name: string) => screen.getByRole('switch', { name }) as HTMLButtonElement;

describe('Toggles', () => {
  it('lists the six toggles with a line of help each', () => {
    renderToggles({});

    [
      'Reconciliation sweep',
      'Collect time entries',
      'Collect changesets',
      'Collect comments',
      'Include private notes',
      'Versions as sprints',
    ].forEach((name) => expect(toggle(name)).toBeDefined());
    expect(screen.getByText(/Store journal notes as issue comments/)).toBeDefined();
  });

  it('shows the defaults for absent toggles and leaves them absent', () => {
    renderToggles({});

    expect(toggle('Collect comments').getAttribute('aria-checked')).toBe('true');
    expect(toggle('Include private notes').getAttribute('aria-checked')).toBe('false');
    expect(screen.getAllByText('default: on')).toHaveLength(4);
    expect(screen.getAllByText('default: off')).toHaveLength(2);
    expect(latest).toEqual({});
  });

  it('writes a toggle only when it is flipped, and drops its default tag', () => {
    renderToggles({});

    fireEvent.click(toggle('Collect comments'));
    expect(latest).toEqual({ collectComments: false });
    expect(toggle('Collect comments').getAttribute('aria-checked')).toBe('false');
    expect(screen.getAllByText('default: on')).toHaveLength(3);

    fireEvent.click(toggle('Versions as sprints'));
    expect(latest).toEqual({ collectComments: false, versionsAsSprints: true });
  });

  it('treats a null from the API as absent', () => {
    renderToggles({ reconcileSweep: null, includePrivateNotes: true });

    expect(toggle('Reconciliation sweep').getAttribute('aria-checked')).toBe('true');
    expect(toggle('Include private notes').getAttribute('aria-checked')).toBe('true');
    expect(screen.getAllByText('default: on')).toHaveLength(4);
    expect(screen.getAllByText('default: off')).toHaveLength(1);
  });
});
