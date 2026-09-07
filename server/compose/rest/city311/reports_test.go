package city311

import (
	"context"
	"net/http"
	"testing"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func reportHTTPDefinition(reportID string) map[string]any {
	return map[string]any{
		"report_id": reportID, "name": "HTTP report", "entity": "service_requests",
		"columns": []string{"request_number", "status"}, "filters": map[string]any{}, "sort": []string{"request_number"},
	}
}

func TestReportHTTPContractLifecycleAndOperations(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	administrator, err := store.LookupUserByEmail(ctx, st, "platform-admin@city311.example.invalid")
	require.NoError(t, err)
	agent, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)

	unauthenticated := executeJSON(t, router, http.MethodGet, "/api/v1/staff/reports/catalogue", nil, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	unknownActor := executeJSON(t, router, http.MethodGet, "/api/v1/staff/reports/catalogue", nil, nil, 999)
	require.Equal(t, http.StatusForbidden, unknownActor.Code)
	catalogue := executeJSON(t, router, http.MethodGet, "/api/v1/staff/reports/catalogue?page_size=2", nil, nil, administrator.ID)
	require.Equal(t, http.StatusOK, catalogue.Code, catalogue.Body.String())
	require.Contains(t, catalogue.Body.String(), `"total_count":5`)
	require.Contains(t, catalogue.Body.String(), `"next_page_token"`)

	definition := reportHTTPDefinition("http-report")
	createdResponse := executeJSON(t, router, http.MethodPost, "/api/v1/staff/reports", definition, nil, administrator.ID)
	require.Equal(t, http.StatusCreated, createdResponse.Code, createdResponse.Body.String())
	require.Equal(t, `"1"`, createdResponse.Header().Get("ETag"))
	created := contract.ReportDefinition{}
	require.NoError(t, decodeResponse(createdResponse, &created))
	require.Equal(t, uint64(1), created.Version)

	listed := executeJSON(t, router, http.MethodGet, "/api/v1/staff/reports", nil, nil, administrator.ID)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	require.Contains(t, listed.Body.String(), `"report_id":"http-report"`)
	malformed := executeJSON(t, router, http.MethodPost, "/api/v1/staff/reports", map[string]any{"unknown": true}, nil, administrator.ID)
	require.Equal(t, http.StatusUnprocessableEntity, malformed.Code)

	definition["name"] = "Updated HTTP report"
	missingVersion := executeJSON(t, router, http.MethodPatch, "/api/v1/staff/reports/http-report", definition, nil, administrator.ID)
	require.Equal(t, http.StatusPreconditionRequired, missingVersion.Code)
	conflict := executeJSON(t, router, http.MethodPatch, "/api/v1/staff/reports/http-report", definition, map[string]string{contract.IfMatchHeader: `"9"`}, administrator.ID)
	require.Equal(t, http.StatusConflict, conflict.Code, conflict.Body.String())
	updatedResponse := executeJSON(t, router, http.MethodPatch, "/api/v1/staff/reports/http-report", definition, map[string]string{contract.IfMatchHeader: `"1"`}, administrator.ID)
	require.Equal(t, http.StatusOK, updatedResponse.Code, updatedResponse.Body.String())
	require.Equal(t, `"2"`, updatedResponse.Header().Get("ETag"))

	nonOwner := executeJSON(t, router, http.MethodPatch, "/api/v1/staff/reports/http-report", definition, map[string]string{contract.IfMatchHeader: `"2"`}, agent.ID)
	require.Equal(t, http.StatusForbidden, nonOwner.Code)
	sharedResponse := executeJSON(t, router, http.MethodPost, "/api/v1/staff/reports/http-report/share", map[string]any{
		"roles": []string{"service_agent"},
	}, map[string]string{contract.IfMatchHeader: `"2"`}, administrator.ID)
	require.Equal(t, http.StatusOK, sharedResponse.Code, sharedResponse.Body.String())
	require.Equal(t, `"3"`, sharedResponse.Header().Get("ETag"))
	sharedList := executeJSON(t, router, http.MethodGet, "/api/v1/staff/reports", nil, nil, agent.ID)
	require.Equal(t, http.StatusOK, sharedList.Code, sharedList.Body.String())
	require.Contains(t, sharedList.Body.String(), `"report_id":"http-report"`)

	runResponse := executeJSON(t, router, http.MethodPost, "/api/v1/staff/reports/run", map[string]any{
		"definition": reportHTTPDefinition("adhoc-http-report"),
	}, nil, agent.ID)
	require.Equal(t, http.StatusAccepted, runResponse.Code, runResponse.Body.String())
	run := contract.Operation{}
	require.NoError(t, decodeResponse(runResponse, &run))
	require.Equal(t, contract.OperationStatusPending, run.Status)
	runCompleted := executeJSON(t, router, http.MethodGet, "/api/v1/operations/"+run.OperationID, nil, nil, agent.ID)
	require.Equal(t, http.StatusOK, runCompleted.Code, runCompleted.Body.String())
	require.Contains(t, runCompleted.Body.String(), `"status":"SUCCEEDED"`)
	require.Contains(t, runCompleted.Body.String(), `"row_count"`)

	exportResponse := executeJSON(t, router, http.MethodPost, "/api/v1/staff/reports/http-report/export", map[string]any{"format": "CSV"}, nil, agent.ID)
	require.Equal(t, http.StatusAccepted, exportResponse.Code, exportResponse.Body.String())
	export := contract.Operation{}
	require.NoError(t, decodeResponse(exportResponse, &export))
	exportCompleted := executeJSON(t, router, http.MethodGet, "/api/v1/operations/"+export.OperationID, nil, nil, agent.ID)
	require.Equal(t, http.StatusOK, exportCompleted.Code, exportCompleted.Body.String())
	require.Contains(t, exportCompleted.Body.String(), `"download_url"`)
	download := executeJSON(t, router, http.MethodGet, "/api/v1/operations/"+export.OperationID+"/result", nil, nil, agent.ID)
	require.Equal(t, http.StatusOK, download.Code, download.Body.String())
	require.Equal(t, "text/csv; charset=utf-8", download.Header().Get("Content-Type"))
	require.Contains(t, download.Header().Get("Content-Disposition"), "report-http-report-")
	require.Contains(t, download.Body.String(), "request_number,status\r\n")
}

func TestReportHTTPValidationAndOwnershipEdges(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	administrator, err := store.LookupUserByEmail(ctx, st, "platform-admin@city311.example.invalid")
	require.NoError(t, err)

	invalidPage := executeJSON(t, router, http.MethodGet, "/api/v1/staff/reports?page_size=101", nil, nil, administrator.ID)
	require.Equal(t, http.StatusUnprocessableEntity, invalidPage.Code)
	invalidRun := executeJSON(t, router, http.MethodPost, "/api/v1/staff/reports/run", map[string]any{
		"definition": map[string]any{"report_id": "invalid", "name": "Invalid", "entity": "unknown", "columns": []string{"bad"}, "filters": map[string]any{}, "sort": []string{}},
	}, nil, administrator.ID)
	require.Equal(t, http.StatusUnprocessableEntity, invalidRun.Code, invalidRun.Body.String())
	missing := executeJSON(t, router, http.MethodPost, "/api/v1/staff/reports/missing/export", map[string]any{"format": "CSV"}, nil, administrator.ID)
	require.Equal(t, http.StatusNotFound, missing.Code)
	invalidFormat := executeJSON(t, router, http.MethodPost, "/api/v1/staff/reports/missing/export", map[string]any{"format": "PDF"}, nil, administrator.ID)
	require.Equal(t, http.StatusUnprocessableEntity, invalidFormat.Code)
	missingShareVersion := executeJSON(t, router, http.MethodPost, "/api/v1/staff/reports/missing/share", map[string]any{"roles": []string{}}, nil, administrator.ID)
	require.Equal(t, http.StatusPreconditionRequired, missingShareVersion.Code)

	for _, request := range []struct {
		method string
		path   string
		body   any
		header map[string]string
	}{
		{method: http.MethodGet, path: "/api/v1/staff/reports"},
		{method: http.MethodPost, path: "/api/v1/staff/reports", body: reportHTTPDefinition("unknown-create")},
		{method: http.MethodPost, path: "/api/v1/staff/reports/run", body: map[string]any{"definition": reportHTTPDefinition("unknown-run")}},
		{method: http.MethodPatch, path: "/api/v1/staff/reports/missing", body: reportHTTPDefinition("missing"), header: map[string]string{contract.IfMatchHeader: `"1"`}},
		{method: http.MethodPost, path: "/api/v1/staff/reports/missing/share", body: map[string]any{"roles": []string{}}, header: map[string]string{contract.IfMatchHeader: `"1"`}},
		{method: http.MethodPost, path: "/api/v1/staff/reports/missing/export", body: map[string]any{"format": "CSV"}},
	} {
		response := executeJSON(t, router, request.method, request.path, request.body, request.header, 999)
		require.Equal(t, http.StatusForbidden, response.Code, request.path+": "+response.Body.String())
	}
}
