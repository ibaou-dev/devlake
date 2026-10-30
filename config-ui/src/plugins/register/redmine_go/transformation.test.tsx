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

import { describe, it, expect, vi, beforeAll, beforeEach } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';

import type { RedmineGoTransformationData } from '@/api/plugin/redmine_go';

import { seedTransformation } from './helpers/defaults';
import {
  choose,
  DEFAULT_STATUSES,
  DEFAULT_TRACKERS,
  DEV_CUSTOM_FIELDS,
  defaultStatuses,
  defaultTrackers,
  EMPTY_OBSERVED,
  Harness,
  section,
  stubBrowserApis,
} from './test-utils';
import { RedmineGoTransformation } from './transformation';

beforeAll(stubBrowserApis);

const trackers = vi.fn();
const issueStatuses = vi.fn();
const customFields = vi.fn();
const observedValues = vi.fn();
const scopeConfigDefaults = vi.fn();

vi.mock('@/api', () => ({
  default: {
    plugin: {
      redmine_go: {
        trackers: (...args: any[]) => trackers(...args),
        issueStatuses: (...args: any[]) => issueStatuses(...args),
        customFields: (...args: any[]) => customFields(...args),
        observedValues: (...args: any[]) => observedValues(...args),
        scopeConfigDefaults: (...args: any[]) => scopeConfigDefaults(...args),
      },
    },
  },
}));

const forbidden = Object.assign(new Error('Request failed with status code 403'), { response: { status: 403 } });

let latest: RedmineGoTransformationData;
let hasError: boolean | undefined;

const renderForm = (initial: RedmineGoTransformationData, scopeConfigId?: string) =>
  render(
    <Harness initial={initial} onState={(state) => (latest = state)}>
      {({ transformation, setTransformation }) => (
        <RedmineGoTransformation
          entities={['TICKET', 'CROSS']}
          connectionId={1}
          scopeConfigId={scopeConfigId}
          transformation={transformation}
          setTransformation={setTransformation}
          setHasError={(value) => (hasError = value as boolean)}
        />
      )}
    </Harness>,
  );

// The dev Redmine example of redmine-go-scope-config.md, abridged to what the
// fixtures below know.
const DEV_EXAMPLE: RedmineGoTransformationData = {
  typeRules: [
    {
      tracker: { id: 1, name: 'Bug' },
      when: {
        field: { id: 21, name: 'Discovery Environment' },
        op: 'in',
        values: [{ value: 'PROD', label: 'PROD' }],
      },
      type: 'INCIDENT',
    },
    { tracker: { id: 1, name: 'Bug' }, type: 'BUG' },
    {
      tracker: { id: 3, name: 'Support' },
      when: { field: { id: 43, name: 'Ticket Severity' }, op: 'in', values: [{ value: '1', label: 'Critical' }] },
      type: 'INCIDENT',
    },
    { tracker: { id: 3, name: 'Support' }, type: 'TASK' },
    { tracker: { id: 2, name: 'Feature' }, type: 'REQUIREMENT' },
  ],
  statusMappings: [
    { status: { id: 2, name: 'In Progress' }, std: 'IN_PROGRESS' },
    { status: { id: 3, name: 'Resolved' }, std: 'IN_PROGRESS' },
    { status: { id: 4, name: 'Feedback' }, std: 'OTHER' },
    { status: { id: 5, name: 'Closed' }, std: 'DONE' },
    { status: { id: 37, name: 'Fix Delivered' }, std: 'DONE' },
  ],
  severity: [
    { field: { id: 18, name: 'Bug Severity' }, map: [] },
    {
      field: { id: 43, name: 'Ticket Severity' },
      map: [
        { value: '1', label: 'Critical', to: 'Critical' },
        { value: '2', label: 'Normal', to: 'Normal' },
      ],
    },
  ],
  environment: [
    {
      field: { id: 21, name: 'Discovery Environment' },
      map: [
        { value: 'PROD', to: 'PRODUCTION' },
        { value: 'ACC', to: 'STAGING' },
        { value: 'env:prod-client', label: 'observed registry key', to: 'PRODUCTION' },
      ],
    },
  ],
  component: { source: 'customField', field: { id: 31, name: 'Component' } },
  collectComments: true,
  includePrivateNotes: false,
};

