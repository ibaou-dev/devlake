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
	"encoding/json"
	"net/http"

	"github.com/apache/devlake/core/dal"
	"github.com/apache/devlake/core/errors"
	"github.com/apache/devlake/core/log"
	"github.com/apache/devlake/core/plugin"
	"github.com/apache/devlake/core/utils"
	helper "github.com/apache/devlake/helpers/pluginhelper/api"
	"github.com/apache/devlake/plugins/redmine_go/models"
)

// Raw tables of the metadata lists, without the `_raw_` prefix.
const (
	RAW_TRACKERS_TABLE         = "redmine_go_trackers"
	RAW_STATUSES_TABLE         = "redmine_go_statuses"
	RAW_PRIORITIES_TABLE       = "redmine_go_priorities"
	RAW_CUSTOM_FIELDS_TABLE    = "redmine_go_custom_fields"
	RAW_VERSIONS_TABLE         = "redmine_go_versions"
	RAW_ISSUE_CATEGORIES_TABLE = "redmine_go_issue_categories"
)

// metadataList describes one Redmine metadata list. Metadata is small and has
// no `updated_on` filter, so every run collects it in full and replaces the
// tool rows of the same origin.
type metadataList struct {
	rawTable string
	// toolTable is the model the list's tool rows are stored in.
	toolTable interface{}
	// url is the request path; project lists use the scope's project id.
	url string
	// listKey names the array inside the response object.
	listKey string
	// perProject lists belong to the scope's project; the others to the
	// whole connection and are shared by every scope of it.
	perProject bool
	// adminOnly lists answer 403 to a key that is not an administrator's.
	adminOnly bool
}

var (
	trackersList     = metadataList{rawTable: RAW_TRACKERS_TABLE, toolTable: &models.RedmineGoTracker{}, url: "trackers.json", listKey: "trackers"}
	statusesList     = metadataList{rawTable: RAW_STATUSES_TABLE, toolTable: &models.RedmineGoStatus{}, url: "issue_statuses.json", listKey: "issue_statuses"}
	prioritiesList   = metadataList{rawTable: RAW_PRIORITIES_TABLE, toolTable: &models.RedmineGoPriority{}, url: "enumerations/issue_priorities.json", listKey: "issue_priorities"}
	customFieldsList = metadataList{rawTable: RAW_CUSTOM_FIELDS_TABLE, toolTable: &models.RedmineGoCustomField{}, url: "custom_fields.json", listKey: "custom_fields", adminOnly: true}
	versionsList     = metadataList{rawTable: RAW_VERSIONS_TABLE, toolTable: &models.RedmineGoVersion{}, url: "projects/{{ .Params.ProjectId }}/versions.json", listKey: "versions", perProject: true}
	categoriesList   = metadataList{rawTable: RAW_ISSUE_CATEGORIES_TABLE, toolTable: &models.RedmineGoIssueCategory{}, url: "projects/{{ .Params.ProjectId }}/issue_categories.json", listKey: "issue_categories", perProject: true}
)

// options returns the raw-data identity of the list for a task: the project for
// project lists, the connection for the rest.
func (l metadataList) options(data *RedmineGoTaskData) helper.TaskOptions {
	if l.perProject {
		return models.RedmineGoApiParams{ConnectionId: data.Options.ConnectionId, ProjectId: data.Options.ProjectId}
	}
	return models.RedmineGoConnectionParams{ConnectionId: data.Options.ConnectionId}
}

func (l metadataList) rawArgs(taskCtx plugin.SubTaskContext) helper.RawDataSubTaskArgs {
	data := taskCtx.GetData().(*RedmineGoTaskData)
	return helper.RawDataSubTaskArgs{Ctx: taskCtx, Options: l.options(data), Table: l.rawTable}
}

