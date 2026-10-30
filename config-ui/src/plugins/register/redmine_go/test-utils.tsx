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

import { useState, type ReactNode } from 'react';
import { expect } from 'vitest';
import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import { ThemeProvider as StyledThemeProvider } from 'styled-components';

import type {
  RedmineCustomField,
  RedmineGoObservedValues,
  RedmineIssueStatus,
  RedmineTracker,
} from '@/api/plugin/redmine_go';
import { getTheme } from '@/theme/tokens';

import type { CustomFieldOption, StatusOption, TrackerOption } from './helpers/options';
import { mergeCustomFields, mergeStatuses, mergeTrackers } from './helpers/options';

// jsdom has no matchMedia, which antd's responsive observer requires
export const stubBrowserApis = () => {
  window.matchMedia =
    window.matchMedia ||
    ((query: string) =>
      ({
        matches: false,
        media: query,
        onchange: null,
        addListener: () => {},
        removeListener: () => {},
        addEventListener: () => {},
        removeEventListener: () => {},
        dispatchEvent: () => false,
      }) as MediaQueryList);

  // jsdom has no ResizeObserver, which rc-components (Table, Select) use
  class ResizeObserverStub {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  window.ResizeObserver = window.ResizeObserver || (ResizeObserverStub as any);
};

export const Themed = ({ children }: { children: ReactNode }) => (
  <StyledThemeProvider theme={getTheme('light')}>{children}</StyledThemeProvider>
);

// Redmine's default data: three trackers whose default status is New, and the
// six statuses of a fresh install.
export const DEFAULT_TRACKERS: RedmineTracker[] = [
  { id: 1, name: 'Bug', default_status: { id: 1, name: 'New' } },
  { id: 2, name: 'Feature', default_status: { id: 1, name: 'New' } },
  { id: 3, name: 'Support', default_status: { id: 1, name: 'New' } },
];

export const DEFAULT_STATUSES: RedmineIssueStatus[] = [
  { id: 1, name: 'New', is_closed: false },
  { id: 2, name: 'In Progress', is_closed: false },
  { id: 3, name: 'Resolved', is_closed: false },
  { id: 4, name: 'Feedback', is_closed: false },
  { id: 5, name: 'Closed', is_closed: true },
  { id: 6, name: 'Rejected', is_closed: true },
];

// Custom fields of the dev Redmine example in redmine-go-scope-config.md.
export const DEV_CUSTOM_FIELDS: RedmineCustomField[] = [
  {
    id: 21,
    name: 'Discovery Environment',
    customized_type: 'issue',
    field_format: 'list',
    possible_values: [
      { value: 'PROD', label: 'PROD' },
      { value: 'ACC', label: 'ACC' },
      { value: 'UAT', label: 'UAT' },
      { value: 'DEV', label: 'DEV' },
    ],
  },
  {
    id: 43,
    name: 'Ticket Severity',
    customized_type: 'issue',
    field_format: 'list',
    possible_values: [
      { value: '1', label: 'Critical' },
      { value: '2', label: 'Normal' },
    ],
  },
  { id: 18, name: 'Bug Severity', customized_type: 'issue', field_format: 'list', possible_values: [] },
  { id: 31, name: 'Component', customized_type: 'issue', field_format: 'string' },
  // not an issue field: never offered
  { id: 99, name: 'Hours per day', customized_type: 'user', field_format: 'int' },
];

export const EMPTY_OBSERVED: RedmineGoObservedValues = { trackers: [], statuses: [], customFields: [], categories: [] };

export const defaultTrackers = (): TrackerOption[] => mergeTrackers(DEFAULT_TRACKERS, []);
export const defaultStatuses = (): StatusOption[] => mergeStatuses(DEFAULT_STATUSES, []);
export const devCustomFields = (): CustomFieldOption[] => mergeCustomFields(DEV_CUSTOM_FIELDS, []);

// Harness owns the transformation state like the scope-config form does and
// reports every state to `onState`.
export const Harness = ({
  initial,
  onState,
  children,
}: {
  initial: any;
  onState?: (state: any) => void;
  children: (props: { transformation: any; setTransformation: React.Dispatch<React.SetStateAction<any>> }) => ReactNode;
}) => {
  const [transformation, setTransformation] = useState<any>(initial);
  onState?.(transformation);
  return <Themed>{children({ transformation, setTransformation })}</Themed>;
};

// choose opens the antd Select with the given accessible name and clicks the
// option with the given title. antd renders each select's dropdown in a portal
// and keeps it in the DOM after closing, hidden or still leaving, so only the
// dropdown that is open is searched; each option carries a `title`.
export const choose = async (selectName: string | RegExp, optionTitle: string) => {
  const select = screen.getByRole('combobox', { name: selectName });
  fireEvent.mouseDown(select);
  const option = await waitFor(() => {
    const found = Array.from(
      document.querySelectorAll(
        '.ant-select-dropdown:not(.ant-select-dropdown-hidden):not(.ant-slide-up-leave) .ant-select-item-option',
      ),
    ).find((el) => el.getAttribute('title') === optionTitle);
    if (!found) {
      throw new Error(`option ${optionTitle} not found`);
    }
    return found;
  });
  fireEvent.click(option);
  // a multiple select stays open after a pick; close it so that the next
  // choose finds a single open dropdown
  fireEvent.keyDown(select, { key: 'Escape', code: 'Escape', keyCode: 27 });
  await waitFor(() => expect(select.getAttribute('aria-expanded')).toBe('false'));
};

// within the section named by its aria-label
export const section = (name: string) => within(screen.getByRole('region', { name }));
