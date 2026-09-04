package city311

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
)

const (
	configurationContactCategory = "CONTACT_CATEGORY"
	configurationCustomField     = "CUSTOM_FIELD"
	configurationListSort        = "code"
	customFieldListSort          = "key"
)

var initialContactCategoryLabels = map[string]string{
	"RESIDENT":                 "Resident",
	"BUSINESS":                 "Business",
	"BUSINESS_OWNER":           "Business owner",
	"VETERAN":                  "Veteran",
	"NEIGHBORHOOD_ASSOCIATION": "Neighborhood association",
	"GOVERNMENT":               "Government",
	"OTHER":                    "Other",
}

type ConfigurationListQuery struct {
	PageSize  uint
	PageToken string
}

func (svc *Service) seedConfigurationDefinitions(ctx context.Context, tx store.Storer, createdAt time.Time) error {
	for _, category := range contract.ContactCategories {
		code := string(category)
		payload := composeTypes.City311JSON{
			"code": code, "active": true, "labels": map[string]string{"EN": initialContactCategoryLabels[code]},
		}
		if err := svc.ensureInitialRevision(ctx, tx, configurationContactCategory, code, "", payload, createdAt); err != nil {
			return err
		}
	}
	return nil
}

func (svc *Service) ListContactCategories(ctx context.Context, actor contract.Actor, query ConfigurationListQuery) (*contract.CategoryList, error) {
	if !canManageContactCategories(actor) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, "A department manager or platform administrator role is required.")
	}
	revisions, err := svc.latestConfigurationResources(ctx, svc.store, configurationContactCategory)
	if err != nil {
		return nil, err
	}
	sort.Slice(revisions, func(i, j int) bool { return revisions[i].ResourceKey < revisions[j].ResourceKey })
	start, end, next, err := configurationPage(query, len(revisions), configurationListSort)
	if err != nil {
		return nil, err
	}
	response := &contract.CategoryList{
		Items: make([]contract.Category, 0, end-start), NextPageToken: next, TotalCount: len(revisions),
		AppliedFilters: map[string]any{}, Sort: []string{configurationListSort},
	}
	for _, revision := range revisions[start:end] {
		response.Items = append(response.Items, *categoryFromRevision(revision))
	}
	return response, nil
}

func (svc *Service) CreateContactCategory(ctx context.Context, actor contract.Actor, input contract.CategoryWrite) (*contract.Category, error) {
	if !canManageContactCategories(actor) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, "A department manager or platform administrator role is required.")
	}
	input.Code = strings.TrimSpace(input.Code)
	if err := validateCategoryWrite(input); err != nil {
		return nil, err
	}
	svc.presentationMu.Lock()
	defer svc.presentationMu.Unlock()
	revisions, err := svc.configurationRevisions(ctx, svc.store, configurationContactCategory, input.Code)
	if err != nil {
		return nil, err
	}
	if len(revisions) > 0 {
		return nil, validationError(contract.FieldError{Field: "/code", Code: contract.ValidationDuplicate})
	}
	revision, err := svc.createInitialConfigurationResource(ctx, actor, configurationContactCategory, input.Code, input, "CONTACT_CATEGORY_CREATED")
	if err != nil {
		return nil, err
	}
	return categoryFromRevision(revision), nil
}

func (svc *Service) UpdateContactCategory(ctx context.Context, actor contract.Actor, code string, input contract.CategoryWrite) (*contract.Category, error) {
	if !canManageContactCategories(actor) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, "A department manager or platform administrator role is required.")
	}
	code = strings.TrimSpace(code)
	input.Code = strings.TrimSpace(input.Code)
	if !validCategoryCode(code) {
		return nil, apiError(http.StatusNotFound, contract.ErrorNotFound, "The contact category was not found.")
	}
	if input.Code != code {
		return nil, validationError(contract.FieldError{Field: "/code", Code: contract.ValidationConflict})
	}
	if err := validateCategoryWrite(input); err != nil {
		return nil, err
	}
	svc.presentationMu.Lock()
	defer svc.presentationMu.Unlock()
	current, err := svc.latestConfigurationRevision(ctx, svc.store, configurationContactCategory, code, "", false)
	if err != nil {
		return nil, apiError(http.StatusNotFound, contract.ErrorNotFound, "The contact category was not found.")
	}
	revision, err := svc.createConfigurationRevision(ctx, actor, current, configurationContactCategory, code, "", input, true, "CONTACT_CATEGORY_UPDATED")
	if err != nil {
		return nil, err
	}
	return categoryFromRevision(revision), nil
}

