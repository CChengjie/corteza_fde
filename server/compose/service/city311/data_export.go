package city311

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
)

const (
	dataExportLimitPerMinute = 60
	dataExportAuditEvent     = "DATA_EXPORTED"
)

var dataExportEntities = map[string]map[string]bool{
	"constituents": {
		"constituent_id": true, "department": true, "district": true, "primary_category": true,
		"preferred_language": true, "email_opt_out": true,
	},
	"service-requests": {
		"request_id": true, "request_number": true, "status": true, "service_type": true,
		"department": true, "district": true, "origin_class": true, "source_channel": true,
		"category": true, "duplicate_group": true,
	},
	"audit-events": {
		"request_id": true, "entity_type": true, "entity_id": true, "event_type": true,
		"actor_type": true, "actor_id": true, "source_channel": true,
	},
	"follow-up-actions": {
		"request_id": true, "action_type": true, "actor": true, "visibility": true,
	},
}

type dataExportRow struct {
	id   uint64
	item map[string]any
}

// CheckDataExportLimit applies the frozen per-client fixed-window limit. A
// positive return value is the number of whole seconds the client must wait.
func (svc *Service) CheckDataExportLimit(clientID uint64) int {
	now := svc.now().UTC()
	window := now.Truncate(time.Minute)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	current := svc.dataExportLimits[clientID]
	if current.WindowStart.IsZero() || !current.WindowStart.Equal(window) {
		current = dataExportLimit{WindowStart: window}
	}
	if current.Count >= dataExportLimitPerMinute {
		remaining := window.Add(time.Minute).Sub(now)
		retryAfter := int((remaining + time.Second - 1) / time.Second)
		if retryAfter < 1 {
			retryAfter = 1
		}
		return retryAfter
	}
	current.Count++
	svc.dataExportLimits[clientID] = current
	return 0
}

func (svc *Service) ExportData(ctx context.Context, actor contract.Actor, entity string, query contract.DataExportQuery) (*contract.ExportResponse, error) {
	entity = strings.TrimSpace(entity)
	baseFilters, supported := dataExportEntities[entity]
	if !supported {
		return nil, invalidExportFilter("The export entity is not supported.")
	}
	allowedFilters := make(map[string]bool, len(baseFilters))
	for key, allowed := range baseFilters {
		allowedFilters[key] = allowed
	}
	definitions, err := svc.exportCustomFieldDefinitions(ctx, entity)
	if err != nil {
		return nil, err
	}
	for key := range definitions {
		allowedFilters["custom_fields."+key] = true
	}
	if query.PageSize == 0 {
		query.PageSize = 50
	}
	if query.PageSize > 100 {
		return nil, invalidExportFilter("page_size must be between 1 and 100.")
	}
	filters, err := normalizeDataExportFilters(query.Filters, allowedFilters)
	if err != nil {
		return nil, err
	}
	activeCategories, err := svc.activeContactCategories(ctx, svc.store)
	if err != nil {
		return nil, err
	}
	if err = validateDataExportFilters(entity, filters, activeCategories); err != nil {
		return nil, err
	}

	tokenBinding, err := dataExportTokenBinding(entity, filters, query.UpdatedSince)
	if err != nil {
		return nil, err
	}
	offset, err := decodePageToken(query.PageToken, []string{tokenBinding})
	if err != nil {
		return nil, invalidPageToken()
	}

	rows, err := svc.dataExportRows(ctx, actor, entity, filters, query.UpdatedSince)
	if err != nil {
		return nil, err
	}
	if offset > len(rows) {
		return nil, invalidPageToken()
	}
	end := offset + int(query.PageSize)
	if end > len(rows) {
		end = len(rows)
	}
	response := &contract.ExportResponse{
		Items: make([]map[string]any, 0, end-offset), GeneratedAt: svc.now().UTC(),
	}
	for _, row := range rows[offset:end] {
		response.Items = append(response.Items, row.item)
	}
	if end < len(rows) {
		next, tokenErr := encodePageToken(end, []string{tokenBinding})
		if tokenErr != nil {
			return nil, tokenErr
		}
		response.NextPageToken = &next
	}
	if err = svc.recordDataExport(ctx, actor, entity, filters, len(response.Items)); err != nil {
		return nil, err
	}
	return response, nil
}

