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
import { fireEvent, render, screen, waitFor } from '@testing-library/react';

import { DataScopeRemote } from '@/plugins/components/data-scope-remote';

import { Themed, stubBrowserApis } from './test-utils';

beforeAll(stubBrowserApis);

const remote = vi.fn();
const searchRemote = vi.fn();
const batch = vi.fn();

vi.mock('@/api', () => ({
  default: {
    scope: {
      remote: (...args: any[]) => remote(...args),
      searchRemote: (...args: any[]) => searchRemote(...args),
      batch: (...args: any[]) => batch(...args),
      scopeDuplicates: vi.fn().mockResolvedValue({ duplicates: [] }),
    },
  },
}));

const project = (id: number, name: string, parentId: number, fullName = name) => ({
  id,
  identifier: name.toLowerCase().replace(/\W+/g, '-'),
  parentId,
  name,
  description: '',
  url: `https://redmine.example.org/projects/${id}`,
  fullName,
});

// the shapes of the backend's remote-scopes: a project with subprojects is a
// group whose first child is the project itself as a scope; closed projects
// carry a suffix
const entry = (data: ReturnType<typeof project>, parentId: string | null, suffix = '') => ({
  type: 'scope',
  parentId,
  id: String(data.id),
  name: `${data.name}${suffix}`,
  fullName: `${data.fullName}${suffix}`,
  data,
});
const group = (id: number, name: string) => ({
  type: 'group',
  parentId: null,
  id: `group:${id}`,
  name,
  fullName: name,
});

const portal = project(5, 'Portal', 0);
const frontend = project(6, 'Frontend', 5, 'Portal / Frontend');
const wiki = project(9, 'Wiki', 0);
const archive = project(12, 'Archive', 0);

const renderScope = (onChange = vi.fn()) =>
  render(
    <Themed>
      <DataScopeRemote
        plugin="redmine_go"
        connectionId={1}
        selectedScope={[]}
        onChangeSelectedScope={onChange}
        footer={null}
      />
    </Themed>,
  );

describe('redmine_go data scope tree', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    remote.mockImplementation(async (_plugin: string, _connectionId: ID, query: { groupId: string | null }) => {
      if (query.groupId === 'group:5') {
        return {
          children: [entry(portal, 'group:5'), entry(frontend, 'group:5')],
          nextPageToken: '',
        };
      }
      return {
        children: [group(5, 'Portal'), entry(wiki, null), entry(archive, null, ' [closed]')],
        nextPageToken: '',
      };
    });
  });

  it('lists the top level with groups and projects, closed ones suffixed', async () => {
    renderScope();

    expect(await screen.findByText('Portal')).toBeDefined();
    expect(screen.getByText('Wiki')).toBeDefined();
    expect(screen.getByText('Archive [closed]')).toBeDefined();
    expect(remote).toHaveBeenCalledWith('redmine_go', 1, expect.objectContaining({ groupId: null }));
  });

  it('expands a group into the project itself and its subprojects, and selects a subproject by its full name', async () => {
    const onChange = vi.fn();
    renderScope(onChange);

    fireEvent.click(await screen.findByText('Portal'));
    await waitFor(() =>
      expect(remote).toHaveBeenCalledWith('redmine_go', 1, expect.objectContaining({ groupId: 'group:5' })),
    );
    const frontendItem = await screen.findByText('Portal / Frontend');
    expect(frontendItem).toBeDefined();

    fireEvent.click(frontendItem.closest('label') as HTMLElement);
    await waitFor(() => expect(onChange).toHaveBeenCalled());
    const selected = onChange.mock.calls.at(-1)![0];
    expect(selected.map((it: any) => it.id)).toEqual(['6']);
    expect(selected[0].data).toMatchObject({ id: 6, parentId: 5, name: 'Frontend' });
  });

  it('searches projects by name or identifier through the backend and lists matches flat', async () => {
    searchRemote.mockResolvedValue({ children: [entry(frontend, null)], count: 1 });
    renderScope();

    await screen.findByText('Wiki');
    fireEvent.change(screen.getByPlaceholderText(/Search projects/), { target: { value: 'front' } });

    await waitFor(
      () => expect(searchRemote).toHaveBeenCalledWith('redmine_go', 1, { search: 'front', page: 1, pageSize: 50 }),
      { timeout: 3000 },
    );
    expect(await screen.findByText('Portal / Frontend')).toBeDefined();
    expect(screen.queryByText('Wiki')).toBeNull();
  });
});