func (svc *Service) ListCustomFields(ctx context.Context, actor contract.Actor, query ConfigurationListQuery) (*contract.CustomFieldDefinitionList, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	revisions, err := svc.latestConfigurationResources(ctx, svc.store, configurationCustomField)
	if err != nil {
		return nil, err
	}
	sort.Slice(revisions, func(i, j int) bool { return revisions[i].ResourceKey < revisions[j].ResourceKey })
	start, end, next, err := configurationPage(query, len(revisions), customFieldListSort)
	if err != nil {
		return nil, err
	}
	response := &contract.CustomFieldDefinitionList{
		Items: make([]contract.CustomFieldDefinition, 0, end-start), NextPageToken: next, TotalCount: len(revisions),
		AppliedFilters: map[string]any{}, Sort: []string{customFieldListSort},
	}
	for _, revision := range revisions[start:end] {
		response.Items = append(response.Items, *customFieldFromRevision(revision))
	}
	return response, nil
}

func (svc *Service) CreateCustomField(ctx context.Context, actor contract.Actor, input contract.CustomFieldDefinition) (*contract.CustomFieldDefinition, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	input.Key = strings.TrimSpace(input.Key)
	if err := validateCustomFieldDefinition(input); err != nil {
		return nil, err
	}
	svc.presentationMu.Lock()
	defer svc.presentationMu.Unlock()
	revisions, err := svc.configurationRevisions(ctx, svc.store, configurationCustomField, input.Key)
	if err != nil {
		return nil, err
	}
	if len(revisions) > 0 {
		return nil, validationError(contract.FieldError{Field: "/key", Code: contract.ValidationDuplicate})
	}
	revision, err := svc.createInitialConfigurationResource(ctx, actor, configurationCustomField, input.Key, input, "CUSTOM_FIELD_CREATED")
	if err != nil {
		return nil, err
	}
	return customFieldFromRevision(revision), nil
}

func (svc *Service) UpdateCustomField(ctx context.Context, actor contract.Actor, key string, expectedVersion uint64, input contract.CustomFieldDefinition) (*contract.CustomFieldDefinition, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	if expectedVersion == 0 {
		return nil, expectedVersionRequired()
	}
	key = strings.TrimSpace(key)
	input.Key = strings.TrimSpace(input.Key)
	if !validResourceKey(key) {
		return nil, apiError(http.StatusNotFound, contract.ErrorNotFound, "The custom-field definition was not found.")
	}
	if input.Key != key {
		return nil, validationError(contract.FieldError{Field: "/key", Code: contract.ValidationConflict})
	}
	if err := validateCustomFieldDefinition(input); err != nil {
		return nil, err
	}
	svc.presentationMu.Lock()
	defer svc.presentationMu.Unlock()
	current, err := svc.latestConfigurationRevision(ctx, svc.store, configurationCustomField, key, "", false)
	if err != nil {
		return nil, apiError(http.StatusNotFound, contract.ErrorNotFound, "The custom-field definition was not found.")
	}
	if uint64(current.Version) != expectedVersion {
		return nil, versionConflict(current.Version)
	}
	revision, err := svc.createConfigurationRevision(ctx, actor, current, configurationCustomField, key, "", input, true, "CUSTOM_FIELD_UPDATED")
	if err != nil {
		return nil, err
	}
	return customFieldFromRevision(revision), nil
}

func (svc *Service) createInitialConfigurationResource(ctx context.Context, actor contract.Actor, resourceType, resourceKey string, value any, eventType string) (*composeTypes.City311ConfigurationRevision, error) {
	payload, err := mapFrom(value)
	if err != nil {
		return nil, err
	}
	delete(payload, "version")
	delete(payload, "updated_at")
	now := svc.now().UTC()
	revision := &composeTypes.City311ConfigurationRevision{
		ID: svc.nextID(), ResourceType: resourceType, ResourceKey: resourceKey, Payload: payload,
		Version: 1, Published: true, CreatedAt: now,
	}
	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		if err := store.CreateCity311ConfigurationRevision(ctx, tx, revision); err != nil {
			return err
		}
		return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
			ID: svc.nextID(), EntityType: strings.ToLower(resourceType), EntityID: resourceKey, EventType: eventType,
			ActorType: contract.AuditActorStaff, ActorID: actor.ID, SourceChannel: contract.SourceChannelStaffInPerson,
			Before: composeTypes.City311JSON{}, After: cloneMap(payload), CreatedAt: now,
		})
	})
	return revision, err
}

