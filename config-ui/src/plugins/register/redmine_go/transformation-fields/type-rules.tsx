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
import { Alert, Button, Card, Flex, Popconfirm, Select, Tag, Tooltip, theme } from 'antd';

import type { RedmineGoIssueType, RedmineGoTypeRule } from '@/api/plugin/redmine_go';

import { toConditionValues, conditionOptions } from '../helpers/condition';
import type { CustomFieldOption, TrackerOption } from '../helpers/options';
import { moveItem, removeItem, replaceItem } from '../helpers/reorder';
import { isStale } from '../helpers/stale';
import {
  ISSUE_TYPES,
  newTypeRule,
  previewTrackerFallbacks,
  validateTypeRule,
  withoutCondition,
  withTracker,
} from '../helpers/type-rules';

import { RuleControls } from './rule-controls';
import { Section } from './section';

const OPS = [
  { value: 'in', label: 'is one of' },
  { value: 'notIn', label: 'is none of' },
];

interface RuleProps {
  index: number;
  count: number;
  rule: RedmineGoTypeRule;
  trackers: TrackerOption[];
  customFields: CustomFieldOption[];
  onChange: (rule: RedmineGoTypeRule) => void;
  onMove: (delta: number) => void;
  onRemove: () => void;
}

const TypeRuleRow = ({ index, count, rule, trackers, customFields, onChange, onMove, onRemove }: RuleProps) => {
  const { token } = theme.useToken();
  const n = index + 1;
  const problems = validateTypeRule(rule);

  const staleTracker = !!rule.tracker && isStale(rule.tracker, trackers);
  const trackerOptions = [
    ...trackers.map((tracker) => ({ value: tracker.id, label: tracker.name })),
    ...(staleTracker && rule.tracker ? [{ value: rule.tracker.id, label: rule.tracker.name }] : []),
  ];

  const condition = rule.when;
  const conditionField = condition ? customFields.find((field) => field.id === condition.field.id) : undefined;
  const staleField = !!condition?.field.id && !conditionField;
  const fieldOptions = [
    ...customFields.map((field) => ({ value: field.id, label: field.name })),
    ...(staleField && condition ? [{ value: condition.field.id, label: condition.field.name }] : []),
  ];

  const pickTracker = (id?: number) => {
    const tracker = trackers.find((it) => it.id === id);
    onChange(withTracker(rule, tracker ? { id: tracker.id, name: tracker.name } : undefined));
  };

  const pickField = (id: number) => {
    const field = customFields.find((it) => it.id === id);
    if (field && condition) {
      onChange({ ...rule, when: { ...condition, field: { id: field.id, name: field.name }, values: [] } });
    }
  };

  return (
    <Card
      size="small"
      style={problems.length ? { borderColor: token.colorError } : undefined}
      title={
        <Flex gap="small" align="center">
          <span>Rule {n}</span>
          {staleTracker && (
            <Tooltip title="This tracker is not in Redmine or the collected data any more. The rule is kept; an id that no longer exists is matched by name.">
              <Tag color="error">stale tracker</Tag>
            </Tooltip>
          )}
          {staleField && (
            <Tooltip title="This custom field is not in Redmine or the collected data any more. The rule is kept.">
              <Tag color="error">stale field</Tag>
            </Tooltip>
          )}
        </Flex>
      }
      extra={<RuleControls subject={`rule ${n}`} index={index} count={count} onMove={onMove} onRemove={onRemove} />}
    >
      <Flex gap="small" align="center" wrap>
        <span>When the tracker is</span>
        <Select
          aria-label={`Tracker of rule ${n}`}
          style={{ minWidth: 200 }}
          allowClear
          showSearch={{ optionFilterProp: 'label' }}
          placeholder="Any tracker"
          options={trackerOptions}
          value={rule.tracker?.id}
          onChange={(id?: number) => pickTracker(id)}
        />
        {!condition && (
          <Button
            size="small"
            type="dashed"
            icon={<PlusOutlined />}
            onClick={() => onChange({ ...rule, when: { field: { id: 0, name: '' }, op: 'in', values: [] } })}
          >
            Add condition
          </Button>
        )}
        <span>→ type</span>
        <Select
          aria-label={`Type of rule ${n}`}
          style={{ width: 160 }}
          options={ISSUE_TYPES.map((type) => ({ value: type, label: type }))}
          value={rule.type}
          onChange={(type: RedmineGoIssueType) => onChange({ ...rule, type })}
        />
      </Flex>
      {condition && (
        <Flex gap="small" align="center" wrap style={{ marginTop: 8 }}>
          <span>and the field</span>
          <Select
            aria-label={`Condition field of rule ${n}`}
            style={{ minWidth: 200 }}
            showSearch={{ optionFilterProp: 'label' }}
            placeholder="Custom field"
            options={fieldOptions}
            value={condition.field.id || undefined}
            onChange={pickField}
          />
          <Select
            aria-label={`Condition operator of rule ${n}`}
            style={{ width: 120 }}
            options={OPS}
            value={condition.op}
            onChange={(op: 'in' | 'notIn') => onChange({ ...rule, when: { ...condition, op } })}
          />
          <Select
            aria-label={`Condition values of rule ${n}`}
            mode="tags"
            style={{ minWidth: 260, flex: 1 }}
            tokenSeparators={[',']}
            placeholder="Pick values, or type one that has not been seen"
            options={conditionOptions(conditionField?.values ?? [], condition.values)}
            value={condition.values.map((value) => value.value)}
            onChange={(selected: string[]) =>
              onChange({
                ...rule,
                when: {
                  ...condition,
                  values: toConditionValues(selected, condition.values, conditionField?.values ?? []),
                },
              })
            }
          />
          <Button size="small" onClick={() => onChange(withoutCondition(rule))}>
            Remove condition
          </Button>
        </Flex>
      )}
      {problems.length > 0 && (
        <div role="alert" style={{ color: token.colorError, marginTop: 8 }}>
          This rule {problems.join(' and ')}.
        </div>
      )}
    </Card>
  );
};

