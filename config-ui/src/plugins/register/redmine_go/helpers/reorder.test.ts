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

import { moveItem, removeItem, replaceItem } from './reorder';

describe('moveItem', () => {
  it('moves an item up and down without touching the input', () => {
    const list = ['a', 'b', 'c'];
    expect(moveItem(list, 1, -1)).toEqual(['b', 'a', 'c']);
    expect(moveItem(list, 1, 1)).toEqual(['a', 'c', 'b']);
    expect(list).toEqual(['a', 'b', 'c']);
  });

  it('leaves the list alone at either end and for bad indexes', () => {
    const list = ['a', 'b', 'c'];
    expect(moveItem(list, 0, -1)).toBe(list);
    expect(moveItem(list, 2, 1)).toBe(list);
    expect(moveItem(list, 5, -1)).toBe(list);
    expect(moveItem(list, 1, 0)).toBe(list);
  });
});

describe('removeItem and replaceItem', () => {
  it('removes and replaces by index', () => {
    expect(removeItem(['a', 'b', 'c'], 1)).toEqual(['a', 'c']);
    expect(replaceItem(['a', 'b', 'c'], 2, 'z')).toEqual(['a', 'b', 'z']);
  });

  it('ignores an index outside the list', () => {
    const list = ['a'];
    expect(removeItem(list, 3)).toBe(list);
    expect(replaceItem(list, -1, 'z')).toBe(list);
  });
});
