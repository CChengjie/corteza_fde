package city311

import (
	"context"
	"encoding/csv"
	"strings"
	"testing"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestContactEmailExportEligibilityScopeCSVAndAudit(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	manager := seededAssignmentActor(t, ctx, svc, st, "department-manager@city311.example.invalid")
	administrator := seededAssignmentActor(t, ctx, svc, st, "platform-admin@city311.example.invalid")

	account := &composeTypes.City311LocalAccount{
		ID: 7001, LoginIdentifier: "eligible", VerifiedEmail: "eligible@example.invalid",
		PreferredLanguage: string(contract.LanguageES), CreatedAt: svc.now(), UpdatedAt: svc.now(),
	}
	require.NoError(t, store.CreateCity311LocalAccount(ctx, st, account))
	require.NoError(t, store.CreateCity311Constituent(ctx, st, &composeTypes.City311Constituent{
		ID: svc.nextID(), ConstituentID: "C-7001", OwningDepartment: contract.DepartmentStreets, CouncilDistrict: contract.DistrictNorth,
		Profile: composeTypes.City311JSON{
			"constituent_id": "C-7001", "display_name": "Eligible, Resident", "emails": []string{"ELIGIBLE@example.invalid"},
			"phone_numbers": []any{}, "addresses": []any{}, "primary_category": string(contract.ContactCategoryResident),
			"preferred_language": string(contract.LanguageES), "email_opt_out": false,
		}, CreatedAt: svc.now(), UpdatedAt: svc.now(),
	}))
	require.NoError(t, store.CreateCity311Constituent(ctx, st, &composeTypes.City311Constituent{
		ID: svc.nextID(), ConstituentID: "C-unverified", OwningDepartment: contract.DepartmentStreets,
		Profile: composeTypes.City311JSON{
			"constituent_id": "C-unverified", "display_name": "Unverified", "emails": []string{"unverified@example.invalid"},
			"phone_numbers": []any{}, "addresses": []any{}, "primary_category": string(contract.ContactCategoryResident),
			"preferred_language": string(contract.LanguageEN), "email_opt_out": false,
		}, CreatedAt: svc.now(), UpdatedAt: svc.now(),
	}))

	pending, err := svc.StartContactEmailExport(ctx, manager, contract.ContactEmailExport{Filters: map[string][]string{
		"email": {"eligible@example.invalid"}, "department": {"STREETS"}, "district": {"NORTH"},
		"primary_category": {"RESIDENT"}, "preferred_language": {"ES"},
	}})
	require.NoError(t, err)
	require.Equal(t, contract.OperationStatusPending, pending.Status)
	download, err := svc.DownloadOperation(ctx, manager, pending.OperationID)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(download.Filename, "contact-emails-"))
	require.Equal(t, "text/csv; charset=utf-8", download.ContentType)
	require.Contains(t, string(download.Content), "\r\n")
	rows, err := csv.NewReader(strings.NewReader(string(download.Content))).ReadAll()
	require.NoError(t, err)
	require.Equal(t, [][]string{
		{"email", "display_name", "primary_category", "preferred_language", "opt_out"},
		{"eligible@example.invalid", "Eligible, Resident", "RESIDENT", "ES", "false"},
	}, rows)

	outside, err := svc.StartContactEmailExport(ctx, manager, contract.ContactEmailExport{Filters: map[string][]string{"department": {"SANITATION"}}})
	require.NoError(t, err)
	outDownload, err := svc.DownloadOperation(ctx, manager, outside.OperationID)
	require.NoError(t, err)
	outRows, err := csv.NewReader(strings.NewReader(string(outDownload.Content))).ReadAll()
	require.NoError(t, err)
	require.Len(t, outRows, 1)

	all, err := svc.StartContactEmailExport(ctx, administrator, contract.ContactEmailExport{Filters: map[string][]string{}})
	require.NoError(t, err)
	allDownload, err := svc.DownloadOperation(ctx, administrator, all.OperationID)
	require.NoError(t, err)
	allRows, err := csv.NewReader(strings.NewReader(string(allDownload.Content))).ReadAll()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(allRows), 2)
	seen := map[string]bool{}
	for _, row := range allRows[1:] {
		require.False(t, seen[row[0]], row[0])
		seen[row[0]] = true
		require.Equal(t, "false", row[4])
		require.NotEqual(t, "unverified@example.invalid", row[0])
	}

	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: contactEmailExportAuditEvent})
	require.NoError(t, err)
	require.Len(t, audits, 3)
	require.Equal(t, contract.AuditActorStaff, audits[0].ActorType)
}

func TestContactEmailExportValidationAndRole(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	manager := seededAssignmentActor(t, ctx, svc, st, "department-manager@city311.example.invalid")
	agent := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")

	_, err := svc.StartContactEmailExport(ctx, agent, contract.ContactEmailExport{Filters: map[string][]string{}})
	requireServiceError(t, err, 403, contract.ErrorForbidden)
	for _, filters := range []map[string][]string{
		{"unknown": {"value"}},
		{"department": {"UNKNOWN"}},
		{"district": {"UNKNOWN"}},
		{"primary_category": {"UNKNOWN"}},
		{"preferred_language": {"UNKNOWN"}},
		{"email": {"not-an-email"}},
		{"email": {}},
	} {
		_, err = svc.StartContactEmailExport(ctx, manager, contract.ContactEmailExport{Filters: filters})
		requireServiceError(t, err, 422, contract.ErrorValidation)
	}
}
