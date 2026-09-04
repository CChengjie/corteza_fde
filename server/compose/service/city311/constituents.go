package city311

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
)

var constituentSearchFilters = map[string]bool{
	"constituent_id": true, "query": true, "display_name": true, "email": true,
	"phone": true, "primary_category": true, "preferred_language": true,
	"email_opt_out": true, "department": true, "district": true,
}

type ConstituentSearchQuery struct {
	Filters   map[string][]string
	PageSize  uint
	PageToken string
	Sort      string
}

func (svc *Service) SearchConstituents(ctx context.Context, actor contract.Actor, query ConstituentSearchQuery) (*contract.ConstituentList, error) {
	if !isStaff(actor) {
		return nil, apiError(403, contract.ErrorForbidden, "A staff role is required.")
	}
	if query.PageSize == 0 {
		query.PageSize = 50
	}
	if query.PageSize > 100 {
		return nil, validationError(contract.FieldError{Field: "/query/page_size", Code: contract.ValidationOutOfRange})
	}
	customDefinitions, err := svc.exportCustomFieldDefinitions(ctx, "constituents")
	if err != nil {
		return nil, err
	}
	filters, err := normalizeConstituentFilters(query.Filters, customDefinitions)
	if err != nil {
		return nil, err
	}
	activeCategories, err := svc.activeContactCategories(ctx, svc.store)
	if err != nil {
		return nil, err
	}
	if err = validateConstituentFilters(filters, activeCategories); err != nil {
		return nil, err
	}
	publishedSort, err := normalizeConstituentSort(query.Sort)
	if err != nil {
		return nil, validationError(contract.FieldError{Field: "/query/sort", Code: contract.ValidationInvalidFormat})
	}
	binding, err := constituentSearchTokenBinding(filters, publishedSort)
	if err != nil {
		return nil, err
	}
	offset, err := decodePageToken(query.PageToken, []string{binding})
	if err != nil {
		return nil, invalidPageToken()
	}

	set, _, err := store.SearchCity311Constituents(ctx, svc.store, composeTypes.City311ConstituentFilter{})
	if err != nil {
		return nil, err
	}
	matching := make(composeTypes.City311ConstituentSet, 0, len(set))
	for _, item := range set {
		if canReadConstituent(actor, item) && matchesConstituentSearch(item, filters) {
			matching = append(matching, item)
		}
	}
	sortConstituents(matching, publishedSort)
	if offset > len(matching) {
		return nil, invalidPageToken()
	}
	end := offset + int(query.PageSize)
	if end > len(matching) {
		end = len(matching)
	}
	response := &contract.ConstituentList{
		Items: make([]contract.Constituent, 0, end-offset), TotalCount: len(matching),
		AppliedFilters: constituentAppliedFilters(filters), Sort: publishedSort,
	}
	for _, item := range matching[offset:end] {
		response.Items = append(response.Items, projectExportConstituent(item))
	}
	if end < len(matching) {
		token, tokenErr := encodePageToken(end, []string{binding})
		if tokenErr != nil {
			return nil, tokenErr
		}
		response.NextPageToken = &token
	}
	return response, nil
}

func (svc *Service) GetStaffConstituent(ctx context.Context, actor contract.Actor, constituentID string) (*contract.Constituent, error) {
	item, err := svc.ResolveConstituent(ctx, actor, constituentID)
	if err != nil {
		return nil, err
	}
	result := projectExportConstituent(item)
	return &result, nil
}

func normalizeConstituentFilters(input map[string][]string, custom map[string]contract.CustomFieldDefinition) (map[string][]string, error) {
	out := make(map[string][]string, len(input))
	for rawKey, rawValues := range input {
		key := strings.TrimSpace(rawKey)
		allowed := constituentSearchFilters[key]
		if strings.HasPrefix(key, "custom_fields.") {
			_, allowed = custom[strings.TrimPrefix(key, "custom_fields.")]
		}
		if !allowed {
			return nil, invalidConstituentFilter(key, contract.ValidationInvalidValue)
		}
		if len(rawValues) == 0 {
			return nil, invalidConstituentFilter(key, contract.ValidationRequired)
		}
		seen := map[string]bool{}
		for _, rawValue := range rawValues {
			value := strings.TrimSpace(rawValue)
			if value == "" || utf8.RuneCountInString(value) > 254 {
				return nil, invalidConstituentFilter(key, contract.ValidationInvalidValue)
			}
			if !seen[value] {
				out[key] = append(out[key], value)
				seen[value] = true
			}
		}
	}
	return out, nil
}

func validateConstituentFilters(filters map[string][]string, categories []contract.ContactCategory) error {
	if values := filters["primary_category"]; len(values) > 0 && !containsEnums(constituentEnums(values, contract.ContactCategory("")), categories) {
		return invalidConstituentFilter("primary_category", contract.ValidationInvalidValue)
	}
	if values := filters["preferred_language"]; len(values) > 0 && !containsEnums(constituentEnums(values, contract.Language("")), contract.Languages) {
		return invalidConstituentFilter("preferred_language", contract.ValidationInvalidValue)
	}
	if values := filters["department"]; len(values) > 0 && !containsEnums(constituentEnums(values, contract.DepartmentCode("")), contract.DepartmentCodes) {
		return invalidConstituentFilter("department", contract.ValidationInvalidValue)
	}
	if values := filters["district"]; len(values) > 0 && !containsEnums(constituentEnums(values, contract.DistrictCode("")), contract.DistrictCodes) {
		return invalidConstituentFilter("district", contract.ValidationInvalidValue)
	}
	for _, value := range filters["email_opt_out"] {
		if _, err := strconv.ParseBool(value); err != nil {
			return invalidConstituentFilter("email_opt_out", contract.ValidationInvalidFormat)
		}
	}
	return nil
}

