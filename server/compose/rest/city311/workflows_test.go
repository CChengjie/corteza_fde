package city311

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"testing"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestWorkflowHTTPDefinitionLifecycleAndRoleEnforcement(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	designer, err := store.LookupUserByEmail(ctx, st, "workflow-designer@city311.example.invalid")
	require.NoError(t, err)
	agent, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	path := "/api/v1/admin/workflows"

	unauthenticated := executeJSON(t, router, http.MethodGet, path, nil, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code, unauthenticated.Body.String())
	forbidden := executeJSON(t, router, http.MethodGet, path, nil, nil, agent.ID)
	require.Equal(t, http.StatusForbidden, forbidden.Code, forbidden.Body.String())
	unknownActor := executeJSON(t, router, http.MethodGet, path, nil, nil, 999999999)
	require.Equal(t, http.StatusForbidden, unknownActor.Code, unknownActor.Body.String())

	input := map[string]any{
		"workflow_id": "wf-http-lifecycle", "name": "HTTP lifecycle", "trigger": "SERVICE_REQUEST_CREATED", "active": false,
		"conditions": []map[string]any{{"field": "service_type", "operator": "EQUALS", "value": "POTHOLE"}},
		"actions":    []map[string]any{{"type": "FIELD_UPDATE", "field": "custom_fields.http_workflow", "value": "applied"}},
	}
	createdResponse := executeJSON(t, router, http.MethodPost, path, input, nil, designer.ID)
	require.Equal(t, http.StatusCreated, createdResponse.Code, createdResponse.Body.String())
	created := contract.WorkflowDefinition{}
	require.NoError(t, decodeResponse(createdResponse, &created))
	require.Equal(t, uint64(1), created.Version)
	require.False(t, created.Active)

	getResponse := executeJSON(t, router, http.MethodGet, path+"/"+created.WorkflowID, nil, nil, designer.ID)
	require.Equal(t, http.StatusOK, getResponse.Code, getResponse.Body.String())
	listed := executeJSON(t, router, http.MethodGet, path+"?filters="+url.QueryEscape(`{"trigger":"SERVICE_REQUEST_CREATED","active":false}`), nil, nil, designer.ID)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	require.Contains(t, listed.Body.String(), `"total_count":1`)

	input["name"] = "Updated HTTP lifecycle"
	missingVersion := executeJSON(t, router, http.MethodPatch, path+"/"+created.WorkflowID, input, nil, designer.ID)
	require.Equal(t, http.StatusPreconditionRequired, missingVersion.Code, missingVersion.Body.String())
	conflict := executeJSON(t, router, http.MethodPatch, path+"/"+created.WorkflowID, input, map[string]string{contract.IfMatchHeader: `"9"`}, designer.ID)
	require.Equal(t, http.StatusConflict, conflict.Code, conflict.Body.String())
	require.Contains(t, conflict.Body.String(), `"current_version":1`)

	updatedResponse := executeJSON(t, router, http.MethodPatch, path+"/"+created.WorkflowID, input, map[string]string{contract.IfMatchHeader: `"1"`}, designer.ID)
	require.Equal(t, http.StatusOK, updatedResponse.Code, updatedResponse.Body.String())
	updated := contract.WorkflowDefinition{}
	require.NoError(t, decodeResponse(updatedResponse, &updated))
	require.Equal(t, uint64(2), updated.Version)
	activatedResponse := executeJSON(t, router, http.MethodPost, path+"/"+created.WorkflowID+"/activate", map[string]any{}, map[string]string{contract.IfMatchHeader: `"2"`}, designer.ID)
	require.Equal(t, http.StatusOK, activatedResponse.Code, activatedResponse.Body.String())
	activated := contract.WorkflowDefinition{}
	require.NoError(t, decodeResponse(activatedResponse, &activated))
	require.True(t, activated.Active)
	require.Equal(t, uint64(3), activated.Version)
	deactivated := executeJSON(t, router, http.MethodPost, path+"/"+created.WorkflowID+"/deactivate", map[string]any{}, map[string]string{contract.IfMatchHeader: `"3"`}, designer.ID)
	require.Equal(t, http.StatusOK, deactivated.Code, deactivated.Body.String())
	require.Contains(t, deactivated.Body.String(), `"active":false`)
}

