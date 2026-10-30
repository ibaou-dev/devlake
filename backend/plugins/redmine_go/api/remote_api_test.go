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
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/helpers/pluginhelper/api"
	dsmodels "github.com/apache/devlake/helpers/pluginhelper/api/models"
	"github.com/apache/devlake/plugins/redmine_go/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProject struct {
	Id         int                    `json:"id"`
	Name       string                 `json:"name"`
	Identifier string                 `json:"identifier"`
	Status     int                    `json:"status"`
	Parent     map[string]interface{} `json:"parent,omitempty"`
}

func project(id int, identifier, name string, parent int) fakeProject {
	p := fakeProject{Id: id, Name: name, Identifier: identifier, Status: 1}
	if parent != 0 {
		p.Parent = map[string]interface{}{"id": parent, "name": "parent"}
	}
	return p
}

// fixtureProjects mirrors the stack Redmine's seeded tree (a project with a
// subproject), plus a closed project, a three-level branch, an orphan whose
// parent the key cannot see, and enough fillers to span three pages.
func fixtureProjects() []fakeProject {
	closed := project(9, "m1-closed", "M1 Closed", 0)
	closed.Status = projectStatusClosed
	projects := []fakeProject{
		project(2, "registry", "Business Registry", 0),
		project(1, "portal", "Citizen Portal", 0),
		project(6, "rgo-fixtures", "RGO Fixtures", 0),
		project(7, "rgo-fixtures-sub", "RGO Fixtures Sub", 6),
		project(8, "rgo-fixtures-b", "RGO Fixtures B", 0),
		closed,
		project(20, "deep-a", "Deep A", 7),
		project(30, "orphan", "Orphan", 999),
	}
	for i := 0; i < 205; i++ {
		projects = append(projects, project(100+i, fmt.Sprintf("filler-%03d", i), fmt.Sprintf("Filler %03d", i), 0))
	}
	return projects
}

type fakeProjectsServer struct {
	server   *httptest.Server
	requests atomic.Int64
	lastKey  string
	queries  []map[string][]string
}

// newFakeProjectsServer serves projects.json the way Redmine does: at most 100
// per page, offset and total_count, and closed projects only when the
// status=* filter is present.
func newFakeProjectsServer(t *testing.T, projects []fakeProject) *fakeProjectsServer {
	f := &fakeProjectsServer{}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects.json" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		f.requests.Add(1)
		f.lastKey = r.Header.Get("X-Redmine-API-Key")
		q := r.URL.Query()
		f.queries = append(f.queries, q)
		limit, offset := 25, 0
		if v := q.Get("limit"); v != "" {
			limit, _ = strconv.Atoi(v)
		}
		limit = min(limit, 100)
		if v := q.Get("offset"); v != "" {
			offset, _ = strconv.Atoi(v)
		}
		allStatuses := q.Get("f[]") == "status" && q.Get("op[status]") == "*"
		visible := []fakeProject{}
		for _, p := range projects {
			if p.Status == projectStatusClosed && !allStatuses {
				continue
			}
			visible = append(visible, p)
		}
		end := min(offset+limit, len(visible))
		page := visible[min(offset, end):end]
		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(map[string]interface{}{
			"projects": page, "total_count": len(visible), "offset": offset, "limit": limit,
		}))
	}))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeProjectsServer) connection() *models.RedmineGoConnection {
	connection := &models.RedmineGoConnection{}
	connection.Endpoint = f.server.URL
	connection.Token = "fixture-key-0123456789"
	return connection
}

func entryIds(entries []scopeEntry) []string {
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.Type + ":" + e.Id
	}
	return ids
}

func TestRemoteScopesTree(t *testing.T) {
	setupTestGlobals()
	fake := newFakeProjectsServer(t, fixtureProjects())
	connection := fake.connection()
	apiClient, err := api.NewApiClientFromConnection(context.TODO(), basicRes, connection)
	require.Nil(t, err)

	t.Run("top level lists groups for projects with subprojects and scopes for the rest", func(t *testing.T) {
		children, next, err := listRedmineRemoteScopes(connection, apiClient, "", struct{}{})
		require.Nil(t, err)
		assert.Nil(t, next, "the level is returned in one page")
		ids := entryIds(children)
		assert.Equal(t, "scope:2", ids[0], "Business Registry sorts first")
		assert.Contains(t, ids, "group:group:6", "RGO Fixtures has a subproject, so it is a group")
		assert.Contains(t, ids, "scope:8", "RGO Fixtures B is a plain scope")
		assert.Contains(t, ids, "scope:9", "closed projects are listed")
		assert.Contains(t, ids, "scope:30", "a project whose parent is invisible is reachable at the top level")
		assert.NotContains(t, ids, "scope:7", "subprojects are not at the top level")
		assert.Len(t, children, 6+205, "every visible root, none of the subprojects")
		for _, e := range children {
			assert.Nil(t, e.ParentId, "top-level entries have no parent")
		}
		byId := map[string]scopeEntry{}
		for _, e := range children {
			byId[e.Id] = e
		}
		assert.Equal(t, "M1 Closed [closed]", byId["9"].Name, "closed projects are marked")
		assert.Equal(t, "M1 Closed", byId["9"].Data.Name, "the stored name stays clean")
		assert.Nil(t, byId["group:6"].Data, "a group carries no scope payload")
		assert.Equal(t, "RGO Fixtures", byId["group:6"].Name)
	})

	t.Run("a group lists the project itself first, then its subprojects", func(t *testing.T) {
		children, _, err := listRedmineRemoteScopes(connection, apiClient, "group:6", struct{}{})
		require.Nil(t, err)
		assert.Equal(t, "scope:6", entryIds(children)[0], "the project itself comes first as a scope")
		require.Len(t, children, 2)
		self := children[0]
		assert.Equal(t, api.RAS_ENTRY_TYPE_SCOPE, self.Type)
		assert.Equal(t, "6", self.Id)
		require.NotNil(t, self.ParentId)
		assert.Equal(t, "group:6", *self.ParentId, "listed under the group of the same project")
		assert.Equal(t, models.RedmineGoProject{
			Id: 6, Identifier: "rgo-fixtures", Name: "RGO Fixtures", Url: connection.Endpoint + "/projects/rgo-fixtures",
		}, *self.Data)
		assert.Equal(t, "group:7", children[1].Id, "RGO Fixtures Sub has a subproject of its own, so it is a group too")
		assert.Equal(t, api.RAS_ENTRY_TYPE_GROUP, children[1].Type)
		assert.Equal(t, "RGO Fixtures / RGO Fixtures Sub", children[1].FullName)
	})

	t.Run("nested groups expand the same way and carry the parent id", func(t *testing.T) {
		children, _, err := listRedmineRemoteScopes(connection, apiClient, "group:7", struct{}{})
		require.Nil(t, err)
		require.Len(t, children, 2)
		assert.Equal(t, "7", children[0].Id)
		assert.EqualValues(t, 6, children[0].Data.ParentId)
		assert.Equal(t, "20", children[1].Id)
		assert.Equal(t, "RGO Fixtures / RGO Fixtures Sub / Deep A", children[1].FullName)
	})

	t.Run("unknown or malformed group ids are rejected", func(t *testing.T) {
		for _, groupId := range []string{"group:4242", "6", "group:x", "group:"} {
			_, _, err := listRedmineRemoteScopes(connection, apiClient, groupId, struct{}{})
			require.NotNil(t, err, groupId)
			assert.Equal(t, errors.BadInput, err.GetType(), groupId)
		}
	})
}

