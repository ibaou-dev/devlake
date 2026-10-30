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
	"reflect"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/models/domainlayer"
	"github.com/apache/devlake/core/models/domainlayer/didgen"
	"github.com/apache/devlake/core/models/domainlayer/ticket"
	"github.com/apache/devlake/core/plugin"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// RAW_PROJECTS_TABLE labels the raw-data lineage of the scope-derived board.
// Projects are added as scopes and have no collector, so it is a logical tag.
const RAW_PROJECTS_TABLE = "redmine_go_projects"

// ConvertProjectsMeta describes the board convertor.
var ConvertProjectsMeta = plugin.SubTaskMeta{
	Name:             "convertProjects",
	EntryPoint:       ConvertProjects,
	EnabledByDefault: true,
	Description:      "Convert the Redmine project scope (_tool_redmine_go_projects) into the domain layer table boards",
	DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
	DependencyTables: []string{models.RedmineGoProject{}.TableName()},
	ProductTables:    []string{ticket.Board{}.TableName()},
}

var _ plugin.SubTaskEntryPoint = ConvertProjects

// ConvertProjects converts the scope's project into its board.
func ConvertProjects(taskCtx plugin.SubTaskContext) errors.Error {
	db := taskCtx.GetDal()
	data := taskCtx.GetData().(*RedmineGoTaskData)
	connectionId := data.Options.ConnectionId
	boardIdGen := didgen.NewDomainIdGenerator(&models.RedmineGoProject{})

	cursor, err := db.Cursor(
		dal.From(&models.RedmineGoProject{}),
		dal.Where("connection_id = ? AND id = ?", connectionId, data.Options.ProjectId),
	)
	if err != nil {
		return err
	}
	defer cursor.Close()

	converter, err := helper.NewDataConverter(helper.DataConverterArgs{
		RawDataSubTaskArgs: helper.RawDataSubTaskArgs{
			Ctx: taskCtx,
			Options: models.RedmineGoApiParams{
				ConnectionId: connectionId,
				ProjectId:    data.Options.ProjectId,
			},
			Table: RAW_PROJECTS_TABLE,
		},
		InputRowType: reflect.TypeOf(models.RedmineGoProject{}),
		Input:        cursor,
		Convert: func(inputRow interface{}) ([]interface{}, errors.Error) {
			project := inputRow.(*models.RedmineGoProject)
			board := &ticket.Board{
				DomainEntity: domainlayer.DomainEntity{Id: boardIdGen.Generate(connectionId, project.Id)},
				Name:         project.Name,
				Description:  project.Description,
				Url:          projectUrl(data.Connection.Endpoint, project),
			}
			return []interface{}{board}, nil
		},
	})
	if err != nil {
		return err
	}
	return converter.Execute()
}

// projectUrl is the project's web page. The stored URL, when the scope has
// one, is authoritative; otherwise it is built from the identifier.
func projectUrl(endpoint string, project *models.RedmineGoProject) string {
	if project.Url != "" {
		return project.Url
	}
	return endpoint + "/projects/" + project.Identifier
}

// issueUrl is the issue's web page.
func issueUrl(endpoint string, issueId uint64) string {
	return endpoint + "/issues/" + idString(issueId)
}
