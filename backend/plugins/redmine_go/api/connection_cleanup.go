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

package api

import (
	"strconv"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/utils"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/apache/devlake/plugins/redmine_go/tasks"
)

// connectionRawTables lists the raw tables whose rows belong to a whole
// connection rather than to one project, without the `_raw_` prefix.
var connectionRawTables = []string{
	tasks.RAW_TRACKERS_TABLE,
	tasks.RAW_STATUSES_TABLE,
	tasks.RAW_PRIORITIES_TABLE,
	tasks.RAW_CUSTOM_FIELDS_TABLE,
}

// connectionToolTables lists the tool models whose rows belong to a whole
// connection: the metadata every scope shares, the accounts and the custom
// fields seen on issues. Deleting a scope removes only what its own raw rows
// produced, which leaves these behind.
func connectionToolTables() []interface{} {
	return []interface{}{
		&models.RedmineGoTracker{}, &models.RedmineGoStatus{}, &models.RedmineGoPriority{},
		&models.RedmineGoCustomField{}, &models.RedmineGoAccount{}, &models.RedmineGoObservedField{},
	}
}

// DeleteConnectionMetadata removes the connection-level metadata of a
// connection, tool and raw rows, in one transaction. The framework deletes a
// connection only after its scopes, and scope deletion cannot reach these rows:
// they belong to the connection, and a connection id is never reused, so
// without this they would stay in the database for good.
func DeleteConnectionMetadata(db dal.Dal, connectionId uint64) (err errors.Error) {
	tx := db.Begin()
	defer func() {
		if err != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				basicRes.GetLogger().Error(rollbackErr, "could not roll back the deletion of the metadata of connection %d", connectionId)
			}
			return
		}
		err = tx.Commit()
	}()

	for _, model := range connectionToolTables() {
		if err = tx.Delete(model, dal.Where("connection_id = ?", connectionId)); err != nil {
			return err
		}
	}
	params := utils.ToJsonString(models.RedmineGoConnectionParams{ConnectionId: connectionId})
	for _, table := range connectionRawTables {
		raw := "_raw_" + table
		// A raw table exists only once a run has collected into it.
		if !tx.HasTable(raw) {
			continue
		}
		if err = tx.Exec("DELETE FROM "+raw+" WHERE params = ?", params); err != nil {
			return err
		}
	}
	return nil
}

// connectionIdOf reads the connection id of a request path.
func connectionIdOf(params map[string]string) (uint64, errors.Error) {
	id, err := strconv.ParseUint(params["connectionId"], 10, 64)
	if err != nil {
		return 0, errors.BadInput.Wrap(err, "connectionId must be a number")
	}
	return id, nil
}
