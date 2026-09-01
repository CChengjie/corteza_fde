package helpers

import (
	"context"
	"fmt"
	"os"

	"github.com/cortezaproject/corteza/server/app"
	"github.com/cortezaproject/corteza/server/pkg/cli"
	"github.com/cortezaproject/corteza/server/pkg/logger"
	"github.com/cortezaproject/corteza/server/pkg/options"
	"github.com/cortezaproject/corteza/server/pkg/rand"
	"github.com/cortezaproject/corteza/server/system/types"

	// Explicitly register SQLite (not done in the app as for testing only)
	_ "github.com/cortezaproject/corteza/server/store/adapters/rdbms/drivers/sqlite"
)

func NewIntegrationTestApp(ctx context.Context, initTestServices func(*app.CortezaApp) error) *app.CortezaApp {
	// Enforce debug logger for tests
	logger.SetDefault(logger.MakeDebugLogger())

	var (
		a = app.New()
	)

	a.Opt = options.Init()

	// When running integration tests, we want to upgrade the db. Always.
	a.Opt.Upgrade.Always = true

	// Create a new JWT secret (to prevent any security weirdness)
	a.Opt.Auth.Secret = string(rand.Bytes(32))
	a.Opt.Auth.DefaultClient = ""
	// City 311 requires the same runtime-only identity inputs in production.
	// Supply isolated values in the integration harness without weakening the
	// application startup validation.
	if os.Getenv("SESSION_SECRET") == "" {
		_ = os.Setenv("SESSION_SECRET", string(rand.Bytes(32)))
	}
	if os.Getenv("APP_BASE_URL") == "" {
		_ = os.Setenv("APP_BASE_URL", "http://city311.integration.test")
	}
	if os.Getenv("CITY311_SEED_CONSTITUENT_PASSWORD") == "" {
		_ = os.Setenv("CITY311_SEED_CONSTITUENT_PASSWORD", fmt.Sprintf("Seed-%x-Aa1!", rand.Bytes(16)))
	}
	if os.Getenv("CITY311_SEED_CONSTITUENT_TWO_PASSWORD") == "" {
		_ = os.Setenv("CITY311_SEED_CONSTITUENT_TWO_PASSWORD", fmt.Sprintf("Seed-%x-Aa1!", rand.Bytes(16)))
	}

	a.Log = logger.Default()

	a.DefaultAuthClient = &types.AuthClient{ID: 1, Handle: "test-auth-client", Secret: "integration-tests"}

	cli.HandleError(a.InitStore(ctx))
	cli.HandleError(initTestServices(a))
	cli.HandleError(a.InitServices(ctx))
	return a
}
