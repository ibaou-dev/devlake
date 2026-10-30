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

import { Button, Popconfirm, Select, Table, Tag, Tooltip } from 'antd';

import type { RedmineGoStatusMapping, RedmineGoStatusStd } from '@/api/plugin/redmine_go';

import type { StatusOption, TrackerOption } from '../helpers/options';
import { buildStatusRows, setStatusStd, STATUS_STDS, type StatusRow } from '../helpers/status-rows';

import { Section } from './section';

const SCROLL_THRESHOLD = 30;
const SCROLL_HEIGHT = 420;
// the select value of "no row in the config": the plugin applies its fallback
const DEFAULT = '__default__';

interface Props {
  mappings: RedmineGoStatusMapping[];
  statuses: StatusOption[];
  trackers: TrackerOption[];
  onChange: (mappings: RedmineGoStatusMapping[]) => void;
  onReset: () => void;
}

// StatusMappingTable maps every status of the instance to a standard status.
// A status the config does not mention shows the built-in fallback it gets:
// DONE when closed in Redmine, TODO when it is a tracker's default status,
// IN_PROGRESS otherwise.
export const StatusMappingTable = ({ mappings, statuses, trackers, onChange, onReset }: Props) => {
  const rows = buildStatusRows(statuses, trackers, mappings);

  const columns = [
    {
      title: 'Redmine status',
      key: 'status',
      width: '40%',
      render: (_: unknown, row: StatusRow) => row.status.name,
    },
    {
      title: 'Standard status',
      key: 'std',
      render: (_: unknown, row: StatusRow) => (
        <>
          <Select
            aria-label={`Standard status of ${row.status.name}`}
            style={{ width: 220 }}
            options={[
              ...(row.stale ? [] : [{ value: DEFAULT, label: `Default (${row.fallback})` }]),
              ...STATUS_STDS.map((std) => ({ value: std, label: std })),
            ]}
            value={row.std ?? DEFAULT}
            onChange={(value: string) =>
              onChange(
                setStatusStd(mappings, row.status, value === DEFAULT ? undefined : (value as RedmineGoStatusStd)),
              )
            }
          />
          {row.stale && (
            <Tooltip title="This status is not in Redmine or the collected data any more. The mapping is kept; an id that no longer exists is matched by name.">
              <Tag color="error" style={{ marginLeft: 8 }}>
                stale
              </Tag>
            </Tooltip>
          )}
          {!row.stale && !row.std && (
            <Tooltip title="The config has no row for this status, so the built-in fallback applies. Pick a value to store a mapping.">
              <Tag style={{ marginLeft: 8 }}>not in config</Tag>
            </Tooltip>
          )}
        </>
      ),
    },
  ];

  return (
    <Section
      title="Status mapping"
      description="Every status of the instance maps to a standard status. The preset is the built-in fallback: closed statuses are DONE, a tracker’s default status is TODO, the rest IN_PROGRESS."
      extra={
        <Popconfirm
          title="Reset status mapping?"
          description="Replaces the table with the built-in mapping for the statuses found."
          okText="Reset"
          onConfirm={onReset}
        >
          <Button size="small">Reset to defaults</Button>
        </Popconfirm>
      }
    >
      <Table
        rowKey={(row: StatusRow) => row.status.id}
        size="small"
        columns={columns}
        dataSource={rows}
        pagination={false}
        locale={{ emptyText: 'No statuses found: Redmine could not be read and nothing has been collected yet.' }}
        scroll={rows.length > SCROLL_THRESHOLD ? { y: SCROLL_HEIGHT } : undefined}
      />
    </Section>
  );
};
