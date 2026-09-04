package city311

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateRuntimeEnvironment(t *testing.T) {
	setValidRuntimeEnvironment(t)
	require.NoError(t, ValidateRuntimeEnvironment())

	tests := []struct {
		name    string
		key     string
		value   string
		message string
	}{
		{name: "missing database", key: "DATABASE_URL", message: "DATABASE_URL is required"},
		{name: "wrong database scheme", key: "DATABASE_URL", value: "mysql://db.example.test/corteza", message: "absolute PostgreSQL URL"},
		{name: "missing database name", key: "DATABASE_URL", value: "postgres://db.example.test", message: "database name"},
		{name: "wrong timezone", key: "BENCHMARK_TIMEZONE", value: "UTC", message: "must be America/New_York"},
		{name: "missing fixture instant", key: "BENCHMARK_NOW", message: "BENCHMARK_NOW is required"},
		{name: "invalid fixture instant", key: "BENCHMARK_NOW", value: "2026-02-03", message: "RFC3339 instant"},
		{name: "missing seed", key: "BENCHMARK_SEED", message: "BENCHMARK_SEED is required"},
		{name: "malformed API client", key: "CRM_API_CLIENT_ID", value: " client ", message: "CRM_API_CLIENT_ID is malformed"},
		{name: "missing API secret", key: "CRM_API_CLIENT_SECRET", message: "CRM_API_CLIENT_SECRET is required"},
		{name: "missing SMTP host", key: "MAIL_SMTP_HOST", message: "MAIL_SMTP_HOST is required"},
		{name: "invalid SMTP port", key: "MAIL_SMTP_PORT", value: "70000", message: "integer between 1 and 65535"},
		{name: "missing SMTP username", key: "MAIL_SMTP_USERNAME", message: "MAIL_SMTP_USERNAME is required"},
		{name: "missing SMTP password", key: "MAIL_SMTP_PASSWORD", message: "MAIL_SMTP_PASSWORD is required"},
		{name: "invalid mail API URL", key: "MAIL_API_BASE_URL", value: "mail.example.test", message: "absolute HTTP or HTTPS URL"},
		{name: "mail API URL credentials", key: "MAIL_API_BASE_URL", value: "https://user@mail.example.test", message: "must not contain credentials"},
		{name: "missing mail API token", key: "MAIL_API_TOKEN", message: "MAIL_API_TOKEN is required"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			setValidRuntimeEnvironment(t)
			t.Setenv(test.key, test.value)
			require.ErrorContains(t, ValidateRuntimeEnvironment(), test.message)
		})
	}
}

func setValidRuntimeEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://corteza:secret@postgres:5432/corteza?sslmode=disable")
	t.Setenv("BENCHMARK_TIMEZONE", city311BusinessTimezone)
	t.Setenv("BENCHMARK_NOW", "2026-02-03T15:04:05Z")
	t.Setenv("BENCHMARK_SEED", "city311-public-v1")
	t.Setenv("CRM_API_CLIENT_ID", "city311-api-client")
	t.Setenv("CRM_API_CLIENT_SECRET", "runtime-api-secret")
	t.Setenv("MAIL_SMTP_HOST", "mail.example.test")
	t.Setenv("MAIL_SMTP_PORT", "587")
	t.Setenv("MAIL_SMTP_USERNAME", "city311")
	t.Setenv("MAIL_SMTP_PASSWORD", "runtime-mail-secret")
	t.Setenv("MAIL_API_BASE_URL", "https://mail.example.test")
	t.Setenv("MAIL_API_TOKEN", "runtime-mail-api-token")
}
