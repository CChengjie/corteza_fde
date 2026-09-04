package city311

import (
	"context"
	"strconv"
	"testing"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestStaffConstituentRelationshipsEnforceCardinalityVersionAndAudit(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	actor := relationshipServiceAgent()

	detail, err := svc.LinkConstituent(ctx, actor, request.ID, 1, contract.ConstituentLink{
		ConstituentID: "C-3", RelationshipType: contract.RelationshipAffectedResident,
		PortalVisible: true, NotifyStatus: true,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(2), detail.Request.Version)
	require.Len(t, detail.ConstituentLinks, 2)

	_, err = svc.LinkConstituent(ctx, actor, request.ID, 2, contract.ConstituentLink{
		ConstituentID: "C-3", RelationshipType: contract.RelationshipAffectedResident,
		PortalVisible: true, NotifyStatus: true,
	})
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 422, serviceErr.Status)

	detail, err = svc.LinkConstituent(ctx, actor, request.ID, 2, contract.ConstituentLink{
		ConstituentID: "C-3", RelationshipType: contract.RelationshipPropertyOwner,
		PortalVisible: true, NotifyStatus: true,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(3), detail.Request.Version)
	require.Len(t, detail.ConstituentLinks, 3)
	recipients, err := svc.RelationshipNotificationRecipients(ctx, request.ID, RelationshipNotificationStatusChange)
	require.NoError(t, err)
	require.Equal(t, []string{"C-2", "C-3"}, recipients)

	reason := "The resident is no longer associated with this request."
	detail, err = svc.UnlinkConstituent(ctx, actor, request.ID, 3, "C-3", contract.ConstituentUnlink{Reason: &reason})
	require.NoError(t, err)
	require.Equal(t, uint64(4), detail.Request.Version)
	require.Len(t, detail.ConstituentLinks, 1)

	_, err = svc.UnlinkConstituent(ctx, actor, request.ID, 4, "C-2", contract.ConstituentUnlink{Reason: &reason})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 422, serviceErr.Status)

	detail, err = svc.LinkConstituent(ctx, actor, request.ID, 4, contract.ConstituentLink{
		ConstituentID: "C-3", RelationshipType: contract.RelationshipPrimaryRequester,
		PortalVisible: true, NotifyStatus: true,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(5), detail.Request.Version)
	require.Len(t, detail.ConstituentLinks, 1)
	require.Equal(t, contract.RelationshipPrimaryRequester, detail.ConstituentLinks[0].RelationshipType)
	require.Equal(t, "C-3", detail.ConstituentLinks[0].ConstituentID)
	require.Equal(t, "C-3", detail.Request.PrimaryRequester.ConstituentID)

	primary, err := requestRelationships(ctx, st, request.ID, contract.RelationshipPrimaryRequester)
	require.NoError(t, err)
	require.Len(t, primary, 1)
	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{RequestID: request.ID})
	require.NoError(t, err)
	require.Len(t, audits, 5)
	require.Equal(t, "CONSTITUENT_LINKED", audits[1].EventType)
	require.Equal(t, "CONSTITUENT_UNLINKED", audits[3].EventType)

	_, err = svc.LinkConstituent(ctx, actor, request.ID, 4, contract.ConstituentLink{
		ConstituentID: "C-2", RelationshipType: contract.RelationshipReporter,
	})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 409, serviceErr.Status)
	require.Equal(t, uint64(5), *serviceErr.Payload.CurrentVersion)
}

func TestRelationshipPermissionsAndNotificationMatrix(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)

	manager := relationshipServiceAgent()
	manager.Roles = []contract.ApplicationRole{contract.ApplicationRoleDepartmentManager}
	_, err = svc.LinkConstituent(ctx, manager, request.ID, 1, contract.ConstituentLink{
		ConstituentID: "C-3", RelationshipType: contract.RelationshipReporter,
	})
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 403, serviceErr.Status)

	wrongDepartment := relationshipServiceAgent()
	wrongDepartment.Department = contract.DepartmentSanitation
	_, err = svc.LinkConstituent(ctx, wrongDepartment, request.ID, 1, contract.ConstituentLink{
		ConstituentID: "C-3", RelationshipType: contract.RelationshipReporter,
	})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 403, serviceErr.Status)

	baseID := uint64(980_000_000_000_000_000)
	now := svc.now()
	links := composeTypes.City311RequestConstituentSet{
		{ID: baseID + 1, RequestID: request.ID, ConstituentID: "C-AFFECTED-OFF", RelationshipType: contract.RelationshipAffectedResident, PortalVisible: true, NotifyStatus: false, CreatedAt: now, UpdatedAt: now},
		{ID: baseID + 2, RequestID: request.ID, ConstituentID: "C-REPORTER-HIDDEN", RelationshipType: contract.RelationshipReporter, PortalVisible: false, NotifyStatus: true, CreatedAt: now, UpdatedAt: now},
		{ID: baseID + 3, RequestID: request.ID, ConstituentID: "C-PROPERTY", RelationshipType: contract.RelationshipPropertyOwner, PortalVisible: true, NotifyStatus: true, CreatedAt: now, UpdatedAt: now},
		{ID: baseID + 4, RequestID: request.ID, ConstituentID: "C-ORG", RelationshipType: contract.RelationshipOrganisationContact, PortalVisible: true, NotifyStatus: true, CreatedAt: now, UpdatedAt: now},
	}
	require.NoError(t, store.CreateCity311RequestConstituentLink(ctx, st, links...))
	recipients, err := svc.RelationshipNotificationRecipients(ctx, request.ID, RelationshipNotificationClosed)
	require.NoError(t, err)
	require.Equal(t, []string{"C-2"}, recipients)

	for _, test := range []struct {
		relationship contract.RelationshipType
		visible      bool
		expected     bool
	}{
		{contract.RelationshipPrimaryRequester, true, true},
		{contract.RelationshipAffectedResident, true, true},
		{contract.RelationshipReporter, true, true},
		{contract.RelationshipPropertyOwner, true, false},
		{contract.RelationshipOrganisationContact, true, false},
		{contract.RelationshipReporter, false, false},
	} {
		require.Equal(t, test.expected, relationshipGrantsPortalView(&composeTypes.City311RequestConstituent{
			RelationshipType: test.relationship, PortalVisible: test.visible,
		}))
	}
}