func TestWorkflowHTTPTestOperationExecutionLogAndValidation(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	designer, err := store.LookupUserByEmail(ctx, st, "workflow-designer@city311.example.invalid")
	require.NoError(t, err)
	profile, err := store.LookupCity311ActorProfileByID(ctx, st, designer.ID)
	require.NoError(t, err)
	profile.ApplicationRoles = append(profile.ApplicationRoles, contract.ApplicationRolePlatformAdministrator)
	require.NoError(t, store.UpdateCity311ActorProfile(ctx, st, profile))

	input := map[string]any{
		"workflow_id": "wf-http-test", "name": "HTTP test operation", "trigger": "SERVICE_REQUEST_CREATED", "active": false,
		"conditions": []map[string]any{}, "actions": []map[string]any{{"type": "FIELD_UPDATE", "field": "custom_fields.http_test", "value": true}},
	}
	createdResponse := executeJSON(t, router, http.MethodPost, "/api/v1/admin/workflows", input, nil, designer.ID)
	require.Equal(t, http.StatusCreated, createdResponse.Code, createdResponse.Body.String())
	created := contract.WorkflowDefinition{}
	require.NoError(t, decodeResponse(createdResponse, &created))
	seeded, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)

	testResponse := executeJSON(t, router, http.MethodPost, "/api/v1/admin/workflows/"+created.WorkflowID+"/test", map[string]any{
		"request_id": strconv.FormatUint(seeded.ID, 10),
	}, nil, designer.ID)
	require.Equal(t, http.StatusAccepted, testResponse.Code, testResponse.Body.String())
	pending := contract.Operation{}
	require.NoError(t, decodeResponse(testResponse, &pending))
	require.Equal(t, contract.OperationStatusPending, pending.Status)

	operationResponse := executeJSON(t, router, http.MethodGet, "/api/v1/operations/"+pending.OperationID, nil, nil, designer.ID)
	require.Equal(t, http.StatusOK, operationResponse.Code, operationResponse.Body.String())
	operation := contract.Operation{}
	require.NoError(t, decodeResponse(operationResponse, &operation))
	require.Equal(t, contract.OperationStatusSucceeded, operation.Status)
	executionID, ok := operation.Result["execution_id"].(string)
	require.True(t, ok)

	executionResponse := executeJSON(t, router, http.MethodGet, "/api/v1/admin/workflow-executions/"+executionID, nil, nil, designer.ID)
	require.Equal(t, http.StatusOK, executionResponse.Code, executionResponse.Body.String())
	execution := contract.WorkflowExecution{}
	require.NoError(t, decodeResponse(executionResponse, &execution))
	require.True(t, execution.Succeeded)
	require.Equal(t, []string{"FIELD_UPDATE"}, execution.ActionsAttempted)
	filters := url.QueryEscape(`{"workflow_id":"` + created.WorkflowID + `","request_id":"` + strconv.FormatUint(seeded.ID, 10) + `","succeeded":true}`)
	listResponse := executeJSON(t, router, http.MethodGet, "/api/v1/admin/workflow-executions?filters="+filters, nil, nil, designer.ID)
	require.Equal(t, http.StatusOK, listResponse.Code, listResponse.Body.String())
	require.Contains(t, listResponse.Body.String(), `"total_count":1`)

	badFilters := executeJSON(t, router, http.MethodGet, "/api/v1/admin/workflows?filters="+url.QueryEscape(`{} {}`), nil, nil, designer.ID)
	require.Equal(t, http.StatusUnprocessableEntity, badFilters.Code, badFilters.Body.String())
	unknownFilter := executeJSON(t, router, http.MethodGet, "/api/v1/admin/workflows?filters="+url.QueryEscape(`{"unknown":true}`), nil, nil, designer.ID)
	require.Equal(t, http.StatusUnprocessableEntity, unknownFilter.Code, unknownFilter.Body.String())
	badRequestID := executeJSON(t, router, http.MethodGet, "/api/v1/admin/workflow-executions?filters="+url.QueryEscape(`{"request_id":"bad"}`), nil, nil, designer.ID)
	require.Equal(t, http.StatusUnprocessableEntity, badRequestID.Code, badRequestID.Body.String())
	badPageSize := executeJSON(t, router, http.MethodGet, "/api/v1/admin/workflows?page_size=0", nil, nil, designer.ID)
	require.Equal(t, http.StatusUnprocessableEntity, badPageSize.Code, badPageSize.Body.String())
	badBody := executeJSON(t, router, http.MethodPost, "/api/v1/admin/workflows", map[string]any{"unknown": true}, nil, designer.ID)
	require.Equal(t, http.StatusUnprocessableEntity, badBody.Code, badBody.Body.String())
}
