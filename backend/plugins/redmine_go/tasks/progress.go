/*
Licensed to the Apache Software Foundation (ASF) under one or more
contributor license agreements.  See the NOTICE file distributed with
this work for additional information regarding copyright ownership.
The ASF licenses this file to You under the Apache License, Version 2.0
(the "License"); you may not use this file except in compliance with
the License.  You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tasks

import (
	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/plugin"
)

// totalProgressContext is a subtask context whose progress reports carry a
// fixed total. The framework's collectors, extractors and convertors open
// with SetProgress(0, -1), and while the total is unknown the task runner
// writes _devlake_subtasks.finished_records after every single record: one
// auto-committed UPDATE per row, which on a database with a slow commit costs
// several times what the row's conversion does. With a total it writes once
// per percent.
type totalProgressContext struct {
	plugin.SubTaskContext
	total int
}

// SetProgress reports the context's total whatever total the caller passes.
func (c *totalProgressContext) SetProgress(current int, _ int) {
	c.SubTaskContext.SetProgress(current, c.total)
}

// withTotal makes ctx report progress against total records. A total below 1
// leaves the context alone: there is nothing to throttle.
func withTotal(ctx plugin.SubTaskContext, total int64) plugin.SubTaskContext {
	if total < 1 {
		return ctx
	}
	return &totalProgressContext{SubTaskContext: ctx, total: int(total)}
}

// withCountedTotal sizes progress by the rows the clauses select. The count
// only paces the progress reports, so a failed count falls back to the
// context as it is and never fails the subtask.
func withCountedTotal(ctx plugin.SubTaskContext, clauses ...dal.Clause) plugin.SubTaskContext {
	total, err := ctx.GetDal().Count(clauses...)
	if err != nil {
		ctx.GetLogger().Warn(err, "could not count the records of %s for progress reporting", ctx.GetName())
		return ctx
	}
	return withTotal(ctx, total)
}

// withRawRowsTotal sizes progress by the project's rows in a raw table.
func withRawRowsTotal(ctx plugin.SubTaskContext, table, params string) plugin.SubTaskContext {
	if !ctx.GetDal().HasTable(table) {
		return ctx
	}
	return withCountedTotal(ctx, dal.From(table), dal.Where("params = ?", params))
}
