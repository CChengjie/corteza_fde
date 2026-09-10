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

var integrationAdministrationRaceSequence atomic.Uint64

func TestIntegrationAdministrationUsesDatabaseAtomicConcurrency(t *testing.T) {
	setIntegrationEnvironment(t)
	ctx := context.Background()
	dsn := fmt.Sprintf("sqlite3://file:%s?_busy_timeout=5000&_journal_mode=WAL", filepath.Join(t.TempDir(), "city311.db"))
	st, err := sqlite.Connect(ctx, dsn)
	require.NoError(t, err)
	for round := 1; round <= 20; round++ {
		t.Run(fmt.Sprintf("round-%02d", round), func(t *testing.T) {
			testIntegrationAdministrationUsesDatabaseAtomicConcurrencyOnce(t, st)
		})
	}
}

func TestIntegrationAdministrationUsesDatabaseAtomicConcurrencyPostgreSQL(t *testing.T) {
	dsn := os.Getenv("CITY311_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("CITY311_POSTGRES_DSN is not configured")
	}
	setIntegrationEnvironment(t)
	st, err := postgres.Connect(context.Background(), dsn)
	require.NoError(t, err)
	testIntegrationAdministrationUsesDatabaseAtomicConcurrencyOnce(t, st)
}

func testIntegrationAdministrationUsesDatabaseAtomicConcurrencyOnce(t *testing.T, st store.Storer) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, store.Upgrade(ctx, zap.NewNop(), st))
	fixedNow := time.Date(2026, 2, 3, 15, 4, 5, 0, time.UTC)
	sequence := integrationAdministrationRaceSequence.Add(1)
	var nextID atomic.Uint64
	nextID.Store(955_000_000_000_000_000 + sequence*1_000_000)
	newService := func() *Service {
		svc := New(st)
		svc.now = func() time.Time { return fixedNow }
		svc.nextID = func() uint64 { return nextID.Add(1) }
		return svc
	}
	first, second := newService(), newService()
	require.NoError(t, first.Seed(ctx, fixedNow))
	administrator := contract.Actor{ID: 44, Roles: []contract.ApplicationRole{contract.ApplicationRolePlatformAdministrator}}

	type result struct {
		connection *contract.IntegrationConnection
		err        error
	}
	runRace := func(eventType string, invokeFirst, invokeSecond func(*Service) (*contract.IntegrationConnection, error)) {
		t.Helper()
		revisionsBefore, _, err := store.SearchCity311ConfigurationRevisions(ctx, st, composeTypes.City311ConfigurationRevisionFilter{
			ResourceType: configurationIntegration, ResourceKey: IntegrationMappingID,
		})
		require.NoError(t, err)
		auditsBefore, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: eventType})
		require.NoError(t, err)

		ready := sync.WaitGroup{}
		ready.Add(2)
		start := make(chan struct{})
		results := make(chan result, 2)
		run := func(svc *Service, invoke func(*Service) (*contract.IntegrationConnection, error)) {
			ready.Done()
			<-start
			connection, writeErr := invoke(svc)
			results <- result{connection: connection, err: writeErr}
		}
		go run(first, invokeFirst)
		go run(second, invokeSecond)
		ready.Wait()
		close(start)
		firstResult, secondResult := <-results, <-results
		if firstResult.err == nil {
			firstResult, secondResult = secondResult, firstResult
		}
		require.NoError(t, secondResult.err)
		require.NotNil(t, secondResult.connection)
		requireServiceError(t, firstResult.err, http.StatusConflict, contract.ErrorVersionConflict)

		revisions, _, err := store.SearchCity311ConfigurationRevisions(ctx, st, composeTypes.City311ConfigurationRevisionFilter{
			ResourceType: configurationIntegration, ResourceKey: IntegrationMappingID,
		})
		require.NoError(t, err)
		require.Len(t, revisions, len(revisionsBefore)+1, "the stale writer must not append a revision")
		audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: eventType})
		require.NoError(t, err)
		require.Len(t, audits, len(auditsBefore)+1, "the stale writer must not append an audit event")
	}

	current, err := first.GetIntegration(ctx, administrator, IntegrationMappingID)
	require.NoError(t, err)
	active := true
	firstSecret, secondSecret := "mapping-update-secret-first", "mapping-update-secret-second"
	runRace("INTEGRATION_UPDATED",
		func(svc *Service) (*contract.IntegrationConnection, error) {
			return svc.UpdateIntegration(ctx, administrator, IntegrationMappingID, current.Version, contract.IntegrationConnectionWrite{
				Active: &active, Configuration: map[string]any{"base_url": "https://mapping-first.example.test"}, Secret: &firstSecret,
			})
		},
		func(svc *Service) (*contract.IntegrationConnection, error) {
			return svc.UpdateIntegration(ctx, administrator, IntegrationMappingID, current.Version, contract.IntegrationConnectionWrite{
				Active: &active, Configuration: map[string]any{"base_url": "https://mapping-second.example.test"}, Secret: &secondSecret,
			})
		},
	)

	current, err = first.GetIntegration(ctx, administrator, IntegrationMappingID)
	require.NoError(t, err)
	rotationVersion := current.Version
	runRace("INTEGRATION_SECRET_ROTATED",
		func(svc *Service) (*contract.IntegrationConnection, error) {
			return svc.RotateIntegrationSecret(ctx, administrator, IntegrationMappingID, rotationVersion, contract.SecretRotation{NewSecret: "mapping-rotation-secret-first"})
		},
		func(svc *Service) (*contract.IntegrationConnection, error) {
			return svc.RotateIntegrationSecret(ctx, administrator, IntegrationMappingID, rotationVersion, contract.SecretRotation{NewSecret: "mapping-rotation-secret-second"})
		},
	)

	current, err = first.GetIntegration(ctx, administrator, IntegrationMappingID)
	require.NoError(t, err)
	revocationVersion := current.Version
	runRace("INTEGRATION_REVOKED",
		func(svc *Service) (*contract.IntegrationConnection, error) {
			return svc.RevokeIntegration(ctx, administrator, IntegrationMappingID, revocationVersion, contract.Reason{Reason: "first revocation"})
		},
		func(svc *Service) (*contract.IntegrationConnection, error) {
			return svc.RevokeIntegration(ctx, administrator, IntegrationMappingID, revocationVersion, contract.Reason{Reason: "second revocation"})
		},
	)
}