func (svc *Service) latestConfigurationResources(ctx context.Context, st store.Storer, resourceType string) (composeTypes.City311ConfigurationRevisionSet, error) {
	set, _, err := store.SearchCity311ConfigurationRevisions(ctx, st, composeTypes.City311ConfigurationRevisionFilter{ResourceType: resourceType})
	if err != nil {
		return nil, err
	}
	latest := make(map[string]*composeTypes.City311ConfigurationRevision)
	for _, revision := range set {
		if revision.ResourceType != resourceType {
			continue
		}
		current := latest[revision.ResourceKey]
		if current == nil || revision.Version > current.Version {
			latest[revision.ResourceKey] = revision
		}
	}
	out := make(composeTypes.City311ConfigurationRevisionSet, 0, len(latest))
	for _, revision := range latest {
		out = append(out, revision)
	}
	return out, nil
}

func configurationPage(query ConfigurationListQuery, total int, sortBinding string) (int, int, *string, error) {
	if query.PageSize == 0 {
		query.PageSize = 50
	}
	if query.PageSize > 100 {
		return 0, 0, nil, validationError(contract.FieldError{Field: "/query/page_size", Code: contract.ValidationOutOfRange})
	}
	offset, err := decodePageToken(query.PageToken, []string{sortBinding})
	if err != nil || offset > total {
		return 0, 0, nil, invalidPageToken()
	}
	end := offset + int(query.PageSize)
	if end > total {
		end = total
	}
	var next *string
	if end < total {
		value, tokenErr := encodePageToken(end, []string{sortBinding})
		if tokenErr != nil {
			return 0, 0, nil, tokenErr
		}
		next = &value
	}
	return offset, end, next, nil
}

func categoryFromRevision(revision *composeTypes.City311ConfigurationRevision) *contract.Category {
	value := &contract.Category{Code: revision.ResourceKey}
	decodeConfigurationPayload(revision.Payload, value)
	value.Code = revision.ResourceKey
	value.Version = uint64(revision.Version)
	value.UpdatedAt = revision.CreatedAt
	if value.Labels == nil {
		value.Labels = map[string]string{}
	}
	return value
}

func customFieldFromRevision(revision *composeTypes.City311ConfigurationRevision) *contract.CustomFieldDefinition {
	value := &contract.CustomFieldDefinition{Key: revision.ResourceKey}
	decodeConfigurationPayload(revision.Payload, value)
	value.Key = revision.ResourceKey
	value.Version = uint64(revision.Version)
	value.UpdatedAt = revision.CreatedAt
	if value.Labels == nil {
		value.Labels = map[string]string{}
	}
	if value.Validation == nil {
		value.Validation = map[string]any{}
	}
	if value.ChoiceValues == nil {
		value.ChoiceValues = []string{}
	}
	return value
}

func validateCategoryWrite(input contract.CategoryWrite) error {
	var fields []contract.FieldError
	if !validCategoryCode(input.Code) {
		fields = append(fields, contract.FieldError{Field: "/code", Code: contract.ValidationInvalidFormat})
	}
	fields = append(fields, validateLocalizedLabels(input.Labels, "/labels")...)
	if len(fields) > 0 {
		return validationError(fields...)
	}
	return nil
}

