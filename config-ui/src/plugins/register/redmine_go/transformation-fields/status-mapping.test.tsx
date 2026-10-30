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

import type { RedmineGoStatusMapping } from '@/api/plugin/redmine_go';

import { seedTransformation } from '../helpers/defaults';
import { choose, defaultStatuses, defaultTrackers, Harness, stubBrowserApis } from '../test-utils';

import { StatusMappingTable } from './status-mapping';

beforeAll(stubBrowserApis);

let latest: any;

const renderTable = (mappings: RedmineGoStatusMapping[]) =>
  render(
    <Harness initial={{ statusMappings: mappings }} onState={(state) => (latest = state)}>
      {({ transformation, setTransformation }) => (
        <StatusMappingTable
          mappings={transformation.statusMappings}
          statuses={defaultStatuses()}
          trackers={defaultTrackers()}
          onChange={(statusMappings) => setTransformation({ ...transformation, statusMappings })}
          onReset={() =>
            setTransformation({
              ...transformation,
              statusMappings: seedTransformation(defaultTrackers(), defaultStatuses()).statusMappings,
            })
          }
        />
      )}
    </Harness>,
  );

const std = (name: string) => latest.statusMappings.find((m: RedmineGoStatusMapping) => m.status.name === name)?.std;

describe('StatusMappingTable', () => {
  it('shows every live status with its seeded mapping', () => {
    renderTable(seedTransformation(defaultTrackers(), defaultStatuses()).statusMappings);

    ['New', 'In Progress', 'Resolved', 'Feedback', 'Closed', 'Rejected'].forEach((name) => {
      expect(screen.getByText(name)).toBeDefined();
    });
    const selected = (name: string) =>
      screen.getByRole('combobox', { name: `Standard status of ${name}` }).closest('.ant-select')?.textContent;
    expect(selected('New')).toBe('TODO');
    expect(selected('Resolved')).toBe('IN_PROGRESS');
    expect(selected('Closed')).toBe('DONE');
    expect(selected('Rejected')).toBe('DONE');
    expect(screen.queryByText('not in config')).toBeNull();
    expect(screen.queryByText('stale')).toBeNull();
  });

  it('changes a status to any of the four standard statuses', async () => {
    renderTable(seedTransformation(defaultTrackers(), defaultStatuses()).statusMappings);

    await choose('Standard status of Feedback', 'OTHER');
    expect(std('Feedback')).toBe('OTHER');
    await choose('Standard status of Rejected', 'IN_PROGRESS');
    expect(std('Rejected')).toBe('IN_PROGRESS');
    expect(latest.statusMappings).toHaveLength(6);
  });

  it('shows the built-in fallback for a status the config does not hold, and stores a value only when picked', async () => {
    renderTable([{ status: { id: 1, name: 'New' }, std: 'TODO' }]);

    expect(screen.getAllByText('not in config')).toHaveLength(5);
    const closed = screen.getByRole('combobox', { name: 'Standard status of Closed' }).closest('.ant-select');
    expect(closed?.textContent).toBe('Default (DONE)');
    expect(latest.statusMappings).toHaveLength(1);

    await choose('Standard status of Closed', 'TODO');
    expect(std('Closed')).toBe('TODO');
    expect(latest.statusMappings).toHaveLength(2);

    await choose('Standard status of Closed', 'Default (DONE)');
    expect(std('Closed')).toBeUndefined();
    expect(latest.statusMappings).toHaveLength(1);
  });

  it('tags a mapped status that is gone as stale and keeps its value', () => {
    renderTable([{ status: { id: 55, name: 'Awaiting client' }, std: 'OTHER' }]);

    expect(screen.getByText('Awaiting client')).toBeDefined();
    expect(screen.getByText('stale')).toBeDefined();
    expect(latest.statusMappings).toEqual([{ status: { id: 55, name: 'Awaiting client' }, std: 'OTHER' }]);
  });

  it('resets to the built-in mapping after confirming', async () => {
    renderTable([{ status: { id: 5, name: 'Closed' }, std: 'OTHER' }]);

    fireEvent.click(screen.getByRole('button', { name: 'Reset to defaults' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Reset' }));
    expect(latest.statusMappings).toHaveLength(6);
    expect(std('Closed')).toBe('DONE');
  });
});