func (svc *Service) dataExportRows(ctx context.Context, actor contract.Actor, entity string, filters map[string][]string, updatedSince *time.Time) ([]dataExportRow, error) {
	var rows []dataExportRow
	switch entity {
	case "constituents":
		set, _, err := store.SearchCity311Constituents(ctx, svc.store, composeTypes.City311ConstituentFilter{})
		if err != nil {
			return nil, err
		}
		for _, item := range set {
			if !canReadConstituent(actor, item) || !updatedAtMatches(item.UpdatedAt, updatedSince) {
				continue
			}
			projected := projectExportConstituent(item)
			if matchesConstituentExport(item, projected, filters) {
				mapped, mapErr := mapFrom(projected)
				if mapErr != nil {
					return nil, mapErr
				}
				if customFields, ok := item.Profile["custom_fields"].(map[string]any); ok {
					mapped["custom_fields"] = cloneMap(customFields)
				}
				rows = append(rows, dataExportRow{id: item.ID, item: mapped})
			}
		}
	case "service-requests":
		set, _, err := store.SearchCity311ServiceRequests(ctx, svc.store, composeTypes.City311ServiceRequestFilter{})
		if err != nil {
			return nil, err
		}
		for _, item := range set {
			if !canRead(actor, item) || !updatedAtMatches(item.UpdatedAt, updatedSince) || !matchesRequestExport(item, filters) {
				continue
			}
			mapped, mapErr := mapFrom(toContract(item))
			if mapErr != nil {
				return nil, mapErr
			}
			rows = append(rows, dataExportRow{id: item.ID, item: mapped})
		}
	case "audit-events", "follow-up-actions":
		set, _, err := store.SearchCity311AuditEvents(ctx, svc.store, composeTypes.City311AuditEventFilter{})
		if err != nil {
			return nil, err
		}
		requestCache := make(map[uint64]*composeTypes.City311ServiceRequest)
		for _, item := range set {
			allowed, scopeErr := svc.dataExportEventInScope(ctx, actor, entity, item, requestCache)
			if scopeErr != nil {
				return nil, scopeErr
			}
			if !allowed || !updatedAtMatches(item.CreatedAt, updatedSince) {
				continue
			}
			if entity == "audit-events" {
				if matchesAuditExport(item, filters) {
					mapped, mapErr := mapFrom(toAuditEvent(item))
					if mapErr != nil {
						return nil, mapErr
					}
					rows = append(rows, dataExportRow{id: item.ID, item: mapped})
				}
				continue
			}
			if item.RequestID == 0 {
				continue
			}
			projected := projectFollowUpAction(item)
			if matchesFollowUpExport(projected, filters) {
				mapped, mapErr := mapFrom(projected)
				if mapErr != nil {
					return nil, mapErr
				}
				rows = append(rows, dataExportRow{id: item.ID, item: mapped})
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].id < rows[j].id })
	return rows, nil
}

func (svc *Service) dataExportEventInScope(ctx context.Context, actor contract.Actor, entity string, event *composeTypes.City311AuditEvent, cache map[uint64]*composeTypes.City311ServiceRequest) (bool, error) {
	if entity == "audit-events" {
		return svc.auditEventInScope(ctx, svc.store, actor, event, cache)
	}
	if event.RequestID == 0 {
		return false, nil
	}
	request, cached := cache[event.RequestID]
	if !cached {
		var err error
		request, err = store.LookupCity311ServiceRequestByID(ctx, svc.store, event.RequestID)
		if err != nil {
			return false, err
		}
		cache[event.RequestID] = request
	}
	return canRead(actor, request), nil
}

func normalizeDataExportFilters(filters map[string][]string, allowed map[string]bool) (map[string][]string, error) {
	normalized := make(map[string][]string, len(filters))
	for key, values := range filters {
		key = strings.TrimSpace(key)
		if !allowed[key] || len(values) == 0 {
			return nil, invalidExportFilter("The filters are not valid for the selected entity.")
		}
		out := make([]string, 0, len(values))
		for _, value := range values {
			value = strings.TrimSpace(value)
			if value == "" || utf8.RuneCountInString(value) > 160 {
				return nil, invalidExportFilter("The filters are not valid for the selected entity.")
			}
			out = append(out, value)
		}
		normalized[key] = out
	}
	return normalized, nil
}

