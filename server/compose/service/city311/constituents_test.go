package city311

import (
	"context"
	"net/http"
	"strings"
	"testing"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestStaffConstituentSearchDetailScopeAndCustomFields(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	administrator := seededAssignmentActor(t, ctx, svc, st, "platform-admin@city311.example.invalid")
	agent := contract.Actor{ID: 701, Roles: []contract.ApplicationRole{contract.ApplicationRoleServiceAgent}, Department: contract.DepartmentPublicWorks, Districts: []contract.DistrictCode{contract.DistrictNorth}}
	constituent := testSearchConstituent(svc, "C-search-alpha", "Ana Example", "Ana@example.invalid", contract.DepartmentPublicWorks, contract.DistrictNorth)
	constituent.Profile["phone_numbers"] = []any{map[string]any{"label": "MOBILE", "value": "+17165550101"}}
	constituent.Profile["custom_fields"] = map[string]any{"neighbourhood": "Elmwood"}
	outOfDistrict := testSearchConstituent(svc, "C-search-south", "South Resident", "south@example.invalid", contract.DepartmentPublicWorks, contract.DistrictSouth)
	outOfDepartment := testSearchConstituent(svc, "C-search-sanitation", "Sanitation Resident", "sanitation@example.invalid", contract.DepartmentSanitation, contract.DistrictNorth)
	require.NoError(t, store.CreateCity311Constituent(ctx, st, constituent, outOfDistrict, outOfDepartment))

	_, err := svc.CreateCustomField(ctx, administrator, contract.CustomFieldDefinition{
		Key: "neighbourhood", Labels: map[string]string{"EN": "Neighbourhood"}, Entity: "constituent",
		FieldType: contract.CustomFieldTypeText, Active: true,
	})
	require.NoError(t, err)

	result, err := svc.SearchConstituents(ctx, agent, ConstituentSearchQuery{
		Filters: map[string][]string{"query": {"ana@"}, "custom_fields.neighbourhood": {"Elmwood"}}, PageSize: 1, Sort: "-display_name",
	})
	require.NoError(t, err)
	require.Equal(t, 1, result.TotalCount)
	require.Len(t, result.Items, 1)
	require.Equal(t, constituent.ConstituentID, result.Items[0].ConstituentID)
	require.Equal(t, "Elmwood", result.Items[0].CustomFields["neighbourhood"])
	require.Equal(t, []string{"-display_name"}, result.Sort)
	require.Equal(t, []string{"ana@"}, result.AppliedFilters["query"])

	detail, err := svc.GetStaffConstituent(ctx, agent, constituent.ConstituentID)
	require.NoError(t, err)
	require.Equal(t, "Ana Example", detail.DisplayName)
	require.Equal(t, "+17165550101", detail.PhoneNumbers[0].Value)
	_, err = svc.GetStaffConstituent(ctx, agent, outOfDistrict.ConstituentID)
	requireServiceError(t, err, http.StatusForbidden, contract.ErrorForbidden)
	_, err = svc.GetStaffConstituent(ctx, agent, "missing")
	requireServiceError(t, err, http.StatusNotFound, contract.ErrorNotFound)
	_, err = svc.SearchConstituents(ctx, contract.Actor{Roles: []contract.ApplicationRole{contract.ApplicationRoleConstituent}}, ConstituentSearchQuery{})
	requireServiceError(t, err, http.StatusForbidden, contract.ErrorForbidden)
}

func TestStaffConstituentSearchPaginationSortAndValidation(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	administrator := seededAssignmentActor(t, ctx, svc, st, "platform-admin@city311.example.invalid")
	first, err := svc.SearchConstituents(ctx, administrator, ConstituentSearchQuery{
		Filters: map[string][]string{"primary_category": {"RESIDENT"}, "email_opt_out": {"false"}}, PageSize: 2, Sort: "display_name,constituent_id",
	})
	require.NoError(t, err)
	require.Greater(t, first.TotalCount, 2)
	require.Len(t, first.Items, 2)
	require.NotNil(t, first.NextPageToken)
	second, err := svc.SearchConstituents(ctx, administrator, ConstituentSearchQuery{
		Filters: map[string][]string{"primary_category": {"RESIDENT"}, "email_opt_out": {"false"}}, PageSize: 2, PageToken: *first.NextPageToken, Sort: "display_name,constituent_id",
	})
	require.NoError(t, err)
	require.NotEqual(t, first.Items[0].ConstituentID, second.Items[0].ConstituentID)

	for _, test := range []struct {
		name  string
		query ConstituentSearchQuery
		field string
		code  contract.ValidationCode
	}{
		{name: "page size", query: ConstituentSearchQuery{PageSize: 101}, field: "/query/page_size", code: contract.ValidationOutOfRange},
		{name: "unknown filter", query: ConstituentSearchQuery{Filters: map[string][]string{"unknown": {"value"}}}, field: "/query/filters/unknown", code: contract.ValidationInvalidValue},
		{name: "invalid category", query: ConstituentSearchQuery{Filters: map[string][]string{"primary_category": {"UNKNOWN"}}}, field: "/query/filters/primary_category", code: contract.ValidationInvalidValue},
		{name: "invalid bool", query: ConstituentSearchQuery{Filters: map[string][]string{"email_opt_out": {"sometimes"}}}, field: "/query/filters/email_opt_out", code: contract.ValidationInvalidFormat},
		{name: "unknown custom field", query: ConstituentSearchQuery{Filters: map[string][]string{"custom_fields.unknown": {"value"}}}, field: "/query/filters/custom_fields.unknown", code: contract.ValidationInvalidValue},
		{name: "invalid sort", query: ConstituentSearchQuery{Sort: "display_name,display_name"}, field: "/query/sort", code: contract.ValidationInvalidFormat},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, searchErr := svc.SearchConstituents(ctx, administrator, test.query)
			requireValidationCode(t, searchErr, test.field, test.code)
		})
	}
	_, err = svc.SearchConstituents(ctx, administrator, ConstituentSearchQuery{
		Filters: map[string][]string{"primary_category": {"BUSINESS"}}, PageToken: *first.NextPageToken, Sort: "display_name,constituent_id",
	})
	requireServiceError(t, err, http.StatusBadRequest, contract.ErrorInvalidPageToken)
}

