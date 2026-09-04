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
	"github.com/cortezaproject/corteza/server/pkg/errors"
	"github.com/cortezaproject/corteza/server/store"
)

const auditExportKind = "AUDIT_EXPORT"

type auditSelection struct {
	requestIDs map[uint64]bool
	actorIDs   map[uint64]bool
	filters    contract.AuditFilter
	sort       []string
}

type OperationDownload struct {
	Content     []byte
	ContentType string
	Filename    string
}

func (svc *Service) ListAuditEvents(ctx context.Context, actor contract.Actor, query contract.AuditQuery) (*contract.AuditListResponse, error) {
	if !canAccessAudit(actor) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, "A department manager or platform administrator role is required.")
	}
	selection, err := prepareAuditSelection(query.Filters, query.Sort, "/query/filters", "/query/sort")
	if err != nil {
		return nil, err
	}
	if query.PageSize == 0 {
		query.PageSize = 50
	}
	if query.PageSize > 100 {
		return nil, validationError(contract.FieldError{Field: "/query/page_size", Code: contract.ValidationOutOfRange})
	}
	offset, err := decodePageToken(query.PageToken, selection.sort)
	if err != nil {
		return nil, validationError(contract.FieldError{Field: "/query/page_token", Code: contract.ValidationInvalidFormat})
	}
	matching, err := svc.selectAuditEvents(ctx, svc.store, actor, selection)
	if err != nil {
		return nil, err
	}
	if offset > len(matching) {
		return nil, validationError(contract.FieldError{Field: "/query/page_token", Code: contract.ValidationInvalidFormat})
	}
	end := offset + int(query.PageSize)
	if end > len(matching) {
		end = len(matching)
	}
	response := &contract.AuditListResponse{
		Items: make([]contract.AuditEvent, 0, end-offset), TotalCount: len(matching),
		AppliedFilters: auditAppliedFilters(query.Filters), Sort: selection.sort,
	}
	for _, event := range matching[offset:end] {
		response.Items = append(response.Items, toAuditEvent(event))
	}
	if end < len(matching) {
		token, tokenErr := encodePageToken(end, selection.sort)
		if tokenErr != nil {
			return nil, tokenErr
		}
		response.NextPageToken = &token
	}
	return response, nil
}

func (svc *Service) StartAuditExport(ctx context.Context, actor contract.Actor, input contract.AuditExport) (*contract.Operation, error) {
	if !canAccessAudit(actor) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, "A department manager or platform administrator role is required.")
	}
	selection, err := prepareAuditSelection(input.Filters, "occurred_at", "/filters", "/filters")
	if err != nil {
		return nil, err
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	pending := &contract.Operation{}
	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		now := svc.now()
		operation := &composeTypes.City311Operation{
			ID: svc.nextID(), Kind: auditExportKind, Status: string(contract.OperationStatusPending), ActorID: actor.ID,
			Result: composeTypes.City311JSON{}, Error: composeTypes.City311JSON{}, CreatedAt: now, UpdatedAt: now,
		}
		if err = store.CreateCity311Operation(ctx, tx, operation); err != nil {
			return err
		}
		*pending = *toOperation(operation)

		events, selectErr := svc.selectAuditEvents(ctx, tx, actor, selection)
		if selectErr != nil {
			return selectErr
		}
		content, encodeErr := encodeAuditCSV(events)
		if encodeErr != nil {
			return encodeErr
		}
		completedAt := svc.now()
		operation.Status = string(contract.OperationStatusSucceeded)
		operation.Progress = 100
		operation.Content = content
		operation.ContentType = "text/csv; charset=utf-8"
		operation.Filename = "audit-events-" + completedAt.UTC().Format("20060102T150405Z") + ".csv"
		operation.Result = composeTypes.City311JSON{"download_url": "/api/v1/operations/" + publicOperationID(operation.ID) + "/result"}
		operation.UpdatedAt = completedAt
		operation.CompletedAt = &completedAt
		if err = store.UpdateCity311Operation(ctx, tx, operation); err != nil {
			return err
		}
		return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
			ID: svc.nextID(), EntityType: "audit_export", EntityID: publicOperationID(operation.ID), EventType: "AUDIT_EXPORTED",
			ActorType: contract.AuditActorStaff, ActorID: actor.ID, SourceChannel: contract.SourceChannelStaffInPerson,
			Before: composeTypes.City311JSON{}, After: composeTypes.City311JSON{
				"department_code": optionalActorDepartment(actor), "filters": auditAppliedFilters(input.Filters), "row_count": len(events),
			}, CreatedAt: completedAt,
		})
	})
	if err != nil {
		return nil, err
	}
	return pending, nil
}

