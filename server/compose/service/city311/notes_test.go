package city311

import (
	"context"
	"strings"
	"testing"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestRequestNotesAreAppendOnlyScopedAndAudited(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	visible := true

	constituentNote, err := svc.CreatePortalNote(ctx, 2, request.ID, contract.RequestNoteWrite{
		Body: "  Please inspect the east side of the street.  ", PortalVisible: &visible,
	})
	require.NoError(t, err)
	require.Equal(t, "Please inspect the east side of the street.", constituentNote.Body)
	require.Equal(t, "C-2", constituentNote.AuthorConstituentID)
	require.Equal(t, contract.AuditActorConstituent, constituentNote.AuthorType)
	require.True(t, constituentNote.PortalVisible)

	hidden := false
	staffNote, err := svc.CreateStaffNote(ctx, relationshipServiceAgent(), request.ID, contract.RequestNoteWrite{
		Body: "Internal routing note", PortalVisible: &hidden,
	})
	require.NoError(t, err)
	require.Equal(t, contract.AuditActorStaff, staffNote.AuthorType)
	require.Empty(t, staffNote.AuthorConstituentID)
	require.False(t, staffNote.PortalVisible)

	persistedRequest, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, 1, persistedRequest.Version, "appending a note must not mutate the request version")

	notes, _, err := store.SearchCity311RequestNotes(ctx, st, composeTypes.City311RequestNoteFilter{RequestID: request.ID})
	require.NoError(t, err)
	require.Len(t, notes, 2)
	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{
		RequestID: request.ID, EventType: "REQUEST_NOTE_CREATED",
	})
	require.NoError(t, err)
	require.Len(t, audits, 2)
	require.Equal(t, "request_note", audits[0].EntityType)

	detail, err := svc.Find(ctx, relationshipServiceAgent(), request.ID)
	require.NoError(t, err)
	require.Len(t, detail.Notes, 2)
	require.Equal(t, constituentNote.NoteID, detail.Notes[0].NoteID)
	require.Equal(t, staffNote.NoteID, detail.Notes[1].NoteID)
}

func TestRequestNoteValidationAndPermissions(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	visible := true

	_, err = svc.CreatePortalNote(ctx, 3, request.ID, contract.RequestNoteWrite{Body: "Not linked", PortalVisible: &visible})
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 404, serviceErr.Status)

	hidden := false
	_, err = svc.CreatePortalNote(ctx, 2, request.ID, contract.RequestNoteWrite{Body: "Hidden portal note", PortalVisible: &hidden})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 422, serviceErr.Status)

	manager := relationshipServiceAgent()
	manager.Roles = []contract.ApplicationRole{contract.ApplicationRoleDepartmentManager}
	_, err = svc.CreateStaffNote(ctx, manager, request.ID, contract.RequestNoteWrite{Body: "Manager note", PortalVisible: &hidden})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 403, serviceErr.Status)

	wrongDepartment := relationshipServiceAgent()
	wrongDepartment.Department = contract.DepartmentSanitation
	_, err = svc.CreateStaffNote(ctx, wrongDepartment, request.ID, contract.RequestNoteWrite{Body: "Out of scope", PortalVisible: &hidden})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 403, serviceErr.Status)

	_, err = svc.CreateStaffNote(ctx, relationshipServiceAgent(), request.ID, contract.RequestNoteWrite{Body: "   ", PortalVisible: &hidden})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 422, serviceErr.Status)
	_, err = svc.CreateStaffNote(ctx, relationshipServiceAgent(), request.ID, contract.RequestNoteWrite{Body: strings.Repeat("界", 2001), PortalVisible: &hidden})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 422, serviceErr.Status)
	_, err = svc.CreateStaffNote(ctx, relationshipServiceAgent(), request.ID, contract.RequestNoteWrite{Body: "Missing visibility"})
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, 422, serviceErr.Status)

	notes, _, err := store.SearchCity311RequestNotes(ctx, st, composeTypes.City311RequestNoteFilter{RequestID: request.ID})
	require.NoError(t, err)
	require.Empty(t, notes)
}
