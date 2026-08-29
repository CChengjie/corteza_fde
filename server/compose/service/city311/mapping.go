package city311

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
)

const (
	mappingGeocodePath    = "/internal/integrations/mapping/geocode"
	mappingResponseLimit  = 1 << 20
	mappingRequestTimeout = 5 * time.Second
)

type (
	MappingOptions struct {
		BaseURL    string
		APIToken   string
		HTTPClient *http.Client
	}

	MappingService struct {
		endpoint   string
		apiToken   string
		httpClient *http.Client
	}

	mappingRequest struct {
		Address string `json:"address"`
	}
)

func NewMapping(options MappingOptions) (*MappingService, error) {
	baseURL, err := validateMappingOptions(options.BaseURL, options.APIToken)
	if err != nil {
		return nil, err
	}

	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + mappingGeocodePath
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: mappingRequestTimeout}
	}

	return &MappingService{
		endpoint:   baseURL.String(),
		apiToken:   options.APIToken,
		httpClient: client,
	}, nil
}

func NewMappingFromEnvironment(client *http.Client) (*MappingService, error) {
	return NewMapping(MappingOptions{
		BaseURL:    os.Getenv("MAP_BASE_URL"),
		APIToken:   os.Getenv("MAP_API_TOKEN"),
		HTTPClient: client,
	})
}

func ValidateMappingEnvironment() error {
	_, err := validateMappingOptions(os.Getenv("MAP_BASE_URL"), os.Getenv("MAP_API_TOKEN"))
	return err
}

func validateMappingOptions(rawBaseURL, apiToken string) (*url.URL, error) {
	if strings.TrimSpace(rawBaseURL) == "" {
		return nil, fmt.Errorf("MAP_BASE_URL is required")
	}
	baseURL, err := url.Parse(rawBaseURL)
	if err != nil || baseURL.Host == "" || (baseURL.Scheme != "http" && baseURL.Scheme != "https") {
		return nil, fmt.Errorf("MAP_BASE_URL must be an absolute HTTP or HTTPS URL")
	}
	if baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" {
		return nil, fmt.Errorf("MAP_BASE_URL must not contain credentials, a query, or a fragment")
	}
	if strings.TrimSpace(apiToken) == "" {
		return nil, fmt.Errorf("MAP_API_TOKEN is required")
	}
	if apiToken != strings.TrimSpace(apiToken) || strings.ContainsAny(apiToken, "\r\n") {
		return nil, fmt.Errorf("MAP_API_TOKEN is malformed")
	}
	return baseURL, nil
}

func (svc *MappingService) Geocode(ctx context.Context, address string) (*contract.GeocodeResponse, error) {
	address = strings.TrimSpace(address)
	if address == "" {
		return nil, validationError(contract.FieldError{Field: "/address", Code: contract.ValidationRequired})
	}

	body, err := json.Marshal(mappingRequest{Address: address})
	if err != nil {
		return nil, mappingUnavailableError()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, svc.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, mappingUnavailableError()
	}
	req.Header.Set("Authorization", "Bearer "+svc.apiToken)
	req.Header.Set("Content-Type", "application/json")

	response, err := svc.httpClient.Do(req)
	if err != nil {
		return nil, mappingUnavailableError()
	}
	defer response.Body.Close()

	limited := io.LimitReader(response.Body, mappingResponseLimit+1)
	responseBody, err := io.ReadAll(limited)
	if err != nil || len(responseBody) > mappingResponseLimit {
		return nil, mappingUnavailableError()
	}

	switch response.StatusCode {
	case http.StatusOK:
		result := &contract.GeocodeResponse{}
		if err = decodeMappingResponse(responseBody, result); err != nil {
			return nil, mappingUnavailableError()
		}
		return result, nil
	case http.StatusNotFound:
		if !isAddressNotFoundResponse(responseBody) {
			return nil, mappingUnavailableError()
		}
		return nil, &ServiceError{Status: http.StatusNotFound, Payload: contract.MockGeocodeNotFound()}
	default:
		// Authentication, transport, malformed-response, and controlled-outage
		// failures are intentionally collapsed into the browser contract's one
		// retryable mapping-service outcome. The server-side credential and the
		// fixture's diagnostic body never cross the proxy boundary.
		return nil, mappingUnavailableError()
	}
}

func isAddressNotFoundResponse(data []byte) bool {
	payload := contract.APIError{}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return false
	}
	return payload.Error == contract.ErrorAddressNotFound && !payload.Retryable
}

func decodeMappingResponse(data []byte, result *contract.GeocodeResponse) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(result); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("mapping response contains trailing data")
	}
	if strings.TrimSpace(result.Address) == "" || result.Latitude < -90 || result.Latitude > 90 ||
		result.Longitude < -180 || result.Longitude > 180 || result.PrecisionDigits != 4 || result.Provider != "BENCHMARK_MAP" {
		return fmt.Errorf("mapping response does not satisfy the City 311 contract")
	}
	return nil
}

func MappingUnavailablePayload() contract.APIError { return contract.MockGeocodeUnavailable() }

func mappingUnavailableError() *ServiceError {
	return &ServiceError{Status: http.StatusServiceUnavailable, Payload: MappingUnavailablePayload()}
}