func constituentEnums[T ~string](values []string, _ T) []T {
	out := make([]T, 0, len(values))
	for _, value := range values {
		out = append(out, T(value))
	}
	return out
}

func invalidConstituentFilter(key string, code contract.ValidationCode) error {
	if key == "" {
		key = "filters"
	}
	return validationError(contract.FieldError{Field: "/query/filters/" + key, Code: code})
}

func matchesConstituentSearch(item *composeTypes.City311Constituent, filters map[string][]string) bool {
	profile := projectExportConstituent(item)
	if !matchesString(profile.ConstituentID, filters["constituent_id"]) ||
		!matchesString(string(profile.PrimaryCategory), filters["primary_category"]) ||
		!matchesString(string(profile.PreferredLanguage), filters["preferred_language"]) ||
		!matchesString(string(item.OwningDepartment), filters["department"]) ||
		!matchesString(string(item.CouncilDistrict), filters["district"]) ||
		!matchesBool(profile.EmailOptOut, filters["email_opt_out"]) ||
		!matchesFoldContains(profile.DisplayName, filters["display_name"]) ||
		!matchesAnyFoldContains(profile.Emails, filters["email"]) ||
		!matchesPhoneSearch(profile.PhoneNumbers, filters["phone"]) ||
		!matchesCustomExportFilters(profile.CustomFields, filters) {
		return false
	}
	for _, query := range filters["query"] {
		if !strings.Contains(strings.ToLower(profile.ConstituentID+" "+profile.DisplayName+" "+strings.Join(profile.Emails, " ")+" "+phoneSearchText(profile.PhoneNumbers)), strings.ToLower(query)) {
			return false
		}
	}
	return true
}

func matchesBool(value bool, expected []string) bool {
	return len(expected) == 0 || matchesString(strconv.FormatBool(value), expected)
}

func matchesFoldContains(value string, expected []string) bool {
	if len(expected) == 0 {
		return true
	}
	for _, candidate := range expected {
		if strings.Contains(strings.ToLower(value), strings.ToLower(candidate)) {
			return true
		}
	}
	return false
}

func matchesAnyFoldContains(values, expected []string) bool {
	if len(expected) == 0 {
		return true
	}
	for _, candidate := range expected {
		for _, value := range values {
			if strings.Contains(strings.ToLower(value), strings.ToLower(candidate)) {
				return true
			}
		}
	}
	return false
}

func matchesPhoneSearch(values []contract.PhoneNumber, expected []string) bool {
	if len(expected) == 0 {
		return true
	}
	for _, candidate := range expected {
		for _, value := range values {
			if strings.Contains(strings.ToLower(string(value.Label)+" "+value.Value), strings.ToLower(candidate)) {
				return true
			}
		}
	}
	return false
}

func phoneSearchText(values []contract.PhoneNumber) string {
	parts := make([]string, 0, len(values)*2)
	for _, value := range values {
		parts = append(parts, string(value.Label), value.Value)
	}
	return strings.Join(parts, " ")
}

func normalizeConstituentSort(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = "display_name,constituent_id"
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 3 {
		return nil, fmt.Errorf("too many sort fields")
	}
	allowed := map[string]bool{
		"constituent_id": true, "display_name": true, "primary_category": true,
		"preferred_language": true, "department": true, "district": true,
	}
	out, seen := make([]string, 0, len(parts)), map[string]bool{}
	for _, expression := range parts {
		expression = strings.TrimSpace(expression)
		descending := strings.HasPrefix(expression, "-")
		field := strings.TrimPrefix(strings.TrimPrefix(expression, "-"), "+")
		if !allowed[field] || seen[field] {
			return nil, fmt.Errorf("unsupported or duplicate sort field")
		}
		seen[field] = true
		if descending {
			field = "-" + field
		}
		out = append(out, field)
	}
	return out, nil
}

func sortConstituents(set composeTypes.City311ConstituentSet, published []string) {
	sort.SliceStable(set, func(i, j int) bool {
		left, right := set[i], set[j]
		for _, expression := range published {
			descending := strings.HasPrefix(expression, "-")
			field := strings.TrimPrefix(expression, "-")
			comparison := compareConstituents(left, right, field)
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

func compareConstituents(left, right *composeTypes.City311Constituent, field string) int {
	leftProfile, rightProfile := projectExportConstituent(left), projectExportConstituent(right)
	switch field {
	case "constituent_id":
		return strings.Compare(left.ConstituentID, right.ConstituentID)
	case "display_name":
		return strings.Compare(strings.ToLower(leftProfile.DisplayName), strings.ToLower(rightProfile.DisplayName))
	case "primary_category":
		return strings.Compare(string(leftProfile.PrimaryCategory), string(rightProfile.PrimaryCategory))
	case "preferred_language":
		return strings.Compare(string(leftProfile.PreferredLanguage), string(rightProfile.PreferredLanguage))
	case "department":
		return strings.Compare(string(left.OwningDepartment), string(right.OwningDepartment))
	case "district":
		return strings.Compare(string(left.CouncilDistrict), string(right.CouncilDistrict))
	default:
		return 0
	}
}

func constituentAppliedFilters(filters map[string][]string) map[string]any {
	out := make(map[string]any, len(filters))
	for key, values := range filters {
		out[key] = append([]string(nil), values...)
	}
	return out
}

func constituentSearchTokenBinding(filters map[string][]string, sortFields []string) (string, error) {
	encoded, err := json.Marshal(map[string]any{"filters": filters, "sort": sortFields})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return "constituent-search:" + hex.EncodeToString(digest[:]), nil
}
