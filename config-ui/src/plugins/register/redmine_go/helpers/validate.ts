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

import type { RedmineGoTransformationData } from '@/api/plugin/redmine_go';

import { validateTypeRule } from './type-rules';

// validateTransformation lists the problems the scope-config API would reject
// with a 400, so the form can block Save and say what to fix. It covers only
// what the form lets a user get wrong; enum values come from selects.
export const validateTransformation = (transformation: RedmineGoTransformationData): string[] => {
  const problems: string[] = [];
  (transformation.typeRules ?? []).forEach((rule, i) => {
    validateTypeRule(rule).forEach((problem) => problems.push(`Type rule ${i + 1} ${problem}`));
  });
  (transformation.environment ?? []).forEach((entry) => {
    entry.map.forEach((mapping) => {
      if (!mapping.to) {
        problems.push(`Environment value “${mapping.value}” of ${entry.field.name} needs a class`);
      }
    });
  });
  const component = transformation.component;
  if (component?.source === 'customField' && !component.field) {
    problems.push('Component needs a custom field');
  }
  return problems;
};
