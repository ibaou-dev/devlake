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
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import type { RedmineGoTypeRule } from '@/api/plugin/redmine_go';

import { choose, defaultTrackers, devCustomFields, Harness, stubBrowserApis } from '../test-utils';

import { TypeRulesEditor } from './type-rules';

beforeAll(stubBrowserApis);

let latest: any;

const renderEditor = (
  rules: RedmineGoTypeRule[],
  { trackers = defaultTrackers(), customFields = devCustomFields(), fromCollected = false } = {},
) =>
  render(
    <Harness initial={{ typeRules: rules }} onState={(state) => (latest = state)}>
      {({ transformation, setTransformation }) => (
        <TypeRulesEditor
          rules={transformation.typeRules}
          trackers={trackers}
          customFields={customFields}
          fieldsFromCollectedData={fromCollected}
          onChange={(typeRules) => setTransformation({ ...transformation, typeRules })}
          onReset={() =>
            setTransformation({ ...transformation, typeRules: [{ type: 'BUG', tracker: { id: 1, name: 'Bug' } }] })
          }
        />
      )}
    </Harness>,
  );

const bug: RedmineGoTypeRule = { tracker: { id: 1, name: 'Bug' }, type: 'BUG' };
const feature: RedmineGoTypeRule = { tracker: { id: 2, name: 'Feature' }, type: 'REQUIREMENT' };
const support: RedmineGoTypeRule = { tracker: { id: 3, name: 'Support' }, type: 'TASK' };

const names = () => latest.typeRules.map((r: RedmineGoTypeRule) => r.tracker?.name);