// collect fetches the whole list and replaces the raw rows of the same
// identity. An admin-only list that a non-admin key may not read is skipped
// with one warning, and the subtask still succeeds. Redmine answers 403 or
// 404 for a project whose issue-tracking module is off, which skips the
// project lists the same way.
func (l metadataList) collect(taskCtx plugin.SubTaskContext) errors.Error {
	data := taskCtx.GetData().(*RedmineGoTaskData)
	collector, err := helper.NewApiCollector(helper.ApiCollectorArgs{
		RawDataSubTaskArgs: l.rawArgs(taskCtx),
		ApiClient:          data.ApiClient,
		UrlTemplate:        l.url,
		ResponseParser:     listResponseParser(l.listKey),
		AfterResponse:      l.afterResponse(taskCtx.GetLogger()),
	})
	if err != nil {
		return err
	}
	return collector.Execute()
}

func (l metadataList) afterResponse(logger log.Logger) plugin.ApiClientAfterResponse {
	if !l.adminOnly {
		return IgnoreMissingItem(logger)
	}
	return func(res *http.Response) errors.Error {
		switch res.StatusCode {
		case http.StatusUnauthorized:
			return errors.Unauthorized.New("authentication failed, please check the Redmine API key")
		case http.StatusForbidden:
			logger.Warn(nil, "skipping %s: Redmine lists it for administrators only, so custom field names and values "+
				"come from the collected issues instead", l.url)
			return helper.ErrIgnoreAndContinue
		}
		return nil
	}
}

// extract turns the raw rows into tool rows with fn and replaces the tool rows
// of the same origin, so an item deleted in Redmine disappears from the tool
// layer. The old rows are deleted up front because the batch saver only
// replaces the rows of a table it receives a row for: a list that became empty
// would otherwise keep its last items.
func (l metadataList) extract(taskCtx plugin.SubTaskContext, fn func(row *helper.RawData) ([]interface{}, errors.Error)) errors.Error {
	data := taskCtx.GetData().(*RedmineGoTaskData)
	if err := taskCtx.GetDal().Delete(l.toolTable, dal.Where("_raw_data_table = ? AND _raw_data_params = ?",
		rawTableName(l.rawTable), utils.ToJsonString(l.options(data).GetParams()))); err != nil {
		return err
	}
	extractor, err := helper.NewApiExtractor(helper.ApiExtractorArgs{
		RawDataSubTaskArgs: l.rawArgs(taskCtx),
		Extract:            fn,
	})
	if err != nil {
		return err
	}
	return extractor.Execute()
}

// rawTableName returns the name of the raw table of a list, as listed in subtask
// metadata.
func rawTableName(table string) string {
	return "_raw_" + table
}

type apiTracker struct {
	Id            uint64  `json:"id"`
	Name          string  `json:"name"`
	DefaultStatus *apiRef `json:"default_status"`
}

type apiStatus struct {
	Id       uint64 `json:"id"`
	Name     string `json:"name"`
	IsClosed bool   `json:"is_closed"`
}

type apiPriority struct {
	Id        uint64 `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
	Active    bool   `json:"active"`
}

type apiCustomField struct {
	Id             uint64          `json:"id"`
	Name           string          `json:"name"`
	CustomizedType string          `json:"customized_type"`
	FieldFormat    string          `json:"field_format"`
	Multiple       bool            `json:"multiple"`
	PossibleValues json.RawMessage `json:"possible_values"`
	Trackers       json.RawMessage `json:"trackers"`
}

type apiVersion struct {
	Id          uint64  `json:"id"`
	Project     *apiRef `json:"project"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Status      string  `json:"status"`
	DueDate     string  `json:"due_date"`
	Sharing     string  `json:"sharing"`
	CreatedOn   string  `json:"created_on"`
	UpdatedOn   string  `json:"updated_on"`
}

type apiCategory struct {
	Id         uint64  `json:"id"`
	Project    *apiRef `json:"project"`
	Name       string  `json:"name"`
	AssignedTo *apiRef `json:"assigned_to"`
}

// jsonText returns a raw JSON value as text, and "" for an absent value or null.
func jsonText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	return string(raw)
}

