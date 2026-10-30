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
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/plugin"
	"github.com/apache/devlake/helpers/pluginhelper/api"
	dsmodels "github.com/apache/devlake/helpers/pluginhelper/api/models"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// projectsPageSize is the page size of `projects.json`; Redmine caps `limit`
// at 100.
const projectsPageSize = 100

// groupIdPrefix tells a group entry's id from the id of the scope entry of the
// same project: the config UI selects by id, so the two must differ.
const groupIdPrefix = "group:"

// projectStatusClosed is the `status` Redmine reports for a closed project.
const projectStatusClosed = 5

// redmineProject mirrors the fields of `projects.json` the scope tree reads.
type redmineProject struct {
	Id          int    `json:"id"`
	Identifier  string `json:"identifier"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      int    `json:"status"`
	Parent      *struct {
		Id int `json:"id"`
	} `json:"parent"`
}

func (p *redmineProject) parentId() int {
	if p.Parent == nil {
		return 0
	}
	return p.Parent.Id
}

// fetchProjects reads every project the API key can see, closed ones included,
// following `total_count`. Archived projects are not served by Redmine's API
// at all. The `status=*` filter is what makes closed projects appear: by
// default the list holds active projects only.
func fetchProjects(apiClient plugin.ApiClient) ([]redmineProject, errors.Error) {
	var projects []redmineProject
	for offset := 0; ; {
		query := url.Values{
			"limit":      {strconv.Itoa(projectsPageSize)},
			"offset":     {strconv.Itoa(offset)},
			"set_filter": {"1"},
			"f[]":        {"status"},
			"op[status]": {"*"},
		}
		var page struct {
			Projects   []redmineProject `json:"projects"`
			TotalCount int              `json:"total_count"`
		}
		if err := getJson(apiClient, "projects.json", query, &page); err != nil {
			return nil, err
		}
		projects = append(projects, page.Projects...)
		offset += len(page.Projects)
		if len(page.Projects) == 0 || offset >= page.TotalCount {
			return projects, nil
		}
	}
}

// projectTree indexes the visible projects by parent. A project whose parent
// the key cannot see is a root, so no visible project is unreachable.
type projectTree struct {
	byId     map[int]*redmineProject
	children map[int][]*redmineProject
	roots    []*redmineProject
}

func buildProjectTree(projects []redmineProject) *projectTree {
	tree := &projectTree{
		byId:     make(map[int]*redmineProject, len(projects)),
		children: make(map[int][]*redmineProject),
	}
	for i := range projects {
		tree.byId[projects[i].Id] = &projects[i]
	}
	for i := range projects {
		p := &projects[i]
		if _, visible := tree.byId[p.parentId()]; visible && p.parentId() != p.Id {
			tree.children[p.parentId()] = append(tree.children[p.parentId()], p)
		} else {
			tree.roots = append(tree.roots, p)
		}
	}
	sortProjects(tree.roots)
	for _, list := range tree.children {
		sortProjects(list)
	}
	return tree
}

func sortProjects(list []*redmineProject) {
	sort.SliceStable(list, func(i, j int) bool {
		a, b := strings.ToLower(list[i].Name), strings.ToLower(list[j].Name)
		if a != b {
			return a < b
		}
		return list[i].Id < list[j].Id
	})
}

// fullName returns the project's name preceded by its visible ancestors,
// for example "Portal / Frontend".
func (t *projectTree) fullName(p *redmineProject) string {
	names := []string{displayName(p)}
	seen := map[int]bool{p.Id: true}
	for parent := t.byId[p.parentId()]; parent != nil && !seen[parent.Id]; parent = t.byId[parent.parentId()] {
		seen[parent.Id] = true
		names = append([]string{parent.Name}, names...)
	}
	return strings.Join(names, " / ")
}

// displayName marks closed projects, which stay selectable.
func displayName(p *redmineProject) string {
	if p.Status == projectStatusClosed {
		return p.Name + " [closed]"
	}
	return p.Name
}

func projectUrl(endpoint, identifier string) string {
	return strings.TrimRight(endpoint, "/") + "/projects/" + identifier
}

type scopeEntry = dsmodels.DsRemoteApiScopeListEntry[models.RedmineGoProject]

// scopeEntry builds the selectable entry of a project. parent is the group the
// entry is listed under, or nil at the top level.
func (t *projectTree) scopeEntry(p *redmineProject, parent *string, endpoint string) scopeEntry {
	return scopeEntry{
		Type:     api.RAS_ENTRY_TYPE_SCOPE,
		ParentId: parent,
		Id:       strconv.Itoa(p.Id),
		Name:     displayName(p),
		FullName: t.fullName(p),
		Data: &models.RedmineGoProject{
			Id:          uint64(p.Id),
			Identifier:  p.Identifier,
			ParentId:    uint64(p.parentId()),
			Name:        p.Name,
			Description: p.Description,
			Url:         projectUrl(endpoint, p.Identifier),
		},
	}
}

// groupEntry builds the expandable entry of a project that has subprojects.
// Groups are not selectable in the config UI, so the project itself is listed
// as the first scope entry inside its group.
func (t *projectTree) groupEntry(p *redmineProject, parent *string) scopeEntry {
	return scopeEntry{
		Type:     api.RAS_ENTRY_TYPE_GROUP,
		ParentId: parent,
		Id:       groupIdPrefix + strconv.Itoa(p.Id),
		Name:     displayName(p),
		FullName: t.fullName(p),
	}
}

// entryFor returns a group entry for a project with subprojects and a scope
// entry for a leaf project.
func (t *projectTree) entryFor(p *redmineProject, parent *string, endpoint string) scopeEntry {
	if len(t.children[p.Id]) > 0 {
		return t.groupEntry(p, parent)
	}
	return t.scopeEntry(p, parent, endpoint)
}

// list returns the entries of one tree level: the top level when groupId is
// empty, otherwise the project of the group followed by its subprojects.
func (t *projectTree) list(groupId, endpoint string) ([]scopeEntry, errors.Error) {
	if groupId == "" {
		entries := make([]scopeEntry, 0, len(t.roots))
		for _, p := range t.roots {
			entries = append(entries, t.entryFor(p, nil, endpoint))
		}
		return entries, nil
	}
	id, err := strconv.Atoi(strings.TrimPrefix(groupId, groupIdPrefix))
	parent := t.byId[id]
	if err != nil || !strings.HasPrefix(groupId, groupIdPrefix) || parent == nil {
		return nil, errors.BadInput.New(fmt.Sprintf("unknown project group %q", groupId))
	}
	entries := make([]scopeEntry, 0, 1+len(t.children[id]))
	entries = append(entries, t.scopeEntry(parent, &groupId, endpoint))
	for _, p := range t.children[id] {
		entries = append(entries, t.entryFor(p, &groupId, endpoint))
	}
	return entries, nil
}

// search returns the projects whose name or identifier contains the term,
// case-insensitively, as flat scope entries. Redmine can filter by name but
// not by identifier, so both are matched here.
func (t *projectTree) search(term, endpoint string) []scopeEntry {
	term = strings.ToLower(strings.TrimSpace(term))
	var matches []*redmineProject
	for _, p := range t.byId {
		if strings.Contains(strings.ToLower(p.Name), term) || strings.Contains(strings.ToLower(p.Identifier), term) {
			matches = append(matches, p)
		}
	}
	sortProjects(matches)
	entries := make([]scopeEntry, 0, len(matches))
	for _, p := range matches {
		entries = append(entries, t.scopeEntry(p, nil, endpoint))
	}
	return entries
}

// listRedmineRemoteScopes lists one level of the project tree for the config
// UI. A project with subprojects is a group (expandable) whose first entry is
// the project itself as a scope, because the UI can only select scopes. The
// whole tree is read on each call so that projects whose parent the key
// cannot see still appear, and the level is returned in one page.
func listRedmineRemoteScopes(
	connection *models.RedmineGoConnection,
	apiClient plugin.ApiClient,
	groupId string,
	_ struct{},
) ([]scopeEntry, *struct{}, errors.Error) {
	projects, err := fetchProjects(apiClient)
	if err != nil {
		return nil, nil, err
	}
	children, err := buildProjectTree(projects).list(groupId, connection.Endpoint)
	return children, nil, err
}

// endpointOf reads the endpoint the API client was built for; the search
// helper hands over the client but not the connection.
func endpointOf(apiClient plugin.ApiClient) string {
	if client, ok := apiClient.(interface{ GetEndpoint() string }); ok {
		return client.GetEndpoint()
	}
	return ""
}

// searchRedmineRemoteScopes finds projects by name or identifier. Results are
// flat and every match is selectable, whether or not it has subprojects.
func searchRedmineRemoteScopes(
	apiClient plugin.ApiClient,
	params *dsmodels.DsRemoteApiScopeSearchParams,
) ([]scopeEntry, errors.Error) {
	projects, err := fetchProjects(apiClient)
	if err != nil {
		return nil, err
	}
	entries := buildProjectTree(projects).search(params.Search, endpointOf(apiClient))
	page, size := max(params.Page, 1), max(params.PageSize, 1)
	start := min((page-1)*size, len(entries))
	end := min(start+size, len(entries))
	return entries[start:end], nil
}

// RemoteScopes lists one level of the Redmine project tree.
// @Summary list one level of the Redmine project tree
// @Description List the projects of a connection. A project with subprojects is a group whose first child is the project itself as a scope; pass the group id as groupId to expand it. Closed projects are listed, archived ones are invisible to the API.
// @Tags plugins/redmine_go
// @Param connectionId path int true "connection ID"
// @Param groupId query string false "group id of the project to expand, empty for the top level"
// @Success 200  {object} dsmodels.DsRemoteApiScopeList[models.RedmineGoProject]
// @Failure 400  {object} shared.ApiBody "Bad Request"
// @Failure 500  {object} shared.ApiBody "Internal Error"
// @Router /plugins/redmine_go/connections/{connectionId}/remote-scopes [GET]
func RemoteScopes(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	return raScopeList.Get(input)
}

// SearchRemoteScopes finds Redmine projects by name or identifier.
// @Summary search Redmine projects
// @Description Search the projects of a connection by name or identifier; matches are returned flat and are all selectable
// @Tags plugins/redmine_go
// @Param connectionId path int true "connection ID"
// @Param search query string true "text to find in the project name or identifier"
// @Param page query int false "page number, default 1"
// @Param pageSize query int false "page size, default 50"
// @Success 200  {object} dsmodels.DsRemoteApiScopeList[models.RedmineGoProject]
// @Failure 400  {object} shared.ApiBody "Bad Request"
// @Failure 500  {object} shared.ApiBody "Internal Error"
// @Router /plugins/redmine_go/connections/{connectionId}/search-remote-scopes [GET]
func SearchRemoteScopes(input *plugin.ApiResourceInput) (*plugin.ApiResourceOutput, errors.Error) {
	return raScopeSearch.Get(input)
}