func TestConstituentSearchFilterAndSortHelpers(t *testing.T) {
	svc, _ := testService(t)
	left := testSearchConstituent(svc, "C-10", "Alpha Resident", "alpha@example.invalid", contract.DepartmentPublicWorks, contract.DistrictNorth)
	left.Profile["phone_numbers"] = []any{map[string]any{"label": "WORK", "value": "+17165550123"}}
	left.Profile["custom_fields"] = map[string]any{"area": "Elmwood"}
	left.Profile["email_opt_out"] = true
	right := testSearchConstituent(svc, "C-20", "Beta Resident", "beta@example.invalid", contract.DepartmentSanitation, contract.DistrictSouth)
	right.Profile["primary_category"] = string(contract.ContactCategoryBusiness)
	right.Profile["preferred_language"] = string(contract.LanguageVI)

	require.True(t, matchesConstituentSearch(left, map[string][]string{
		"display_name": {"resident", "alpha"}, "email": {"missing", "ALPHA@"}, "phone": {"HOME", "50123"},
		"email_opt_out": {"true"}, "department": {"PUBLIC_WORKS"}, "district": {"NORTH"},
		"primary_category": {"RESIDENT"}, "preferred_language": {"EN"}, "custom_fields.area": {"Elmwood"}, "query": {"C-10"},
	}))
	require.False(t, matchesConstituentSearch(left, map[string][]string{"display_name": {"missing"}}))
	require.False(t, matchesConstituentSearch(left, map[string][]string{"email": {"missing"}}))
	require.False(t, matchesConstituentSearch(left, map[string][]string{"phone": {"missing"}}))
	require.False(t, matchesConstituentSearch(left, map[string][]string{"query": {"missing"}}))
	require.True(t, matchesFoldContains("value", nil))
	require.True(t, matchesAnyFoldContains(nil, nil))
	require.True(t, matchesPhoneSearch(nil, nil))

	for expression, ascending := range map[string]*composeTypes.City311Constituent{
		"constituent_id": left, "display_name": left, "primary_category": right,
		"preferred_language": left, "department": left, "district": left,
	} {
		set := composeTypes.City311ConstituentSet{right, left}
		sortConstituents(set, []string{expression})
		require.Equal(t, ascending, set[0], expression)
		sortConstituents(set, []string{"-" + expression})
		require.NotEqual(t, ascending, set[0], expression)
	}

	for key, values := range map[string][]string{
		"preferred_language": {"FR"}, "department": {"UNKNOWN"}, "district": {"UNKNOWN"},
	} {
		err := validateConstituentFilters(map[string][]string{key: values}, contract.ContactCategories)
		requireValidationCode(t, err, "/query/filters/"+key, contract.ValidationInvalidValue)
	}
	_, err := normalizeConstituentFilters(map[string][]string{"query": {}}, nil)
	requireValidationCode(t, err, "/query/filters/query", contract.ValidationRequired)
	_, err = normalizeConstituentFilters(map[string][]string{"query": {" "}}, nil)
	requireValidationCode(t, err, "/query/filters/query", contract.ValidationInvalidValue)
	_, err = normalizeConstituentFilters(map[string][]string{"query": {strings.Repeat("x", 255)}}, nil)
	requireValidationCode(t, err, "/query/filters/query", contract.ValidationInvalidValue)
}

func testSearchConstituent(svc *Service, id, name, email string, department contract.DepartmentCode, district contract.DistrictCode) *composeTypes.City311Constituent {
	return &composeTypes.City311Constituent{
		ID: svc.nextID(), ConstituentID: id, OwningDepartment: department, CouncilDistrict: district,
		Profile: composeTypes.City311JSON{
			"constituent_id": id, "display_name": name, "emails": []string{email}, "phone_numbers": []any{}, "addresses": []any{},
			"primary_category": string(contract.ContactCategoryResident), "preferred_language": string(contract.LanguageEN), "email_opt_out": false,
		},
		CreatedAt: svc.now(), UpdatedAt: svc.now(),
	}
}
