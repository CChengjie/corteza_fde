package city311

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	systemTypes "github.com/cortezaproject/corteza/server/system/types"
)

const (
	configurationSavedReport  = "SAVED_REPORT"
	reportRunOperationKind    = "REPORT_RUN"
	reportExportOperationKind = "REPORT_EXPORT"
	reportListSort            = "-updated_at"
)

var reportCatalogue = []contract.ReportCatalogueItem{
	{
		ReportKey: "request_volume", Name: "Request volume",
		SupportedFilters:  []string{"created_from", "created_to", "service_type", "department", "district", "source_channel", "origin_class"},
		SupportedGrouping: []string{"created_date", "service_type", "department", "district", "source_channel", "origin_class"},
		SupportedSort:     []string{"created_date", "service_type", "department", "district", "source_channel", "origin_class", "count"},
	},
	{
		ReportKey: "request_status_age", Name: "Request status and age",
		SupportedFilters:  []string{"created_from", "created_to", "service_type", "department", "status"},
		SupportedGrouping: []string{"service_type", "department", "status"},
		SupportedSort:     []string{"request_number", "service_type", "department", "status", "created_at", "updated_at", "age"},
	},
	{
		ReportKey: "assignment_workload", Name: "Assignment workload",
		SupportedFilters:  []string{"department", "district", "status", "primary_assignee"},
		SupportedGrouping: []string{"primary_assignee"},
		SupportedSort:     []string{"primary_assignee", "active_request_count", "overdue_reminder_count"},
	},
	{
		ReportKey: "resolution_performance", Name: "Resolution performance",
		SupportedFilters:  []string{"created_from", "created_to", "resolved_from", "resolved_to", "service_type", "department"},
		SupportedGrouping: []string{"service_type", "department", "reopened"},
		SupportedSort:     []string{"request_number", "service_type", "submitted_at", "resolved_at", "elapsed_duration", "reopened"},
	},
	{
		ReportKey: "follow_up_activity", Name: "Follow-up activity",
		SupportedFilters:  []string{"occurred_from", "occurred_to", "action_type", "actor", "visibility", "department"},
		SupportedGrouping: []string{"action_type", "actor", "visibility", "department"},
		SupportedSort:     []string{"request_number", "action_type", "actor", "occurred_at", "visibility", "department"},
	},
}

var reportEntityFields = map[string]map[string]bool{
	"service_requests": {
		"request_id": true, "request_number": true, "summary": true, "description": true,
		"service_type": true, "department": true, "district": true, "status": true,
		"source_channel": true, "origin_class": true, "primary_assignee": true,
		"collaborator_count": true, "created_date": true, "created_at": true, "updated_at": true, "age": true,
		"submitted_at": true, "resolved_at": true, "elapsed_duration": true, "reopened": true,
		"active_request_count": true, "overdue_reminder_count": true, "count": true,
	},
	"constituents": {
		"constituent_id": true, "display_name": true, "emails": true, "phone_numbers": true,
		"addresses": true, "primary_category": true, "preferred_language": true,
		"email_opt_out": true, "department": true, "district": true, "created_at": true, "updated_at": true,
	},
	"follow_up_actions": {
		"request_id": true, "request_number": true, "action_type": true, "actor": true,
		"occurred_at": true, "local_display_time": true, "visibility": true, "department": true,
	},
}

var reportDateFilterFields = map[string]map[string]string{
	"service_requests": {
		"created_from": "created_at", "created_to": "created_at", "updated_from": "updated_at", "updated_to": "updated_at",
		"resolved_from": "resolved_at", "resolved_to": "resolved_at",
	},
	"constituents": {
		"created_from": "created_at", "created_to": "created_at", "updated_from": "updated_at", "updated_to": "updated_at",
	},
	"follow_up_actions": {"occurred_from": "occurred_at", "occurred_to": "occurred_at"},
}

type savedReportPayload struct {
	ReportID          string                     `json:"report_id"`
	Name              string                     `json:"name"`
	Entity            string                     `json:"entity"`
	Columns           []string                   `json:"columns"`
	Filters           map[string]any             `json:"filters"`
	Grouping          *string                    `json:"grouping,omitempty"`
	Sort              []string                   `json:"sort"`
	OwnerID           string                     `json:"owner_id"`
	SharedRoles       []contract.ApplicationRole `json:"shared_roles"`
	OwnerDepartment   contract.DepartmentCode    `json:"owner_department,omitempty"`
	OwnerDistricts    []contract.DistrictCode    `json:"owner_districts,omitempty"`
	OwnerAllDistricts bool                       `json:"owner_all_districts"`
	OwnerPlatform     bool                       `json:"owner_platform_administrator"`
}

func (svc *Service) ReportCatalogue(ctx context.Context, actor contract.Actor, query ConfigurationListQuery) (*contract.ReportCatalogueList, error) {
	_ = ctx
	if !isStaff(actor) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, "A staff role is required.")
	}
	start, end, next, err := configurationPage(query, len(reportCatalogue), "report_key")
	if err != nil {
		return nil, err
	}
	items := append([]contract.ReportCatalogueItem(nil), reportCatalogue[start:end]...)
	return &contract.ReportCatalogueList{
		Items: items, NextPageToken: next, TotalCount: len(reportCatalogue),
		AppliedFilters: map[string]any{}, Sort: []string{"report_key"},
	}, nil
}

