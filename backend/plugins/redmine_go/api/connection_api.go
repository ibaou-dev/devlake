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
	"net/url"
	"strings"

	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/apache/devlake/server/api/shared"
)

// endpointExample is the endpoint shape shown in validation errors.
const endpointExample = "https://redmine.example.org"

// maxTestResponseBytes bounds how much of a connection-test response is read.
const maxTestResponseBytes = 1 << 20

// normalizeEndpoint validates a Redmine base URL and returns it without a
// trailing slash. Redmine may live under a sub-URI, so the path is kept:
// `https://host/redmine/` becomes `https://host/redmine`. The URL must be
// absolute, use http or https, and carry neither credentials nor a query:
// credentials would be stored unencrypted, and a query would corrupt every
// request path built on it.
func normalizeEndpoint(endpoint string) (string, errors.Error) {
	normalized := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	reject := func(reason string) (string, errors.Error) {
		return "", errors.BadInput.New(fmt.Sprintf("endpoint %s, expected an http(s) URL such as `%s`", reason, endpointExample))
	}
	u, err := url.Parse(normalized)
	if err != nil || !u.IsAbs() || u.Host == "" {
		return reject("is not an absolute URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return reject(fmt.Sprintf("uses scheme %q", u.Scheme))
	}
	if u.User != nil {
		return reject("must not contain credentials")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return reject("must not contain a query or fragment")
	}
	return normalized, nil
}

// redmineUser holds the fields of `users/current.json` the connection test
// reads. The response also carries the user's `api_key` and e-mail address,
// which are deliberately not decoded.
type redmineUser struct {
	Id        int    `json:"id"`
	Login     string `json:"login"`
	Firstname string `json:"firstname"`
	Lastname  string `json:"lastname"`
	Admin     bool   `json:"admin"`
}

// TestUser identifies the Redmine account behind the API key.
type TestUser struct {
	Id    int    `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
}

// RedmineGoTestConnResponse is the result of a connection test.
type RedmineGoTestConnResponse struct {
	shared.ApiBody
	Connection *models.RedmineGoConn `json:"connection"`
	User       *TestUser             `json:"user"`
	// IsAdmin tells whether the key belongs to a Redmine administrator, which
	// decides whether custom field definitions can be read.
	IsAdmin bool `json:"isAdmin"`
}

// diagnoseTestFailure maps a non-200 `users/current.json` answer to an
// actionable message. Authentication failures are reported as 400 so that the
// browser never pops up a Basic Auth dialog.
func diagnoseTestFailure(statusCode int) errors.Error {
	switch statusCode {
	case http.StatusUnauthorized:
		return errors.HttpStatus(http.StatusBadRequest).New(
			"the Redmine server rejected the API key (401), check it under My account > API access key")
	case http.StatusForbidden:
		return errors.HttpStatus(http.StatusBadRequest).New(
			"the Redmine server answered 403: the REST web service is disabled (Administration > Settings > API) or the key's account is locked")
	case http.StatusNotFound:
		return errors.HttpStatus(http.StatusBadRequest).New(
			"not found (404), check the endpoint; a Redmine under a sub-URI needs it, for example `https://host/redmine`")
	default:
		return errors.HttpStatus(http.StatusBadRequest).New(fmt.Sprintf(
			"unexpected status code %d while testing the connection", statusCode))
	}
}

