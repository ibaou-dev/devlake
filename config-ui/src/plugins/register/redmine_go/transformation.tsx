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

import { useEffect, useMemo, useRef } from 'react';
import { Alert, Flex } from 'antd';

import API from '@/api';
import type { RedmineGoToggleKey, RedmineGoTransformationData } from '@/api/plugin/redmine_go';
import { PageLoading } from '@/components';
import { useProxyPrefix, useRefreshData } from '@/hooks';

import { applySeed, needsSeed, seedTransformation } from './helpers/defaults';
import { loadRedmineOptions, type RedmineOptions } from './helpers/load-options';
import { validateTransformation } from './helpers/validate';
import { ComponentSource } from './transformation-fields/component-source';
import { StatusMappingTable } from './transformation-fields/status-mapping';
import { Toggles } from './transformation-fields/toggles';
import { TypeRulesEditor } from './transformation-fields/type-rules';
import { ValueMapEditor } from './transformation-fields/value-map';

interface Props {
  entities: string[];
  connectionId: ID;
  scopeConfigId?: ID;
  transformation: RedmineGoTransformationData;
  setTransformation: React.Dispatch<React.SetStateAction<any>>;
  setHasError?: React.Dispatch<React.SetStateAction<boolean>>;
}

export const RedmineGoTransformation = ({
  connectionId,
  scopeConfigId,
  transformation,
  setTransformation,
  setHasError,
}: Props) => {
  const prefix = useProxyPrefix({ plugin: 'redmine_go', connectionId });
  const seeding = useRef(false);

  const { ready, data } = useRefreshData<RedmineOptions>(
    () => loadRedmineOptions(prefix, connectionId),
    [prefix, connectionId],
  );

  const update = (patch: Partial<RedmineGoTransformationData>) =>
    setTransformation((prev: RedmineGoTransformationData) => ({ ...prev, ...patch }));

  // A new scope config opens pre-filled with the mapping the plugin would
  // apply anyway, resolved against the live trackers and statuses, so the
  // admin edits the effective mapping instead of starting empty. The backend
  // answers with it; if that call fails the same mapping is derived from the
  // options already loaded. The seed lands in form state only and is saved
  // with the config.
  const mustSeed = !scopeConfigId && needsSeed(transformation);
  useEffect(() => {
    if (!ready || !data || !mustSeed || seeding.current) {
      return;
    }
    seeding.current = true;
    (async () => {
      let seed: RedmineGoTransformationData;
      try {
        seed = await API.plugin.redmine_go.scopeConfigDefaults(connectionId);
      } catch {
        seed = seedTransformation(data.trackers, data.statuses);
      }
      setTransformation((prev: RedmineGoTransformationData) => (needsSeed(prev) ? applySeed(prev, seed) : prev));
      seeding.current = false;
    })();
  }, [ready, data, mustSeed, connectionId]);

  const problems = useMemo(() => validateTransformation(transformation), [transformation]);

  const hasProblems = problems.length > 0;
  useEffect(() => {
    setHasError?.(hasProblems);
    return () => setHasError?.(false);
  }, [hasProblems]);

  if (!ready || !data || mustSeed) {
    return <PageLoading />;
  }

  const { trackers, statuses, customFields, customFieldsFromCollectedData } = data;
  const seed = seedTransformation(trackers, statuses);

  return (
    <div>
      {data.liveErrors.map((error) => (
        <Alert key={error} style={{ marginBottom: 16 }} type="error" showIcon title={error} />
      ))}
      {customFieldsFromCollectedData && data.redmineUnreadable && (
        <Alert
          style={{ marginBottom: 16 }}
          type="warning"
          showIcon
          title="Redmine could not be read"
          description="Trackers, statuses and custom fields below are the ones seen in collected data. Check the connection, and that the REST web service is enabled under Administration → Settings → API, then reopen this form."
        />
      )}
      {customFieldsFromCollectedData && !data.redmineUnreadable && (
        <Alert
          style={{ marginBottom: 16 }}
          type="info"
          showIcon
          title="Custom-field lists come from collected data"
          description={
            data.nothingCollected
              ? 'This connection’s key is not a Redmine administrator, so it cannot read custom field definitions, and nothing has been collected yet. Custom fields and their values appear here after the first collection, or use an administrator key.'
              : 'This connection’s key is not a Redmine administrator, so it cannot read custom field definitions. The fields and values below are the ones seen in collected data; a value never seen can still be typed in.'
          }
        />
      )}
      <p style={{ marginBottom: 0 }}>
        This is the mapping DevLake applies to the issues of your Redmine. It starts as the built-in default, so a
        default Redmine works without changes; edit it for your own trackers and custom fields.
      </p>
      <TypeRulesEditor
        rules={transformation.typeRules ?? []}
        trackers={trackers}
        customFields={customFields}
        fieldsFromCollectedData={customFieldsFromCollectedData}
        onChange={(typeRules) => update({ typeRules })}
        onReset={() => update({ typeRules: seed.typeRules })}
      />
      <StatusMappingTable
        mappings={transformation.statusMappings ?? []}
        statuses={statuses}
        trackers={trackers}
        onChange={(statusMappings) => update({ statusMappings })}
        onReset={() => update({ statusMappings: seed.statusMappings })}
      />
      <ValueMapEditor
        kind="severity"
        title="Severity"
        description="Fields are tried in order and the first one with a value sets the issue’s severity. A value you do not map keeps its label."
        entries={transformation.severity ?? []}
        customFields={customFields}
        fieldsFromCollectedData={customFieldsFromCollectedData}
        onChange={(severity) => update({ severity })}
      />
      <ValueMapEditor
        kind="environment"
        title="Environment class"
        description="Fields are tried in order and the first one with a value sets the environment class. A value you do not map records nothing."
        entries={transformation.environment ?? []}
        customFields={customFields}
        fieldsFromCollectedData={customFieldsFromCollectedData}
        onChange={(environment) => update({ environment })}
      />
      <ComponentSource
        component={transformation.component}
        customFields={customFields}
        onChange={(component) => update({ component })}
      />
      <Toggles
        transformation={transformation}
        onChange={(key: RedmineGoToggleKey, value: boolean) => update({ [key]: value })}
      />
      {problems.length > 0 && (
        <Flex vertical style={{ marginTop: 24 }}>
          <Alert
            type="error"
            showIcon
            title="Fix these before saving"
            description={
              <ul style={{ margin: 0, paddingLeft: 20 }}>
                {problems.map((problem) => (
                  <li key={problem}>{problem}</li>
                ))}
              </ul>
            }
          />
        </Flex>
      )}
    </div>
  );
};