func (svc *Service) ListSavedReports(ctx context.Context, actor contract.Actor, query ConfigurationListQuery) (*contract.ReportDefinitionList, error) {
	if !isStaff(actor) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, "A staff role is required.")
	}
	revisions, err := svc.latestConfigurationResources(ctx, svc.store, configurationSavedReport)
	if err != nil {
		return nil, err
	}
	visible := make(composeTypes.City311ConfigurationRevisionSet, 0, len(revisions))
	for _, revision := range revisions {
		payload := savedReportFromRevision(revision)
		if reportVisibleTo(actor, payload) {
			visible = append(visible, revision)
		}
	}
	sort.Slice(visible, func(i, j int) bool {
		if visible[i].CreatedAt.Equal(visible[j].CreatedAt) {
			return visible[i].ResourceKey < visible[j].ResourceKey
		}
		return visible[i].CreatedAt.After(visible[j].CreatedAt)
	})
	start, end, next, err := configurationPage(query, len(visible), reportListSort)
	if err != nil {
		return nil, err
	}
	response := &contract.ReportDefinitionList{
		Items: make([]contract.ReportDefinition, 0, end-start), NextPageToken: next, TotalCount: len(visible),
		AppliedFilters: map[string]any{}, Sort: []string{reportListSort},
	}
	for _, revision := range visible[start:end] {
		response.Items = append(response.Items, reportDefinitionFromRevision(revision))
	}
	return response, nil
}

func (svc *Service) CreateSavedReport(ctx context.Context, actor contract.Actor, input contract.ReportDefinition) (*contract.ReportDefinition, error) {
	if !isStaff(actor) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, "A staff role is required.")
	}
	normalized, err := svc.validateReportDefinition(ctx, input)
	if err != nil {
		return nil, err
	}
	svc.reportMu.Lock()
	defer svc.reportMu.Unlock()
	revisions, err := svc.configurationRevisions(ctx, svc.store, configurationSavedReport, normalized.ReportID)
	if err != nil {
		return nil, err
	}
	if len(revisions) > 0 {
		return nil, validationError(contract.FieldError{Field: "/report_id", Code: contract.ValidationDuplicate})
	}
	payload := newSavedReportPayload(normalized, actor)
	revision, err := svc.createInitialConfigurationResource(ctx, actor, configurationSavedReport, normalized.ReportID, payload, "REPORT_CREATED")
	if err != nil {
		return nil, err
	}
	definition := reportDefinitionFromRevision(revision)
	return &definition, nil
}

func (svc *Service) UpdateSavedReport(ctx context.Context, actor contract.Actor, reportID string, expectedVersion uint64, input contract.ReportDefinition) (*contract.ReportDefinition, error) {
	if !isStaff(actor) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, "A staff role is required.")
	}
	if expectedVersion == 0 {
		return nil, expectedVersionRequired()
	}
	reportID = strings.TrimSpace(reportID)
	if input.ReportID != reportID {
		return nil, validationError(contract.FieldError{Field: "/report_id", Code: contract.ValidationConflict})
	}
	normalized, err := svc.validateReportDefinition(ctx, input)
	if err != nil {
		return nil, err
	}
	svc.reportMu.Lock()
	defer svc.reportMu.Unlock()
	current, payload, err := svc.lookupSavedReport(ctx, svc.store, actor, reportID, true)
	if err != nil {
		return nil, err
	}
	if uint64(current.Version) != expectedVersion {
		return nil, versionConflict(current.Version)
	}
	payload.ReportID, payload.Name, payload.Entity = normalized.ReportID, normalized.Name, normalized.Entity
	payload.Columns, payload.Filters, payload.Grouping, payload.Sort = normalized.Columns, normalized.Filters, normalized.Grouping, normalized.Sort
	revision, err := svc.createConfigurationRevision(ctx, actor, current, configurationSavedReport, reportID, "", payload, true, "REPORT_UPDATED")
	if err != nil {
		return nil, err
	}
	definition := reportDefinitionFromRevision(revision)
	return &definition, nil
}

func (svc *Service) ShareSavedReport(ctx context.Context, actor contract.Actor, reportID string, expectedVersion uint64, input contract.ReportShare) (*contract.ReportDefinition, error) {
	if !isStaff(actor) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, "A staff role is required.")
	}
	if expectedVersion == 0 {
		return nil, expectedVersionRequired()
	}
	roles, err := validateReportShare(actor, input.Roles)
	if err != nil {
		return nil, err
	}
	svc.reportMu.Lock()
	defer svc.reportMu.Unlock()
	current, payload, err := svc.lookupSavedReport(ctx, svc.store, actor, reportID, true)
	if err != nil {
		return nil, err
	}
	if uint64(current.Version) != expectedVersion {
		return nil, versionConflict(current.Version)
	}
	payload.SharedRoles = roles
	payload.OwnerDepartment = actor.Department
	payload.OwnerDistricts = append([]contract.DistrictCode(nil), actor.Districts...)
	payload.OwnerAllDistricts = hasRole(actor, contract.ApplicationRoleDepartmentManager)
	payload.OwnerPlatform = hasRole(actor, contract.ApplicationRolePlatformAdministrator)
	revision, err := svc.createConfigurationRevision(ctx, actor, current, configurationSavedReport, current.ResourceKey, "", payload, true, "REPORT_SHARED")
	if err != nil {
		return nil, err
	}
	definition := reportDefinitionFromRevision(revision)
	return &definition, nil
}