func (svc *Service) GetOperation(ctx context.Context, actor contract.Actor, rawID string) (*contract.Operation, error) {
	operation, err := svc.lookupOperation(ctx, actor, rawID)
	if err != nil {
		return nil, err
	}
	return toOperation(operation), nil
}

func (svc *Service) DownloadOperation(ctx context.Context, actor contract.Actor, rawID string) (*OperationDownload, error) {
	operation, err := svc.lookupOperation(ctx, actor, rawID)
	if err != nil {
		return nil, err
	}
	if operation.Status != string(contract.OperationStatusSucceeded) || len(operation.Content) == 0 {
		return nil, apiError(http.StatusNotFound, contract.ErrorNotFound, "The operation result is not available.")
	}
	return &OperationDownload{Content: append([]byte(nil), operation.Content...), ContentType: operation.ContentType, Filename: operation.Filename}, nil
}

func (svc *Service) lookupOperation(ctx context.Context, actor contract.Actor, rawID string) (*composeTypes.City311Operation, error) {
	operationID, err := parseOperationID(rawID)
	if err != nil {
		return nil, validationError(contract.FieldError{Field: "/path/operation_id", Code: contract.ValidationInvalidFormat})
	}
	operation, err := store.LookupCity311OperationByID(ctx, svc.store, operationID)
	if errors.IsNotFound(err) {
		return nil, apiError(http.StatusNotFound, contract.ErrorNotFound, "The operation was not found.")
	}
	if err != nil {
		return nil, err
	}
	if operation.ActorID != actor.ID && !hasRole(actor, contract.ApplicationRolePlatformAdministrator) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, "The operation belongs to another actor.")
	}
	return operation, nil
}

func prepareAuditSelection(filters contract.AuditFilter, rawSort, filterPrefix, sortField string) (auditSelection, error) {
	var fields []contract.FieldError
	requestIDs, requestErrors := parseAuditIDs(filters.RequestIDs, filterPrefix+"/request_id", false)
	actorIDs, actorErrors := parseAuditIDs(filters.ActorIDs, filterPrefix+"/actor_id", true)
	fields = append(fields, requestErrors...)
	fields = append(fields, actorErrors...)
	if !containsEnums(filters.ActorTypes, contract.AuditActorTypes) {
		fields = append(fields, contract.FieldError{Field: filterPrefix + "/actor_type", Code: contract.ValidationInvalidValue})
	}
	if !containsEnums(filters.SourceChannels, contract.SourceChannels) {
		fields = append(fields, contract.FieldError{Field: filterPrefix + "/source_channel", Code: contract.ValidationInvalidValue})
	}
	if filters.OccurredFrom != nil && filters.OccurredTo != nil && filters.OccurredFrom.After(*filters.OccurredTo) {
		fields = append(fields, contract.FieldError{Field: filterPrefix + "/occurred_from", Code: contract.ValidationOutOfRange})
	}
	for field, values := range map[string][]string{
		"entity_type": filters.EntityTypes, "entity_id": filters.EntityIDs, "event_type": filters.EventTypes,
	} {
		for _, value := range values {
			length := utf8.RuneCountInString(strings.TrimSpace(value))
			if length == 0 || length > 96 {
				fields = append(fields, contract.FieldError{Field: filterPrefix + "/" + field, Code: contract.ValidationInvalidValue})
				break
			}
		}
	}
	if len(fields) > 0 {
		return auditSelection{}, validationError(fields...)
	}
	publishedSort, err := normalizeAuditSort(rawSort)
	if err != nil {
		return auditSelection{}, validationError(contract.FieldError{Field: sortField, Code: contract.ValidationInvalidFormat})
	}
	return auditSelection{requestIDs: requestIDs, actorIDs: actorIDs, filters: filters, sort: publishedSort}, nil
}

func parseAuditIDs(values []string, field string, allowZero bool) (map[uint64]bool, []contract.FieldError) {
	parsed := make(map[uint64]bool, len(values))
	for _, value := range values {
		id, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
		if err != nil || (!allowZero && id == 0) {
			return nil, []contract.FieldError{{Field: field, Code: contract.ValidationInvalidFormat}}
		}
		parsed[id] = true
	}
	return parsed, nil
}