var (
	CollectTrackersMeta = plugin.SubTaskMeta{
		Name:             "collectTrackers",
		EntryPoint:       CollectTrackers,
		EnabledByDefault: true,
		Description:      "Collect the Redmine trackers, with their default status",
		DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
		ProductTables:    []string{rawTableName(RAW_TRACKERS_TABLE)},
	}
	ExtractTrackersMeta = plugin.SubTaskMeta{
		Name:             "extractTrackers",
		EntryPoint:       ExtractTrackers,
		EnabledByDefault: true,
		Description:      "Extract trackers into _tool_redmine_go_trackers",
		DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
		DependencyTables: []string{rawTableName(RAW_TRACKERS_TABLE)},
		ProductTables:    []string{models.RedmineGoTracker{}.TableName()},
	}
	CollectStatusesMeta = plugin.SubTaskMeta{
		Name:             "collectStatuses",
		EntryPoint:       CollectStatuses,
		EnabledByDefault: true,
		Description:      "Collect the Redmine issue statuses, with is_closed",
		DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
		ProductTables:    []string{rawTableName(RAW_STATUSES_TABLE)},
	}
	ExtractStatusesMeta = plugin.SubTaskMeta{
		Name:             "extractStatuses",
		EntryPoint:       ExtractStatuses,
		EnabledByDefault: true,
		Description:      "Extract issue statuses into _tool_redmine_go_statuses",
		DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
		DependencyTables: []string{rawTableName(RAW_STATUSES_TABLE)},
		ProductTables:    []string{models.RedmineGoStatus{}.TableName()},
	}
	CollectPrioritiesMeta = plugin.SubTaskMeta{
		Name:             "collectPriorities",
		EntryPoint:       CollectPriorities,
		EnabledByDefault: true,
		Description:      "Collect the Redmine issue priorities",
		DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
		ProductTables:    []string{rawTableName(RAW_PRIORITIES_TABLE)},
	}
	ExtractPrioritiesMeta = plugin.SubTaskMeta{
		Name:             "extractPriorities",
		EntryPoint:       ExtractPriorities,
		EnabledByDefault: true,
		Description:      "Extract issue priorities into _tool_redmine_go_priorities",
		DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
		DependencyTables: []string{rawTableName(RAW_PRIORITIES_TABLE)},
		ProductTables:    []string{models.RedmineGoPriority{}.TableName()},
	}
	CollectCustomFieldsMeta = plugin.SubTaskMeta{
		Name:             "collectCustomFields",
		EntryPoint:       CollectCustomFields,
		EnabledByDefault: true,
		Description:      "Collect the Redmine custom fields; Redmine lists them for administrators only, so any other key skips the step with one warning",
		DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
		ProductTables:    []string{rawTableName(RAW_CUSTOM_FIELDS_TABLE)},
	}
	ExtractCustomFieldsMeta = plugin.SubTaskMeta{
		Name:             "extractCustomFields",
		EntryPoint:       ExtractCustomFields,
		EnabledByDefault: true,
		Description:      "Extract issue custom fields, with format, possible values and trackers, into _tool_redmine_go_custom_fields",
		DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
		DependencyTables: []string{rawTableName(RAW_CUSTOM_FIELDS_TABLE)},
		ProductTables:    []string{models.RedmineGoCustomField{}.TableName()},
	}
	CollectVersionsMeta = plugin.SubTaskMeta{
		Name:             "collectVersions",
		EntryPoint:       CollectVersions,
		EnabledByDefault: true,
		Description:      "Collect the versions of the project",
		DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
		ProductTables:    []string{rawTableName(RAW_VERSIONS_TABLE)},
	}
	ExtractVersionsMeta = plugin.SubTaskMeta{
		Name:             "extractVersions",
		EntryPoint:       ExtractVersions,
		EnabledByDefault: true,
		Description:      "Extract versions into _tool_redmine_go_versions",
		DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
		DependencyTables: []string{rawTableName(RAW_VERSIONS_TABLE)},
		ProductTables:    []string{models.RedmineGoVersion{}.TableName()},
	}
	CollectIssueCategoriesMeta = plugin.SubTaskMeta{
		Name:             "collectIssueCategories",
		EntryPoint:       CollectIssueCategories,
		EnabledByDefault: true,
		Description:      "Collect the issue categories of the project",
		DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
		ProductTables:    []string{rawTableName(RAW_ISSUE_CATEGORIES_TABLE)},
	}
	ExtractIssueCategoriesMeta = plugin.SubTaskMeta{
		Name:             "extractIssueCategories",
		EntryPoint:       ExtractIssueCategories,
		EnabledByDefault: true,
		Description:      "Extract issue categories into _tool_redmine_go_issue_categories",
		DomainTypes:      []string{plugin.DOMAIN_TYPE_TICKET},
		DependencyTables: []string{rawTableName(RAW_ISSUE_CATEGORIES_TABLE)},
		ProductTables:    []string{models.RedmineGoIssueCategory{}.TableName()},
	}
)