func (svc *Service) StartReportRun(ctx context.Context, actor contract.Actor, input contract.ReportRun) (*contract.Operation, error) {
	if !isStaff(actor) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, "A staff role is required.")
	}
	definition, err := svc.validateReportDefinition(ctx, input.Definition)
	if err != nil {
		return nil, err
	}
	rows, err := svc.buildReportRows(ctx, actor, definition)
	if err != nil {
		return nil, err
	}
	return svc.persistReportRun(ctx, actor, definition, rows)
}

func (svc *Service) StartReportExport(ctx context.Context, actor contract.Actor, reportID string, input contract.ReportExport) (*contract.Operation, error) {
	if !isStaff(actor) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, "A staff role is required.")
	}
	if input.Format != "CSV" {
		return nil, validationError(contract.FieldError{Field: "/format", Code: contract.ValidationInvalidValue})
	}
	_, payload, err := svc.lookupSavedReport(ctx, svc.store, actor, reportID, false)
	if err != nil {
		return nil, err
	}
	definition := reportDefinitionFromPayload(payload)
	definition, err = svc.validateReportDefinition(ctx, definition)
	if err != nil {
		return nil, err
	}
	rows, err := svc.buildReportRows(ctx, actor, definition)
	if err != nil {
		return nil, err
	}
	content, err := encodeReportCSV(definition.Columns, rows)
	if err != nil {
		return nil, err
	}
	return svc.persistReportExport(ctx, actor, definition, rows, content)
}

func (svc *Service) persistReportRun(ctx context.Context, actor contract.Actor, definition contract.ReportDefinition, rows []map[string]any) (*contract.Operation, error) {
	svc.reportMu.Lock()
	defer svc.reportMu.Unlock()
	pending := &contract.Operation{}
	err := store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		now := svc.now().UTC()
		operation := &composeTypes.City311Operation{
			ID: svc.nextID(), Kind: reportRunOperationKind, Status: string(contract.OperationStatusPending), ActorID: actor.ID,
			Result: composeTypes.City311JSON{}, Error: composeTypes.City311JSON{}, CreatedAt: now, UpdatedAt: now,
		}
		if err := store.CreateCity311Operation(ctx, tx, operation); err != nil {
			return err
		}
		*pending = *toOperation(operation)
		completedAt := svc.now().UTC()
		operation.Status, operation.Progress = string(contract.OperationStatusSucceeded), 100
		operation.Result = composeTypes.City311JSON{"columns": definition.Columns, "rows": rows, "row_count": len(rows)}
		operation.UpdatedAt, operation.CompletedAt = completedAt, &completedAt
		return store.UpdateCity311Operation(ctx, tx, operation)
	})
	return pending, err
}

func (svc *Service) persistReportExport(ctx context.Context, actor contract.Actor, definition contract.ReportDefinition, rows []map[string]any, content []byte) (*contract.Operation, error) {
	svc.reportMu.Lock()
	defer svc.reportMu.Unlock()
	pending := &contract.Operation{}
	err := store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		now := svc.now().UTC()
		operation := &composeTypes.City311Operation{
			ID: svc.nextID(), Kind: reportExportOperationKind, Status: string(contract.OperationStatusPending), ActorID: actor.ID,
			Result: composeTypes.City311JSON{}, Error: composeTypes.City311JSON{}, CreatedAt: now, UpdatedAt: now,
		}
		if err := store.CreateCity311Operation(ctx, tx, operation); err != nil {
			return err
		}
		*pending = *toOperation(operation)
		completedAt := svc.now().UTC()
		operation.Status, operation.Progress = string(contract.OperationStatusSucceeded), 100
		operation.Content, operation.ContentType = content, "text/csv; charset=utf-8"
		operation.Filename = "report-" + definition.ReportID + "-" + completedAt.Format("20060102T150405Z") + ".csv"
		operation.Result = composeTypes.City311JSON{"download_url": "/api/v1/operations/" + publicOperationID(operation.ID) + "/result", "row_count": len(rows)}
		operation.UpdatedAt, operation.CompletedAt = completedAt, &completedAt
		if err := store.UpdateCity311Operation(ctx, tx, operation); err != nil {
			return err
		}
		return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
			ID: svc.nextID(), EntityType: "report", EntityID: definition.ReportID, EventType: "REPORT_EXPORTED",
			ActorType: contract.AuditActorStaff, ActorID: actor.ID, SourceChannel: contract.SourceChannelStaffInPerson,
			Before: composeTypes.City311JSON{}, After: composeTypes.City311JSON{
				"operation_id": publicOperationID(operation.ID), "row_count": len(rows), "columns": definition.Columns,
			}, CreatedAt: completedAt,
		})
	})
	return pending, err
}

