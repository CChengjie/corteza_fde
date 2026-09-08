package city311

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestStaffConstituentSearchAndDetailHTTPContract(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	administrator, err := store.LookupUserByEmail(ctx, st, "platform-admin@city311.example.invalid")
	require.NoError(t, err)
	agent, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	visible := &composeTypes.City311Constituent{
		ID: 910000000000000001, ConstituentID: "C-http-search", OwningDepartment: contract.DepartmentStreets, CouncilDistrict: contract.DistrictNorth,
		Profile: composeTypes.City311JSON{
			"constituent_id": "C-http-search", "display_name": "HTTP Search Resident", "emails": []string{"search@example.invalid"},
			"phone_numbers": []any{}, "addresses": []any{}, "primary_category": "RESIDENT", "preferred_language": "EN", "email_opt_out": false,
		},
		CreatedAt: time.Date(2026, 2, 3, 15, 4, 5, 0, time.UTC), UpdatedAt: time.Date(2026, 2, 3, 15, 4, 5, 0, time.UTC),
	}
	outside := &composeTypes.City311Constituent{
		ID: 910000000000000002, ConstituentID: "C-http-outside", OwningDepartment: contract.DepartmentSanitation, CouncilDistrict: contract.DistrictSouth,
		Profile: composeTypes.City311JSON{
			"constituent_id": "C-http-outside", "display_name": "Outside Resident", "emails": []string{"outside@example.invalid"},
			"phone_numbers": []any{}, "addresses": []any{}, "primary_category": "RESIDENT", "preferred_language": "EN", "email_opt_out": false,
		},
		CreatedAt: time.Date(2026, 2, 3, 15, 4, 5, 0, time.UTC), UpdatedAt: time.Date(2026, 2, 3, 15, 4, 5, 0, time.UTC),
	}
	require.NoError(t, store.CreateCity311Constituent(ctx, st, visible, outside))

	unauthenticated := executeJSON(t, router, http.MethodGet, "/api/v1/staff/constituents", nil, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	unknown := executeJSON(t, router, http.MethodGet, "/api/v1/staff/constituents", nil, nil, 999)
	require.Equal(t, http.StatusForbidden, unknown.Code)

	filters := url.QueryEscape(`{"query":["search@example"],"department":["STREETS"]}`)
	listed := executeJSON(t, router, http.MethodGet, "/api/v1/staff/constituents?filters="+filters+"&page_size=1&sort=-constituent_id", nil, nil, agent.ID)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	require.Contains(t, listed.Body.String(), `"constituent_id":"C-http-search"`)
	require.NotContains(t, listed.Body.String(), "C-http-outside")
	require.Contains(t, listed.Body.String(), `"sort":["-constituent_id"]`)

	detail := executeJSON(t, router, http.MethodGet, "/api/v1/staff/constituents/C-http-search", nil, nil, agent.ID)
	require.Equal(t, http.StatusOK, detail.Code, detail.Body.String())
	require.Contains(t, detail.Body.String(), `"display_name":"HTTP Search Resident"`)
	forbidden := executeJSON(t, router, http.MethodGet, "/api/v1/staff/constituents/C-http-outside", nil, nil, agent.ID)
	require.Equal(t, http.StatusForbidden, forbidden.Code)
	adminDetail := executeJSON(t, router, http.MethodGet, "/api/v1/staff/constituents/C-http-outside", nil, nil, administrator.ID)
	require.Equal(t, http.StatusOK, adminDetail.Code, adminDetail.Body.String())
	missing := executeJSON(t, router, http.MethodGet, "/api/v1/staff/constituents/missing", nil, nil, administrator.ID)
	require.Equal(t, http.StatusNotFound, missing.Code)

	invalidFilters := executeJSON(t, router, http.MethodGet, "/api/v1/staff/constituents?filters="+url.QueryEscape(`{"unknown":"value"}`), nil, nil, administrator.ID)
	require.Equal(t, http.StatusUnprocessableEntity, invalidFilters.Code)
	invalidPage := executeJSON(t, router, http.MethodGet, "/api/v1/staff/constituents?page_size=101", nil, nil, administrator.ID)
	require.Equal(t, http.StatusUnprocessableEntity, invalidPage.Code)
	invalidSort := executeJSON(t, router, http.MethodGet, "/api/v1/staff/constituents?sort=unknown", nil, nil, administrator.ID)
	require.Equal(t, http.StatusUnprocessableEntity, invalidSort.Code)
	invalidToken := executeJSON(t, router, http.MethodGet, "/api/v1/staff/constituents?page_token=invalid", nil, nil, administrator.ID)
	require.Equal(t, http.StatusBadRequest, invalidToken.Code)
	require.Contains(t, invalidToken.Body.String(), `"error":"INVALID_PAGE_TOKEN"`)

	for _, test := range []struct {
		name  string
		query string
		field string
		code  contract.ValidationCode
	}{
		{name: "duplicate values", query: "filters=" + url.QueryEscape(`{"query":["resident","resident"]}`), field: "/query/filters/query", code: contract.ValidationDuplicate},
		{name: "non-canonical boolean", query: "filters=" + url.QueryEscape(`{"email_opt_out":["1"]}`), field: "/query/filters/email_opt_out", code: contract.ValidationInvalidFormat},
		{name: "scalar JSON value", query: "filters=" + url.QueryEscape(`{"query":"resident"}`), field: "/query/filters", code: contract.ValidationInvalidFormat},
		{name: "null JSON value", query: "filters=" + url.QueryEscape(`{"query":null}`), field: "/query/filters", code: contract.ValidationInvalidFormat},
		{name: "null filters object", query: "filters=" + url.QueryEscape(`null`), field: "/query/filters", code: contract.ValidationInvalidFormat},
		{name: "null custom fields object", query: "filters=" + url.QueryEscape(`{"custom_fields":null}`), field: "/query/filters", code: contract.ValidationInvalidFormat},
		{name: "empty JSON array", query: "filters=" + url.QueryEscape(`{"query":[]}`), field: "/query/filters/query", code: contract.ValidationRequired},
		{name: "empty exploded value", query: "query=", field: "/query/filters/query", code: contract.ValidationInvalidValue},
		{name: "empty bracketed value", query: "filters%5Bquery%5D=", field: "/query/filters/query", code: contract.ValidationInvalidValue},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := executeJSON(t, router, http.MethodGet, "/api/v1/staff/constituents?"+test.query, nil, nil, administrator.ID)
			require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Body.String())
			require.Contains(t, response.Body.String(), `"field":"`+test.field+`"`)
			require.Contains(t, response.Body.String(), `"code":"`+string(test.code)+`"`)
		})
	}
}