func validCategoryCode(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

func validateLocalizedLabels(labels map[string]string, field string) []contract.FieldError {
	if strings.TrimSpace(labels["EN"]) == "" {
		return []contract.FieldError{{Field: field + "/EN", Code: contract.ValidationRequired}}
	}
	for language, label := range labels {
		if language != "EN" && language != "ES" && language != "VI" {
			return []contract.FieldError{{Field: field + "/" + language, Code: contract.ValidationInvalidValue}}
		}
		length := utf8.RuneCountInString(strings.TrimSpace(label))
		if length == 0 || length > 120 {
			return []contract.FieldError{{Field: field + "/" + language, Code: contract.ValidationInvalidValue}}
		}
	}
	return nil
}

func validateCustomFieldDefinition(input contract.CustomFieldDefinition) error {
	var fields []contract.FieldError
	if !validResourceKey(input.Key) {
		fields = append(fields, contract.FieldError{Field: "/key", Code: contract.ValidationInvalidFormat})
	}
	fields = append(fields, validateLocalizedLabels(input.Labels, "/labels")...)
	if input.Entity != "constituent" && input.Entity != "service_request" {
		fields = append(fields, contract.FieldError{Field: "/entity", Code: contract.ValidationInvalidValue})
	}
	if !containsEnums([]contract.CustomFieldType{input.FieldType}, contract.CustomFieldTypes) {
		fields = append(fields, contract.FieldError{Field: "/field_type", Code: contract.ValidationInvalidValue})
	}
	fields = append(fields, validateCustomFieldVocabulary(input)...)
	fields = append(fields, validateCustomFieldRules(input)...)
	if input.Default != nil {
		if code := validateCustomFieldValue(input, input.Default); code != "" {
			fields = append(fields, contract.FieldError{Field: "/default", Code: code})
		}
	}
	if len(fields) > 0 {
		return validationError(fields...)
	}
	return nil
}

func validateCustomFieldVocabulary(input contract.CustomFieldDefinition) []contract.FieldError {
	choice := input.FieldType == contract.CustomFieldTypeSingleChoice || input.FieldType == contract.CustomFieldTypeMultiChoice
	if !choice && len(input.ChoiceValues) > 0 {
		return []contract.FieldError{{Field: "/choice_values", Code: contract.ValidationInvalidValue}}
	}
	if choice && len(input.ChoiceValues) == 0 {
		return []contract.FieldError{{Field: "/choice_values", Code: contract.ValidationRequired}}
	}
	seen := map[string]bool{}
	for index, value := range input.ChoiceValues {
		value = strings.TrimSpace(value)
		if value == "" || utf8.RuneCountInString(value) > 120 {
			return []contract.FieldError{{Field: fmt.Sprintf("/choice_values/%d", index), Code: contract.ValidationInvalidValue}}
		}
		if seen[value] {
			return []contract.FieldError{{Field: fmt.Sprintf("/choice_values/%d", index), Code: contract.ValidationDuplicate}}
		}
		seen[value] = true
	}
	return nil
}

func validateCustomFieldRules(input contract.CustomFieldDefinition) []contract.FieldError {
	allowed := map[string]bool{}
	switch input.FieldType {
	case contract.CustomFieldTypeText:
		allowed["min_length"], allowed["max_length"] = true, true
	case contract.CustomFieldTypeInteger, contract.CustomFieldTypeDecimal:
		allowed["minimum"], allowed["maximum"] = true, true
	case contract.CustomFieldTypeDate, contract.CustomFieldTypeDateTime:
		allowed["minimum"], allowed["maximum"] = true, true
	}
	for key := range input.Validation {
		if !allowed[key] {
			return []contract.FieldError{{Field: "/validation/" + key, Code: contract.ValidationInvalidValue}}
		}
	}
	if input.FieldType == contract.CustomFieldTypeText {
		minimum, minOK := integerValue(input.Validation["min_length"])
		maximum, maxOK := integerValue(input.Validation["max_length"])
		if input.Validation["min_length"] != nil && (!minOK || minimum < 0) {
			return []contract.FieldError{{Field: "/validation/min_length", Code: contract.ValidationOutOfRange}}
		}
		if input.Validation["max_length"] != nil && (!maxOK || maximum < 1) {
			return []contract.FieldError{{Field: "/validation/max_length", Code: contract.ValidationOutOfRange}}
		}
		if minOK && maxOK && minimum > maximum {
			return []contract.FieldError{{Field: "/validation/min_length", Code: contract.ValidationOutOfRange}}
		}
	}
	if input.FieldType == contract.CustomFieldTypeInteger || input.FieldType == contract.CustomFieldTypeDecimal {
		minimum, minOK := numberValue(input.Validation["minimum"])
		maximum, maxOK := numberValue(input.Validation["maximum"])
		if input.Validation["minimum"] != nil && !minOK {
			return []contract.FieldError{{Field: "/validation/minimum", Code: contract.ValidationInvalidValue}}
		}
		if input.Validation["maximum"] != nil && !maxOK {
			return []contract.FieldError{{Field: "/validation/maximum", Code: contract.ValidationInvalidValue}}
		}
		if minOK && maxOK && minimum > maximum {
			return []contract.FieldError{{Field: "/validation/minimum", Code: contract.ValidationOutOfRange}}
		}
	}
	if input.FieldType == contract.CustomFieldTypeDate || input.FieldType == contract.CustomFieldTypeDateTime {
		minimum, minOK := customFieldTime(input.FieldType, input.Validation["minimum"])
		maximum, maxOK := customFieldTime(input.FieldType, input.Validation["maximum"])
		if input.Validation["minimum"] != nil && !minOK {
			return []contract.FieldError{{Field: "/validation/minimum", Code: contract.ValidationInvalidFormat}}
		}
		if input.Validation["maximum"] != nil && !maxOK {
			return []contract.FieldError{{Field: "/validation/maximum", Code: contract.ValidationInvalidFormat}}
		}
		if minOK && maxOK && minimum.After(maximum) {
			return []contract.FieldError{{Field: "/validation/minimum", Code: contract.ValidationOutOfRange}}
		}
	}
	return nil
}

func validateCustomFieldValue(definition contract.CustomFieldDefinition, value any) contract.ValidationCode {
	switch definition.FieldType {
	case contract.CustomFieldTypeText:
		text, ok := value.(string)
		if !ok {
			return contract.ValidationInvalidValue
		}
		length := utf8.RuneCountInString(text)
		if minimum, ok := integerValue(definition.Validation["min_length"]); ok && length < minimum {
			return contract.ValidationTooShort
		}
		if maximum, ok := integerValue(definition.Validation["max_length"]); ok && length > maximum {
			return contract.ValidationTooLong
		}
	case contract.CustomFieldTypeInteger:
		number, ok := numberValue(value)
		if !ok || math.Trunc(number) != number {
			return contract.ValidationInvalidValue
		}
		if !customFieldNumberInRange(definition, number) {
			return contract.ValidationOutOfRange
		}
	case contract.CustomFieldTypeDecimal:
		number, ok := numberValue(value)
		if !ok || !customFieldNumberInRange(definition, number) {
			if !ok {
				return contract.ValidationInvalidValue
			}
			return contract.ValidationOutOfRange
		}
	case contract.CustomFieldTypeDate, contract.CustomFieldTypeDateTime:
		parsed, ok := customFieldTime(definition.FieldType, value)
		if !ok {
			return contract.ValidationInvalidFormat
		}
		if minimum, ok := customFieldTime(definition.FieldType, definition.Validation["minimum"]); ok && parsed.Before(minimum) {
			return contract.ValidationOutOfRange
		}
		if maximum, ok := customFieldTime(definition.FieldType, definition.Validation["maximum"]); ok && parsed.After(maximum) {
			return contract.ValidationOutOfRange
		}
	case contract.CustomFieldTypeBoolean:
		if _, ok := value.(bool); !ok {
			return contract.ValidationInvalidValue
		}
	case contract.CustomFieldTypeSingleChoice:
		choice, ok := value.(string)
		if !ok || !containsString(definition.ChoiceValues, choice) {
			return contract.ValidationInvalidValue
		}
	case contract.CustomFieldTypeMultiChoice:
		values, ok := stringSliceValue(value)
		if !ok {
			return contract.ValidationInvalidValue
		}
		seen := map[string]bool{}
		for _, choice := range values {
			if seen[choice] || !containsString(definition.ChoiceValues, choice) {
				return contract.ValidationInvalidValue
			}
			seen[choice] = true
		}
	default:
		return contract.ValidationInvalidValue
	}
	return ""
}

func (svc *Service) validateAndDefaultCustomFields(ctx context.Context, st store.Storer, entity string, values map[string]any) (map[string]any, error) {
	definitions, err := svc.customFieldDefinitions(ctx, st, entity)
	if err != nil {
		return nil, err
	}
	out := cloneMap(values)
	if out == nil {
		out = map[string]any{}
	}
	if len(definitions) == 0 {
		return out, nil
	}
	byKey := make(map[string]contract.CustomFieldDefinition, len(definitions))
	for _, definition := range definitions {
		byKey[definition.Key] = definition
	}
	var fields []contract.FieldError
	for key, value := range out {
		definition, found := byKey[key]
		if !found {
			fields = append(fields, contract.FieldError{Field: "/custom_fields/" + key, Code: contract.ValidationInvalidValue})
			continue
		}
		if !definition.Active {
			fields = append(fields, contract.FieldError{Field: "/custom_fields/" + key, Code: contract.ValidationInactiveValue})
			continue
		}
		if code := validateCustomFieldValue(definition, value); code != "" {
			fields = append(fields, contract.FieldError{Field: "/custom_fields/" + key, Code: code})
		}
	}
	for _, definition := range definitions {
		if !definition.Active {
			continue
		}
		if _, present := out[definition.Key]; !present && definition.Default != nil {
			out[definition.Key] = definition.Default
		}
		if _, present := out[definition.Key]; definition.Required && !present {
			fields = append(fields, contract.FieldError{Field: "/custom_fields/" + definition.Key, Code: contract.ValidationRequired})
		}
	}
	if len(fields) > 0 {
		sort.SliceStable(fields, func(i, j int) bool { return fields[i].Field < fields[j].Field })
		return nil, validationError(fields...)
	}
	return out, nil
}

func (svc *Service) customFieldDefinitions(ctx context.Context, st store.Storer, entity string) ([]contract.CustomFieldDefinition, error) {
	revisions, err := svc.latestConfigurationResources(ctx, st, configurationCustomField)
	if err != nil {
		return nil, err
	}
	out := make([]contract.CustomFieldDefinition, 0, len(revisions))
	for _, revision := range revisions {
		definition := customFieldFromRevision(revision)
		if entity == "" || definition.Entity == entity {
			out = append(out, *definition)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (svc *Service) activeContactCategories(ctx context.Context, st store.Storer) ([]contract.ContactCategory, error) {
	revisions, err := svc.latestConfigurationResources(ctx, st, configurationContactCategory)
	if err != nil {
		return nil, err
	}
	out := make([]contract.ContactCategory, 0, len(revisions))
	for _, revision := range revisions {
		category := categoryFromRevision(revision)
		if category.Active {
			out = append(out, contract.ContactCategory(category.Code))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func (svc *Service) validateRequestCustomFieldFilters(ctx context.Context, filters map[string][]string) error {
	if len(filters) == 0 {
		return nil
	}
	definitions, err := svc.customFieldDefinitions(ctx, svc.store, "service_request")
	if err != nil {
		return err
	}
	active := make(map[string]bool, len(definitions))
	for _, definition := range definitions {
		active[definition.Key] = definition.Active
	}
	for key, values := range filters {
		if !active[key] || len(values) == 0 {
			return validationError(contract.FieldError{Field: "/query/filters/custom_fields/" + key, Code: contract.ValidationInvalidValue})
		}
		for _, value := range values {
			if strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > 160 {
				return validationError(contract.FieldError{Field: "/query/filters/custom_fields/" + key, Code: contract.ValidationInvalidValue})
			}
		}
	}
	return nil
}

func matchesCustomFields(values map[string]any, filters map[string][]string) bool {
	for key, expected := range filters {
		value, found := values[key]
		if !found || !matchesCustomFieldValue(value, expected) {
			return false
		}
	}
	return true
}

func matchesCustomFieldValue(value any, expected []string) bool {
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			if matchesString(item, expected) {
				return true
			}
		}
		return false
	case []any:
		for _, item := range typed {
			if matchesString(customFieldFilterValue(item), expected) {
				return true
			}
		}
		return false
	default:
		return matchesString(customFieldFilterValue(value), expected)
	}
}

func canManageContactCategories(actor contract.Actor) bool {
	return hasRole(actor, contract.ApplicationRoleDepartmentManager) || hasRole(actor, contract.ApplicationRolePlatformAdministrator)
}

func customFieldNumberInRange(definition contract.CustomFieldDefinition, value float64) bool {
	if minimum, ok := numberValue(definition.Validation["minimum"]); ok && value < minimum {
		return false
	}
	if maximum, ok := numberValue(definition.Validation["maximum"]); ok && value > maximum {
		return false
	}
	return true
}

func integerValue(value any) (int, bool) {
	number, ok := numberValue(value)
	if !ok || math.Trunc(number) != number || number > math.MaxInt || number < math.MinInt {
		return 0, false
	}
	return int(number), true
}

func numberValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, !math.IsNaN(typed) && !math.IsInf(typed, 0)
	default:
		return 0, false
	}
}

func customFieldTime(fieldType contract.CustomFieldType, value any) (time.Time, bool) {
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) != text || text == "" {
		return time.Time{}, false
	}
	layout := time.RFC3339
	if fieldType == contract.CustomFieldTypeDate {
		layout = "2006-01-02"
	}
	parsed, err := time.Parse(layout, text)
	return parsed, err == nil
}

func stringSliceValue(value any) ([]string, bool) {
	switch typed := value.(type) {
	case []string:
		return typed, true
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, text)
		}
		return out, true
	default:
		return nil, false
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