func normalizeAuditSort(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "-occurred_at"
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 3 {
		return nil, fmt.Errorf("too many sort fields")
	}
	allowed := map[string]bool{
		"occurred_at": true, "entity_type": true, "entity_id": true, "event_type": true,
		"actor_type": true, "actor_id": true, "source_channel": true,
	}
	out := make([]string, 0, len(parts))
	for _, expression := range parts {
		expression = strings.TrimSpace(expression)
		descending := strings.HasPrefix(expression, "-")
		field := strings.TrimPrefix(strings.TrimPrefix(expression, "-"), "+")
		if !allowed[field] {
			return nil, fmt.Errorf("unsupported sort field")
		}
		if descending {
			field = "-" + field
		}
		out = append(out, field)
	}
	return out, nil
}

func (svc *Service) selectAuditEvents(ctx context.Context, st store.Storer, actor contract.Actor, selection auditSelection) (composeTypes.City311AuditEventSet, error) {
	set, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{})
	if err != nil {
		return nil, err
	}
	requestCache := make(map[uint64]*composeTypes.City311ServiceRequest)
	out := make(composeTypes.City311AuditEventSet, 0, len(set))
	for _, event := range set {
		allowed, scopeErr := svc.auditEventInScope(ctx, st, actor, event, requestCache)
		if scopeErr != nil {
			return nil, scopeErr
		}
		if allowed && matchesAuditSelection(event, selection) {
			out = append(out, event)
		}
	}
	sortAuditEvents(out, selection.sort)
	return out, nil
}

func (svc *Service) auditEventInScope(ctx context.Context, st store.Storer, actor contract.Actor, event *composeTypes.City311AuditEvent, cache map[uint64]*composeTypes.City311ServiceRequest) (bool, error) {
	if hasRole(actor, contract.ApplicationRolePlatformAdministrator) {
		return true, nil
	}
	if !hasRole(actor, contract.ApplicationRoleDepartmentManager) {
		return false, nil
	}
	if event.RequestID == 0 {
		return auditDepartment(event) == actor.Department, nil
	}
	request, cached := cache[event.RequestID]
	if !cached {
		var err error
		request, err = store.LookupCity311ServiceRequestByID(ctx, st, event.RequestID)
		if errors.IsNotFound(err) {
			cache[event.RequestID] = nil
			return false, nil
		}
		if err != nil {
			return false, err
		}
		cache[event.RequestID] = request
	}
	return request != nil && canRead(actor, request), nil
}

func matchesAuditSelection(event *composeTypes.City311AuditEvent, selection auditSelection) bool {
	filters := selection.filters
	return (len(selection.requestIDs) == 0 || selection.requestIDs[event.RequestID]) &&
		(len(selection.actorIDs) == 0 || selection.actorIDs[event.ActorID]) &&
		matchesString(event.EntityType, filters.EntityTypes) &&
		matchesString(event.EntityID, filters.EntityIDs) &&
		matchesString(event.EventType, filters.EventTypes) &&
		matchesString(string(event.ActorType), filters.ActorTypes) &&
		matchesString(string(event.SourceChannel), filters.SourceChannels) &&
		(filters.OccurredFrom == nil || !event.CreatedAt.Before(*filters.OccurredFrom)) &&
		(filters.OccurredTo == nil || !event.CreatedAt.After(*filters.OccurredTo))
}

func sortAuditEvents(set composeTypes.City311AuditEventSet, publishedSort []string) {
	sort.SliceStable(set, func(i, j int) bool {
		left, right := set[i], set[j]
		for _, expression := range publishedSort {
			descending := strings.HasPrefix(expression, "-")
			comparison := compareAuditField(left, right, strings.TrimPrefix(expression, "-"))
			if comparison == 0 {
				continue
			}
			if descending {
				return comparison > 0
			}
			return comparison < 0
		}
		return left.ID < right.ID
	})
}

func compareAuditField(left, right *composeTypes.City311AuditEvent, field string) int {
	switch field {
	case "occurred_at":
		return left.CreatedAt.Compare(right.CreatedAt)
	case "entity_type":
		return strings.Compare(left.EntityType, right.EntityType)
	case "entity_id":
		return strings.Compare(left.EntityID, right.EntityID)
	case "event_type":
		return strings.Compare(left.EventType, right.EventType)
	case "actor_type":
		return strings.Compare(string(left.ActorType), string(right.ActorType))
	case "actor_id":
		return compareUint64(left.ActorID, right.ActorID)
	case "source_channel":
		return strings.Compare(string(left.SourceChannel), string(right.SourceChannel))
	default:
		return 0
	}
}

