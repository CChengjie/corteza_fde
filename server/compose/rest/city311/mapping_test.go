package city311

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	city311Service "github.com/cortezaproject/corteza/server/compose/service/city311"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestMappingHTTPContract(t *testing.T) {
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"address":"100 Example Street, Buffalo, NY 14201","latitude":42.9001,"longitude":-78.8801,"precision_digits":4,"provider":"BENCHMARK_MAP"}`)
	}))
	defer fixture.Close()
	service, err := city311Service.NewMapping(city311Service.MappingOptions{
		BaseURL: fixture.URL, APIToken: "private-map-token", HTTPClient: fixture.Client(),
	})
	require.NoError(t, err)

	router := chi.NewRouter()
	router.Route("/api/v1", MountMappingRoutesWithService(service))

	response := executeMappingJSON(t, router, `{"address":"100 Example Street"}`)
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, "application/json", response.Header().Get("Content-Type"))
	require.JSONEq(t, `{"address":"100 Example Street, Buffalo, NY 14201","latitude":42.9001,"longitude":-78.8801,"precision_digits":4,"provider":"BENCHMARK_MAP"}`, response.Body.String())

	// The endpoint is session-optional: an invalid cookie is ignored and never
	// forwarded to the mapping fixture.
	request := httptest.NewRequest(http.MethodPost, "/api/v1/geocode", bytes.NewBufferString(`{"address":"100 Example Street"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Cookie", "city311_session=invalid")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code)
}

func TestMappingHTTPValidationAndUnavailableConfiguration(t *testing.T) {
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"ADDRESS_NOT_FOUND","message":"fixture detail","retryable":false}`)
	}))
	defer fixture.Close()
	service, err := city311Service.NewMapping(city311Service.MappingOptions{
		BaseURL: fixture.URL, APIToken: "private-map-token", HTTPClient: fixture.Client(),
	})
	require.NoError(t, err)
	router := chi.NewRouter()
	router.Route("/api/v1", MountMappingRoutesWithService(service))

	for _, body := range []string{`{"address":""}`, `{"address":"100 Example Street","unknown":true}`, `{`} {
		response := executeMappingJSON(t, router, body)
		require.Equal(t, http.StatusUnprocessableEntity, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), string(contract.ErrorValidation))
	}

	notFound := executeMappingJSON(t, router, `{"address":"Unknown Address"}`)
	require.Equal(t, http.StatusNotFound, notFound.Code)
	require.JSONEq(t, `{"error":"ADDRESS_NOT_FOUND","message":"The address was not found.","retryable":false}`, notFound.Body.String())

	t.Setenv("MAP_BASE_URL", "")
	t.Setenv("MAP_API_TOKEN", "")
	unconfigured := chi.NewRouter()
	unconfigured.Route("/api/v1", MountMappingRoutes())
	unavailable := executeMappingJSON(t, unconfigured, `{"address":"100 Example Street"}`)
	require.Equal(t, http.StatusServiceUnavailable, unavailable.Code)
	require.JSONEq(t, `{"error":"MAP_TEMPORARILY_UNAVAILABLE","message":"The mapping service is temporarily unavailable.","retryable":true}`, unavailable.Body.String())
}

func executeMappingJSON(t *testing.T, router http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/geocode", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}
