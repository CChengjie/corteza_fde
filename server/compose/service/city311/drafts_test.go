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

func TestDraftLifecycleUsesConstituentOwnershipAndOptimisticConcurrency(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	user, err := store.LookupUserByHandle(ctx, st, "city311-constituent")
	require.NoError(t, err)

	summary := "Pothole draft"
	draft, err := svc.CreateDraft(ctx, user.ID, contract.PortalDraftWrite{Summary: &summary})
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusDraft, draft.Status)
	require.Empty(t, draft.RequestNumber)
	require.Equal(t, uint64(1), draft.Version)
	sequence, err := store.LookupCity311RequestSequenceByID(ctx, st, 2026)
	require.NoError(t, err)
	require.Equal(t, uint64(41), sequence.NextNumber)

	description := "A deep pothole is blocking the eastbound traffic lane."
	serviceType := contract.ServiceTypePothole
	latitude, longitude := 42.88645, -78.87837
	location := &contract.LocationInput{Address: "100 Example Street, Buffalo, NY 14201", Latitude: &latitude, Longitude: &longitude}
	requester := &contract.RequesterInput{DisplayName: "City 311 Constituent", Email: "constituent1@city311.example.invalid"}
	updated, err := svc.UpdateDraft(ctx, user.ID, draftID(draft), 1, contract.PortalDraftWrite{
		Description: &description, ServiceType: &serviceType, Requester: requester, Location: location,
	})
	require.NoError(t, err)
	require.Equal(t, uint64(2), updated.Version)
	require.Equal(t, contract.DepartmentStreets, updated.OwningDepartment)

	_, err = svc.UpdateDraft(ctx, user.ID, draftID(draft), 1, contract.PortalDraftWrite{Summary: &summary})
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 409, serviceErr.Status)
	require.Equal(t, uint64(2), *serviceErr.Payload.CurrentVersion)

	submitted, err := svc.SubmitDraft(ctx, user.ID, draftID(draft), 2)
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusSubmitted, submitted.Status)
	require.Equal(t, "SR-2026-00041", submitted.RequestNumber)
	require.Equal(t, uint64(3), submitted.Version)

	stored, err := store.LookupCity311ServiceRequestByID(ctx, st, draftID(draft))
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusSubmitted, stored.Status)
	history, _, err := store.SearchCity311PublicHistoryItems(ctx, st, composeTypes.City311PublicHistoryItemFilter{RequestID: stored.ID})
	require.NoError(t, err)
	require.Len(t, history, 1)
	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{RequestID: stored.ID})
	require.NoError(t, err)
	require.Len(t, audits, 3)

	_, err = svc.GetDraft(ctx, user.ID, stored.ID)
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 404, serviceErr.Status)
}

func TestDraftValidationAndForeignOwnershipDoNotWrite(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	first, err := store.LookupUserByHandle(ctx, st, "city311-constituent")
	require.NoError(t, err)
	second, err := store.LookupUserByHandle(ctx, st, "city311-constituent-two")
	require.NoError(t, err)

	invalidType := contract.ServiceType("UNKNOWN")
	_, err = svc.CreateDraft(ctx, first.ID, contract.PortalDraftWrite{ServiceType: &invalidType})
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 422, serviceErr.Status)

	summary := "Valid draft"
	draft, err := svc.CreateDraft(ctx, first.ID, contract.PortalDraftWrite{Summary: &summary})
	require.NoError(t, err)
	_, err = svc.GetDraft(ctx, second.ID, draftID(draft))
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 404, serviceErr.Status)
	_, err = svc.SubmitDraft(ctx, second.ID, draftID(draft), 1)
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 404, serviceErr.Status)

	tokens := []string{"unmerged-attachment-token"}
	_, err = svc.UpdateDraft(ctx, first.ID, draftID(draft), 1, contract.PortalDraftWrite{AttachmentTokens: &tokens})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 422, serviceErr.Status)

	unchanged, err := svc.GetDraft(ctx, first.ID, draftID(draft))
	require.NoError(t, err)
	require.Equal(t, uint64(1), unchanged.Version)
	require.Equal(t, contract.ServiceRequestStatusDraft, unchanged.Status)
	require.Empty(t, unchanged.RequestNumber)
}