func compareUint64(left, right uint64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func encodeAuditCSV(events composeTypes.City311AuditEventSet) ([]byte, error) {
	buffer := &bytes.Buffer{}
	writer := csv.NewWriter(buffer)
	writer.UseCRLF = true
	if err := writer.Write([]string{"entity_type", "entity_id", "event_type", "actor_type", "actor_id", "occurred_at", "source_channel", "before", "after"}); err != nil {
		return nil, err
	}
	for _, event := range events {
		before, err := json.Marshal(event.Before)
		if err != nil {
			return nil, err
		}
		after, err := json.Marshal(event.After)
		if err != nil {
			return nil, err
		}
		if err = writer.Write([]string{
			event.EntityType, event.EntityID, event.EventType, string(event.ActorType), strconv.FormatUint(event.ActorID, 10),
			event.CreatedAt.UTC().Format(time.RFC3339Nano), string(event.SourceChannel), string(before), string(after),
		}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func toAuditEvent(event *composeTypes.City311AuditEvent) contract.AuditEvent {
	return contract.AuditEvent{
		EntityType: event.EntityType, EntityID: event.EntityID, EventType: event.EventType,
		ActorType: event.ActorType, ActorID: strconv.FormatUint(event.ActorID, 10), OccurredAt: event.CreatedAt,
		SourceChannel: event.SourceChannel, Before: cloneMap(event.Before), After: cloneMap(event.After),
	}
}

func toOperation(operation *composeTypes.City311Operation) *contract.Operation {
	var result map[string]any
	if len(operation.Result) > 0 {
		result = cloneMap(operation.Result)
	}
	var operationError *contract.APIError
	if len(operation.Error) > 0 {
		encoded, err := json.Marshal(operation.Error)
		if err == nil {
			operationError = &contract.APIError{}
			if json.Unmarshal(encoded, operationError) != nil {
				operationError = nil
			}
		}
	}
	return &contract.Operation{
		OperationID: publicOperationID(operation.ID), Kind: operation.Kind, Status: contract.OperationStatus(operation.Status),
		Progress: operation.Progress, Result: result, Error: operationError, CreatedAt: operation.CreatedAt,
		UpdatedAt: operation.UpdatedAt, CompletedAt: operation.CompletedAt,
	}
}

func publicOperationID(id uint64) string {
	return "op-" + strconv.FormatUint(id, 10)
}

func parseOperationID(raw string) (uint64, error) {
	if !strings.HasPrefix(raw, "op-") {
		return 0, fmt.Errorf("operation id must start with op-")
	}
	id, err := strconv.ParseUint(strings.TrimPrefix(raw, "op-"), 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("operation id must contain a positive decimal identifier")
	}
	return id, nil
}

func canAccessAudit(actor contract.Actor) bool {
	return hasRole(actor, contract.ApplicationRoleDepartmentManager) || hasRole(actor, contract.ApplicationRolePlatformAdministrator)
}

func auditDepartment(event *composeTypes.City311AuditEvent) contract.DepartmentCode {
	for _, values := range []composeTypes.City311JSON{event.After, event.Before} {
		if department, ok := values["department_code"].(string); ok {
			return contract.DepartmentCode(department)
		}
		if department, ok := values["owning_department"].(string); ok {
			return contract.DepartmentCode(department)
		}
	}
	return ""
}

func optionalActorDepartment(actor contract.Actor) any {
	if actor.Department == "" {
		return nil
	}
	return actor.Department
}

func auditAppliedFilters(filters contract.AuditFilter) map[string]any {
	out := map[string]any{}
	if len(filters.RequestIDs) > 0 {
		out["request_id"] = filters.RequestIDs
	}
	if len(filters.EntityTypes) > 0 {
		out["entity_type"] = filters.EntityTypes
	}
	if len(filters.EntityIDs) > 0 {
		out["entity_id"] = filters.EntityIDs
	}
	if len(filters.EventTypes) > 0 {
		out["event_type"] = filters.EventTypes
	}
	if len(filters.ActorTypes) > 0 {
		out["actor_type"] = filters.ActorTypes
	}
	if len(filters.ActorIDs) > 0 {
		out["actor_id"] = filters.ActorIDs
	}
	if len(filters.SourceChannels) > 0 {
		out["source_channel"] = filters.SourceChannels
	}
	if filters.OccurredFrom != nil {
		out["occurred_from"] = filters.OccurredFrom.UTC().Format(time.RFC3339Nano)
	}
	if filters.OccurredTo != nil {
		out["occurred_to"] = filters.OccurredTo.UTC().Format(time.RFC3339Nano)
	}
	return out
}