func validateDataExportFilters(entity string, filters map[string][]string, categorySets ...[]contract.ContactCategory) error {
	categories := contract.ContactCategories
	if len(categorySets) > 0 {
		categories = categorySets[0]
	}
	for key, values := range filters {
		switch key {
		case "request_id", "actor_id":
			for _, value := range values {
				id, err := strconv.ParseUint(value, 10, 64)
				if err != nil || id == 0 {
					return invalidExportFilter("An identifier filter is invalid.")
				}
			}
		case "status":
			if !validExportEnum(values, contract.ServiceRequestStatuses) {
				return invalidExportFilter("A status filter is invalid.")
			}
		case "service_type":
			if !validExportEnum(values, contract.ServiceTypes) {
				return invalidExportFilter("A service_type filter is invalid.")
			}
		case "department":
			if !validExportEnum(values, contract.DepartmentCodes) {
				return invalidExportFilter("A department filter is invalid.")
			}
		case "district":
			if !validExportEnum(values, contract.DistrictCodes) {
				return invalidExportFilter("A district filter is invalid.")
			}
		case "origin_class":
			if !validExportEnum(values, contract.OriginClasses) {
				return invalidExportFilter("An origin_class filter is invalid.")
			}
		case "source_channel":
			if !validExportEnum(values, contract.SourceChannels) {
				return invalidExportFilter("A source_channel filter is invalid.")
			}
		case "primary_category", "category":
			if !validExportEnum(values, categories) {
				return invalidExportFilter("A category filter is invalid.")
			}
		case "preferred_language":
			if !validExportEnum(values, contract.Languages) {
				return invalidExportFilter("A language filter is invalid.")
			}
		case "actor_type":
			if !validExportEnum(values, contract.AuditActorTypes) {
				return invalidExportFilter("An actor_type filter is invalid.")
			}
		case "email_opt_out":
			for _, value := range values {
				if value != "true" && value != "false" {
					return invalidExportFilter("The email_opt_out filter is invalid.")
				}
			}
		case "visibility":
			for _, value := range values {
				if value != "PUBLIC" && value != "STAFF" {
					return invalidExportFilter("A visibility filter is invalid.")
				}
			}
		}
	}
	_ = entity
	return nil
}