describe('TypeRulesEditor', () => {
  it('adds a rule, and flags it until it has a tracker or a condition', async () => {
    renderEditor([]);

    fireEvent.click(screen.getByRole('button', { name: /Add rule/ }));
    expect(latest.typeRules).toEqual([{ type: 'TASK' }]);
    expect(screen.getByRole('alert').textContent).toContain('needs a tracker or a condition');

    await choose('Tracker of rule 1', 'Support');
    expect(latest.typeRules).toEqual([{ tracker: { id: 3, name: 'Support' }, type: 'TASK' }]);
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('feeds the tracker select with the live trackers', async () => {
    renderEditor([{ type: 'TASK' }]);

    fireEvent.mouseDown(screen.getByRole('combobox', { name: 'Tracker of rule 1' }));
    await waitFor(() => {
      const titles = Array.from(document.querySelectorAll('.ant-select-item-option')).map((el) =>
        el.getAttribute('title'),
      );
      expect(titles).toEqual(['Bug', 'Feature', 'Support']);
    });
  });

  it('changes the target type of a rule', async () => {
    renderEditor([bug]);

    await choose('Type of rule 1', 'INCIDENT');
    expect(latest.typeRules).toEqual([{ tracker: { id: 1, name: 'Bug' }, type: 'INCIDENT' }]);
  });

  it('moves rules up and down, and disables the buttons at either end', () => {
    renderEditor([bug, feature, support]);

    expect((screen.getByRole('button', { name: 'Move rule 1 up' }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole('button', { name: 'Move rule 3 down' }) as HTMLButtonElement).disabled).toBe(true);

    fireEvent.click(screen.getByRole('button', { name: 'Move rule 3 up' }));
    expect(names()).toEqual(['Bug', 'Support', 'Feature']);

    fireEvent.click(screen.getByRole('button', { name: 'Move rule 1 down' }));
    expect(names()).toEqual(['Support', 'Bug', 'Feature']);
  });

  it('removes a rule', () => {
    renderEditor([bug, feature, support]);

    fireEvent.click(screen.getByRole('button', { name: 'Remove rule 2' }));
    expect(names()).toEqual(['Bug', 'Support']);
  });

  it('builds a condition from a field, an operator and values, with free entry for an unseen value', async () => {
    renderEditor([bug]);

    fireEvent.click(screen.getByRole('button', { name: /Add condition/ }));
    expect(screen.getByRole('alert').textContent).toContain('condition needs a custom field');

    await choose('Condition field of rule 1', 'Discovery Environment');
    expect(latest.typeRules[0].when).toEqual({
      field: { id: 21, name: 'Discovery Environment' },
      op: 'in',
      values: [],
    });

    await choose('Condition operator of rule 1', 'is none of');
    expect(latest.typeRules[0].when.op).toBe('notIn');

    // a known value takes its label from the field's possible values
    await choose('Condition values of rule 1', 'PROD');
    expect(latest.typeRules[0].when.values).toEqual([{ value: 'PROD', label: 'PROD' }]);

    // a value that was never seen is typed in
    const values = screen.getByRole('combobox', { name: 'Condition values of rule 1' });
    fireEvent.change(values, { target: { value: 'env:prod-client' } });
    fireEvent.keyDown(values, { key: 'Enter', keyCode: 13, code: 'Enter' });
    await waitFor(() =>
      expect(latest.typeRules[0].when.values).toEqual([{ value: 'PROD', label: 'PROD' }, { value: 'env:prod-client' }]),
    );
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('stores the label of a value whose label differs from the raw value', async () => {
    renderEditor([
      {
        tracker: { id: 12, name: 'Support Ticket' },
        type: 'TASK',
        when: { field: { id: 43, name: 'Ticket Severity' }, op: 'in', values: [] },
      },
    ]);

    await choose('Condition values of rule 1', 'Critical (1)');
    expect(latest.typeRules[0].when.values).toEqual([{ value: '1', label: 'Critical' }]);
  });

  it('clears the values when the condition field changes, and drops the condition on request', async () => {
    renderEditor([
      {
        tracker: { id: 1, name: 'Bug' },
        type: 'INCIDENT',
        when: {
          field: { id: 21, name: 'Discovery Environment' },
          op: 'in',
          values: [{ value: 'PROD', label: 'PROD' }],
        },
      },
    ]);

    await choose('Condition field of rule 1', 'Ticket Severity');
    expect(latest.typeRules[0].when).toEqual({ field: { id: 43, name: 'Ticket Severity' }, op: 'in', values: [] });

    fireEvent.click(screen.getByRole('button', { name: 'Remove condition' }));
    expect(latest.typeRules[0]).toEqual({ tracker: { id: 1, name: 'Bug' }, type: 'INCIDENT' });
  });

  it('tags a rule whose tracker is gone as stale and keeps it', () => {
    renderEditor([{ tracker: { id: 99, name: 'Retired tracker' }, type: 'TASK' }]);

    expect(screen.getByText('stale tracker')).toBeDefined();
    expect(names()).toEqual(['Retired tracker']);
  });

  it('previews the fallback for trackers no rule covers', () => {
    renderEditor([bug], {
      trackers: [...defaultTrackers(), { id: 4, name: 'Fehler', source: 'live' }],
    });

    const preview = screen.getByTestId('type-fallback-preview');
    expect(preview.textContent).toContain('Feature → REQUIREMENT');
    expect(preview.textContent).toContain('Support → TASK');
    expect(preview.textContent).toContain('Fehler → FEHLER');
    expect(preview.textContent).not.toContain('Bug →');
  });

  it('says every tracker has a rule when none is left uncovered', () => {
    renderEditor([bug, feature, support]);

    expect(screen.getByTestId('type-fallback-preview').textContent).toContain('Every tracker has a rule.');
  });

  it('warns that conditions need custom fields when a non-admin key has none to offer', () => {
    renderEditor([bug], { customFields: [], fromCollected: true });

    expect(screen.getByText(/No custom fields to build a condition on/)).toBeDefined();
  });

  it('resets to the built-in mapping after confirming', async () => {
    renderEditor([feature, support]);

    fireEvent.click(screen.getByRole('button', { name: 'Reset to defaults' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Reset' }));
    expect(latest.typeRules).toEqual([{ type: 'BUG', tracker: { id: 1, name: 'Bug' } }]);
  });
});
