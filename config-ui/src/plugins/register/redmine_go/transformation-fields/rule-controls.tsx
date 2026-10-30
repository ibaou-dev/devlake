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

import { ArrowDownOutlined, ArrowUpOutlined, DeleteOutlined } from '@ant-design/icons';
import { Button, Space, Tooltip } from 'antd';

interface Props {
  // what the controls act on, for the accessible names, for example `rule 2`
  subject: string;
  index: number;
  count: number;
  onMove: (delta: number) => void;
  onRemove: () => void;
}

// RuleControls are the move up, move down and remove buttons of an ordered
// list entry.
export const RuleControls = ({ subject, index, count, onMove, onRemove }: Props) => (
  <Space size={4}>
    <Tooltip title="Move up">
      <Button
        size="small"
        icon={<ArrowUpOutlined />}
        aria-label={`Move ${subject} up`}
        disabled={index === 0}
        onClick={() => onMove(-1)}
      />
    </Tooltip>
    <Tooltip title="Move down">
      <Button
        size="small"
        icon={<ArrowDownOutlined />}
        aria-label={`Move ${subject} down`}
        disabled={index === count - 1}
        onClick={() => onMove(1)}
      />
    </Tooltip>
    <Tooltip title="Remove">
      <Button size="small" danger icon={<DeleteOutlined />} aria-label={`Remove ${subject}`} onClick={onRemove} />
    </Tooltip>
  </Space>
);
