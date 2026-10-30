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
	stderrors "errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"syscall"

	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	"github.com/apache/devlake/helpers/pluginhelper/api"
)

// statusError reports a non-200 answer of Redmine as a 400 that names the path
// and says what usually causes it. Redmine answers 403 both for a key that may
// not read a resource and for a disabled REST web service, so the message
// names both.
func statusError(path string, status int) errors.Error {
	hint := "check the connection and the key's permissions"
	switch status {
	case http.StatusUnauthorized:
		hint = "the API key is wrong or was revoked (My account > API access key)"
	case http.StatusForbidden:
		hint = "the key may not read this resource (custom fields need an administrator key) " +
			"or the REST web service is disabled (Administration > Settings > API)"
	case http.StatusNotFound:
		hint = "check the endpoint; a Redmine under a sub-URI needs it, for example `https://host/redmine`"
	}
	return errors.HttpStatus(http.StatusBadRequest).New(
		fmt.Sprintf("the Redmine server answered %d for %s: %s", status, path, hint))
}

// transportError turns a failed request into a one-line message the browser
// can show. The wrapped error chain is long and full of Go type names, so only
// the cause class is kept and the chain goes to the log.
func transportError(path string, cause errors.Error) errors.Error {
	basicRes.GetLogger().Warn(cause, "cannot reach Redmine")
	return errors.HttpStatus(http.StatusBadRequest).New(fmt.Sprintf(
		"cannot reach Redmine for %s: %s; check the endpoint, the network and the proxy", path, transportReason(cause)))
}

// transportReason names the class of a transport failure.
func transportReason(cause error) string {
	var dnsErr *net.DNSError
	var netErr net.Error
	switch {
	case stderrors.As(cause, &dnsErr):
		return "the host name does not resolve"
	case stderrors.Is(cause, syscall.ECONNREFUSED):
		return "the connection was refused"
	case stderrors.As(cause, &netErr) && netErr.Timeout():
		return "the request timed out"
	default:
		return "the request failed"
	}
}

// getJson requests a Redmine resource and decodes its JSON body into out. Any
// status but 200 is reported as a bad request that names the path, because the
// API handlers using it run on behalf of a user who can fix the connection.
func getJson(apiClient plugin.ApiClient, path string, query url.Values, out interface{}) errors.Error {
	res, err := apiClient.Get(path, query, nil)
	if err != nil {
		return transportError(path, err)
	}
	if res.StatusCode != http.StatusOK {
		_ = res.Body.Close()
		return statusError(path, res.StatusCode)
	}
	if err := api.UnmarshalResponse(res, out); err != nil {
		return errors.Default.Wrap(err, fmt.Sprintf("failed to decode the Redmine %s response", path))
	}
	return nil
}
