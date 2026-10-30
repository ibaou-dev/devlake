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

// moveItem returns a copy of the list with the item at `index` moved by
// `delta` places (-1 up, +1 down). Moves past either end, and bad indexes,
// return the list unchanged, so the editors can bind the buttons without
// guarding them.
export const moveItem = <T>(items: T[], index: number, delta: number): T[] => {
  const target = index + delta;
  if (index < 0 || index >= items.length || target < 0 || target >= items.length || delta === 0) {
    return items;
  }
  const next = [...items];
  const [item] = next.splice(index, 1);
  next.splice(target, 0, item);
  return next;
};

// removeItem returns a copy of the list without the item at `index`.
export const removeItem = <T>(items: T[], index: number): T[] =>
  index < 0 || index >= items.length ? items : items.filter((_, i) => i !== index);

// replaceItem returns a copy of the list with the item at `index` replaced.
export const replaceItem = <T>(items: T[], index: number, item: T): T[] =>
  index < 0 || index >= items.length ? items : items.map((it, i) => (i === index ? item : it));
