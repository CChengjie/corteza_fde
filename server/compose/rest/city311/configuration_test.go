package city311

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestContactCategoryHTTPContract(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	manager, err := store.LookupUserByEmail(ctx, st, "department-manager@city311.example.invalid")
	require.NoError(t, err)
	agent, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)

	unauthenticated := executeJSON(t, router, http.MethodGet, "/api/v1/admin/contact-categories", nil, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	forbidden := executeJSON(t, router, http.MethodGet, "/api/v1/admin/contact-categories", nil, nil, agent.ID)
	require.Equal(t, http.StatusForbidden, forbidden.Code)
	unknownActor := executeJSON(t, router, http.MethodGet, "/api/v1/admin/contact-categories", nil, nil, 999)
	require.Equal(t, http.StatusForbidden, unknownActor.Code)

	listed := executeJSON(t, router, http.MethodGet, "/api/v1/admin/contact-categories?page_size=2", nil, nil, manager.ID)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	require.Contains(t, listed.Body.String(), `"total_count":7`)
	require.Contains(t, listed.Body.String(), `"next_page_token"`)
	invalidPage := executeJSON(t, router, http.MethodGet, "/api/v1/admin/contact-categories?page_size=101", nil, nil, manager.ID)
	require.Equal(t, http.StatusUnprocessableEntity, invalidPage.Code)

	body := map[string]any{"code": "NONPROFIT", "active": true, "labels": map[string]string{"EN": "Nonprofit"}}
	created := executeJSON(t, router, http.MethodPost, "/api/v1/admin/contact-categories", body, nil, manager.ID)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	require.Equal(t, `"1"`, created.Header().Get("ETag"))
	malformed := executeJSON(t, router, http.MethodPost, "/api/v1/admin/contact-categories", map[string]any{"unknown": true}, nil, manager.ID)
	require.Equal(t, http.StatusUnprocessableEntity, malformed.Code)
	unknownActor = executeJSON(t, router, http.MethodPost, "/api/v1/admin/contact-categories", body, nil, 999)
	require.Equal(t, http.StatusForbidden, unknownActor.Code)

	body["code"] = "RENAMED"
	conflict := executeJSON(t, router, http.MethodPatch, "/api/v1/admin/contact-categories/NONPROFIT", body, nil, manager.ID)
	require.Equal(t, http.StatusUnprocessableEntity, conflict.Code)
	require.Contains(t, conflict.Body.String(), `"code":"CONFLICT"`)
	body["code"] = "NONPROFIT"
	body["active"] = false
	updated := executeJSON(t, router, http.MethodPatch, "/api/v1/admin/contact-categories/NONPROFIT", body, nil, manager.ID)
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	require.Equal(t, `"2"`, updated.Header().Get("ETag"))
	malformed = executeJSON(t, router, http.MethodPatch, "/api/v1/admin/contact-categories/NONPROFIT", map[string]any{"unknown": true}, nil, manager.ID)
	require.Equal(t, http.StatusUnprocessableEntity, malformed.Code)
	unknownActor = executeJSON(t, router, http.MethodPatch, "/api/v1/admin/contact-categories/NONPROFIT", body, nil, 999)
	require.Equal(t, http.StatusForbidden, unknownActor.Code)
}

func TestCustomFieldHTTPContractAndQueueFiltering(t *testing.T) {
	router, st, svc := testRouter(t)
	ctx := context.Background()
	administrator, err := store.LookupUserByEmail(ctx, st, "platform-admin@city311.example.invalid")
	require.NoError(t, err)
	manager, err := store.LookupUserByEmail(ctx, st, "department-manager@city311.example.invalid")
	require.NoError(t, err)
	unknownList := executeJSON(t, router, http.MethodGet, "/api/v1/admin/custom-fields", nil, nil, 999)
	require.Equal(t, http.StatusForbidden, unknownList.Code)

	body := map[string]any{
		"key": "impact", "labels": map[string]string{"EN": "Impact"}, "entity": "service_request",
		"field_type": "SINGLE_CHOICE", "required": false, "active": true,
		"validation": map[string]any{}, "choice_values": []string{"LOW", "HIGH"},
	}
	forbidden := executeJSON(t, router, http.MethodPost, "/api/v1/admin/custom-fields", body, nil, manager.ID)
	require.Equal(t, http.StatusForbidden, forbidden.Code)
	created := executeJSON(t, router, http.MethodPost, "/api/v1/admin/custom-fields", body, nil, administrator.ID)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	require.Equal(t, `"1"`, created.Header().Get("ETag"))
	malformed := executeJSON(t, router, http.MethodPost, "/api/v1/admin/custom-fields", map[string]any{"unknown": true}, nil, administrator.ID)
	require.Equal(t, http.StatusUnprocessableEntity, malformed.Code)
	unknownActor := executeJSON(t, router, http.MethodPost, "/api/v1/admin/custom-fields", body, nil, 999)
	require.Equal(t, http.StatusForbidden, unknownActor.Code)

	missingVersion := executeJSON(t, router, http.MethodPatch, "/api/v1/admin/custom-fields/impact", body, nil, administrator.ID)
	require.Equal(t, http.StatusPreconditionRequired, missingVersion.Code)
	malformed = executeJSON(t, router, http.MethodPatch, "/api/v1/admin/custom-fields/impact", map[string]any{"unknown": true}, map[string]string{contract.IfMatchHeader: `"1"`}, administrator.ID)
	require.Equal(t, http.StatusUnprocessableEntity, malformed.Code)
	body["labels"] = map[string]string{"EN": "Community impact"}
	updated := executeJSON(t, router, http.MethodPatch, "/api/v1/admin/custom-fields/impact", body, map[string]string{contract.IfMatchHeader: `"1"`}, administrator.ID)
	require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
	require.Equal(t, `"2"`, updated.Header().Get("ETag"))

	actor, err := svc.FindActor(ctx, administrator.ID)
	require.NoError(t, err)
	input := validPortalBody()
	input["custom_fields"] = map[string]any{"impact": "HIGH"}
	createdRequest := executeJSON(t, router, http.MethodPost, "/api/v1/staff/service-requests", map[string]any{
		"request": input, "constituent": map[string]any{"display_name": "Queue Contact", "email": "queue@example.invalid"},
	}, nil, administrator.ID)
	require.Equal(t, http.StatusCreated, createdRequest.Code, createdRequest.Body.String())
	require.NotZero(t, actor.ID)

	filters := url.QueryEscape(`{"custom_fields":{"impact":"HIGH"}}`)
	queue := executeJSON(t, router, http.MethodGet, "/api/v1/staff/service-requests?filters="+filters, nil, nil, administrator.ID)
	require.Equal(t, http.StatusOK, queue.Code, queue.Body.String())
	require.Contains(t, queue.Body.String(), `"total_count":1`)

	listed := executeJSON(t, router, http.MethodGet, "/api/v1/admin/custom-fields", nil, nil, administrator.ID)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	require.Contains(t, listed.Body.String(), `"key":"impact"`)
	invalidPage := executeJSON(t, router, http.MethodGet, "/api/v1/admin/custom-fields?page_size=101", nil, nil, administrator.ID)
	require.Equal(t, http.StatusUnprocessableEntity, invalidPage.Code)
}