interface Props {
  rules: RedmineGoTypeRule[];
  trackers: TrackerOption[];
  customFields: CustomFieldOption[];
  // true when custom fields come from collected data only
  fieldsFromCollectedData: boolean;
  onChange: (rules: RedmineGoTypeRule[]) => void;
  onReset: () => void;
}

// TypeRulesEditor edits the ordered type rules. The first rule that matches an
// issue sets its type, so the order matters; trackers no unconditional rule
// covers fall back to the plugin's built-in mapping, previewed below the list.
export const TypeRulesEditor = ({
  rules,
  trackers,
  customFields,
  fieldsFromCollectedData,
  onChange,
  onReset,
}: Props) => {
  const fallbacks = previewTrackerFallbacks(rules, trackers);

  return (
    <Section
      title="Issue type rules"
      description="The first rule that matches an issue sets its type. Put a rule with a condition above the plain rule for the same tracker. The tracker name is always kept as the issue’s original type."
      extra={
        <Popconfirm
          title="Reset type rules?"
          description="Replaces the list with the built-in mapping for the trackers found."
          okText="Reset"
          onConfirm={onReset}
        >
          <Button size="small">Reset to defaults</Button>
        </Popconfirm>
      }
    >
      <Flex vertical gap="small">
        {rules.length === 0 && (
          <Alert type="info" showIcon title="No rules: every tracker uses the built-in fallback below." />
        )}
        {rules.map((rule, index) => (
          <TypeRuleRow
            key={index}
            index={index}
            count={rules.length}
            rule={rule}
            trackers={trackers}
            customFields={customFields}
            onChange={(next) => onChange(replaceItem(rules, index, next))}
            onMove={(delta) => onChange(moveItem(rules, index, delta))}
            onRemove={() => onChange(removeItem(rules, index))}
          />
        ))}
        <div>
          <Button icon={<PlusOutlined />} onClick={() => onChange([...rules, newTypeRule()])}>
            Add rule
          </Button>
        </div>
      </Flex>
      {fieldsFromCollectedData && customFields.length === 0 && (
        <Alert
          style={{ marginTop: 12 }}
          type="warning"
          showIcon
          title="No custom fields to build a condition on: this key cannot read them and nothing has been collected yet."
        />
      )}
      <div style={{ marginTop: 16 }} data-testid="type-fallback-preview">
        <strong>Fallback for trackers no rule covers</strong>
        <div style={{ marginTop: 8 }}>
          {fallbacks.length === 0 ? (
            <span>Every tracker has a rule.</span>
          ) : (
            <Flex gap={8} wrap>
              {fallbacks.map(({ tracker, type, byDefaultName }) => (
                <Tooltip
                  key={tracker.id}
                  title={
                    byDefaultName
                      ? 'Redmine’s default tracker name'
                      : 'Not a default tracker name: the upper-cased name is used'
                  }
                >
                  <Tag color={byDefaultName ? 'processing' : undefined}>
                    {tracker.name} → {type}
                  </Tag>
                </Tooltip>
              ))}
            </Flex>
          )}
        </div>
      </div>
    </Section>
  );
};