func (svc *Service) validateReportDefinition(ctx context.Context, input contract.ReportDefinition) (contract.ReportDefinition, error) {
	input.ReportID = strings.TrimSpace(input.ReportID)
	input.Name = strings.TrimSpace(input.Name)
	input.Entity = strings.TrimSpace(input.Entity)
	if input.Filters == nil {
		input.Filters = map[string]any{}
	}
	var fields []contract.FieldError
	if !validResourceKey(input.ReportID) {
		fields = append(fields, contract.FieldError{Field: "/report_id", Code: contract.ValidationInvalidFormat})
	}
	if length := utf8.RuneCountInString(input.Name); length == 0 || length > 120 {
		fields = append(fields, contract.FieldError{Field: "/name", Code: contract.ValidationInvalidValue})
	}
	allowed, supported := reportEntityFields[input.Entity]
	if !supported {
		fields = append(fields, contract.FieldError{Field: "/entity", Code: contract.ValidationInvalidValue})
		allowed = map[string]bool{}
	}
	allowed = cloneBoolMap(allowed)
	definitionEntity := ""
	customDefinitions := map[string]contract.CustomFieldDefinition{}
	if input.Entity == "service_requests" {
		definitionEntity = "service_request"
	} else if input.Entity == "constituents" {
		definitionEntity = "constituent"
	}
	if definitionEntity != "" {
		definitions, err := svc.customFieldDefinitions(ctx, svc.store, definitionEntity)
		if err != nil {
			return contract.ReportDefinition{}, err
		}
		for _, definition := range definitions {
			if definition.Active {
				field := "custom_fields." + definition.Key
				allowed[field] = true
				customDefinitions[field] = definition
			}
		}
	}
	vocabularies := reportVocabularies()
	if input.Entity == "constituents" {
		categories, err := svc.activeContactCategories(ctx, svc.store)
		if err != nil {
			return contract.ReportDefinition{}, err
		}
		vocabularies["primary_category"] = reportEnumValues(categories)
	}
	if len(input.Columns) == 0 {
		fields = append(fields, contract.FieldError{Field: "/columns", Code: contract.ValidationRequired})
	} else if len(input.Columns) > 20 {
		fields = append(fields, contract.FieldError{Field: "/columns", Code: contract.ValidationTooManyItems})
	}
	seen := map[string]bool{}
	for index, column := range input.Columns {
		column = strings.TrimSpace(column)
		input.Columns[index] = column
		if !allowed[column] {
			fields = append(fields, contract.FieldError{Field: fmt.Sprintf("/columns/%d", index), Code: contract.ValidationInvalidValue})
		} else if seen[column] {
			fields = append(fields, contract.FieldError{Field: fmt.Sprintf("/columns/%d", index), Code: contract.ValidationDuplicate})
		}
		seen[column] = true
	}
	if input.Grouping != nil {
		grouping := strings.TrimSpace(*input.Grouping)
		input.Grouping = &grouping
		if grouping == "" || !allowed[grouping] || grouping == "count" {
			fields = append(fields, contract.FieldError{Field: "/grouping", Code: contract.ValidationInvalidValue})
		}
	}
	if len(input.Sort) > 3 {
		fields = append(fields, contract.FieldError{Field: "/sort", Code: contract.ValidationTooManyItems})
	}
	for index, expression := range input.Sort {
		expression = strings.TrimSpace(expression)
		descending := strings.HasPrefix(expression, "-")
		field := strings.TrimPrefix(strings.TrimPrefix(expression, "-"), "+")
		if field == "" || !allowed[field] {
			fields = append(fields, contract.FieldError{Field: fmt.Sprintf("/sort/%d", index), Code: contract.ValidationInvalidValue})
			continue
		}
		input.Sort[index] = field
		if descending {
			input.Sort[index] = "-" + field
		}
	}
	fields = append(fields, validateReportFilters(input.Entity, input.Filters, allowed, customDefinitions, vocabularies)...)
	if len(fields) > 0 {
		return contract.ReportDefinition{}, validationError(fields...)
	}
	return input, nil
}

func validateReportFilters(entity string, filters map[string]any, allowed map[string]bool, customDefinitions map[string]contract.CustomFieldDefinition, vocabularies map[string][]string) []contract.FieldError {
	var fields []contract.FieldError
	dateFields := reportDateFilterFields[entity]
	for key, value := range filters {
		if field, isDate := dateFields[key]; isDate {
			_ = field
			if _, ok := reportFilterTime(value); !ok {
				fields = append(fields, contract.FieldError{Field: "/filters/" + key, Code: contract.ValidationInvalidFormat})
			}
			continue
		}
		if !allowed[key] || key == "count" || !validReportFilterValue(value) || !validReportTypedFilter(key, value, customDefinitions, vocabularies) {
			fields = append(fields, contract.FieldError{Field: "/filters/" + key, Code: contract.ValidationInvalidValue})
		}
	}
	for _, pair := range [][2]string{{"created_from", "created_to"}, {"updated_from", "updated_to"}, {"resolved_from", "resolved_to"}, {"occurred_from", "occurred_to"}} {
		from, fromOK := reportFilterTime(filters[pair[0]])
		to, toOK := reportFilterTime(filters[pair[1]])
		if fromOK && toOK && from.After(to) {
			fields = append(fields, contract.FieldError{Field: "/filters/" + pair[0], Code: contract.ValidationOutOfRange})
		}
	}
	sort.SliceStable(fields, func(i, j int) bool { return fields[i].Field < fields[j].Field })
	return fields
}