func validExportEnum[T ~string](values []string, allowed []T) bool {
	for _, value := range values {
		found := false
		for _, candidate := range allowed {
			if value == string(candidate) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func dataExportTokenBinding(entity string, filters map[string][]string, updatedSince *time.Time) (string, error) {
	payload := map[string]any{"entity": entity, "filters": filters}
	if updatedSince != nil {
		payload["updated_since"] = updatedSince.UTC().Format(time.RFC3339Nano)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "data-export:" + hex.EncodeToString(digest[:]), nil
}

func projectExportConstituent(item *composeTypes.City311Constituent) contract.Constituent {
	projected := contract.Constituent{}
	if encoded, err := json.Marshal(item.Profile); err == nil {
		_ = json.Unmarshal(encoded, &projected)
	}
	if projected.ConstituentID == "" {
		projected.ConstituentID = item.ConstituentID
	}
	if projected.Emails == nil {
		projected.Emails = []string{}
	}
	if projected.PhoneNumbers == nil {
		projected.PhoneNumbers = []contract.PhoneNumber{}
	}
	if projected.Addresses == nil {
		projected.Addresses = []contract.Address{}
	}
	return projected
}

func matchesConstituentExport(item *composeTypes.City311Constituent, projected contract.Constituent, filters map[string][]string) bool {
	return matchesString(projected.ConstituentID, filters["constituent_id"]) &&
		matchesString(string(item.OwningDepartment), filters["department"]) &&
		matchesString(string(item.CouncilDistrict), filters["district"]) &&
		matchesString(string(projected.PrimaryCategory), filters["primary_category"]) &&
		matchesString(string(projected.PreferredLanguage), filters["preferred_language"]) &&
		(len(filters["email_opt_out"]) == 0 || matchesString(strconv.FormatBool(projected.EmailOptOut), filters["email_opt_out"])) &&
		matchesCustomExportFilters(profileCustomFields(item.Profile), filters)
}

func matchesRequestExport(item *composeTypes.City311ServiceRequest, filters map[string][]string) bool {
	category := ""
	if raw, ok := item.PrimaryRequester["primary_category"].(string); ok {
		category = raw
	}
	return matchesString(strconv.FormatUint(item.ID, 10), filters["request_id"]) &&
		matchesString(item.RequestNumber, filters["request_number"]) &&
		matchesString(string(item.Status), filters["status"]) && matchesString(string(item.ServiceType), filters["service_type"]) &&
		matchesString(string(item.OwningDepartment), filters["department"]) && matchesString(string(item.CouncilDistrict), filters["district"]) &&
		matchesString(string(item.OriginClass), filters["origin_class"]) && matchesString(string(item.SourceChannel), filters["source_channel"]) &&
		matchesString(category, filters["category"]) && matchesString(item.DuplicateGroupID, filters["duplicate_group"]) &&
		matchesCustomExportFilters(item.CustomFields, filters)
}

func (svc *Service) exportCustomFieldDefinitions(ctx context.Context, entity string) (map[string]contract.CustomFieldDefinition, error) {
	definitionEntity := ""
	switch entity {
	case "constituents":
		definitionEntity = "constituent"
	case "service-requests":
		definitionEntity = "service_request"
	default:
		return map[string]contract.CustomFieldDefinition{}, nil
	}
	set, err := svc.customFieldDefinitions(ctx, svc.store, definitionEntity)
	if err != nil {
		return nil, err
	}
	out := make(map[string]contract.CustomFieldDefinition, len(set))
	for _, definition := range set {
		if definition.Active {
			out[definition.Key] = definition
		}
	}
	return out, nil
}

func profileCustomFields(profile map[string]any) map[string]any {
	if custom, ok := profile["custom_fields"].(map[string]any); ok {
		return custom
	}
	return map[string]any{}
}

func matchesCustomExportFilters(values map[string]any, filters map[string][]string) bool {
	for filter, expected := range filters {
		if !strings.HasPrefix(filter, "custom_fields.") {
			continue
		}
		key := strings.TrimPrefix(filter, "custom_fields.")
		value, found := values[key]
		if !found || !matchesCustomFieldValue(value, expected) {
			return false
		}
	}
	return true
}

func customFieldFilterValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.Itoa(typed)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	case []string:
		return strings.Join(typed, ",")
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			parts = append(parts, customFieldFilterValue(item))
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprint(value)
	}
}

func matchesAuditExport(item *composeTypes.City311AuditEvent, filters map[string][]string) bool {
	return matchesString(strconv.FormatUint(item.RequestID, 10), filters["request_id"]) &&
		matchesString(item.EntityType, filters["entity_type"]) && matchesString(item.EntityID, filters["entity_id"]) &&
		matchesString(item.EventType, filters["event_type"]) && matchesString(string(item.ActorType), filters["actor_type"]) &&
		matchesString(strconv.FormatUint(item.ActorID, 10), filters["actor_id"]) && matchesString(string(item.SourceChannel), filters["source_channel"])
}

func projectFollowUpAction(item *composeTypes.City311AuditEvent) contract.FollowUpAction {
	visibility := "STAFF"
	if public, ok := item.After["portal_visible"].(bool); ok && public {
		visibility = "PUBLIC"
	}
	local := item.CreatedAt.UTC()
	if location, err := time.LoadLocation("America/New_York"); err == nil {
		local = item.CreatedAt.In(location)
	}
	return contract.FollowUpAction{
		ActionType: item.EventType, Actor: fmt.Sprintf("%s:%d", item.ActorType, item.ActorID),
		OccurredAt: item.CreatedAt.UTC(), LocalDisplayTime: local.Format(time.RFC3339),
		RequestID: strconv.FormatUint(item.RequestID, 10), Visibility: visibility,
		Payload: map[string]any{"before": cloneMap(item.Before), "after": cloneMap(item.After)},
	}
}

func matchesFollowUpExport(item contract.FollowUpAction, filters map[string][]string) bool {
	return matchesString(item.RequestID, filters["request_id"]) && matchesString(item.ActionType, filters["action_type"]) &&
		matchesString(item.Actor, filters["actor"]) && matchesString(item.Visibility, filters["visibility"])
}

func updatedAtMatches(value time.Time, since *time.Time) bool {
	return since == nil || !value.Before(since.UTC())
}

func (svc *Service) recordDataExport(ctx context.Context, actor contract.Actor, entity string, filters map[string][]string, itemCount int) error {
	return store.CreateCity311AuditEvent(ctx, svc.store, &composeTypes.City311AuditEvent{
		ID: svc.nextID(), EntityType: "data_export", EntityID: entity, EventType: dataExportAuditEvent,
		ActorType: contract.AuditActorIntegrationClient, ActorID: actor.ID, SourceChannel: contract.SourceChannelAPI,
		Before: composeTypes.City311JSON{}, After: composeTypes.City311JSON{
			"department_code": optionalActorDepartment(actor), "entity": entity, "filters": filters, "item_count": itemCount,
		}, CreatedAt: svc.now().UTC(),
	})
}

func invalidExportFilter(message string) *ServiceError {
	return &ServiceError{Status: http.StatusUnprocessableEntity, Payload: contract.APIError{
		Error: contract.ErrorInvalidFilter, Message: message, Retryable: false,
	}}
}

func invalidPageToken() *ServiceError {
	return &ServiceError{Status: http.StatusBadRequest, Payload: contract.APIError{
		Error: contract.ErrorInvalidPageToken, Message: "The page token is invalid.", Retryable: false,
	}}
}