func TestAnonymousLinkAndAuthenticatedSubmissionAppearInPortalRequests(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	user, err := store.LookupUserByHandle(ctx, st, "city311-constituent")
	require.NoError(t, err)
	otherUser, err := store.LookupUserByHandle(ctx, st, "city311-constituent-two")
	require.NoError(t, err)

	anonymousInput := validSubmission()
	anonymousInput.Requester.Email = "constituent1@city311.example.invalid"
	anonymous, _, err := svc.Submit(ctx, anonymousInput, "anonymous-link", SubmissionOptions{
		Operation: "portal_service_request_submit", SourceChannel: contract.SourceChannelPortalAnonymous,
		ActorType: contract.AuditActorConstituent, RequireIdempotency: true,
	})
	require.NoError(t, err)
	requestID, err := strconv.ParseUint(anonymous.RequestID, 10, 64)
	require.NoError(t, err)
	_, err = svc.LinkAnonymousRequest(ctx, otherUser.ID, contract.AnonymousRequestLink{
		RequestNumber: anonymous.RequestNumber, Email: anonymousInput.Requester.Email,
	})
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 404, serviceErr.Status)

	before, err := svc.ListPortalRequests(ctx, user.ID, PortalRequestFilter{})
	require.NoError(t, err)
	require.Empty(t, before.Items)
	_, err = svc.LinkAnonymousRequest(ctx, user.ID, contract.AnonymousRequestLink{
		RequestNumber: anonymous.RequestNumber, Email: "wrong@example.invalid",
	})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 404, serviceErr.Status)
	unchanged, err := store.LookupCity311ServiceRequestByID(ctx, st, requestID)
	require.NoError(t, err)
	require.Equal(t, 1, unchanged.Version)

	linked, err := svc.LinkAnonymousRequest(ctx, user.ID, contract.AnonymousRequestLink{
		RequestNumber: anonymous.RequestNumber, Email: anonymousInput.Requester.Email,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(2), linked.Version)
	require.Equal(t, "C-"+strconv.FormatUint(user.ID, 10), linked.PrimaryRequester.ConstituentID)

	after, err := svc.ListPortalRequests(ctx, user.ID, PortalRequestFilter{PageSize: 1, Sort: "-updated_at"})
	require.NoError(t, err)
	require.Equal(t, 1, after.TotalCount)
	require.Len(t, after.Items, 1)
	require.Equal(t, anonymous.RequestNumber, after.Items[0].RequestNumber)

	authenticated, _, err := svc.Submit(ctx, validSubmission(), "authenticated-link", SubmissionOptions{
		Operation: "portal_service_request_submit", SourceChannel: contract.SourceChannelPortalAuthenticated,
		ActorType: contract.AuditActorConstituent, ActorID: user.ID, RequireIdempotency: true,
	})
	require.NoError(t, err)
	after, err = svc.ListPortalRequests(ctx, user.ID, PortalRequestFilter{PageSize: 1, Sort: "-updated_at"})
	require.NoError(t, err)
	require.Equal(t, 2, after.TotalCount)
	require.Len(t, after.Items, 1)
	require.NotNil(t, after.NextPageToken)
	secondPage, err := svc.ListPortalRequests(ctx, user.ID, PortalRequestFilter{PageSize: 1, PageToken: *after.NextPageToken, Sort: "-updated_at"})
	require.NoError(t, err)
	require.Len(t, secondPage.Items, 1)
	numbers := map[string]bool{after.Items[0].RequestNumber: true, secondPage.Items[0].RequestNumber: true}
	require.True(t, numbers[anonymous.RequestNumber])
	require.True(t, numbers[authenticated.RequestNumber])
}

func TestSeedBackfillsExactlyOnePrimaryRelationship(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	require.NoError(t, svc.Seed(ctx, svc.now()))
	requests, _, err := store.SearchCity311ServiceRequests(ctx, st, composeTypes.City311ServiceRequestFilter{})
	require.NoError(t, err)
	for _, request := range requests {
		primary, relationshipErr := requestRelationships(ctx, st, request.ID, contract.RelationshipPrimaryRequester)
		require.NoError(t, relationshipErr)
		require.Len(t, primary, 1)
	}
}

func relationshipServiceAgent() contract.Actor {
	return contract.Actor{
		ID: 100, Roles: []contract.ApplicationRole{contract.ApplicationRoleServiceAgent},
		Department: contract.DepartmentStreets, Districts: []contract.DistrictCode{contract.DistrictNorth},
	}
}
