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

package migrationscripts

import (
	"github.com/apache/devlake/core/context"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/helpers/migrationhelper"
	"github.com/apache/devlake/plugins/redmine_go/models/migrationscripts/archived"
)

type addIssueTables struct{}

// Up creates the metadata, issue, journal and account tables of the issue
// collection from their archived snapshots.
func (*addIssueTables) Up(basicRes context.BasicRes) errors.Error {
	return migrationhelper.AutoMigrateTables(
		basicRes,
		&archived.RedmineGoTracker{},
		&archived.RedmineGoStatus{},
		&archived.RedmineGoPriority{},
		&archived.RedmineGoCustomField{},
		&archived.RedmineGoVersion{},
		&archived.RedmineGoIssueCategory{},
		&archived.RedmineGoAccount{},
		&archived.RedmineGoObservedField{},
		&archived.RedmineGoIssue{},
		&archived.RedmineGoJournal{},
		&archived.RedmineGoJournalDetail{},
	)
}

func (*addIssueTables) Version() uint64 {
	return 20261030000002
}

func (*addIssueTables) Name() string {
	return "redmine_go issue, journal and metadata schemas"
}
