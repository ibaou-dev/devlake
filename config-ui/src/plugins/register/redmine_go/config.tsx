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

import { Flex } from 'antd';

import type { RedmineGoTestResult } from '@/api/plugin/redmine_go';
import { IPluginConfig } from '@/types';

import Icon from './assets/icon.svg?react';

// DEFAULT_RATE_LIMIT matches the backend's default of ten requests a second,
// gentle enough for a small self-hosted Redmine.
const DEFAULT_RATE_LIMIT = 36000;

export const RedmineGoConfig: IPluginConfig = {
  plugin: 'redmine_go',
  name: 'Redmine Go',
  icon: ({ color }) => <Icon fill={color} />,
  sort: 19.6,
  connection: {
    docLink: 'https://www.redmine.org/projects/redmine/wiki/Rest_api',
    initialValues: {
      rateLimitPerHour: DEFAULT_RATE_LIMIT,
    },
    // the test API names the account behind the key and says whether it is an
    // administrator, which decides if the mapping form can read custom fields
    formatTestResult: (res: RedmineGoTestResult) => {
      const user = res.user;
      const who = user ? (user.name && user.name !== user.login ? `${user.name} (${user.login})` : user.login) : '';
      if (!res.isAdmin) {
        return {
          level: 'warning',
          content: (
            <Flex vertical>
              <span>
                {who ? `Connected as ${who}, not an administrator.` : 'Connected with a non-administrator key.'}
              </span>
              <span>
                Custom field definitions cannot be read with this key, so the mapping form offers the values seen in
                collected data instead. Use an administrator key to pick from all custom fields.
              </span>
            </Flex>
          ),
        };
      }
      return {
        level: 'success',
        content: (
          <Flex vertical>
            <span>{who ? `Connected as ${who}, an administrator.` : 'Connected with an administrator key.'}</span>
            <span>Custom fields and their values can be read.</span>
          </Flex>
        ),
      };
    },
    fields: [
      'name',
      {
        key: 'endpoint',
        subLabel:
          'Base URL of your Redmine, without a trailing slash, for example https://redmine.example.org. A Redmine under a sub-URI needs it, for example https://host/redmine.',
      },
      {
        key: 'token',
        label: 'API Key',
        subLabel:
          'Your Redmine REST API key (My account → API access key). The REST web service must be enabled under Administration → Settings → API. An administrator key lets the mapping form read custom fields directly.',
      },
      'proxy',
      {
        key: 'rateLimitPerHour',
        subLabel:
          'Maximum number of API requests per hour. Redmine has no rate limit of its own, so this protects your server. Leave the default unless collection is too slow or Redmine is struggling.',
        defaultValue: DEFAULT_RATE_LIMIT,
      },
    ],
  },
  dataScope: {
    searchPlaceholder: 'Search projects by name or identifier',
    title: 'Projects',
    millerColumn: {
      columnCount: 2.5,
      firstColumnTitle: 'Projects',
    },
  },
  scopeConfig: {
    entities: ['TICKET', 'CROSS'],
    // empty on purpose: the transformation form seeds a new scope config from
    // the connection's scope-config-defaults
    transformation: {},
  },
};