func validReportTypedFilter(field string, value any, customDefinitions map[string]contract.CustomFieldDefinition, vocabularies map[string][]string) bool {
	values := reportFilterValues(value)
	if definition, ok := customDefinitions[field]; ok {
		for _, candidate := range values {
			if definition.FieldType == contract.CustomFieldTypeMultiChoice {
				choice, isString := candidate.(string)
				if !isString || !containsString(definition.ChoiceValues, choice) {
					return false
				}
				continue
			}
			if validateCustomFieldValue(definition, candidate) != "" {
				return false
			}
		}
		return true
	}
	if vocabulary, ok := vocabularies[field]; ok {
		for _, candidate := range values {
			if !containsString(vocabulary, reportValueString(candidate)) {
				return false
			}
		}
	}
	return true
}

func reportVocabularies() map[string][]string {
	return map[string][]string{
		"service_type":       reportEnumValues(contract.ServiceTypes),
		"department":         reportEnumValues(contract.DepartmentCodes),
		"district":           reportEnumValues(contract.DistrictCodes),
		"status":             reportEnumValues(contract.ServiceRequestStatuses),
		"source_channel":     reportEnumValues(contract.SourceChannels),
		"origin_class":       reportEnumValues(contract.OriginClasses),
		"preferred_language": reportEnumValues(contract.Languages),
		"visibility":         {"STAFF", "PUBLIC"},
	}
}

func reportEnumValues[T ~string](input []T) []string {
	out := make([]string, 0, len(input))
	for _, value := range input {
		out = append(out, string(value))
	}
	return out
}

func (svc *Service) buildReportRows(ctx context.Context, actor contract.Actor, definition contract.ReportDefinition) ([]map[string]any, error) {
	var rows []map[string]any
	var err error
	switch definition.Entity {
	case "service_requests":
		rows, err = svc.serviceRequestReportRows(ctx, actor)
	case "constituents":
		rows, err = svc.constituentReportRows(ctx, actor)
	case "follow_up_actions":
		rows, err = svc.followUpReportRows(ctx, actor)
	}
	if err != nil {
		return nil, err
	}
	filtered := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		if matchesReportFilters(row, definition.Filters, reportDateFilterFields[definition.Entity]) {
			filtered = append(filtered, row)
		}
	}
	filtered = aggregateReportRows(filtered, definition)
	sortReportRows(filtered, definition.Sort, definition.Grouping)
	projected := make([]map[string]any, 0, len(filtered))
	for _, row := range filtered {
		item := make(map[string]any, len(definition.Columns))
		for _, column := range definition.Columns {
			item[column] = row[column]
		}
		projected = append(projected, item)
	}
	return projected, nil
}

func (svc *Service) serviceRequestReportRows(ctx context.Context, actor contract.Actor) ([]map[string]any, error) {
	requests, _, err := store.SearchCity311ServiceRequests(ctx, svc.store, composeTypes.City311ServiceRequestFilter{})
	if err != nil {
		return nil, err
	}
	reminders, _, err := store.SearchReminders(ctx, svc.store, systemTypes.ReminderFilter{})
	if err != nil {
		return nil, err
	}
	audits, _, err := store.SearchCity311AuditEvents(ctx, svc.store, composeTypes.City311AuditEventFilter{})
	if err != nil {
		return nil, err
	}
	overdueByRequest := map[uint64]int{}
	for _, reminder := range reminders {
		payload, decodeErr := decodeReminderPayload(reminder)
		if decodeErr != nil || reminder.Resource != requestReminderResource(payload.RequestID) {
			continue
		}
		if (payload.Status == contract.ReminderStatusScheduled || payload.Status == contract.ReminderStatusSnoozed) && payload.DueAt.Before(svc.now()) {
			overdueByRequest[payload.RequestID]++
		}
	}
	resolvedAt, reopened := map[uint64]time.Time{}, map[uint64]bool{}
	for _, audit := range audits {
		if audit.RequestID == 0 {
			continue
		}
		if audit.EventType == "SERVICE_REQUEST_TRANSITIONED" && fmt.Sprint(audit.After["status"]) == string(contract.ServiceRequestStatusResolved) {
			resolvedAt[audit.RequestID] = audit.CreatedAt.UTC()
		}
		if strings.Contains(audit.EventType, "REOPEN") || fmt.Sprint(audit.After["status"]) == string(contract.ServiceRequestStatusReopened) {
			reopened[audit.RequestID] = true
		}
	}
	rows := make([]map[string]any, 0, len(requests))
	activeByAssignee, overdueByAssignee := map[string]int{}, map[string]int{}
	for _, request := range requests {
		if !canRead(actor, request) {
			continue
		}
		assignee := ""
		if request.PrimaryAssigneeID != 0 {
			assignee = strconv.FormatUint(request.PrimaryAssigneeID, 10)
		}
		if request.Status != contract.ServiceRequestStatusResolved && request.Status != contract.ServiceRequestStatusClosed {
			activeByAssignee[assignee]++
		}
		overdueByAssignee[assignee] += overdueByRequest[request.ID]
		resolved := resolvedAt[request.ID]
		elapsed := int64(0)
		if !resolved.IsZero() {
			elapsed = int64(resolved.Sub(request.CreatedAt.UTC()).Seconds())
		}
		row := map[string]any{
			"request_id": strconv.FormatUint(request.ID, 10), "request_number": request.RequestNumber,
			"summary": request.Summary, "description": request.Description, "service_type": string(request.ServiceType),
			"department": string(request.OwningDepartment), "district": string(request.CouncilDistrict), "status": string(request.Status),
			"source_channel": string(request.SourceChannel), "origin_class": string(request.OriginClass), "primary_assignee": assignee,
			"collaborator_count": len(request.CollaboratorIDs), "created_date": request.CreatedAt.UTC().Format(time.DateOnly),
			"created_at": reportTime(request.CreatedAt), "updated_at": reportTime(request.UpdatedAt),
			"age": int64(svc.now().Sub(request.CreatedAt).Seconds()), "submitted_at": reportTime(request.CreatedAt), "resolved_at": reportOptionalTime(resolved),
			"elapsed_duration": elapsed, "reopened": reopened[request.ID], "count": 1,
		}
		addReportCustomFields(row, request.CustomFields)
		rows = append(rows, row)
	}
	for _, row := range rows {
		assignee := fmt.Sprint(row["primary_assignee"])
		row["active_request_count"], row["overdue_reminder_count"] = activeByAssignee[assignee], overdueByAssignee[assignee]
	}
	return rows, nil
}

