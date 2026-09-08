package city311

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/cortezaproject/corteza/server/store/adapters/rdbms/drivers/postgres"
	"github.com/cortezaproject/corteza/server/store/adapters/rdbms/drivers/sqlite"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

var identityConfigurationRaceSequence atomic.Uint64

func TestIdentityConfigurationWritesUseDatabaseAtomicConcurrency(t *testing.T) {
	ctx := context.Background()
	dsn := fmt.Sprintf("sqlite3://file:%s?_busy_timeout=5000&_journal_mode=WAL", filepath.Join(t.TempDir(), "city311.db"))
	st, err := sqlite.Connect(ctx, dsn)
	require.NoError(t, err)
	for round := 1; round <= 20; round++ {
		t.Run(fmt.Sprintf("round-%02d", round), func(t *testing.T) {
			testIdentityConfigurationWritesUseDatabaseAtomicConcurrencyOnce(t, st)
		})
	}
}

func TestIdentityConfigurationWritesUseDatabaseAtomicConcurrencyPostgreSQL(t *testing.T) {
	dsn := os.Getenv("CITY311_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("CITY311_POSTGRES_DSN is not configured")
	}
	st, err := postgres.Connect(context.Background(), dsn)
	require.NoError(t, err)
	testIdentityConfigurationWritesUseDatabaseAtomicConcurrencyOnce(t, st)
}

func testIdentityConfigurationWritesUseDatabaseAtomicConcurrencyOnce(t *testing.T, st store.Storer) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, store.Upgrade(ctx, zap.NewNop(), st))
	fixedNow := time.Date(2026, 2, 3, 15, 4, 5, 0, time.UTC)

	runtime := &IdentityRuntimeConfiguration{
		BaseURL: "https://city311.example.test", OIDCIssuerURL: "https://identity.example.test",
		OIDCStaffClientID: "city311-staff", OIDCPublicClientID: "city311-public", OIDCClientSecret: "configured",
		SAMLMetadataURL: "https://identity.example.test/saml/metadata", SAMLServiceProvider: "https://city311.example.test/saml",
	}
	sequence := identityConfigurationRaceSequence.Add(1)
	var nextID atomic.Uint64
	nextID.Store(940_000_000_000_000_000 + sequence*1_000_000)
	options := func() IdentityOptions {
		return IdentityOptions{
			Secret: []byte("identity-configuration-concurrency-secret"), Runtime: runtime,
			Now: func() time.Time { return fixedNow }, NextID: func() uint64 { return nextID.Add(1) },
		}
	}
	first := NewIdentity(st, options())
	second := NewIdentity(st, options())
	administrator := contract.Actor{ID: 44, Roles: []contract.ApplicationRole{contract.ApplicationRolePlatformAdministrator}}
	configuration, err := first.IdentityConfiguration(ctx, administrator)
	require.NoError(t, err)
	expectedVersion := configuration.Version
	auditsBefore, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: "IDENTITY_CONFIGURATION_UPDATED"})
	require.NoError(t, err)

	type result struct {
		configuration *contract.IdentityConfiguration
		err           error
	}
	ready := sync.WaitGroup{}
	ready.Add(2)
	start := make(chan struct{})
	results := make(chan result, 2)
	run := func(identity *IdentityService, input contract.IdentityConfigurationWrite) {
		ready.Done()
		<-start
		configuration, updateErr := identity.UpdateIdentityConfiguration(ctx, administrator, expectedVersion, input)
		results <- result{configuration: configuration, err: updateErr}
	}
	go run(first, contract.IdentityConfigurationWrite{OIDCEnabled: boolPointer(false)})
	go run(second, contract.IdentityConfigurationWrite{SAMLEnabled: boolPointer(false)})
	ready.Wait()
	close(start)
	firstResult, secondResult := <-results, <-results
	if firstResult.err == nil {
		firstResult, secondResult = secondResult, firstResult
	}
	require.NoError(t, secondResult.err)
	require.Equal(t, expectedVersion+1, secondResult.configuration.Version)
	requireIdentityError(t, firstResult.err, http.StatusConflict, contract.ErrorVersionConflict)

	revisions, _, err := store.SearchCity311ConfigurationRevisions(ctx, st, composeTypes.City311ConfigurationRevisionFilter{
		ResourceType: configurationIdentity, ResourceKey: identityConfigurationKey,
	})
	require.NoError(t, err)
	require.Len(t, revisions, int(expectedVersion+1))
	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: "IDENTITY_CONFIGURATION_UPDATED"})
	require.NoError(t, err)
	require.Len(t, audits, len(auditsBefore)+1, "the stale writer must not append an audit event")
}