describe('RedmineGoTransformation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    hasError = undefined;
    trackers.mockResolvedValue(DEFAULT_TRACKERS);
    issueStatuses.mockResolvedValue(DEFAULT_STATUSES);
    customFields.mockResolvedValue(DEV_CUSTOM_FIELDS);
    observedValues.mockResolvedValue(EMPTY_OBSERVED);
    scopeConfigDefaults.mockResolvedValue({
      ...seedTransformation(defaultTrackers(), defaultStatuses()),
      reconcileSweep: true,
      collectTimeEntries: true,
      collectChangesets: true,
      collectComments: true,
      includePrivateNotes: false,
      versionsAsSprints: false,
    });
  });

  describe('a new scope config on default Redmine data', () => {
    it('is pre-filled from scope-config-defaults, with the toggles left absent', async () => {
      renderForm({});

      expect(await screen.findByText('Issue type rules')).toBeDefined();
      expect(scopeConfigDefaults).toHaveBeenCalledWith(1);

      expect(latest.typeRules).toEqual([
        { tracker: { id: 1, name: 'Bug' }, type: 'BUG' },
        { tracker: { id: 2, name: 'Feature' }, type: 'REQUIREMENT' },
        { tracker: { id: 3, name: 'Support' }, type: 'TASK' },
      ]);
      expect(latest.statusMappings?.map((m) => `${m.status.name}=${m.std}`)).toEqual([
        'New=TODO',
        'In Progress=IN_PROGRESS',
        'Resolved=IN_PROGRESS',
        'Feedback=IN_PROGRESS',
        'Closed=DONE',
        'Rejected=DONE',
      ]);
      expect(latest.component).toEqual({ source: 'category' });
      expect(latest.severity).toEqual([]);
      expect(latest.environment).toEqual([]);
      for (const key of [
        'reconcileSweep',
        'collectTimeEntries',
        'collectChangesets',
        'collectComments',
        'includePrivateNotes',
        'versionsAsSprints',
      ]) {
        expect(latest).not.toHaveProperty(key);
      }
    });

    it('renders every section from the seeded state', async () => {
      renderForm({});

      await screen.findByText('Issue type rules');
      expect(within(screen.getByRole('region', { name: 'Issue type rules' })).getAllByText(/^Rule \d$/)).toHaveLength(
        3,
      );
      expect(screen.getByRole('combobox', { name: 'Standard status of Rejected' })).toBeDefined();
      expect(screen.getByRole('radio', { name: 'Issue category' })).toBeDefined();
      expect(screen.getByRole('switch', { name: 'Collect comments' })).toBeDefined();
      expect(screen.getByTestId('type-fallback-preview').textContent).toContain('Every tracker has a rule.');
      expect(screen.queryByText('Custom-field lists come from collected data')).toBeNull();
    });

    it('derives the same mapping from the live options when scope-config-defaults fails', async () => {
      scopeConfigDefaults.mockRejectedValue(new Error('404'));
      renderForm({});

      await screen.findByText('Issue type rules');
      expect(latest.typeRules).toHaveLength(3);
      expect(latest.statusMappings).toHaveLength(6);
      expect(latest.statusMappings?.find((m) => m.status.name === 'New')?.std).toBe('TODO');
      expect(latest.component).toEqual({ source: 'category' });
    });

    it('does not seed again once the user edited the form, even down to empty lists', async () => {
      renderForm({ typeRules: [], statusMappings: [] });

      await screen.findByText('Issue type rules');
      expect(scopeConfigDefaults).not.toHaveBeenCalled();
      expect(latest.typeRules).toEqual([]);
      expect(latest.statusMappings).toEqual([]);
    });
  });

  it('never seeds a scope config that already exists, and shows what it holds', async () => {
    renderForm(DEV_EXAMPLE, '7');

    await screen.findByText('Issue type rules');
    expect(scopeConfigDefaults).not.toHaveBeenCalled();
    expect(latest).toEqual(DEV_EXAMPLE);
  });

  it('shows the dev Redmine example with every section filled', async () => {
    renderForm(DEV_EXAMPLE, '7');

    await screen.findByText('Issue type rules');
    expect(section('Issue type rules').getAllByText(/^Rule \d$/)).toHaveLength(5);
    expect(
      screen.getByRole('combobox', { name: 'Condition field of rule 1' }).closest('.ant-select')?.textContent,
    ).toBe('Discovery Environment');
    // the status the instance does not have is stale, the others carry their mapping
    expect(section('Status mapping').getByText('Fix Delivered')).toBeDefined();
    expect(section('Status mapping').getByText('stale')).toBeDefined();
    expect(
      screen.getByRole('combobox', { name: 'Standard status of Feedback' }).closest('.ant-select')?.textContent,
    ).toBe('OTHER');
    // severity: two fields in order; environment: the registry key row
    expect(section('Severity').getByText('Bug Severity')).toBeDefined();
    expect(section('Severity').getByText('Ticket Severity')).toBeDefined();
    expect(section('Environment class').getByText('observed registry key (env:prod-client)')).toBeDefined();
    expect(screen.getByRole('combobox', { name: 'Component custom field' }).closest('.ant-select')?.textContent).toBe(
      'Component',
    );
    // saved toggles show their value without the default tag
    expect(screen.getByRole('switch', { name: 'Collect comments' }).getAttribute('aria-checked')).toBe('true');
    expect(hasError).toBe(false);
  });

  it('writes edits of each section into the transformation', async () => {
    renderForm({});
    await screen.findByText('Issue type rules');

    await choose('Type of rule 3', 'INCIDENT');
    expect(latest.typeRules?.[2]).toEqual({ tracker: { id: 3, name: 'Support' }, type: 'INCIDENT' });

    await choose('Standard status of Feedback', 'OTHER');
    expect(latest.statusMappings?.find((m) => m.status.name === 'Feedback')?.std).toBe('OTHER');

    await choose('Add a severity field', 'Ticket Severity');
    expect(latest.severity?.map((f) => f.field.name)).toEqual(['Ticket Severity']);

    await choose('Add an environment field', 'Discovery Environment');
    expect(latest.environment?.map((f) => f.field.name)).toEqual(['Discovery Environment']);

    fireEvent.click(screen.getByRole('radio', { name: 'None' }));
    expect(latest.component).toEqual({ source: 'none' });

    fireEvent.click(screen.getByRole('switch', { name: 'Include private notes' }));
    expect(latest.includePrivateNotes).toBe(true);
    expect(latest).not.toHaveProperty('collectComments');
  });

  it('tells the admin of a non-admin key that custom fields come from collected data', async () => {
    customFields.mockRejectedValue(forbidden);
    observedValues.mockResolvedValue({
      ...EMPTY_OBSERVED,
      customFields: [
        {
          id: 43,
          name: 'Ticket Severity',
          fieldFormat: 'list',
          multiple: false,
          values: [
            { value: '1', label: 'Critical' },
            { value: 'env:prod-client', label: 'env:prod-client' },
          ],
        },
      ],
    });
    renderForm({});

    expect(await screen.findByText('Custom-field lists come from collected data')).toBeDefined();
    expect(screen.getByText(/not a Redmine administrator/)).toBeDefined();
    expect(screen.queryByText(/nothing has been collected yet/)).toBeNull();

    // the observed field is offered, and mapping still works
    await choose('Add a severity field', 'Ticket Severity');
    expect(section('Severity').getByText('Critical (1)')).toBeDefined();
    expect(section('Severity').getByText('env:prod-client')).toBeDefined();
    // trackers and statuses are still live
    expect(latest.statusMappings).toHaveLength(6);
  });

  it('says nothing has been collected yet for a non-admin key on a fresh connection', async () => {
    customFields.mockRejectedValue(forbidden);
    renderForm({});

    expect((await screen.findAllByText(/nothing has been collected yet/)).length).toBeGreaterThan(0);
    expect(section('Severity').getByText(/No custom fields to choose from/)).toBeDefined();
  });

  it('blocks saving while a rule is incomplete and names the problem', async () => {
    renderForm({});
    await screen.findByText('Issue type rules');
    expect(hasError).toBe(false);

    fireEvent.click(screen.getByRole('button', { name: /Add rule/ }));
    await waitFor(() => expect(hasError).toBe(true));
    expect(screen.getByText('Fix these before saving')).toBeDefined();
    expect(screen.getAllByText('Type rule 4 needs a tracker or a condition').length).toBeGreaterThan(0);

    fireEvent.click(screen.getByRole('button', { name: 'Remove rule 4' }));
    await waitFor(() => expect(hasError).toBe(false));
  });

  it('reports a Redmine that cannot be read, and keeps the observed trackers and statuses', async () => {
    trackers.mockRejectedValue(new Error('connect ECONNREFUSED'));
    issueStatuses.mockRejectedValue(new Error('connect ECONNREFUSED'));
    customFields.mockRejectedValue(new Error('connect ECONNREFUSED'));
    scopeConfigDefaults.mockRejectedValue(new Error('502'));
    observedValues.mockResolvedValue({
      ...EMPTY_OBSERVED,
      trackers: [{ id: 1, name: 'Bug' }],
      statuses: [{ id: 5, name: 'Closed', isClosed: true }],
    });
    renderForm({});

    expect(await screen.findByText(/Trackers could not be loaded from Redmine/)).toBeDefined();
    expect(screen.getByText(/Statuses could not be loaded from Redmine/)).toBeDefined();
    expect(latest.typeRules).toEqual([{ tracker: { id: 1, name: 'Bug' }, type: 'BUG' }]);
    expect(latest.statusMappings).toEqual([{ status: { id: 5, name: 'Closed' }, std: 'DONE' }]);
  });
});
