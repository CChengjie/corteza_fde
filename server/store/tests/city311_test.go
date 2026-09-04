package tests

import (
	"context"
	"testing"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func testCity311ActorProfiles(t *testing.T, s store.City311ActorProfiles) {
	ctx := context.Background()
	require.NoError(t, s.TruncateCity311ActorProfiles(ctx))
	profile := &composeTypes.City311ActorProfile{
		ID: 101, ApplicationRoles: composeTypes.City311ApplicationRoleSet{contract.ApplicationRoleServiceAgent},
		Department: contract.DepartmentStreets, Districts: composeTypes.City311DistrictCodeSet{contract.DistrictNorth},
		CreatedAt: *now(), UpdatedAt: *now(),
	}
	require.NoError(t, s.CreateCity311ActorProfile(ctx, profile))
	fetched, err := s.LookupCity311ActorProfileByID(ctx, profile.ID)
	require.NoError(t, err)
	require.Equal(t, profile.ApplicationRoles, fetched.ApplicationRoles)
	set, _, err := s.SearchCity311ActorProfiles(ctx, composeTypes.City311ActorProfileFilter{Department: string(contract.DepartmentStreets)})
	require.NoError(t, err)
	require.Len(t, set, 1)
	fetched.Districts = append(fetched.Districts, contract.DistrictCentral)
	require.NoError(t, s.UpdateCity311ActorProfile(ctx, fetched))
	require.NoError(t, s.DeleteCity311ActorProfileByID(ctx, profile.ID))
}

func testCity311Constituents(t *testing.T, s store.City311Constituents) {
	ctx := context.Background()
	require.NoError(t, s.TruncateCity311Constituents(ctx))
	constituent := &composeTypes.City311Constituent{
		ID: 151, ConstituentID: "C-151",
		Profile:          composeTypes.City311JSON{"constituent_id": "C-151", "display_name": "Alex Resident"},
		OwningDepartment: contract.DepartmentStreets, CouncilDistrict: contract.DistrictNorth,
		CreatedAt: *now(), UpdatedAt: *now(),
	}
	require.NoError(t, s.CreateCity311Constituent(ctx, constituent))
	fetched, err := s.LookupCity311ConstituentByConstituentID(ctx, constituent.ConstituentID)
	require.NoError(t, err)
	require.Equal(t, constituent.Profile, fetched.Profile)
	set, _, err := s.SearchCity311Constituents(ctx, composeTypes.City311ConstituentFilter{
		OwningDepartment: string(contract.DepartmentStreets), CouncilDistrict: string(contract.DistrictNorth),
	})
	require.NoError(t, err)
	require.Len(t, set, 1)
	fetched.Profile["display_name"] = "Updated Resident"
	require.NoError(t, s.UpdateCity311Constituent(ctx, fetched))
	require.NoError(t, s.DeleteCity311ConstituentByID(ctx, fetched.ID))
}

func testCity311AuditEvents(t *testing.T, s store.City311AuditEvents) {
	ctx := context.Background()
	require.NoError(t, s.TruncateCity311AuditEvents(ctx))
	event := &composeTypes.City311AuditEvent{
		ID: 201, RequestID: 301, EventType: "SERVICE_REQUEST_SUBMITTED", ActorType: contract.AuditActorSystem,
		EntityType: "service_request", EntityID: "301",
		SourceChannel: contract.SourceChannelAPI, Before: composeTypes.City311JSON{}, After: composeTypes.City311JSON{"status": "SUBMITTED"}, CreatedAt: *now(),
	}
	require.NoError(t, s.CreateCity311AuditEvent(ctx, event))
	fetched, err := s.LookupCity311AuditEventByID(ctx, event.ID)
	require.NoError(t, err)
	require.Equal(t, "SUBMITTED", fetched.After["status"])
	set, _, err := s.SearchCity311AuditEvents(ctx, composeTypes.City311AuditEventFilter{RequestID: event.RequestID})
	require.NoError(t, err)
	require.Len(t, set, 1)
	require.NoError(t, s.DeleteCity311AuditEvent(ctx, event))
}

func testCity311StagedAttachments(t *testing.T, s store.City311StagedAttachments) {
	ctx := context.Background()
	require.NoError(t, s.TruncateCity311StagedAttachments(ctx))
	item := &composeTypes.City311StagedAttachment{ID: 250, TokenHash: "digest", OwnerID: 42, Filename: "note.txt", MediaType: "text/plain", Content: []byte{0, 255}, CreatedAt: *now(), ExpiresAt: now().Add(time.Hour)}
	require.NoError(t, s.CreateCity311StagedAttachment(ctx, item))
	loaded, err := s.LookupCity311StagedAttachmentByTokenHash(ctx, item.TokenHash)
	require.NoError(t, err)
	require.Equal(t, item, loaded)
	items, _, err := s.SearchCity311StagedAttachments(ctx, composeTypes.City311StagedAttachmentFilter{OwnerID: 42})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.NoError(t, s.DeleteCity311StagedAttachmentByID(ctx, item.ID))
}

func testCity311RequestAttachments(t *testing.T, s store.City311RequestAttachments) {
	ctx := context.Background()
	require.NoError(t, s.TruncateCity311RequestAttachments(ctx))
	attachment := &composeTypes.City311RequestAttachment{
		ID: 251, RequestID: 301, Filename: "photo.png", MediaType: "image/png", Size: 4, Content: []byte("data"), CreatedAt: *now(),
	}
	require.NoError(t, s.CreateCity311RequestAttachment(ctx, attachment))
	fetched, err := s.LookupCity311RequestAttachmentByID(ctx, attachment.ID)
	require.NoError(t, err)
	require.Equal(t, attachment.Content, fetched.Content)
	set, _, err := s.SearchCity311RequestAttachments(ctx, composeTypes.City311RequestAttachmentFilter{RequestID: attachment.RequestID})
	require.NoError(t, err)
	require.Len(t, set, 1)
	require.NoError(t, s.DeleteCity311RequestAttachmentByID(ctx, attachment.ID))
}

func testCity311RequestConstituentLinks(t *testing.T, s store.City311RequestConstituentLinks) {
	ctx := context.Background()
	require.NoError(t, s.TruncateCity311RequestConstituentLinks(ctx))
	link := &composeTypes.City311RequestConstituent{
		ID: 275, RequestID: 301, ConstituentID: "C-151",
		RelationshipType: contract.RelationshipAffectedResident, PortalVisible: true, NotifyStatus: true,
		CreatedAt: *now(), UpdatedAt: *now(),
	}
	require.NoError(t, s.CreateCity311RequestConstituentLink(ctx, link))
	duplicate := *link
	duplicate.ID++
	require.Error(t, s.CreateCity311RequestConstituentLink(ctx, &duplicate))
	primary := &composeTypes.City311RequestConstituent{
		ID: 277, RequestID: 301, ConstituentID: "C-151",
		RelationshipType: contract.RelationshipPrimaryRequester, PortalVisible: true, NotifyStatus: true,
		CreatedAt: *now(), UpdatedAt: *now(),
	}
	require.NoError(t, s.CreateCity311RequestConstituentLink(ctx, primary))
	secondPrimary := *primary
	secondPrimary.ID++
	secondPrimary.ConstituentID = "C-152"
	require.Error(t, s.CreateCity311RequestConstituentLink(ctx, &secondPrimary))
	fetched, err := s.LookupCity311RequestConstituentLinkByID(ctx, link.ID)
	require.NoError(t, err)
	require.Equal(t, contract.RelationshipAffectedResident, fetched.RelationshipType)
	set, _, err := s.SearchCity311RequestConstituentLinks(ctx, composeTypes.City311RequestConstituentFilter{
		RequestID: link.RequestID, ConstituentID: link.ConstituentID,
	})
	require.NoError(t, err)
	require.Len(t, set, 1)
	fetched.NotifyStatus = false
	require.NoError(t, s.UpdateCity311RequestConstituentLink(ctx, fetched))
	require.NoError(t, s.DeleteCity311RequestConstituentLinkByID(ctx, link.ID))
	require.NoError(t, s.DeleteCity311RequestConstituentLinkByID(ctx, primary.ID))
}

func testCity311RequestNotes(t *testing.T, s store.City311RequestNotes) {
	ctx := context.Background()
	require.NoError(t, s.TruncateCity311RequestNotes(ctx))
	note := &composeTypes.City311RequestNote{
		ID: 290, RequestID: 301, AuthorType: contract.AuditActorConstituent,
		AuthorID: 151, AuthorConstituentID: "C-151", Body: "Please check the east side of the street.",
		PortalVisible: true, CreatedAt: *now(),
	}
	require.NoError(t, s.CreateCity311RequestNote(ctx, note))
	fetched, err := s.LookupCity311RequestNoteByID(ctx, note.ID)
	require.NoError(t, err)
	require.Equal(t, note.Body, fetched.Body)
	set, _, err := s.SearchCity311RequestNotes(ctx, composeTypes.City311RequestNoteFilter{RequestID: note.RequestID})
	require.NoError(t, err)
	require.Len(t, set, 1)
	require.Equal(t, note.AuthorConstituentID, set[0].AuthorConstituentID)
}

func testCity311PublicHistoryItems(t *testing.T, s store.City311PublicHistoryItems) {
	ctx := context.Background()
	require.NoError(t, s.TruncateCity311PublicHistoryItems(ctx))
	item := &composeTypes.City311PublicHistoryItem{
		ID: 275, RequestID: 301, Action: "SUBMITTED", ResponsibleDepartment: contract.DepartmentStreets, OccurredAt: *now(),
	}
	require.NoError(t, s.CreateCity311PublicHistoryItem(ctx, item))
	fetched, err := s.LookupCity311PublicHistoryItemByID(ctx, item.ID)
	require.NoError(t, err)
	require.Equal(t, item.Action, fetched.Action)
	set, _, err := s.SearchCity311PublicHistoryItems(ctx, composeTypes.City311PublicHistoryItemFilter{RequestID: item.RequestID})
	require.NoError(t, err)
	require.Len(t, set, 1)
	require.NoError(t, s.DeleteCity311PublicHistoryItemByID(ctx, item.ID))
}

func testCity311IdempotencyRecords(t *testing.T, s store.City311IdempotencyRecords) {
	ctx := context.Background()
	require.NoError(t, s.TruncateCity311IdempotencyRecords(ctx))
	record := &composeTypes.City311IdempotencyRecord{
		ID: 401, Operation: "service_request_create", KeyHash: "key-hash", RequestHash: "request-hash", ResponseStatus: 201,
		ResponseBody: composeTypes.City311JSON{"request_id": "301"}, RequestID: 301, CreatedAt: *now(), ExpiresAt: now().Add(24 * time.Hour),
	}
	require.NoError(t, s.CreateCity311IdempotencyRecord(ctx, record))
	fetched, err := s.LookupCity311IdempotencyRecordByOperationKeyHash(ctx, record.Operation, record.KeyHash)
	require.NoError(t, err)
	require.Equal(t, record.RequestHash, fetched.RequestHash)
	require.NoError(t, s.DeleteCity311IdempotencyRecordByID(ctx, record.ID))
}

func testCity311IdentityNotifications(t *testing.T, s store.City311IdentityNotifications) {
	ctx := context.Background()
	require.NoError(t, s.TruncateCity311IdentityNotifications(ctx))
	createdAt := *now()
	notification := &composeTypes.City311IdentityNotification{
		ID: 451, UserID: 501, Kind: "PASSWORD_RESET", Recipient: "resident@example.invalid",
		DeliveryKey: "password-reset:501:451", Payload: composeTypes.City311JSON{"language": "EN"},
		Status: "PENDING", CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	require.NoError(t, s.CreateCity311IdentityNotification(ctx, notification))
	fetched, err := s.LookupCity311IdentityNotificationByID(ctx, notification.ID)
	require.NoError(t, err)
	require.Equal(t, "EN", fetched.Payload["language"])
	fetched.Status = "SENT"
	fetched.Attempts = 1
	fetched.UpdatedAt = *now()
	require.NoError(t, s.UpdateCity311IdentityNotification(ctx, fetched))
	set, _, err := s.SearchCity311IdentityNotifications(ctx, composeTypes.City311IdentityNotificationFilter{UserID: notification.UserID, Status: "SENT"})
	require.NoError(t, err)
	require.Len(t, set, 1)
	require.NoError(t, s.DeleteCity311IdentityNotificationByID(ctx, notification.ID))
}

func testCity311IdentitySessions(t *testing.T, s store.City311IdentitySessions) {
	ctx := context.Background()
	require.NoError(t, s.TruncateCity311IdentitySessions(ctx))
	issuedAt := *now()
	session := &composeTypes.City311IdentitySession{
		ID: 551, TokenHash: "session-token-hash", UserID: 501, IssuedAt: issuedAt, LastSeenAt: issuedAt,
		ExpiresAt: issuedAt.Add(30 * time.Minute), AbsoluteExpiresAt: issuedAt.Add(8 * time.Hour),
	}
	require.NoError(t, s.CreateCity311IdentitySession(ctx, session))
	fetched, err := s.LookupCity311IdentitySessionByTokenHash(ctx, session.TokenHash)
	require.NoError(t, err)
	require.Equal(t, session.UserID, fetched.UserID)
	fetched.LastSeenAt = issuedAt.Add(time.Minute)
	require.NoError(t, s.UpdateCity311IdentitySession(ctx, fetched))
	set, _, err := s.SearchCity311IdentitySessions(ctx, composeTypes.City311IdentitySessionFilter{UserID: session.UserID})
	require.NoError(t, err)
	require.Len(t, set, 1)
	require.NoError(t, s.DeleteCity311IdentitySessionByID(ctx, session.ID))
}

func testCity311LocalAccounts(t *testing.T, s store.City311LocalAccounts) {
	ctx := context.Background()
	require.NoError(t, s.TruncateCity311LocalAccounts(ctx))
	createdAt := *now()
	account := &composeTypes.City311LocalAccount{
		ID: 501, LoginIdentifier: "resident-501", VerifiedEmail: "resident@example.invalid",
		PreferredLanguage: "EN", CreatedAt: createdAt, UpdatedAt: createdAt,
	}
	require.NoError(t, s.CreateCity311LocalAccount(ctx, account))
	fetched, err := s.LookupCity311LocalAccountByLoginIdentifier(ctx, account.LoginIdentifier)
	require.NoError(t, err)
	require.Equal(t, account.VerifiedEmail, fetched.VerifiedEmail)
	byEmail, err := s.LookupCity311LocalAccountByVerifiedEmail(ctx, account.VerifiedEmail)
	require.NoError(t, err)
	require.Equal(t, account.ID, byEmail.ID)
	fetched.PreferredLanguage = "ES"
	require.NoError(t, s.UpdateCity311LocalAccount(ctx, fetched))
	duplicate := *account
	duplicate.ID++
	duplicate.VerifiedEmail = "other@example.invalid"
	require.Error(t, s.CreateCity311LocalAccount(ctx, &duplicate))
	require.NoError(t, s.DeleteCity311LocalAccountByID(ctx, account.ID))
}

func testCity311PasswordResetTokens(t *testing.T, s store.City311PasswordResetTokens) {
	ctx := context.Background()
	require.NoError(t, s.TruncateCity311PasswordResetTokens(ctx))
	createdAt := *now()
	token := &composeTypes.City311PasswordResetToken{
		ID: 601, TokenHash: "password-reset-token-hash", UserID: 501,
		CreatedAt: createdAt, ExpiresAt: createdAt.Add(15 * time.Minute),
	}
	require.NoError(t, s.CreateCity311PasswordResetToken(ctx, token))
	fetched, err := s.LookupCity311PasswordResetTokenByTokenHash(ctx, token.TokenHash)
	require.NoError(t, err)
	require.Nil(t, fetched.UsedAt)
	usedAt := createdAt.Add(time.Minute)
	fetched.UsedAt = &usedAt
	require.NoError(t, s.UpdateCity311PasswordResetToken(ctx, fetched))
	set, _, err := s.SearchCity311PasswordResetTokens(ctx, composeTypes.City311PasswordResetTokenFilter{UserID: token.UserID})
	require.NoError(t, err)
	require.Len(t, set, 1)
	require.Equal(t, usedAt, *set[0].UsedAt)
	require.NoError(t, s.DeleteCity311PasswordResetTokenByID(ctx, token.ID))
}

func testCity311RequestSequences(t *testing.T, s store.City311RequestSequences) {
	ctx := context.Background()
	require.NoError(t, s.TruncateCity311RequestSequences(ctx))
	sequence := &composeTypes.City311RequestSequence{ID: 2026, NextNumber: 41}
	require.NoError(t, s.CreateCity311RequestSequence(ctx, sequence))
	sequence.NextNumber = 42
	require.NoError(t, s.UpdateCity311RequestSequence(ctx, sequence))
	fetched, err := s.LookupCity311RequestSequenceByID(ctx, 2026)
	require.NoError(t, err)
	require.Equal(t, uint64(42), fetched.NextNumber)
	require.NoError(t, s.DeleteCity311RequestSequence(ctx, sequence))
}

func testCity311ServiceRequests(t *testing.T, s store.City311ServiceRequests) {
	ctx := context.Background()
	require.NoError(t, s.TruncateCity311ServiceRequests(ctx))
	request := &composeTypes.City311ServiceRequest{
		ID: 301, RequestNumber: "SR-2026-00041", Summary: "Pothole on Example Street", Description: "A deep pothole blocks one traffic lane.",
		ServiceType: contract.ServiceTypePothole, OwningDepartment: contract.DepartmentStreets, CouncilDistrict: contract.DistrictNorth,
		SourceChannel: contract.SourceChannelAPI, OriginClass: contract.OriginClassExternal, Status: contract.ServiceRequestStatusSubmitted,
		PrimaryRequester: composeTypes.City311JSON{"constituent_id": "C-301"}, Location: composeTypes.City311JSON{"address": "100 Example Street"},
		CustomFields: composeTypes.City311JSON{}, CollaboratorIDs: composeTypes.City311Uint64Set{}, Version: 1, CreatedAt: *now(), UpdatedAt: *now(),
	}
	require.NoError(t, s.CreateCity311ServiceRequest(ctx, request))
	fetched, err := s.LookupCity311ServiceRequestByRequestNumber(ctx, request.RequestNumber)
	require.NoError(t, err)
	require.Equal(t, "C-301", fetched.PrimaryRequester["constituent_id"])
	set, _, err := s.SearchCity311ServiceRequests(ctx, composeTypes.City311ServiceRequestFilter{Status: string(contract.ServiceRequestStatusSubmitted)})
	require.NoError(t, err)
	require.Len(t, set, 1)
	fetched.Status = contract.ServiceRequestStatusTriaged
	fetched.Version++
	require.NoError(t, s.UpdateCity311ServiceRequest(ctx, fetched))
	require.NoError(t, s.DeleteCity311ServiceRequestByID(ctx, request.ID))
}
