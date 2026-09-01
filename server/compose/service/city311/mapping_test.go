package city311

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/stretchr/testify/require"
)

func TestMappingRuntimeConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name    string
		baseURL string
		token   string
		message string
	}{
		{name: "missing URL", token: "map-token", message: "MAP_BASE_URL is required"},
		{name: "relative URL", baseURL: "/mapping", token: "map-token", message: "MAP_BASE_URL must be an absolute HTTP or HTTPS URL"},
		{name: "unsupported URL scheme", baseURL: "file:///mapping", token: "map-token", message: "MAP_BASE_URL must be an absolute HTTP or HTTPS URL"},
		{name: "embedded credentials", baseURL: "https://user:password@mapping.example.invalid", token: "map-token", message: "MAP_BASE_URL must not contain credentials, a query, or a fragment"},
		{name: "query", baseURL: "https://mapping.example.invalid?token=leak", token: "map-token", message: "MAP_BASE_URL must not contain credentials, a query, or a fragment"},
		{name: "missing token", baseURL: "https://mapping.example.invalid", message: "MAP_API_TOKEN is required"},
		{name: "malformed token", baseURL: "https://mapping.example.invalid", token: " token ", message: "MAP_API_TOKEN is malformed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewMapping(MappingOptions{BaseURL: tc.baseURL, APIToken: tc.token})
			require.EqualError(t, err, tc.message)
		})
	}

	t.Setenv("MAP_BASE_URL", "https://mapping.example.invalid/prefix/")
	t.Setenv("MAP_API_TOKEN", "runtime-map-token")
	service, err := NewMappingFromEnvironment(&http.Client{})
	require.NoError(t, err)
	require.Equal(t, "https://mapping.example.invalid/prefix/internal/integrations/mapping/geocode", service.endpoint)
	require.NoError(t, ValidateMappingEnvironment())

	service, err = NewMapping(MappingOptions{BaseURL: "https://mapping.example.invalid", APIToken: "runtime-map-token"})
	require.NoError(t, err)
	require.Equal(t, mappingRequestTimeout, service.httpClient.Timeout)
}

