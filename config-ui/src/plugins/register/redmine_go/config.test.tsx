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
import { render, screen } from '@testing-library/react';

import { getPluginConfig } from '@/plugins';
import { pluginConfigs } from '@/plugins/register';

import { RedmineGoConfig } from './config';

const renderContent = (content: React.ReactNode) => render(<div data-testid="toast">{content}</div>);

describe('redmine_go plugin registration', () => {
  it('is registered next to the Python redmine plugin, under its own plugin id', () => {
    expect(pluginConfigs.filter((config) => config.plugin === 'redmine_go')).toHaveLength(1);
    expect(getPluginConfig('redmine_go')).toBe(RedmineGoConfig);
    expect(getPluginConfig('redmine').name).not.toBe(RedmineGoConfig.name);
    expect(pluginConfigs.map((config) => config.plugin)).toContain('redmine');
  });

  it('asks for endpoint, API key, proxy and a rate limit that defaults to 36000 requests an hour', () => {
    const { fields, initialValues } = RedmineGoConfig.connection;
    const keys = fields.map((field) => (typeof field === 'string' ? field : field.key));
    expect(keys).toEqual(['name', 'endpoint', 'token', 'proxy', 'rateLimitPerHour']);
    expect(fields.find((field) => field.key === 'rateLimitPerHour').defaultValue).toBe(36000);
    expect(initialValues?.rateLimitPerHour).toBe(36000);
    expect(fields.find((field) => field.key === 'token').label).toBe('API Key');
  });

  it('browses projects as a searchable miller-column tree', () => {
    expect(RedmineGoConfig.dataScope.localSearch).toBeUndefined();
    expect(RedmineGoConfig.dataScope.millerColumn?.columnCount).toBeGreaterThan(1);
    expect(RedmineGoConfig.dataScope.searchPlaceholder).toMatch(/search/i);
  });

  it('collects the ticket and cross domains and starts with an empty transformation to be seeded', () => {
    expect(RedmineGoConfig.scopeConfig?.entities).toEqual(['TICKET', 'CROSS']);
    expect(RedmineGoConfig.scopeConfig?.transformation).toEqual({});
  });
});

describe('redmine_go connection test result', () => {
  const format = RedmineGoConfig.connection.formatTestResult!;

  it('names the user and says the key is an administrator key', () => {
    const { level, content } = format({
      success: true,
      message: 'ok',
      isAdmin: true,
      user: { id: 1, login: 'jsmith', name: 'John Smith' },
    });
    renderContent(content);

    expect(level).toBe('success');
    expect(screen.getByTestId('toast').textContent).toContain('Connected as John Smith (jsmith), an administrator.');
    expect(screen.getByText('Custom fields and their values can be read.')).toBeDefined();
  });

  it('warns for a non-admin key that custom fields come from collected data', () => {
    const { level, content } = format({
      success: true,
      message: 'ok',
      isAdmin: false,
      user: { id: 2, login: 'reader', name: 'reader' },
    });
    renderContent(content);

    expect(level).toBe('warning');
    const text = screen.getByTestId('toast').textContent;
    expect(text).toContain('Connected as reader, not an administrator.');
    expect(text).toContain('collected data');
  });

  it('copes with a response that carries no user', () => {
    const { level, content } = format({ success: true, message: 'ok', isAdmin: false });
    renderContent(content);

    expect(level).toBe('warning');
    expect(screen.getByTestId('toast').textContent).toContain('non-administrator key');
  });
});
