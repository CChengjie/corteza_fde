package city311

import (
	"context"
	"sort"
	"strconv"
	"strings"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/errors"
	"github.com/cortezaproject/corteza/server/store"
)

// CreateStaffNote appends a note without changing the service-request version.
// The frozen endpoint is not versioned and append-only notes never overwrite
// request state.
func (svc *Service) CreateStaffNote(ctx context.Context, actor contract.Actor, requestID uint64, input contract.RequestNoteWrite) (*contract.RequestNote, error) {
	body, portalVisible, err := validateRequestNote(input)
	if err != nil {
		return nil, err
	}
	if !hasRole(actor, contract.ApplicationRoleServiceAgent) {
		return nil, apiError(403, contract.ErrorForbidden, "A service agent role is required.")
	}

	return svc.appendRequestNote(ctx, requestID, body, portalVisible, contract.AuditActorStaff, actor.ID, "", func(ctx context.Context, tx store.Storer) error {
		_, err := lookupScopedRequest(ctx, tx, actor, requestID)
		return err
	})
}

// CreatePortalNote appends a constituent-authored note only when the actor has
// a portal-visible relationship to the request.
func (svc *Service) CreatePortalNote(ctx context.Context, ownerID, requestID uint64, input contract.RequestNoteWrite) (*contract.RequestNote, error) {
	if ownerID == 0 {
		return nil, apiError(401, contract.ErrorUnauthenticated, "Authentication is required.")
	}
	body, portalVisible, err := validateRequestNote(input)
	if err != nil {
		return nil, err
	}
	if !portalVisible {
		return nil, validationError(contract.FieldError{Field: "/portal_visible", Code: contract.ValidationInvalidValue})
	}
	constituentID := "C-" + strconv.FormatUint(ownerID, 10)
	return svc.appendRequestNote(ctx, requestID, body, true, contract.AuditActorConstituent, ownerID, constituentID, func(ctx context.Context, tx store.Storer) error {
		return ensurePortalRequestAccess(ctx, tx, requestID, constituentID)
	})
}

func validateRequestNote(input contract.RequestNoteWrite) (string, bool, error) {
	body := strings.TrimSpace(input.Body)
	if fields := validateBoundedText(body, "/body", 1, 2000); len(fields) > 0 {
		return "", false, validationError(fields...)
	}
	if input.PortalVisible == nil {
		return "", false, validationError(contract.FieldError{Field: "/portal_visible", Code: contract.ValidationRequired})
	}
	return body, *input.PortalVisible, nil
}

func (svc *Service) appendRequestNote(
	ctx context.Context,
	requestID uint64,
	body string,
	portalVisible bool,
	authorType contract.AuditActorType,
	authorID uint64,
	authorConstituentID string,
	authorize func(context.Context, store.Storer) error,
) (*contract.RequestNote, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	var created *composeTypes.City311RequestNote
	err := store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		if err := authorize(ctx, tx); err != nil {
			return err
		}
		note := &composeTypes.City311RequestNote{
			ID: svc.nextID(), RequestID: requestID, AuthorType: authorType, AuthorID: authorID,
			AuthorConstituentID: authorConstituentID, Body: body, PortalVisible: portalVisible, CreatedAt: svc.now(),
		}
		if err := store.CreateCity311RequestNote(ctx, tx, note); err != nil {
			return err
		}
		if err := svc.persistRequestNoteAudit(ctx, tx, note); err != nil {
			return err
		}
		created = note
		return nil
	})
	if err != nil {
		return nil, err
	}
	result := requestNoteContract(created)
	return &result, nil
}

func ensurePortalRequestAccess(ctx context.Context, tx store.Storer, requestID uint64, constituentID string) error {
	if _, err := store.LookupCity311ServiceRequestByID(ctx, tx, requestID); err != nil {
		if errors.IsNotFound(err) {
			return portalRequestNotFound()
		}
		return err
	}
	links, _, err := store.SearchCity311RequestConstituentLinks(ctx, tx, composeTypes.City311RequestConstituentFilter{
		RequestID: requestID, ConstituentID: constituentID,
	})
	if err != nil {
		return err
	}
	for _, link := range links {
		if relationshipGrantsPortalView(link) {
			return nil
		}
	}
	return portalRequestNotFound()
}

func portalRequestNotFound() *ServiceError {
	return apiError(404, contract.ErrorNotFound, "The service request was not found.")
}

func (svc *Service) persistRequestNoteAudit(ctx context.Context, tx store.Storer, note *composeTypes.City311RequestNote) error {
	source := contract.SourceChannelStaffInPerson
	if note.AuthorType == contract.AuditActorConstituent {
		source = contract.SourceChannelPortalAuthenticated
	}
	return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
		ID: svc.nextID(), RequestID: note.RequestID, EntityType: "request_note", EntityID: strconv.FormatUint(note.ID, 10),
		EventType: "REQUEST_NOTE_CREATED", ActorType: note.AuthorType, ActorID: note.AuthorID, SourceChannel: source,
		Before: map[string]any{}, After: requestNoteSnapshot(note), CreatedAt: note.CreatedAt,
	})
}

func requestNoteSnapshot(note *composeTypes.City311RequestNote) map[string]any {
	return map[string]any{
		"note_id": strconv.FormatUint(note.ID, 10), "request_id": strconv.FormatUint(note.RequestID, 10),
		"author_type": note.AuthorType, "author_id": strconv.FormatUint(note.AuthorID, 10),
		"author_constituent_id": note.AuthorConstituentID, "body": note.Body,
		"portal_visible": note.PortalVisible, "created_at": note.CreatedAt,
	}
}

func requestNoteContract(note *composeTypes.City311RequestNote) contract.RequestNote {
	return contract.RequestNote{
		NoteID: strconv.FormatUint(note.ID, 10), RequestID: strconv.FormatUint(note.RequestID, 10),
		AuthorType: note.AuthorType, AuthorID: strconv.FormatUint(note.AuthorID, 10),
		AuthorConstituentID: note.AuthorConstituentID, Body: note.Body,
		PortalVisible: note.PortalVisible, CreatedAt: note.CreatedAt,
	}
}

func listRequestNotes(ctx context.Context, st store.Storer, requestID uint64) ([]contract.RequestNote, error) {
	notes, _, err := store.SearchCity311RequestNotes(ctx, st, composeTypes.City311RequestNoteFilter{RequestID: requestID})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(notes, func(i, j int) bool {
		if notes[i].CreatedAt.Equal(notes[j].CreatedAt) {
			return notes[i].ID < notes[j].ID
		}
		return notes[i].CreatedAt.Before(notes[j].CreatedAt)
	})
	out := make([]contract.RequestNote, 0, len(notes))
	for _, note := range notes {
		out = append(out, requestNoteContract(note))
	}
	return out, nil
}