func TestMappingGeocodeForwardsOnlyTheServerCredential(t *testing.T) {
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, mappingGeocodePath, r.URL.Path)
		require.Equal(t, "Bearer private-map-token", r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		require.JSONEq(t, `{"address":"100 Example Street"}`, string(body))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"address":"100 Example Street, Buffalo, NY 14201","latitude":42.9001,"longitude":-78.8801,"precision_digits":4,"provider":"BENCHMARK_MAP"}`)
	}))
	defer fixture.Close()

	service, err := NewMapping(MappingOptions{BaseURL: fixture.URL, APIToken: "private-map-token", HTTPClient: fixture.Client()})
	require.NoError(t, err)
	result, err := service.Geocode(context.Background(), " 100 Example Street ")
	require.NoError(t, err)
	require.Equal(t, contract.MockGeocodeSuccess(), *result)
}

func TestMappingGeocodePublishesOnlyDeclaredBrowserOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		body       string
		wantStatus int
		wantError  contract.ErrorCode
		retryable  bool
	}{
		{name: "not found", status: http.StatusNotFound, body: `{"error":"ADDRESS_NOT_FOUND","message":"fixture detail","retryable":false}`, wantStatus: http.StatusNotFound, wantError: contract.ErrorAddressNotFound},
		{name: "malformed not found", status: http.StatusNotFound, body: `{"error":"OTHER","message":"wrong route","retryable":false}`, wantStatus: http.StatusServiceUnavailable, wantError: contract.ErrorMapTemporarilyUnavailable, retryable: true},
		{name: "invalid not found body", status: http.StatusNotFound, body: `{`, wantStatus: http.StatusServiceUnavailable, wantError: contract.ErrorMapTemporarilyUnavailable, retryable: true},
		{name: "trailing not found body", status: http.StatusNotFound, body: `{"error":"ADDRESS_NOT_FOUND","message":"fixture detail","retryable":false} {}`, wantStatus: http.StatusServiceUnavailable, wantError: contract.ErrorMapTemporarilyUnavailable, retryable: true},
		{name: "retryable not found body", status: http.StatusNotFound, body: `{"error":"ADDRESS_NOT_FOUND","message":"fixture detail","retryable":true}`, wantStatus: http.StatusServiceUnavailable, wantError: contract.ErrorMapTemporarilyUnavailable, retryable: true},
		{name: "fixture authentication", status: http.StatusUnauthorized, body: `{"error":"MAP_UNAUTHENTICATED","message":"secret diagnostic","retryable":false}`, wantStatus: http.StatusServiceUnavailable, wantError: contract.ErrorMapTemporarilyUnavailable, retryable: true},
		{name: "controlled outage", status: http.StatusServiceUnavailable, body: `{"error":"MAP_TEMPORARILY_UNAVAILABLE","message":"fixture detail","retryable":true}`, wantStatus: http.StatusServiceUnavailable, wantError: contract.ErrorMapTemporarilyUnavailable, retryable: true},
		{name: "undeclared fixture failure", status: http.StatusBadGateway, body: `upstream detail`, wantStatus: http.StatusServiceUnavailable, wantError: contract.ErrorMapTemporarilyUnavailable, retryable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer fixture.Close()
			service, err := NewMapping(MappingOptions{BaseURL: fixture.URL, APIToken: "private-map-token", HTTPClient: fixture.Client()})
			require.NoError(t, err)
			_, err = service.Geocode(context.Background(), "100 Example Street")
			serviceError := requireMappingServiceError(t, err)
			require.Equal(t, tc.wantStatus, serviceError.Status)
			require.Equal(t, tc.wantError, serviceError.Payload.Error)
			require.Equal(t, tc.retryable, serviceError.Payload.Retryable)
			require.NotContains(t, serviceError.Payload.Message, "fixture")
			require.NotContains(t, serviceError.Payload.Message, "secret")
		})
	}
}

func TestMappingGeocodeValidatesInputTransportAndFixtureResponse(t *testing.T) {
	service, err := NewMapping(MappingOptions{
		BaseURL: "https://mapping.example.invalid", APIToken: "private-map-token",
		HTTPClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("fixture unavailable")
		})},
	})
	require.NoError(t, err)
	_, err = service.Geocode(context.Background(), " ")
	validation := requireMappingServiceError(t, err)
	require.Equal(t, http.StatusUnprocessableEntity, validation.Status)
	require.Equal(t, contract.ErrorValidation, validation.Payload.Error)
	require.Equal(t, []contract.FieldError{{Field: "/address", Code: contract.ValidationRequired}}, validation.Payload.Errors)

	_, err = service.Geocode(context.Background(), "100 Example Street")
	require.Equal(t, contract.ErrorMapTemporarilyUnavailable, requireMappingServiceError(t, err).Payload.Error)

	service, err = NewMapping(MappingOptions{
		BaseURL: "https://mapping.example.invalid", APIToken: "private-map-token",
		HTTPClient: &http.Client{Transport: roundTripperFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: errorReadCloser{}}, nil
		})},
	})
	require.NoError(t, err)
	_, err = service.Geocode(context.Background(), "100 Example Street")
	require.Equal(t, contract.ErrorMapTemporarilyUnavailable, requireMappingServiceError(t, err).Payload.Error)

	for _, body := range []string{
		`{"address":"100 Example Street","latitude":91,"longitude":-78.8801,"precision_digits":4,"provider":"BENCHMARK_MAP"}`,
		`{"address":"100 Example Street","latitude":42.9001,"longitude":-78.8801,"precision_digits":3,"provider":"BENCHMARK_MAP"}`,
		`{"address":"100 Example Street","latitude":42.9001,"longitude":-78.8801,"precision_digits":4,"provider":"OTHER"}`,
		`{"address":"100 Example Street","latitude":42.9001,"longitude":-78.8801,"precision_digits":4,"provider":"BENCHMARK_MAP","extra":true}`,
		`{"address":"100 Example Street","latitude":42.9001,"longitude":-78.8801,"precision_digits":4,"provider":"BENCHMARK_MAP"} {}`,
	} {
		fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, body)
		}))
		service, err = NewMapping(MappingOptions{BaseURL: fixture.URL, APIToken: "private-map-token", HTTPClient: fixture.Client()})
		require.NoError(t, err)
		_, err = service.Geocode(context.Background(), "100 Example Street")
		require.Equal(t, contract.ErrorMapTemporarilyUnavailable, requireMappingServiceError(t, err).Payload.Error)
		fixture.Close()
	}

	oversized := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("x", mappingResponseLimit+1))
	}))
	defer oversized.Close()
	service, err = NewMapping(MappingOptions{BaseURL: oversized.URL, APIToken: "private-map-token", HTTPClient: oversized.Client()})
	require.NoError(t, err)
	_, err = service.Geocode(context.Background(), "100 Example Street")
	require.Equal(t, contract.ErrorMapTemporarilyUnavailable, requireMappingServiceError(t, err).Payload.Error)
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

type errorReadCloser struct{}

func (errorReadCloser) Read([]byte) (int, error) { return 0, errors.New("response read failed") }
func (errorReadCloser) Close() error             { return nil }

func requireMappingServiceError(t *testing.T, err error) *ServiceError {
	t.Helper()
	var serviceError *ServiceError
	require.ErrorAs(t, err, &serviceError)
	return serviceError
}