var (
	_ plugin.SubTaskEntryPoint = CollectTrackers
	_ plugin.SubTaskEntryPoint = ExtractTrackers
	_ plugin.SubTaskEntryPoint = CollectStatuses
	_ plugin.SubTaskEntryPoint = ExtractStatuses
	_ plugin.SubTaskEntryPoint = CollectPriorities
	_ plugin.SubTaskEntryPoint = ExtractPriorities
	_ plugin.SubTaskEntryPoint = CollectCustomFields
	_ plugin.SubTaskEntryPoint = ExtractCustomFields
	_ plugin.SubTaskEntryPoint = CollectVersions
	_ plugin.SubTaskEntryPoint = ExtractVersions
	_ plugin.SubTaskEntryPoint = CollectIssueCategories
	_ plugin.SubTaskEntryPoint = ExtractIssueCategories
)

// CollectTrackers collects the connection's trackers.
func CollectTrackers(taskCtx plugin.SubTaskContext) errors.Error {
	return trackersList.collect(taskCtx)
}

// ExtractTrackers extracts trackers, keeping each one's default status.
func ExtractTrackers(taskCtx plugin.SubTaskContext) errors.Error {
	connectionId := taskCtx.GetData().(*RedmineGoTaskData).Options.ConnectionId
	return trackersList.extract(taskCtx, func(row *helper.RawData) ([]interface{}, errors.Error) {
		var item apiTracker
		if err := errors.Convert(json.Unmarshal(row.Data, &item)); err != nil {
			return nil, err
		}
		tracker := &models.RedmineGoTracker{ConnectionId: connectionId, Id: item.Id, Name: item.Name}
		if item.DefaultStatus != nil {
			tracker.DefaultStatusId = item.DefaultStatus.Id
			tracker.DefaultStatusName = item.DefaultStatus.Name
		}
		return []interface{}{tracker}, nil
	})
}

// CollectStatuses collects the connection's issue statuses.
func CollectStatuses(taskCtx plugin.SubTaskContext) errors.Error {
	return statusesList.collect(taskCtx)
}

// ExtractStatuses extracts issue statuses, keeping is_closed.
func ExtractStatuses(taskCtx plugin.SubTaskContext) errors.Error {
	connectionId := taskCtx.GetData().(*RedmineGoTaskData).Options.ConnectionId
	return statusesList.extract(taskCtx, func(row *helper.RawData) ([]interface{}, errors.Error) {
		var item apiStatus
		if err := errors.Convert(json.Unmarshal(row.Data, &item)); err != nil {
			return nil, err
		}
		return []interface{}{&models.RedmineGoStatus{
			ConnectionId: connectionId, Id: item.Id, Name: item.Name, IsClosed: item.IsClosed,
		}}, nil
	})
}

// CollectPriorities collects the connection's issue priorities.
func CollectPriorities(taskCtx plugin.SubTaskContext) errors.Error {
	return prioritiesList.collect(taskCtx)
}

// ExtractPriorities extracts issue priorities.
func ExtractPriorities(taskCtx plugin.SubTaskContext) errors.Error {
	connectionId := taskCtx.GetData().(*RedmineGoTaskData).Options.ConnectionId
	return prioritiesList.extract(taskCtx, func(row *helper.RawData) ([]interface{}, errors.Error) {
		var item apiPriority
		if err := errors.Convert(json.Unmarshal(row.Data, &item)); err != nil {
			return nil, err
		}
		return []interface{}{&models.RedmineGoPriority{
			ConnectionId: connectionId, Id: item.Id, Name: item.Name, IsDefault: item.IsDefault, Active: item.Active,
		}}, nil
	})
}

