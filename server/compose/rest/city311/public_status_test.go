package city311

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	service "github.com/cortezaproject/corteza/server/compose/service/city311"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/stretchr/testify/require"
)

func TestPublicStatusHTTPUniformMismatchAndNoPrivateData(t *testing.T) {
	router, _, svc := testRouter(t)
	input := contract.ServiceRequestCreate{Summary: "Public status test", Description: "A public inquiry with private contact details.", ServiceType: contract.ServiceTypeGeneralInquiry, Requester: contract.RequesterInput{DisplayName: "Private Person", Email: "private@example.invalid"}}
	created, _, err := svc.Submit(context.Background(), input, "status-http", service.SubmissionOptions{})
	require.NoError(t, err)
	call := func(raw string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/public/service-request-status", bytes.NewBufferString(raw))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
		return response
	}
	body, err := json.Marshal(contract.AnonymousStatusLookupRequest{RequestNumber: created.RequestNumber, Email: input.Requester.Email})
	require.NoError(t, err)
	found := call(string(body))
	require.Equal(t, http.StatusOK, found.Code, found.Body.String())
	require.NotContains(t, found.Body.String(), input.Requester.Email)
	require.NotContains(t, found.Body.String(), input.Requester.DisplayName)
	for _, raw := range []string{
		`{"request_number":"` + created.RequestNumber + `","email":"wrong@example.invalid"}`,
		`{"request_number":"SR-2026-99999","email":"private@example.invalid"}`,
		`{}`, `null`, `[]`, `{"email":1}`, `{"unknown":true}`, `{`, string(body) + ` {}`,
		`{"email":"` + strings.Repeat("x", 4100) + `"}`,
	} {
		missing := call(raw)
		require.Equal(t, http.StatusNotFound, missing.Code, raw)
		require.JSONEq(t, `{"request_detail":null}`, missing.Body.String())
	}
}