func (svc *Service) constituentReportRows(ctx context.Context, actor contract.Actor) ([]map[string]any, error) {
	set, _, err := store.SearchCity311Constituents(ctx, svc.store, composeTypes.City311ConstituentFilter{})
	if err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(set))
	for _, constituent := range set {
		if !canReadConstituent(actor, constituent) {
			continue
		}
		profile := constituent.Profile
		row := map[string]any{
			"constituent_id": constituent.ConstituentID, "display_name": profile["display_name"], "emails": profile["emails"],
			"phone_numbers": profile["phone_numbers"], "addresses": profile["addresses"], "primary_category": profile["primary_category"],
			"preferred_language": profile["preferred_language"], "email_opt_out": profile["email_opt_out"],
			"department": string(constituent.OwningDepartment), "district": string(constituent.CouncilDistrict),
			"created_at": reportTime(constituent.CreatedAt), "updated_at": reportTime(constituent.UpdatedAt),
		}
		addReportCustomFields(row, profileCustomFields(profile))
		rows = append(rows, row)
	}
	return rows, nil
}

func (svc *Service) followUpReportRows(ctx context.Context, actor contract.Actor) ([]map[string]any, error) {
	audits, _, err := store.SearchCity311AuditEvents(ctx, svc.store, composeTypes.City311AuditEventFilter{})
	if err != nil {
		return nil, err
	}
	requests := map[uint64]*composeTypes.City311ServiceRequest{}
	rows := make([]map[string]any, 0, len(audits))
	for _, audit := range audits {
		if audit.RequestID == 0 {
			continue
		}
		request := requests[audit.RequestID]
		if request == nil {
			request, err = store.LookupCity311ServiceRequestByID(ctx, svc.store, audit.RequestID)
			if err != nil {
				return nil, err
			}
			requests[audit.RequestID] = request
		}
		if !canRead(actor, request) {
			continue
		}
		followUp := projectFollowUpAction(audit)
		rows = append(rows, map[string]any{
			"request_id": followUp.RequestID, "request_number": request.RequestNumber, "action_type": followUp.ActionType,
			"actor": followUp.Actor, "occurred_at": reportTime(followUp.OccurredAt), "local_display_time": followUp.LocalDisplayTime,
			"visibility": followUp.Visibility, "department": string(request.OwningDepartment),
		})
	}
	return rows, nil
}

func aggregateReportRows(rows []map[string]any, definition contract.ReportDefinition) []map[string]any {
	if containsString(definition.Columns, "active_request_count") || containsString(definition.Columns, "overdue_reminder_count") {
		field := "primary_assignee"
		if definition.Grouping != nil {
			field = *definition.Grouping
		}
		return distinctReportRows(rows, field)
	}
	if !containsString(definition.Columns, "count") {
		return rows
	}
	if definition.Grouping == nil {
		row := map[string]any{"count": len(rows)}
		return []map[string]any{row}
	}
	groups := map[string]map[string]any{}
	order := []string{}
	for _, row := range rows {
		key := reportValueString(row[*definition.Grouping])
		group := groups[key]
		if group == nil {
			group = cloneMap(row)
			group["count"] = 0
			groups[key] = group
			order = append(order, key)
		}
		group["count"] = group["count"].(int) + 1
	}
	out := make([]map[string]any, 0, len(order))
	for _, key := range order {
		out = append(out, groups[key])
	}
	return out
}

