package city311

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const city311BusinessTimezone = "America/New_York"

// ValidateRuntimeEnvironment verifies the public Chapter 13 configuration
// which is not owned by an individual integration service.
func ValidateRuntimeEnvironment() error {
	if err := validateDatabaseURL(os.Getenv("DATABASE_URL")); err != nil {
		return err
	}
	if timezone := strings.TrimSpace(os.Getenv("BENCHMARK_TIMEZONE")); timezone != city311BusinessTimezone {
		return fmt.Errorf("BENCHMARK_TIMEZONE must be %s", city311BusinessTimezone)
	}
	benchmarkNow := strings.TrimSpace(os.Getenv("BENCHMARK_NOW"))
	if benchmarkNow == "" {
		return fmt.Errorf("BENCHMARK_NOW is required")
	}
	if _, err := time.Parse(time.RFC3339, benchmarkNow); err != nil {
		return fmt.Errorf("BENCHMARK_NOW must be an RFC3339 instant")
	}
	for _, key := range []string{"BENCHMARK_SEED", "CRM_API_CLIENT_ID", "CRM_API_CLIENT_SECRET"} {
		if err := validateRuntimeValue(key, os.Getenv(key)); err != nil {
			return err
		}
	}
	if err := validateMailEnvironment(); err != nil {
		return err
	}
	return nil
}

func validateDatabaseURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" || strings.Trim(parsed.Path, "/") == "" {
		return fmt.Errorf("DATABASE_URL must be an absolute PostgreSQL URL with a database name")
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("DATABASE_URL must not contain a fragment")
	}
	return nil
}

func validateMailEnvironment() error {
	for _, key := range []string{"MAIL_SMTP_HOST", "MAIL_SMTP_USERNAME", "MAIL_SMTP_PASSWORD", "MAIL_API_TOKEN"} {
		if err := validateRuntimeValue(key, os.Getenv(key)); err != nil {
			return err
		}
	}
	port, err := strconv.Atoi(strings.TrimSpace(os.Getenv("MAIL_SMTP_PORT")))
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("MAIL_SMTP_PORT must be an integer between 1 and 65535")
	}
	apiBaseURL := strings.TrimSpace(os.Getenv("MAIL_API_BASE_URL"))
	parsed, err := url.Parse(apiBaseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("MAIL_API_BASE_URL must be an absolute HTTP or HTTPS URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("MAIL_API_BASE_URL must not contain credentials, a query, or a fragment")
	}
	return nil
}

func validateRuntimeValue(key, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", key)
	}
	if value != strings.TrimSpace(value) || strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("%s is malformed", key)
	}
	return nil
}
