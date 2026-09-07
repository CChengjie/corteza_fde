package city311

import (
	"context"
	"testing"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestOriginAndScopeOverrides(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	manager := seededAssignmentActor(t, ctx, svc, st, "department-manager@city311.example.invalid")
	administrator := seededAssignmentActor(t, ctx, svc, st, "platform-admin@city311.example.invalid")

	origin, err := svc.OverrideOrigin(ctx, manager, request.ID, 1, contract.OriginOverride{
		OriginClass: contract.OriginClassExternal, Reason: "Correct the classification after manager review.",
	})
	require.NoError(t, err)
	require.Equal(t, contract.OriginClassExternal, origin.Request.OriginClass)
	require.Equal(t, uint64(2), origin.Request.Version)
	originAudit := origin.Audit[len(origin.Audit)-1]
	require.Equal(t, "ORIGIN_CLASS_OVERRIDDEN", originAudit.EventType)
	require.Equal(t, contract.OriginClassInternal, contract.OriginClass(originAudit.Before["origin_class"].(string)))
	require.Equal(t, "Correct the classification after manager review.", originAudit.After["reason"])

	scoped, err := svc.OverrideScope(ctx, administrator, request.ID, 2, contract.ScopeOverride{
		DepartmentCode: contract.DepartmentSanitation,
		DistrictCodes:  []contract.DistrictCode{contract.DistrictSouth},
		Reason:         "Sanitation must coordinate work in the south district.",
	})
	require.NoError(t, err)
	require.Equal(t, uint64(3), scoped.Request.Version)
	scopeAudit := scoped.Audit[len(scoped.Audit)-1]
	require.Equal(t, "REQUEST_SCOPE_OVERRIDDEN", scopeAudit.EventType)
	require.Nil(t, scopeAudit.Before["department_code"])
	require.Equal(t, "Sanitation must coordinate work in the south district.", scopeAudit.After["reason"])

	persisted, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, contract.DepartmentSanitation, *persisted.ScopeDepartment)
	require.Equal(t, composeTypes.City311DistrictCodeSet{contract.DistrictSouth}, persisted.ScopeDistricts)

	targetAgent := contract.Actor{
		ID: 991, Roles: []contract.ApplicationRole{contract.ApplicationRoleServiceAgent},
		Department: contract.DepartmentSanitation, Districts: []contract.DistrictCode{contract.DistrictSouth},
	}
	_, err = svc.Find(ctx, targetAgent, request.ID)
	require.NoError(t, err)
	targetOtherDistrict := targetAgent
	targetOtherDistrict.Districts = []contract.DistrictCode{contract.DistrictNorth}
	_, err = svc.Find(ctx, targetOtherDistrict, request.ID)
	requireServiceError(t, err, 403, contract.ErrorForbidden)

	// Scope overrides are additive: the original owning department retains
	// access even after a second department receives visibility.
	_, err = svc.Find(ctx, manager, request.ID)
	require.NoError(t, err)
}

func TestOverridesRejectUnauthorizedInvalidAndStaleWrites(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	agent := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")
	manager := seededAssignmentActor(t, ctx, svc, st, "department-manager@city311.example.invalid")

	_, err = svc.OverrideOrigin(ctx, agent, request.ID, 1, contract.OriginOverride{OriginClass: contract.OriginClassExternal, Reason: "Unauthorized."})
	requireServiceError(t, err, 403, contract.ErrorForbidden)
	_, err = svc.OverrideOrigin(ctx, manager, request.ID, 0, contract.OriginOverride{OriginClass: contract.OriginClassExternal, Reason: "Missing version."})
	requireServiceError(t, err, 428, contract.ErrorExpectedVersionRequired)
	_, err = svc.OverrideOrigin(ctx, manager, request.ID, 1, contract.OriginOverride{OriginClass: "UNKNOWN", Reason: "Invalid class."})
	requireServiceError(t, err, 422, contract.ErrorValidation)
	_, err = svc.OverrideOrigin(ctx, manager, request.ID, 1, contract.OriginOverride{OriginClass: contract.OriginClassExternal, Reason: " "})
	requireServiceError(t, err, 422, contract.ErrorValidation)

	_, err = svc.OverrideScope(ctx, manager, request.ID, 1, contract.ScopeOverride{
		DepartmentCode: contract.DepartmentSanitation, DistrictCodes: []contract.DistrictCode{contract.DistrictSouth}, Reason: "Wrong department.",
	})
	requireServiceError(t, err, 403, contract.ErrorForbidden)
	_, err = svc.OverrideScope(ctx, manager, request.ID, 1, contract.ScopeOverride{
		DepartmentCode: manager.Department, DistrictCodes: []contract.DistrictCode{contract.DistrictNorth, contract.DistrictNorth}, Reason: "Duplicate district.",
	})
	requireServiceError(t, err, 422, contract.ErrorValidation)

	updated, err := svc.OverrideOrigin(ctx, manager, request.ID, 1, contract.OriginOverride{OriginClass: contract.OriginClassExternal, Reason: "Valid correction."})
	require.NoError(t, err)
	_, err = svc.OverrideScope(ctx, manager, request.ID, 1, contract.ScopeOverride{
		DepartmentCode: manager.Department, DistrictCodes: nil, Reason: "Stale version.",
	})
	requireServiceError(t, err, 409, contract.ErrorVersionConflict)
	require.Equal(t, uint64(2), updated.Request.Version)
}