func distinctReportRows(rows []map[string]any, field string) []map[string]any {
	indices := map[string]int{}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		key := reportValueString(row[field])
		index, found := indices[key]
		if !found {
			item := cloneMap(row)
			item["collaborator_count"] = 0
			indices[key] = len(out)
			out = append(out, item)
			index = len(out) - 1
		}
		if count, ok := numberValue(row["collaborator_count"]); ok {
			out[index]["collaborator_count"] = out[index]["collaborator_count"].(int) + int(count)
		}
	}
	return out
}

func sortReportRows(rows []map[string]any, expressions []string, grouping *string) {
	sorts := append([]string(nil), expressions...)
	if grouping != nil && !reportSortContains(sorts, *grouping) {
		sorts = append([]string{*grouping}, sorts...)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		for _, expression := range sorts {
			descending := strings.HasPrefix(expression, "-")
			field := strings.TrimPrefix(expression, "-")
			comparison := compareReportValues(rows[i][field], rows[j][field])
			if comparison == 0 {
				continue
			}
			if descending {
				return comparison > 0
			}
			return comparison < 0
		}
		return reportValueString(rows[i]) < reportValueString(rows[j])
	})
}

func matchesReportFilters(row map[string]any, filters map[string]any, dateFields map[string]string) bool {
	for key, expected := range filters {
		if field, ok := dateFields[key]; ok {
			boundary, valid := reportFilterTime(expected)
			actual, actualOK := reportFilterTime(row[field])
			if !valid || !actualOK {
				return false
			}
			if strings.HasSuffix(key, "_from") && actual.Before(boundary) {
				return false
			}
			if strings.HasSuffix(key, "_to") && actual.After(boundary) {
				return false
			}
			continue
		}
		if !reportValueMatches(row[key], expected) {
			return false
		}
	}
	return true
}

func reportValueMatches(actual, expected any) bool {
	for _, actualValue := range reportFilterValues(actual) {
		for _, expectedValue := range reportFilterValues(expected) {
			if reportValueString(actualValue) == reportValueString(expectedValue) {
				return true
			}
		}
	}
	return false
}

func reportFilterValues(value any) []any {
	switch values := value.(type) {
	case []any:
		return values
	case []string:
		out := make([]any, 0, len(values))
		for _, item := range values {
			out = append(out, item)
		}
		return out
	default:
		return []any{value}
	}
}

