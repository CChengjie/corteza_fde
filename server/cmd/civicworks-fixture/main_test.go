package main

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRunBuildsFixtureServerFromEnvironment(t *testing.T) {
	t.Setenv("CIVICWORKS_API_TOKEN", "consumer-token")
	t.Setenv("CIVICWORKS_CONTROL_TOKEN", "control-token")
	t.Setenv("CIVICWORKS_WEBHOOK_SECRET", "webhook-secret")
	t.Setenv("BENCHMARK_RUN_ID", "run-41")
	t.Setenv("CIVICWORKS_PUBLIC_BASE_URL", "http://civicworks:8080")
	t.Setenv("CIVICWORKS_FIXTURE_ADDR", " 127.0.0.1:18081 ")

	called := false
	err := run(func(server *http.Server) error {
		called = true
		require.Equal(t, "127.0.0.1:18081", server.Addr)
		require.Equal(t, 5*time.Second, server.ReadHeaderTimeout)
		require.NotNil(t, server.Handler)
		return http.ErrServerClosed
	})
	require.NoError(t, err)
	require.True(t, called)
}

func TestRunUsesDefaultAddressAndReturnsServeError(t *testing.T) {
	t.Setenv("CIVICWORKS_API_TOKEN", "consumer-token")
	t.Setenv("CIVICWORKS_CONTROL_TOKEN", "control-token")
	t.Setenv("CIVICWORKS_WEBHOOK_SECRET", "webhook-secret")
	t.Setenv("BENCHMARK_RUN_ID", "run-41")
	t.Setenv("CIVICWORKS_PUBLIC_BASE_URL", "")
	t.Setenv("CIVICWORKS_FIXTURE_ADDR", "")

	expected := errors.New("listen failed")
	err := run(func(server *http.Server) error {
		require.Equal(t, ":8080", server.Addr)
		return expected
	})
	require.ErrorIs(t, err, expected)
}

func TestRunRejectsInvalidFixtureConfigurationBeforeServing(t *testing.T) {
	t.Setenv("CIVICWORKS_API_TOKEN", "same-token")
	t.Setenv("CIVICWORKS_CONTROL_TOKEN", "same-token")
	t.Setenv("CIVICWORKS_WEBHOOK_SECRET", "webhook-secret")
	t.Setenv("BENCHMARK_RUN_ID", "run-41")

	called := false
	err := run(func(*http.Server) error {
		called = true
		return nil
	})
	require.EqualError(t, err, "CIVICWORKS_API_TOKEN and CIVICWORKS_CONTROL_TOKEN must differ")
	require.False(t, called)
}
