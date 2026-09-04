package city311

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	service "github.com/cortezaproject/corteza/server/compose/service/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestScopedReadHTTPAuthenticationAndResponses(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	for _, item := range []struct {
		handle, path string
		status       int
	}{
		{"", "/api/v1/staff/constituents", 401},
		{"", "/api/v1/staff/audit-events", 401},
		{"city311-workflow-designer", "/api/v1/staff/constituents", 403},
		{"city311-workflow-designer", "/api/v1/staff/constituents?page_size=bad", 403},
		{"city311-workflow-designer", "/api/v1/staff/constituents/C-1", 403},
		{"city311-service-agent", "/api/v1/staff/audit-events", 403},
		{"city311-constituent", "/api/v1/staff/constituents", 403},
		{"city311-platform-administrator", "/api/v1/staff/constituents", 200},
		{"city311-platform-administrator", "/api/v1/staff/constituents/C-1", 200},
		{"city311-platform-administrator", "/api/v1/staff/constituents/C-missing", 404},
		{"city311-department-manager", "/api/v1/staff/audit-events", 200},
	} {
		var userID uint64
		if item.handle != "" {
			user, err := store.LookupUserByHandle(ctx, st, item.handle)
			require.NoError(t, err)
			userID = user.ID
		}
		response := executeJSON(t, router, http.MethodGet, item.path, nil, nil, userID)
		require.Equal(t, item.status, response.Code, item.path+response.Body.String())
		if item.status == 200 {
			require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		}
	}
	admin, err := store.LookupUserByHandle(ctx, st, "city311-platform-administrator")
	require.NoError(t, err)
	first := executeJSON(t, router, http.MethodGet, "/api/v1/staff/constituents?page_size=1", nil, nil, admin.ID)
	require.Equal(t, 200, first.Code, first.Body.String())
	var page service.RecordReadPage
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &page))
	require.Equal(t, 10, page.TotalCount)
	require.Len(t, page.Items, 1)
	require.NotNil(t, page.NextPageToken)
	second := executeJSON(t, router, http.MethodGet, "/api/v1/staff/constituents?page_size=1&page_token="+url.QueryEscape(*page.NextPageToken), nil, nil, admin.ID)
	require.Equal(t, 200, second.Code)
	require.NotEqual(t, first.Body.String(), second.Body.String())
	changed := executeJSON(t, router, http.MethodGet, "/api/v1/staff/constituents?q=changed&page_token="+url.QueryEscape(*page.NextPageToken), nil, nil, admin.ID)
	require.Equal(t, 422, changed.Code)
	for _, endpoint := range []string{"constituents", "audit-events"} {
		query := "entity_type=absent"
		filter := `{"entity_type":"absent"}`
		if endpoint == "constituents" {
			query, filter = "q=absent", `{"q":"absent"}`
		}
		exploded := executeJSON(t, router, http.MethodGet, "/api/v1/staff/"+endpoint+"?"+query, nil, nil, admin.ID)
		encoded := executeJSON(t, router, http.MethodGet, "/api/v1/staff/"+endpoint+"?filters="+url.QueryEscape(filter), nil, nil, admin.ID)
		require.Equal(t, 200, exploded.Code, exploded.Body.String())
		require.JSONEq(t, exploded.Body.String(), encoded.Body.String())
		require.Contains(t, encoded.Body.String(), `"items":[]`)
	}
}

func TestScopedReadHTTPRejectsInvalidQuery(t *testing.T) {
	router, st, _ := testRouter(t)
	admin, err := store.LookupUserByHandle(context.Background(), st, "city311-platform-administrator")
	require.NoError(t, err)
	queries := []string{"page_size=0", "page_size=101", "page_size=-1", "page_size=abc", "q=a&q=b", "unknown=x", "page_token=!", "sort=bad", "filters=null", "filters=[]", "filters=1", "filters={", "filters=" + url.QueryEscape(`{"q":null}`), "filters=" + url.QueryEscape(`{"q":2}`), "filters=" + url.QueryEscape(`{"bad":"value"}`), "q=a&filters=" + url.QueryEscape(`{"q":"b"}`)}
	for _, query := range queries {
		response := executeJSON(t, router, http.MethodGet, "/api/v1/staff/constituents?"+query, nil, nil, admin.ID)
		require.Equal(t, 422, response.Code, query+response.Body.String())
		require.Contains(t, response.Body.String(), `"field":"/query/`)
	}
	for _, query := range []string{"a~/b=x", "filters=" + url.QueryEscape(`{"a~/b":null}`), "filters=" + url.QueryEscape(`{"a~/b":"x"}`)} {
		response := executeJSON(t, router, http.MethodGet, "/api/v1/staff/constituents?"+query, nil, nil, admin.ID)
		require.Equal(t, 422, response.Code)
		require.Contains(t, response.Body.String(), "a~0~1b")
	}
	badAudit := executeJSON(t, router, http.MethodGet, "/api/v1/staff/audit-events?from=bad", nil, nil, admin.ID)
	require.Equal(t, 422, badAudit.Code)
}
