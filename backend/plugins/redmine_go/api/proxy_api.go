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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"

	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
)

// maxProxyResponseBytes bounds how much of a metadata answer is read. Custom
// field lists with many possible values are the largest answers.
const maxProxyResponseBytes = 32 << 20

// proxyAllowlist holds the Redmine paths the config UI may read through the
// proxy: the metadata that fills the mapping forms. Anything else is refused.
// Redmine's `users/current.json` in particular returns the caller's own API
// key, so a generic forwarder would hand the connection's secret to any
// client of the DevLake API.
var proxyAllowlist = []*regexp.Regexp{
	regexp.MustCompile(`^projects\.json$`),
	regexp.MustCompile(`^projects/[A-Za-z0-9_-]+\.json$`),
	regexp.MustCompile(`^projects/[A-Za-z0-9_-]+/versions\.json$`),
	regexp.MustCompile(`^projects/[A-Za-z0-9_-]+/issue_categories\.json$`),
	regexp.MustCompile(`^trackers\.json$`),
	regexp.MustCompile(`^issue_statuses\.json$`),
	regexp.MustCompile(`^enumerations/[a-z_]+\.json$`),
	regexp.MustCompile(`^custom_fields\.json$`),
}

// proxyPathAllowed reports whether a request path, with its leading slash
// removed, is one of the allowlisted metadata paths. A path that is not in
// canonical form (dot segments, doubled slashes) is refused outright, so
// traversal cannot reach a path outside the list.
func proxyPathAllowed(requestPath string) bool {
	if requestPath == "" || path.Clean(requestPath) != requestPath {
		return false
	}
	for _, allowed := range proxyAllowlist {
		if allowed.MatchString(requestPath) {
			return true
		}
	}
	return false
}

// Proxy forwards a GET request for Redmine metadata to the REST API through
// the connection. The config UI uses it to load trackers, statuses,
// priorities, custom fields, versions and issue categories live, so mapping
// forms offer the instance's real values. Only the allowlisted paths are
// forwarded; any other path is answered with 403.
// @Summary Forward an allowlisted metadata request to the Redmine REST API
// @Description Forward a GET request for Redmine metadata (projects, trackers, issue statuses, enumerations, custom fields, versions, issue categories) through the connection's key; other paths are refused with 403
// @Tags plugins/redmine_go
// @Param connectionId path int true "connection ID"
// @Param path path string true "allowlisted path of the Redmine REST API"
// @Success 200 {object} interface{} "Success"
// @Failure 400  {object} shared.ApiBody "Bad Request"
// @Failure 403  {object} shared.ApiBody "Path not allowed"
// @Failure 500  {object} shared.ApiBody "Internal Error"
// @Router /plugins/redmine_go/connections/{connectionId}/proxy/rest/{path} [GET]
func Proxy(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	requestPath := strings.TrimPrefix(input.Params["path"], "/")
	if !proxyPathAllowed(requestPath) {
		return nil, errors.Forbidden.New(fmt.Sprintf(
			"the proxy only forwards Redmine metadata paths (projects, trackers, issue_statuses, enumerations, custom_fields, versions, issue_categories); %q is not allowed",
			requestPath))
	}
	connection, err := dsHelper.ConnApi.FindByPk(input)
	if err != nil {
		return nil, err
	}
	apiClient, err := helper.NewApiClientFromConnection(context.TODO(), basicRes, connection)
	if err != nil {
		return nil, transportError(requestPath, err)
	}
	res, err := apiClient.Get(requestPath, input.Query, nil)
	if err != nil {
		return nil, transportError(requestPath, err)
	}
	defer res.Body.Close()
	return forwardMetadata(requestPath, res)
}

// forwardMetadata turns Redmine's answer to a metadata request into the proxy
// response. Anything but a 200 with a JSON body becomes a 400 that says what
// to check, instead of surfacing a JSON decoding error from an empty 403 body
// (what Redmine sends to a non-administrator key and to a disabled REST web
// service). Only `x-` response headers pass through, such as paging hints.
func forwardMetadata(requestPath string, res *http.Response) (*plugin.ApiResourceOutput, errors.Error) {
	if res.StatusCode != http.StatusOK {
		return nil, statusError(requestPath, res.StatusCode)
	}
	body, readErr := io.ReadAll(io.LimitReader(res.Body, maxProxyResponseBytes))
	if readErr != nil {
		return nil, errors.Default.Wrap(readErr, fmt.Sprintf("error reading the Redmine %s response", requestPath))
	}
	if !json.Valid(body) {
		return nil, errors.HttpStatus(http.StatusBadRequest).New(fmt.Sprintf(
			"the Redmine server did not answer %s with JSON: the endpoint is probably not the Redmine root URL", requestPath))
	}
	headers := http.Header{}
	for name, values := range res.Header {
		if !strings.HasPrefix(strings.ToLower(name), "x-") {
			continue
		}
		for _, value := range values {
			headers.Add(name, value)
		}
	}
	return &plugin.ApiResourceOutput{Status: res.StatusCode, Body: json.RawMessage(body), Header: headers}, nil
}
