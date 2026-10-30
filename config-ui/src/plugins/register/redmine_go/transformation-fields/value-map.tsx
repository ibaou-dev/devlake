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

import { PlusOutlined } from '@ant-design/icons';
import { Alert, Button, Card, Flex, Input, Select, Space, Table, Tag, Tooltip } from 'antd';
import { useState } from 'react';

import type { RedmineGoValueMapField } from '@/api/plugin/redmine_go';

import type { CustomFieldOption } from '../helpers/options';
import { moveItem, removeItem, replaceItem } from '../helpers/reorder';
import { isStale } from '../helpers/stale';
import {
  addValueMapField,
  buildValueMapRows,
  ENVIRONMENT_CLASSES,
  setValueMapEntry,
  type ValueMapRow,
} from '../helpers/value-maps';

import { RuleControls } from './rule-controls';
import { Section } from './section';

const SCROLL_THRESHOLD = 30;
const SCROLL_HEIGHT = 320;

type Kind = 'severity' | 'environment';

const COPY: Record<Kind, { subject: string; add: string; targetTitle: string; targetPlaceholder: string }> = {
  severity: {
    subject: 'severity field',
    add: 'Add a severity field',
    targetTitle: 'Severity',
    targetPlaceholder: 'Use the label',
  },
  environment: {
    subject: 'environment field',
    add: 'Add an environment field',
    targetTitle: 'Environment class',
    targetPlaceholder: 'Not mapped',
  },
};

interface FieldProps {
  kind: Kind;
  index: number;
  count: number;
  entry: RedmineGoValueMapField;
  customFields: CustomFieldOption[];
  onChange: (entry: RedmineGoValueMapField) => void;
  onMove: (delta: number) => void;
  onRemove: () => void;
}

const ValueMapFieldCard = ({ kind, index, count, entry, customFields, onChange, onMove, onRemove }: FieldProps) => {
  const [unseen, setUnseen] = useState('');
  const field = customFields.find((it) => it.id === entry.field.id);
  const stale = isStale(entry.field, customFields);
  const rows = buildValueMapRows(entry.map, field?.values ?? []);
  const copy = COPY[kind];
  const n = index + 1;

  const setTo = (row: ValueMapRow, to?: string) =>
    onChange({ ...entry, map: setValueMapEntry(entry.map, row.value, row.label, to) });

  const addUnseen = () => {
    const value = unseen.trim();
    if (!value || rows.some((row) => row.value === value)) {
      return;
    }
    // a new unseen value starts with an identity mapping for severity; an
    // environment value waits for the class the user picks
    if (kind === 'severity') {
      onChange({ ...entry, map: setValueMapEntry(entry.map, value, undefined, value) });
    } else {
      onChange({ ...entry, map: [...entry.map, { value, to: '' }] });
    }
    setUnseen('');
  };

  const columns = [
    {
      title: 'Redmine value',
      key: 'value',
      width: '40%',
      render: (_: unknown, row: ValueMapRow) => (
        <>
          {row.label !== row.value ? `${row.label} (${row.value})` : row.value}
          {!row.known && (
            <Tooltip title="Neither Redmine nor the collected data lists this value. The entry is kept.">
              <Tag style={{ marginLeft: 8 }}>not seen</Tag>
            </Tooltip>
          )}
        </>
      ),
    },
    {
      title: copy.targetTitle,
      key: 'to',
      render: (_: unknown, row: ValueMapRow) =>
        kind === 'severity' ? (
          <Input
            aria-label={`Severity of ${row.value}`}
            style={{ width: 240 }}
            allowClear
            placeholder={copy.targetPlaceholder}
            value={row.to ?? ''}
            onChange={(e) => setTo(row, e.target.value)}
          />
        ) : (
          <Select
            aria-label={`Environment class of ${row.value}`}
            style={{ width: 240 }}
            allowClear
            placeholder={copy.targetPlaceholder}
            options={ENVIRONMENT_CLASSES.map((cls) => ({ value: cls, label: cls }))}
            value={row.to || undefined}
            onChange={(to?: string) => setTo(row, to)}
          />
        ),
    },
  ];

  return (
    <Card
      size="small"
      title={
        <Flex gap="small" align="center">
          <span>{entry.field.name}</span>
          {stale && (
            <Tooltip title="This custom field is not in Redmine or the collected data any more. The entry is kept.">
              <Tag color="error">stale field</Tag>
            </Tooltip>
          )}
          {field && field.source === 'observed' && <Tag>from collected data</Tag>}
        </Flex>
      }
      extra={
        <RuleControls
          subject={`${copy.subject} ${n}`}
          index={index}
          count={count}
          onMove={onMove}
          onRemove={onRemove}
        />
      }
    >
      <Table
        rowKey="value"
        size="small"
        columns={columns}
        dataSource={rows}
        pagination={false}
        locale={{ emptyText: 'No values known for this field yet. Add one below.' }}
        scroll={rows.length > SCROLL_THRESHOLD ? { y: SCROLL_HEIGHT } : undefined}
      />
      <Space.Compact style={{ marginTop: 8, width: 420 }}>
        <Input
          aria-label={`Value not seen yet for ${entry.field.name}`}
          placeholder="A value that has not been seen yet"
          value={unseen}
          onChange={(e) => setUnseen(e.target.value)}
          onPressEnter={addUnseen}
        />
        <Button disabled={!unseen.trim()} onClick={addUnseen}>
          Add value
        </Button>
      </Space.Compact>
    </Card>
  );
};

interface Props {
  kind: Kind;
  title: string;
  description: string;
  entries: RedmineGoValueMapField[];
  customFields: CustomFieldOption[];
  fieldsFromCollectedData: boolean;
  onChange: (entries: RedmineGoValueMapField[]) => void;
}

// ValueMapEditor edits an ordered list of custom fields, each with a table
// that translates its values. Used for severity (free-text targets) and
// environment class (the four classes). The first field with a value wins.
export const ValueMapEditor = ({
  kind,
  title,
  description,
  entries,
  customFields,
  fieldsFromCollectedData,
  onChange,
}: Props) => {
  const available = customFields.filter((field) => !entries.some((entry) => entry.field.id === field.id));

  return (
    <Section title={title} description={description}>
      <Flex vertical gap="small">
        {entries.map((entry, index) => (
          <ValueMapFieldCard
            key={entry.field.id}
            kind={kind}
            index={index}
            count={entries.length}
            entry={entry}
            customFields={customFields}
            onChange={(next) => onChange(replaceItem(entries, index, next))}
            onMove={(delta) => onChange(moveItem(entries, index, delta))}
            onRemove={() => onChange(removeItem(entries, index))}
          />
        ))}
        <Flex align="center" gap="small">
          <Select
            aria-label={COPY[kind].add}
            style={{ width: 320 }}
            showSearch={{ optionFilterProp: 'label' }}
            placeholder={`Add a custom field…`}
            suffixIcon={<PlusOutlined />}
            options={available.map((field) => ({ value: field.id, label: field.name }))}
            value={null}
            onChange={(id: number) => {
              const field = customFields.find((it) => it.id === id);
              if (field) {
                onChange(addValueMapField(entries, field));
              }
            }}
          />
          {entries.length === 0 && (
            <span>None: {kind === 'severity' ? 'issues have no severity' : 'no environment class is recorded'}.</span>
          )}
        </Flex>
        {fieldsFromCollectedData && customFields.length === 0 && (
          <Alert
            type="warning"
            showIcon
            title="No custom fields to choose from: this key cannot read them and nothing has been collected yet."
          />
        )}
      </Flex>
    </Section>
  );
};