func validReportFilterValue(value any) bool {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != "" && utf8.RuneCountInString(typed) <= 160
	case bool, float64, float32, int, int64, uint64:
		return true
	case []string:
		if len(typed) == 0 {
			return false
		}
		for _, item := range typed {
			if !validReportFilterValue(item) {
				return false
			}
		}
		return true
	case []any:
		if len(typed) == 0 {
			return false
		}
		for _, item := range typed {
			if !validReportFilterValue(item) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func reportFilterTime(value any) (time.Time, bool) {
	text, ok := value.(string)
	if !ok || text == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339, text)
	return parsed.UTC(), err == nil
}

func encodeReportCSV(columns []string, rows []map[string]any) ([]byte, error) {
	buffer := &bytes.Buffer{}
	writer := csv.NewWriter(buffer)
	writer.UseCRLF = true
	if err := writer.Write(columns); err != nil {
		return nil, err
	}
	for _, row := range rows {
		record := make([]string, len(columns))
		for index, column := range columns {
			record[index] = reportValueString(row[column])
		}
		if err := writer.Write(record); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	return buffer.Bytes(), writer.Error()
}

func (svc *Service) lookupSavedReport(ctx context.Context, st store.Storer, actor contract.Actor, reportID string, ownerOnly bool) (*composeTypes.City311ConfigurationRevision, savedReportPayload, error) {
	reportID = strings.TrimSpace(reportID)
	if !validResourceKey(reportID) {
		return nil, savedReportPayload{}, apiError(http.StatusNotFound, contract.ErrorNotFound, "The saved report was not found.")
	}
	revision, err := svc.latestConfigurationRevision(ctx, st, configurationSavedReport, reportID, "", false)
	if err != nil {
		return nil, savedReportPayload{}, apiError(http.StatusNotFound, contract.ErrorNotFound, "The saved report was not found.")
	}
	payload := savedReportFromRevision(revision)
	actorID := strconv.FormatUint(actor.ID, 10)
	if ownerOnly && payload.OwnerID != actorID {
		return nil, savedReportPayload{}, apiError(http.StatusForbidden, contract.ErrorForbidden, "Only the report owner may change the saved report.")
	}
	if !ownerOnly && !reportVisibleTo(actor, payload) {
		return nil, savedReportPayload{}, apiError(http.StatusForbidden, contract.ErrorForbidden, "The saved report is not shared with this actor.")
	}
	return revision, payload, nil
}

func newSavedReportPayload(definition contract.ReportDefinition, actor contract.Actor) savedReportPayload {
	return savedReportPayload{
		ReportID: definition.ReportID, Name: definition.Name, Entity: definition.Entity,
		Columns: append([]string(nil), definition.Columns...), Filters: cloneMap(definition.Filters), Grouping: definition.Grouping,
		Sort: append([]string(nil), definition.Sort...), OwnerID: strconv.FormatUint(actor.ID, 10), SharedRoles: []contract.ApplicationRole{},
		OwnerDepartment: actor.Department, OwnerDistricts: append([]contract.DistrictCode(nil), actor.Districts...),
		OwnerAllDistricts: hasRole(actor, contract.ApplicationRoleDepartmentManager), OwnerPlatform: hasRole(actor, contract.ApplicationRolePlatformAdministrator),
	}
}

func savedReportFromRevision(revision *composeTypes.City311ConfigurationRevision) savedReportPayload {
	payload := savedReportPayload{ReportID: revision.ResourceKey}
	decodeConfigurationPayload(revision.Payload, &payload)
	payload.ReportID = revision.ResourceKey
	if payload.Filters == nil {
		payload.Filters = map[string]any{}
	}
	if payload.SharedRoles == nil {
		payload.SharedRoles = []contract.ApplicationRole{}
	}
	return payload
}

func reportDefinitionFromRevision(revision *composeTypes.City311ConfigurationRevision) contract.ReportDefinition {
	payload := savedReportFromRevision(revision)
	definition := reportDefinitionFromPayload(payload)
	definition.Version, definition.UpdatedAt = uint64(revision.Version), revision.CreatedAt
	return definition
}

func reportDefinitionFromPayload(payload savedReportPayload) contract.ReportDefinition {
	return contract.ReportDefinition{
		ReportID: payload.ReportID, Name: payload.Name, Entity: payload.Entity,
		Columns: append([]string(nil), payload.Columns...), Filters: cloneMap(payload.Filters), Grouping: payload.Grouping,
		Sort: append([]string(nil), payload.Sort...),
	}
}

func reportVisibleTo(actor contract.Actor, payload savedReportPayload) bool {
	if strconv.FormatUint(actor.ID, 10) == payload.OwnerID {
		return true
	}
	roleShared := false
	for _, role := range payload.SharedRoles {
		if hasRole(actor, role) {
			roleShared = true
			break
		}
	}
	if !roleShared {
		return false
	}
	if payload.OwnerPlatform {
		return isStaff(actor)
	}
	if hasRole(actor, contract.ApplicationRolePlatformAdministrator) || actor.Department != payload.OwnerDepartment {
		return false
	}
	if payload.OwnerAllDistricts {
		return true
	}
	if hasRole(actor, contract.ApplicationRoleDepartmentManager) {
		return false
	}
	for _, district := range actor.Districts {
		if !containsEnums([]contract.DistrictCode{district}, payload.OwnerDistricts) {
			return false
		}
	}
	return true
}

func validateReportShare(actor contract.Actor, roles []contract.ApplicationRole) ([]contract.ApplicationRole, error) {
	seen := map[contract.ApplicationRole]bool{}
	out := make([]contract.ApplicationRole, 0, len(roles))
	ownerRank := reportRoleRank(actor.Roles)
	for index, role := range roles {
		rank := reportRoleRank([]contract.ApplicationRole{role})
		if rank == 0 || rank > ownerRank {
			return nil, validationError(contract.FieldError{Field: fmt.Sprintf("/roles/%d", index), Code: contract.ValidationInvalidValue})
		}
		if seen[role] {
			return nil, validationError(contract.FieldError{Field: fmt.Sprintf("/roles/%d", index), Code: contract.ValidationDuplicate})
		}
		seen[role] = true
		out = append(out, role)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func reportRoleRank(roles []contract.ApplicationRole) int {
	rank := 0
	for _, role := range roles {
		candidate := 0
		switch role {
		case contract.ApplicationRolePlatformAdministrator:
			candidate = 4
		case contract.ApplicationRoleDepartmentManager:
			candidate = 3
		case contract.ApplicationRoleSupervisor:
			candidate = 2
		case contract.ApplicationRoleServiceAgent, contract.ApplicationRoleWorkflowDesigner:
			candidate = 1
		}
		if candidate > rank {
			rank = candidate
		}
	}
	return rank
}

func addReportCustomFields(row map[string]any, values map[string]any) {
	for key, value := range values {
		row["custom_fields."+key] = value
	}
}

func cloneBoolMap(input map[string]bool) map[string]bool {
	out := make(map[string]bool, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func reportTime(value time.Time) string {
	return value.UTC().Format(time.RFC3339)
}

func reportOptionalTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return reportTime(value)
}

func reportValueString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case time.Time:
		return reportTime(typed)
	default:
		encoded, err := json.Marshal(value)
		if err == nil {
			return string(encoded)
		}
		return fmt.Sprint(value)
	}
}

func compareReportValues(left, right any) int {
	leftNumber, leftOK := numberValue(left)
	rightNumber, rightOK := numberValue(right)
	if leftOK && rightOK {
		switch {
		case leftNumber < rightNumber:
			return -1
		case leftNumber > rightNumber:
			return 1
		default:
			return 0
		}
	}
	return strings.Compare(reportValueString(left), reportValueString(right))
}

func reportSortContains(expressions []string, field string) bool {
	for _, expression := range expressions {
		if strings.TrimPrefix(expression, "-") == field {
			return true
		}
	}
	return false
}
