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

import type { RedmineGoComponent } from '@/api/plugin/redmine_go';

import { choose, devCustomFields, Harness, stubBrowserApis } from '../test-utils';

import { ComponentSource } from './component-source';

beforeAll(stubBrowserApis);

let latest: any;

const renderSource = (component?: RedmineGoComponent | null) =>
  render(
    <Harness initial={{ component }} onState={(state) => (latest = state)}>
      {({ transformation, setTransformation }) => (
        <ComponentSource
          component={transformation.component}
          customFields={devCustomFields()}
          onChange={(next) => setTransformation({ ...transformation, component: next })}
        />
      )}
    </Harness>,
  );

const radio = (name: string) => screen.getByRole('radio', { name }) as HTMLInputElement;

describe('ComponentSource', () => {
  it('shows the issue category when the config holds no component', () => {
    renderSource(null);

    expect(radio('Issue category').checked).toBe(true);
    expect(screen.queryByRole('combobox', { name: 'Component custom field' })).toBeNull();
  });

  it('switches to none and back to the category', () => {
    renderSource({ source: 'category' });

    fireEvent.click(radio('None'));
    expect(latest.component).toEqual({ source: 'none' });
    fireEvent.click(radio('Issue category'));
    expect(latest.component).toEqual({ source: 'category' });
  });

  it('picks a custom field and stores it as a reference', async () => {
    renderSource({ source: 'category' });

    fireEvent.click(radio('Custom field'));
    expect(latest.component).toEqual({ source: 'customField' });
    await choose('Component custom field', 'Component');
    expect(latest.component).toEqual({ source: 'customField', field: { id: 31, name: 'Component' } });
  });

  it('drops the field when the source changes away from a custom field', () => {
    renderSource({ source: 'customField', field: { id: 31, name: 'Component' } });

    expect(screen.getByRole('combobox', { name: 'Component custom field' })).toBeDefined();
    fireEvent.click(radio('None'));
    expect(latest.component).toEqual({ source: 'none' });
  });

  it('tags a field that is gone as stale', () => {
    renderSource({ source: 'customField', field: { id: 77, name: 'Retired' } });

    expect(screen.getByText('stale field')).toBeDefined();
  });
});
