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

import { Flex, Radio, Select, Tag, Tooltip } from 'antd';

import type { RedmineGoComponent, RedmineGoComponentSource } from '@/api/plugin/redmine_go';

import type { CustomFieldOption } from '../helpers/options';
import { isStale } from '../helpers/stale';

import { Section } from './section';

interface Props {
  // null or undefined is the plugin's default, the issue category
  component?: RedmineGoComponent | null;
  customFields: CustomFieldOption[];
  onChange: (component: RedmineGoComponent) => void;
}

// ComponentSource picks where an issue's component comes from: nowhere, the
// Redmine issue category, or the values of a custom field.
export const ComponentSource = ({ component, customFields, onChange }: Props) => {
  const source: RedmineGoComponentSource = component?.source ?? 'category';
  const field = component?.field;
  const stale = !!field && isStale(field, customFields);
  const options = [
    ...customFields.map((it) => ({ value: it.id, label: it.name })),
    ...(stale && field ? [{ value: field.id, label: field.name }] : []),
  ];

  return (
    <Section
      title="Component"
      description="What fills an issue’s component. A custom field’s values are joined with commas."
    >
      <Flex gap="middle" align="center" wrap>
        <Radio.Group
          aria-label="Component source"
          value={source}
          onChange={(e) => {
            const next = e.target.value as RedmineGoComponentSource;
            onChange(next === 'customField' ? { source: next, ...(field ? { field } : {}) } : { source: next });
          }}
        >
          <Radio value="none">None</Radio>
          <Radio value="category">Issue category</Radio>
          <Radio value="customField">Custom field</Radio>
        </Radio.Group>
        {source === 'customField' && (
          <>
            <Select
              aria-label="Component custom field"
              style={{ minWidth: 240 }}
              showSearch={{ optionFilterProp: 'label' }}
              placeholder="Custom field"
              options={options}
              value={field?.id}
              onChange={(id: number) => {
                const picked = customFields.find((it) => it.id === id);
                if (picked) {
                  onChange({ source: 'customField', field: { id: picked.id, name: picked.name } });
                }
              }}
            />
            {stale && (
              <Tooltip title="This custom field is not in Redmine or the collected data any more. The setting is kept.">
                <Tag color="error">stale field</Tag>
              </Tooltip>
            )}
          </>
        )}
      </Flex>
    </Section>
  );
};