// parseCurrentUser decodes a `users/current.json` body. A body that is not a
// Redmine user, typically the HTML login page of an authentication proxy or of
// a Redmine whose REST service is off, yields a message that says so.
func parseCurrentUser(body []byte) (*redmineUser, errors.Error) {
	var payload struct {
		User *redmineUser `json:"user"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.User == nil || payload.User.Id == 0 {
		return nil, errors.HttpStatus(http.StatusBadRequest).New(
			"the endpoint did not answer with a Redmine user: it is probably not the Redmine root URL, " +
				"or the REST web service is disabled (Administration > Settings > API) and a login page answered instead")
	}
	return payload.User, nil
}

func (u *redmineUser) displayName() string {
	if name := strings.TrimSpace(u.Firstname + " " + u.Lastname); name != "" {
		return name
	}
	return u.Login
}

func testConnection(ctx context.Context, connection models.RedmineGoConn) (*RedmineGoTestConnResponse, errors.Error) {
	if vld != nil {
		if err := vld.Struct(connection); err != nil {
			return nil, errors.BadInput.Wrap(err, "invalid connection")
		}
	}
	endpoint, err := normalizeEndpoint(connection.Endpoint)
	if err != nil {
		return nil, err
	}
	connection.Endpoint = endpoint
	apiClient, err := helper.NewApiClientFromConnection(ctx, basicRes, &connection)
	if err != nil {
		return nil, errors.BadInput.Wrap(err, "cannot reach Redmine")
	}
	res, err := apiClient.Get("users/current.json", nil, nil)
	if err != nil {
		return nil, errors.BadInput.Wrap(err, "error checking the Redmine connection")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, diagnoseTestFailure(res.StatusCode)
	}
	body, readErr := io.ReadAll(io.LimitReader(res.Body, maxTestResponseBytes))
	if readErr != nil {
		return nil, errors.BadInput.Wrap(readErr, "error reading the Redmine response")
	}
	user, err := parseCurrentUser(body)
	if err != nil {
		return nil, err
	}
	message := fmt.Sprintf("success: authenticated as %s", user.Login)
	if user.Admin {
		message += " with an administrator key"
	} else {
		message += " with a non-administrator key: custom field definitions cannot be read, " +
			"so the mapping form offers the field values seen in collected data instead"
	}
	connection = connection.Sanitize()
	result := &RedmineGoTestConnResponse{
		Connection: &connection,
		User:       &TestUser{Id: user.Id, Login: user.Login, Name: user.displayName()},
		IsAdmin:    user.Admin,
	}
	result.Success = true
	result.Message = message
	return result, nil
}

// TestConnection test redmine_go connection
// @Summary test redmine_go connection
// @Description Test a Redmine connection: authenticates with the API key and reports the account and whether it is an administrator
// @Tags plugins/redmine_go
// @Param body body models.RedmineGoConn true "json body"
// @Success 200  {object} RedmineGoTestConnResponse "Success"
// @Failure 400  {string} errcode.Error "Bad Request"
// @Failure 500  {string} errcode.Error "Internal Error"
// @Router /plugins/redmine_go/test [POST]
func TestConnection(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	var connection models.RedmineGoConn
	if err := helper.Decode(input.Body, &connection, vld); err != nil {
		return nil, err
	}
	result, err := testConnection(context.TODO(), connection)
	if err != nil {
		return nil, plugin.WrapTestConnectionErrResp(basicRes, err)
	}
	return &plugin.ApiResourceOutput{Body: result, Status: http.StatusOK}, nil
}

// TestExistingConnection test redmine_go connection by ID
// @Summary test redmine_go connection
// @Description Test a stored Redmine connection, optionally with fields overridden by the request body; an empty or sanitized token keeps the stored key
// @Tags plugins/redmine_go
// @Param connectionId path int true "connection ID"
// @Success 200  {object} RedmineGoTestConnResponse "Success"
// @Failure 400  {string} errcode.Error "Bad Request"
// @Failure 500  {string} errcode.Error "Internal Error"
// @Router /plugins/redmine_go/connections/{connectionId}/test [POST]
func TestExistingConnection(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	connection, err := dsHelper.ConnApi.FindByPk(input)
	if err != nil {
		return nil, errors.BadInput.Wrap(err, "find connection from db")
	}
	if input.Body != nil {
		if mergeErr := connection.MergeFromRequest(connection, input.Body); mergeErr != nil {
			return nil, errors.BadInput.Wrap(mergeErr, "invalid connection")
		}
	}
	result, testErr := testConnection(context.TODO(), connection.RedmineGoConn)
	if testErr != nil {
		return nil, plugin.WrapTestConnectionErrResp(basicRes, testErr)
	}
	return &plugin.ApiResourceOutput{Body: result, Status: http.StatusOK}, nil
}

// PostConnections create redmine_go connection
// @Summary create redmine_go connection
// @Description Create a Redmine connection; the endpoint is normalized and the rate limit defaults when 0
// @Tags plugins/redmine_go
// @Param body body models.RedmineGoConnection true "json body"
// @Success 200  {object} models.RedmineGoConnection
// @Failure 400  {string} errcode.Error "Bad Request"
// @Failure 500  {string} errcode.Error "Internal Error"
// @Router /plugins/redmine_go/connections [POST]
func PostConnections(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	var connection models.RedmineGoConnection
	if err := helper.DecodeMapStruct(input.Body, &connection, false); err != nil {
		return nil, err
	}
	endpoint, err := normalizeEndpoint(connection.Endpoint)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(connection.Token) == "" {
		return nil, errors.BadInput.New("token is required: it is the Redmine API key")
	}
	input.Body["endpoint"] = endpoint
	if connection.RateLimitPerHour == 0 {
		input.Body["rateLimitPerHour"] = models.DefaultRateLimitPerHour
	}
	return dsHelper.ConnApi.Post(input)
}

// PatchConnection patch redmine_go connection
// @Summary patch redmine_go connection
// @Description Patch a Redmine connection; an empty or sanitized token keeps the stored key
// @Tags plugins/redmine_go
// @Param body body models.RedmineGoConnection true "json body"
// @Success 200  {object} models.RedmineGoConnection
// @Failure 400  {string} errcode.Error "Bad Request"
// @Failure 500  {string} errcode.Error "Internal Error"
// @Router /plugins/redmine_go/connections/{connectionId} [PATCH]
func PatchConnection(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	// a rename-only PATCH carries no endpoint and must not re-trigger the URL rule
	if rawEndpoint, ok := input.Body["endpoint"]; ok {
		endpoint, ok := rawEndpoint.(string)
		if !ok {
			return nil, errors.BadInput.New("endpoint must be a string")
		}
		normalized, err := normalizeEndpoint(endpoint)
		if err != nil {
			return nil, err
		}
		input.Body["endpoint"] = normalized
	}
	return dsHelper.ConnApi.Patch(input)
}

// DeleteConnection delete a redmine_go connection
// @Summary delete a redmine_go connection
// @Description Delete a Redmine connection and, with it, the trackers, statuses, priorities, custom fields, accounts and observed fields collected for it
// @Tags plugins/redmine_go
// @Success 200  {object} models.RedmineGoConnection
// @Failure 400  {string} errcode.Error "Bad Request"
// @Failure 409  {object} services.BlueprintProjectPairs "References exist to this connection"
// @Failure 500  {string} errcode.Error "Internal Error"
// @Router /plugins/redmine_go/connections/{connectionId} [DELETE]
func DeleteConnection(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	connectionId, err := connectionIdOf(input.Params)
	if err != nil {
		return nil, err
	}
	output, err := dsHelper.ConnApi.Delete(input)
	if err != nil {
		return output, err
	}
	if err := DeleteConnectionMetadata(basicRes.GetDal(), connectionId); err != nil {
		return nil, errors.Default.Wrap(err, "the connection was deleted but its collected metadata could not be removed")
	}
	return output, nil
}

// ListConnections get all redmine_go connections
// @Summary get all redmine_go connections
// @Description Get all Redmine connections
// @Tags plugins/redmine_go
// @Success 200  {object} []models.RedmineGoConnection
// @Failure 400  {string} errcode.Error "Bad Request"
// @Failure 500  {string} errcode.Error "Internal Error"
// @Router /plugins/redmine_go/connections [GET]
func ListConnections(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	return dsHelper.ConnApi.GetAll(input)
}

// GetConnection get redmine_go connection detail
// @Summary get redmine_go connection detail
// @Description Get Redmine connection detail
// @Tags plugins/redmine_go
// @Success 200  {object} models.RedmineGoConnection
// @Failure 400  {string} errcode.Error "Bad Request"
// @Failure 500  {string} errcode.Error "Internal Error"
// @Router /plugins/redmine_go/connections/{connectionId} [GET]
func GetConnection(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	return dsHelper.ConnApi.GetDetail(input)
}
