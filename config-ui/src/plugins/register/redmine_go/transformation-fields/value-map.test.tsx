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
import { fireEvent, render, screen, within } from '@testing-library/react';

import type { RedmineGoValueMapField } from '@/api/plugin/redmine_go';

import { choose, devCustomFields, Harness, stubBrowserApis } from '../test-utils';

import { ValueMapEditor } from './value-map';

beforeAll(stubBrowserApis);

let latest: any;

const renderEditor = (
  kind: 'severity' | 'environment',
  entries: RedmineGoValueMapField[],
  customFields = devCustomFields(),
) =>
  render(
    <Harness initial={{ list: entries }} onState={(state) => (latest = state)}>
      {({ transformation, setTransformation }) => (
        <ValueMapEditor
          kind={kind}
          title={kind === 'severity' ? 'Severity' : 'Environment class'}
          description="description"
          entries={transformation.list}
          customFields={customFields}
          fieldsFromCollectedData={false}
          onChange={(list) => setTransformation({ ...transformation, list })}
        />
      )}
    </Harness>,
  );

const typeIn = (name: string, value: string) =>
  fireEvent.change(screen.getByRole('textbox', { name }), { target: { value } });

describe('ValueMapEditor for severity', () => {
  it('adds fields in order and lists each field’s known values', async () => {
    renderEditor('severity', []);

    await choose('Add a severity field', 'Bug Severity');
    await choose('Add a severity field', 'Ticket Severity');
    expect(latest.list.map((f: RedmineGoValueMapField) => f.field.name)).toEqual(['Bug Severity', 'Ticket Severity']);

    // a field already added is no longer offered
    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Add a severity field' }));
    const offered = Array.from(document.querySelectorAll('.ant-select-item-option')).map((el) =>
      el.getAttribute('title'),
    );
    expect(offered).toEqual(['Discovery Environment', 'Component']);

    expect(screen.getByText('Critical (1)')).toBeDefined();
    expect(screen.getByText('Normal (2)')).toBeDefined();
  });

  it('stores a value→label entry, with the label of the value, and removes it when emptied', () => {
    renderEditor('severity', [{ field: { id: 43, name: 'Ticket Severity' }, map: [] }]);

    typeIn('Severity of 1', 'Sev1');
    expect(latest.list[0].map).toEqual([{ value: '1', label: 'Critical', to: 'Sev1' }]);

    typeIn('Severity of 1', '');
    expect(latest.list[0].map).toEqual([]);
  });

  it('adds a value that has not been seen yet', () => {
    renderEditor('severity', [{ field: { id: 18, name: 'Bug Severity' }, map: [] }]);

    typeIn('Value not seen yet for Bug Severity', 'blocker');
    fireEvent.click(screen.getByRole('button', { name: 'Add value' }));
    expect(latest.list[0].map).toEqual([{ value: 'blocker', to: 'blocker' }]);
    expect(screen.getByText('not seen')).toBeDefined();
  });

  it('reorders and removes fields', () => {
    renderEditor('severity', [
      { field: { id: 18, name: 'Bug Severity' }, map: [] },
      { field: { id: 43, name: 'Ticket Severity' }, map: [] },
    ]);

    fireEvent.click(screen.getByRole('button', { name: 'Move severity field 2 up' }));
    expect(latest.list.map((f: RedmineGoValueMapField) => f.field.id)).toEqual([43, 18]);

    fireEvent.click(screen.getByRole('button', { name: 'Remove severity field 1' }));
    expect(latest.list.map((f: RedmineGoValueMapField) => f.field.id)).toEqual([18]);
  });

  it('tags a field that is gone as stale and keeps its entries', () => {
    renderEditor('severity', [{ field: { id: 77, name: 'Retired field' }, map: [{ value: 'a', to: 'b' }] }]);

    expect(screen.getByText('stale field')).toBeDefined();
    expect(screen.getByText('not seen')).toBeDefined();
    expect(latest.list[0].map).toEqual([{ value: 'a', to: 'b' }]);
  });
});

describe('ValueMapEditor for environment class', () => {
  const env: RedmineGoValueMapField = { field: { id: 21, name: 'Discovery Environment' }, map: [] };

  it('maps each value to one of the four classes', async () => {
    renderEditor('environment', [env]);

    await choose('Environment class of PROD', 'PRODUCTION');
    await choose('Environment class of ACC', 'STAGING');
    await choose('Environment class of UAT', 'TESTING');
    await choose('Environment class of DEV', 'DEVELOPMENT');
    expect(latest.list[0].map).toEqual([
      { value: 'PROD', to: 'PRODUCTION' },
      { value: 'ACC', to: 'STAGING' },
      { value: 'UAT', to: 'TESTING' },
      { value: 'DEV', to: 'DEVELOPMENT' },
    ]);
  });

  it('keeps an unmapped value out of the config', async () => {
    renderEditor('environment', [{ ...env, map: [{ value: 'PROD', to: 'PRODUCTION' }] }]);

    const row = screen.getByRole('combobox', { name: 'Environment class of PROD' }).closest('tr') as HTMLElement;
    expect(within(row).getByText('PRODUCTION')).toBeDefined();
    fireEvent.mouseEnter(row);
    fireEvent.click(row.querySelector('.ant-select-clear') as Element);
    expect(latest.list[0].map).toEqual([]);
  });

  it('offers observed registry keys as rows, next to the possible values', () => {
    const fields = devCustomFields();
    fields[0].values.push({ value: 'env:prod-client', label: 'env:prod-client', source: 'observed' });
    renderEditor('environment', [env], fields);

    expect(screen.getByText('env:prod-client')).toBeDefined();
  });

  it('flags a typed-in value until it has a class', () => {
    renderEditor('environment', [env]);

    typeIn('Value not seen yet for Discovery Environment', 'env:prod-client');
    fireEvent.click(screen.getByRole('button', { name: 'Add value' }));
    expect(latest.list[0].map).toEqual([{ value: 'env:prod-client', to: '' }]);
  });
});