// CollectCustomFields collects the custom fields; it is admin-only.
func CollectCustomFields(taskCtx plugin.SubTaskContext) errors.Error {
	return customFieldsList.collect(taskCtx)
}

// ExtractCustomFields extracts the custom fields of issues; fields of other
// customized types (projects, users, time entries, versions) do not take part
// in issue mapping and are dropped.
func ExtractCustomFields(taskCtx plugin.SubTaskContext) errors.Error {
	connectionId := taskCtx.GetData().(*RedmineGoTaskData).Options.ConnectionId
	return customFieldsList.extract(taskCtx, func(row *helper.RawData) ([]interface{}, errors.Error) {
		var item apiCustomField
		if err := errors.Convert(json.Unmarshal(row.Data, &item)); err != nil {
			return nil, err
		}
		if item.CustomizedType != "issue" {
			return nil, nil
		}
		return []interface{}{&models.RedmineGoCustomField{
			ConnectionId:   connectionId,
			Id:             item.Id,
			Name:           item.Name,
			CustomizedType: item.CustomizedType,
			FieldFormat:    item.FieldFormat,
			Multiple:       item.Multiple,
			PossibleValues: jsonText(item.PossibleValues),
			Trackers:       jsonText(item.Trackers),
		}}, nil
	})
}

// CollectVersions collects the versions of the scope's project, including
// those other projects share with it.
func CollectVersions(taskCtx plugin.SubTaskContext) errors.Error {
	return versionsList.collect(taskCtx)
}

// ExtractVersions extracts versions with their status, due date and sharing.
func ExtractVersions(taskCtx plugin.SubTaskContext) errors.Error {
	data := taskCtx.GetData().(*RedmineGoTaskData)
	return versionsList.extract(taskCtx, func(row *helper.RawData) ([]interface{}, errors.Error) {
		var item apiVersion
		if err := errors.Convert(json.Unmarshal(row.Data, &item)); err != nil {
			return nil, err
		}
		version := &models.RedmineGoVersion{
			ConnectionId:     data.Options.ConnectionId,
			Id:               item.Id,
			ProjectId:        data.Options.ProjectId,
			Name:             item.Name,
			Description:      item.Description,
			Status:           item.Status,
			Sharing:          item.Sharing,
			DueDate:          parseDate(item.DueDate),
			RedmineCreatedOn: parseTime(item.CreatedOn),
			RedmineUpdatedOn: parseTime(item.UpdatedOn),
		}
		if item.Project != nil && item.Project.Id != 0 {
			version.ProjectId = item.Project.Id
		}
		return []interface{}{version}, nil
	})
}

// CollectIssueCategories collects the issue categories of the scope's project.
func CollectIssueCategories(taskCtx plugin.SubTaskContext) errors.Error {
	return categoriesList.collect(taskCtx)
}

// ExtractIssueCategories extracts issue categories.
func ExtractIssueCategories(taskCtx plugin.SubTaskContext) errors.Error {
	data := taskCtx.GetData().(*RedmineGoTaskData)
	return categoriesList.extract(taskCtx, func(row *helper.RawData) ([]interface{}, errors.Error) {
		var item apiCategory
		if err := errors.Convert(json.Unmarshal(row.Data, &item)); err != nil {
			return nil, err
		}
		category := &models.RedmineGoIssueCategory{
			ConnectionId: data.Options.ConnectionId,
			Id:           item.Id,
			ProjectId:    data.Options.ProjectId,
			Name:         item.Name,
		}
		if item.Project != nil && item.Project.Id != 0 {
			category.ProjectId = item.Project.Id
		}
		if item.AssignedTo != nil {
			category.AssignedToId = item.AssignedTo.Id
			category.AssignedToName = item.AssignedTo.Name
		}
		return []interface{}{category}, nil
	})
}
