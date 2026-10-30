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
	"fmt"
	"net/http"
	"net/url"

	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	"github.com/apache/devlake/helpers/pluginhelper/api"
)

// getJson requests a Redmine resource and decodes its JSON body into out. Any
// status but 200 is reported as a bad request that names the path, because the
// API handlers using it run on behalf of a user who can fix the connection.
func getJson(apiClient plugin.ApiClient, path string, query url.Values, out interface{}) errors.Error {
	res, err := apiClient.Get(path, query, nil)
	if err != nil {
		return errors.Default.Wrap(err, fmt.Sprintf("failed to query Redmine %s", path))
	}
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		return errors.HttpStatus(http.StatusBadRequest).New(
			fmt.Sprintf("the Redmine server answered %d for %s, check the connection and the key's permissions", res.StatusCode, path))
	}
	if err := api.UnmarshalResponse(res, out); err != nil {
		return errors.Default.Wrap(err, fmt.Sprintf("failed to decode the Redmine %s response", path))
	}
	return nil
}