func TestFetchProjectsPagesByTotalCountAndSendsTheKeyInTheHeader(t *testing.T) {
	setupTestGlobals()
	fake := newFakeProjectsServer(t, fixtureProjects())
	connection := fake.connection()
	apiClient, err := api.NewApiClientFromConnection(context.TODO(), basicRes, connection)
	require.Nil(t, err)

	projects, err := fetchProjects(apiClient)
	require.Nil(t, err)
	assert.Len(t, projects, len(fixtureProjects()), "closed projects included, nothing lost across pages")
	assert.EqualValues(t, 3, fake.requests.Load(), "213 projects take three pages of at most 100")
	assert.Equal(t, "fixture-key-0123456789", fake.lastKey)
	for i, q := range fake.queries {
		assert.Equal(t, "100", q["limit"][0])
		assert.Equal(t, strconv.Itoa(i*100), q["offset"][0])
		assert.Empty(t, q["key"], "the key never goes into the query string")
	}
}

func TestFetchProjectsReportsRedmineFailures(t *testing.T) {
	setupTestGlobals()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()
	connection := &models.RedmineGoConnection{}
	connection.Endpoint = server.URL
	connection.Token = "k"
	apiClient, err := api.NewApiClientFromConnection(context.TODO(), basicRes, connection)
	require.Nil(t, err)
	_, _, err = listRedmineRemoteScopes(connection, apiClient, "", struct{}{})
	require.NotNil(t, err)
	assert.Equal(t, http.StatusBadRequest, err.GetType().GetHttpCode())
	assert.Contains(t, err.Error(), "answered 401")
}

func TestSearchRemoteScopes(t *testing.T) {
	setupTestGlobals()
	fake := newFakeProjectsServer(t, fixtureProjects())
	connection := fake.connection()
	apiClient, err := api.NewApiClientFromConnection(context.TODO(), basicRes, connection)
	require.Nil(t, err)
	search := func(term string, page, size int) []scopeEntry {
		entries, err := searchRedmineRemoteScopes(apiClient, &dsmodels.DsRemoteApiScopeSearchParams{Search: term, Page: page, PageSize: size})
		require.Nil(t, err)
		return entries
	}

	t.Run("matches the name, case-insensitively, and returns flat selectable scopes", func(t *testing.T) {
		entries := search("fixtures", 1, 50)
		assert.Equal(t, []string{"scope:6", "scope:8", "scope:7"}, entryIds(entries),
			"parents are selectable in results too; sorted by name")
		for _, e := range entries {
			assert.Equal(t, api.RAS_ENTRY_TYPE_SCOPE, e.Type)
			assert.Nil(t, e.ParentId)
		}
		assert.Equal(t, "RGO Fixtures / RGO Fixtures Sub", entries[2].FullName, "the path disambiguates results")
		assert.Equal(t, fake.server.URL+"/projects/rgo-fixtures-sub", entries[2].Data.Url, "the URL comes from the client's endpoint")
	})

	t.Run("matches the identifier", func(t *testing.T) {
		entries := search("DEEP-", 1, 50)
		assert.Equal(t, []string{"scope:20"}, entryIds(entries))
		assert.Empty(t, search("no such project", 1, 50))
	})

	t.Run("pages the matches", func(t *testing.T) {
		assert.Len(t, search("filler", 1, 100), 100)
		assert.Len(t, search("filler", 3, 100), 5)
		assert.Empty(t, search("filler", 4, 100))
	})
}

func TestProjectTreeToleratesCyclesAndSelfParents(t *testing.T) {
	tree := buildProjectTree([]redmineProject{
		{Id: 1, Name: "Self", Parent: &struct {
			Id int `json:"id"`
		}{Id: 1}},
	})
	entries, err := tree.list("", "https://r")
	require.Nil(t, err)
	assert.Equal(t, []string{"scope:1"}, entryIds(entries))
	assert.Equal(t, "Self", tree.fullName(tree.byId[1]))
}
