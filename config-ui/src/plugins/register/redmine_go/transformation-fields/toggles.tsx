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

import { Flex, Switch, Tag, theme } from 'antd';

import type { RedmineGoToggleKey, RedmineGoTransformationData } from '@/api/plugin/redmine_go';

import { isToggleSet, toggleValue, TOGGLES } from '../helpers/toggles';

import { Section } from './section';

interface Props {
  transformation: RedmineGoTransformationData;
  onChange: (key: RedmineGoToggleKey, value: boolean) => void;
}

// Toggles lists the collection switches. A toggle the scope config does not
// hold shows its default and is tagged as such; it is written only when the
// user flips it.
export const Toggles = ({ transformation, onChange }: Props) => {
  const { token } = theme.useToken();
  return (
    <Section
      title="Collection options"
      description="Optional behaviours of the collection. An environment flag on the server can still switch one off for everyone."
    >
      <Flex vertical gap="middle">
        {TOGGLES.map((def) => (
          <Flex key={def.key} gap="middle" align="flex-start">
            <Switch
              aria-label={def.label}
              checked={toggleValue(transformation, def)}
              onChange={(checked) => onChange(def.key, checked)}
            />
            <div>
              <div>
                {def.label}
                {!isToggleSet(transformation, def.key) && (
                  <Tag style={{ marginLeft: 8 }}>default: {def.defaultValue ? 'on' : 'off'}</Tag>
                )}
              </div>
              <div style={{ color: token.colorTextSecondary }}>{def.help}</div>
            </div>
          </Flex>
        ))}
      </Flex>
    </Section>
  );
};