func TestIncompleteDraftSubmitIsTransactional(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	user, err := store.LookupUserByHandle(ctx, st, "city311-constituent")
	require.NoError(t, err)

	draft, err := svc.CreateDraft(ctx, user.ID, contract.PortalDraftWrite{})
	require.NoError(t, err)
	require.Empty(t, draft.RequestNumber)

	_, err = svc.SubmitDraft(ctx, user.ID, draftID(draft), 1)
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 422, serviceErr.Status)

	stored, err := store.LookupCity311ServiceRequestByID(ctx, st, draftID(draft))
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusDraft, stored.Status)
	require.Equal(t, 1, stored.Version)
	require.Equal(t, internalDraftNumber(stored.ID), stored.RequestNumber)
	sequence, err := store.LookupCity311RequestSequenceByID(ctx, st, 2026)
	require.NoError(t, err)
	require.Equal(t, uint64(41), sequence.NextNumber)
}

func TestAuthorizedStaffCanListAndSubmitCompleteDraft(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	user, err := store.LookupUserByHandle(ctx, st, "city311-constituent")
	require.NoError(t, err)

	input := validSubmission()
	draft, err := svc.CreateDraft(ctx, user.ID, contract.PortalDraftWrite{
		Summary: &input.Summary, Description: &input.Description, ServiceType: &input.ServiceType,
		Requester: &input.Requester, Location: input.Location, CustomFields: &input.CustomFields,
	})
	require.NoError(t, err)
	actor := contract.Actor{
		ID: 100, Roles: []contract.ApplicationRole{contract.ApplicationRoleServiceAgent},
		Department: contract.DepartmentStreets, Districts: []contract.DistrictCode{contract.DistrictNorth},
	}

	queue, err := svc.List(ctx, actor, RequestFilter{Statuses: []contract.ServiceRequestStatus{contract.ServiceRequestStatusDraft}})
	require.NoError(t, err)
	var listed bool
	for _, item := range queue.Items {
		if item.RequestID == draft.RequestID {
			listed = true
			require.Empty(t, item.RequestNumber)
		}
	}
	require.True(t, listed)

	detail, err := svc.Transition(ctx, actor, draftID(draft), 1, contract.RequestTransition{ToStatus: contract.ServiceRequestStatusSubmitted})
	require.NoError(t, err)
	require.Equal(t, contract.ServiceRequestStatusSubmitted, detail.Request.Status)
	require.Equal(t, "SR-2026-00041", detail.Request.RequestNumber)
	require.Equal(t, uint64(2), detail.Request.Version)
	sequence, err := store.LookupCity311RequestSequenceByID(ctx, st, 2026)
	require.NoError(t, err)
	require.Equal(t, uint64(42), sequence.NextNumber)
}

func TestDeleteDraftPreservesAuditTrail(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	user, err := store.LookupUserByHandle(ctx, st, "city311-constituent")
	require.NoError(t, err)

	draft, err := svc.CreateDraft(ctx, user.ID, contract.PortalDraftWrite{})
	require.NoError(t, err)
	require.NoError(t, svc.DeleteDraft(ctx, user.ID, draftID(draft), 1))
	_, err = store.LookupCity311ServiceRequestByID(ctx, st, draftID(draft))
	require.Error(t, err)

	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{RequestID: draftID(draft)})
	require.NoError(t, err)
	require.Len(t, audits, 2)
	require.Equal(t, "DRAFT_CREATED", audits[0].EventType)
	require.Equal(t, "DRAFT_DELETED", audits[1].EventType)
}

func draftID(request *contract.ServiceRequest) uint64 {
	value, _ := strconv.ParseUint(request.RequestID, 10, 64)
	return value
}
