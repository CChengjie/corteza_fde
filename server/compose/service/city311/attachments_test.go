package city311

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/cortezaproject/corteza/server/store/adapters/rdbms"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func attachmentTestService(t *testing.T) (*Service, store.Storer) {
	t.Helper()
	svc, st := testService(t)
	// The baseline SQLite driver disables transactions. Enable real SQL
	// transactions explicitly for these rollback/consumption regressions.
	database := st.(*rdbms.Store)
	database.TxRetryLimit = 0
	database.DB.(*sqlx.DB).SetMaxOpenConns(1)
	return svc, st
}

func TestAttachmentValidationPrecedesPersistence(t *testing.T) {
	svc, st := attachmentTestService(t)
	ctx := context.Background()
	for _, tt := range []struct {
		name, filename, media string
		data                  []byte
	}{
		{"empty", "note.txt", "text/plain", nil},
		{"oversize", "note.txt", "text/plain", make([]byte, maximumAttachmentSize+1)},
		{"filename", "../", "text/plain", []byte("x")},
		{"long", strings.Repeat("界", 121), "text/plain", []byte("x")},
		{"control", "bad\n.txt", "text/plain", []byte("x")},
		{"media", "evil.html", "text/html", []byte("x")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.StageAttachment(ctx, 0, tt.filename, tt.media, tt.data)
			requireIdentityError(t, err, 422, contract.ErrorValidation)
		})
	}
	staged, _, err := store.SearchCity311StagedAttachments(ctx, st, composeTypes.City311StagedAttachmentFilter{})
	require.NoError(t, err)
	require.Empty(t, staged)
	for media := range allowedAttachmentMediaTypes {
		receipt, err := svc.StageAttachment(ctx, 0, `C:\private\résumé.txt`, media, []byte{0, 255, 128, 42})
		require.NoError(t, err)
		require.Equal(t, "résumé.txt", receipt.Filename)
		require.Equal(t, svc.now().Add(time.Hour), receipt.ExpiresAt)
		stored, err := store.LookupCity311StagedAttachmentByTokenHash(ctx, st, hashKey(receipt.AttachmentToken))
		require.NoError(t, err)
		require.NotEqual(t, receipt.AttachmentToken, stored.TokenHash)
		require.Equal(t, []byte{0, 255, 128, 42}, stored.Content)
	}
	_, err = svc.StageAttachment(ctx, 0, strings.Repeat("界", 120), "text/plain", make([]byte, maximumAttachmentSize))
	require.NoError(t, err)
}

func TestStagedAttachmentsAreAtomicSingleUseAndReplayable(t *testing.T) {
	svc, st := attachmentTestService(t)
	ctx := context.Background()
	first, err := svc.StageAttachment(ctx, 12, "first.txt", "text/plain", []byte("first"))
	require.NoError(t, err)
	options := SubmissionOptions{Operation: "portal_service_request_submit", SourceChannel: contract.SourceChannelPortalAnonymous, ActorID: 12, AttachmentTokens: []string{first.AttachmentToken}, RequireIdempotency: true}
	for _, tt := range []struct {
		name    string
		tokens  []string
		actorID uint64
	}{
		{"foreign", options.AttachmentTokens, 13},
		{"missing", []string{first.AttachmentToken, strings.Repeat("a", 43)}, 12},
		{"duplicate", []string{first.AttachmentToken, first.AttachmentToken}, 12},
		{"too-many", []string{"1", "2", "3", "4", "5", "6"}, 12},
	} {
		t.Run(tt.name, func(t *testing.T) {
			bad := options
			bad.ActorID, bad.AttachmentTokens = tt.actorID, tt.tokens
			_, _, err := svc.Submit(ctx, validSubmission(), tt.name, bad)
			requireIdentityError(t, err, 422, contract.ErrorValidation)
		})
	}
	requests, _, err := store.SearchCity311ServiceRequests(ctx, st, composeTypes.City311ServiceRequestFilter{})
	require.NoError(t, err)
	require.Empty(t, requests)
	_, err = store.LookupCity311StagedAttachmentByTokenHash(ctx, st, hashKey(first.AttachmentToken))
	require.NoError(t, err)
	created, status, err := svc.Submit(ctx, validSubmission(), "one", options)
	require.NoError(t, err)
	require.Equal(t, 201, status)
	_, _, err = svc.Submit(ctx, validSubmission(), "different", options)
	requireIdentityError(t, err, 422, contract.ErrorValidation)
	restarted := New(st)
	now := svc.now().Add(2 * time.Hour)
	restarted.now = func() time.Time { return now }
	replay, status, err := restarted.Submit(ctx, validSubmission(), "one", options)
	require.NoError(t, err)
	require.Equal(t, 200, status)
	require.Equal(t, created, replay)
	foreignReplay := options
	foreignReplay.ActorID++
	_, _, err = restarted.Submit(ctx, validSubmission(), "one", foreignReplay)
	requireIdentityError(t, err, 409, contract.ErrorIdempotencyConflict)
	changed := options
	changed.AttachmentTokens = nil
	_, _, err = svc.Submit(ctx, validSubmission(), "one", changed)
	requireIdentityError(t, err, 409, contract.ErrorIdempotencyConflict)
	requestID, _ := strconv.ParseUint(created.RequestID, 10, 64)
	attachments, _, err := store.SearchCity311RequestAttachments(ctx, st, composeTypes.City311RequestAttachmentFilter{RequestID: requestID})
	require.NoError(t, err)
	require.Len(t, attachments, 1)
	require.Equal(t, []byte("first"), attachments[0].Content)
	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{RequestID: requestID, EventType: "ATTACHMENT_ADDED"})
	require.NoError(t, err)
	require.Len(t, audits, 1)
	require.Equal(t, strconv.FormatUint(attachments[0].ID, 10), audits[0].EntityID)
	require.NotContains(t, fmt.Sprint(audits[0]), first.AttachmentToken)
}

func TestSubmissionReplayRemainsActorBoundWithoutAttachments(t *testing.T) {
	svc, _ := attachmentTestService(t)
	ctx := context.Background()
	in := validSubmission()
	created, status, err := svc.Submit(ctx, in, "actor-bound", SubmissionOptions{ActorID: 77})
	require.NoError(t, err)
	require.Equal(t, 201, status)
	response, status, err := svc.Submit(ctx, in, "actor-bound", SubmissionOptions{ActorID: 77})
	require.NoError(t, err)
	require.Equal(t, 200, status) // Integration API replay; portal replay remains 201.
	require.Equal(t, created, response)
	_, _, err = svc.Submit(ctx, in, "actor-bound", SubmissionOptions{ActorID: 78})
	requireIdentityError(t, err, 409, contract.ErrorIdempotencyConflict)
}

func TestFiveStagedAttachmentsRemainAddressableAfterSubmission(t *testing.T) {
	svc, st := attachmentTestService(t)
	ctx := context.Background()
	options := SubmissionOptions{SourceChannel: contract.SourceChannelAPI}
	for i := 0; i < 5; i++ {
		receipt, err := svc.StageAttachment(ctx, 0, fmt.Sprintf("file-%d.txt", i), "text/plain", []byte{byte(i)})
		require.NoError(t, err)
		options.AttachmentTokens = append(options.AttachmentTokens, receipt.AttachmentToken)
	}
	response, _, err := svc.Submit(ctx, validSubmission(), "five", options)
	require.NoError(t, err)
	require.Len(t, response.Attachments, 5)
	require.NoError(t, store.Upgrade(ctx, zap.NewNop(), st))
	admin := contract.Actor{ID: 9, Roles: []contract.ApplicationRole{contract.ApplicationRolePlatformAdministrator}}
	restarted := New(st)
	for i, item := range response.Attachments {
		attachmentID, _ := strconv.ParseUint(item.AttachmentID, 10, 64)
		result, err := restarted.DownloadAttachment(ctx, admin, attachmentID)
		require.NoError(t, err)
		require.Equal(t, base64.StdEncoding.EncodeToString([]byte{byte(i)}), result.Body)
	}
	requestID, _ := strconv.ParseUint(response.RequestID, 10, 64)
	detail, err := restarted.Find(ctx, admin, requestID)
	require.NoError(t, err)
	attachments := 0
	for _, event := range detail.Audit {
		if event.EventType == "ATTACHMENT_ADDED" {
			attachments++
			require.Equal(t, "attachment", event.EntityType)
			require.NotEqual(t, response.RequestID, event.EntityID)
		}
	}
	require.Equal(t, 5, attachments)
}

func TestAttachmentConsumeRollsBackOnLaterFailure(t *testing.T) {
	svc, st := attachmentTestService(t)
	ctx := context.Background()
	upload, err := svc.StageAttachment(ctx, 0, "note.txt", "text/plain", []byte("bytes"))
	require.NoError(t, err)
	options := SubmissionOptions{AttachmentTokens: []string{upload.AttachmentToken}}
	// Collide with the existing stage ID and then force request and audit ID
	// reuse. The failed transaction must leave its upload reusable.
	svc.nextID = func() uint64 { return 44 }
	_, _, err = svc.Submit(ctx, validSubmission(), "", options)
	require.Error(t, err)
	_, err = store.LookupCity311StagedAttachmentByTokenHash(ctx, st, hashKey(upload.AttachmentToken))
	require.NoError(t, err)
	attachments, _, err := store.SearchCity311RequestAttachments(ctx, st, composeTypes.City311RequestAttachmentFilter{})
	require.NoError(t, err)
	require.Empty(t, attachments)
	next := uint64(100)
	svc.nextID = func() uint64 { next++; return next }
	_, _, err = svc.Submit(ctx, validSubmission(), "", options)
	require.NoError(t, err)
}

func TestStagedAttachmentExpiryCleanupAndMigrationPreservation(t *testing.T) {
	svc, st := attachmentTestService(t)
	ctx := context.Background()
	for i := 0; i < 205; i++ {
		_, err := svc.StageAttachment(ctx, 0, "note.txt", "text/plain", []byte("expired"))
		require.NoError(t, err)
	}
	before := svc.now()
	svc.now = func() time.Time { return before.Add(time.Hour) }
	live, err := svc.StageAttachment(ctx, 0, "new.txt", "text/plain", []byte("live"))
	require.NoError(t, err)
	require.NoError(t, store.Upgrade(ctx, zap.NewNop(), st))
	restarted := New(st)
	restarted.now = svc.now
	require.NoError(t, restarted.CleanupExpiredAttachments(ctx))
	staged, _, err := store.SearchCity311StagedAttachments(ctx, st, composeTypes.City311StagedAttachmentFilter{})
	require.NoError(t, err)
	require.Len(t, staged, 1)
	require.Equal(t, hashKey(live.AttachmentToken), staged[0].TokenHash)
	svc.now = func() time.Time { return before.Add(2 * time.Hour) }
	_, _, err = svc.Submit(ctx, validSubmission(), "expired", SubmissionOptions{AttachmentTokens: []string{live.AttachmentToken}})
	requireIdentityError(t, err, 422, contract.ErrorValidation)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, restarted.CleanupExpiredAttachments(cancelled), context.Canceled)
	// Startup cleanup is scheduled independently of new upload traffic.
	done, stop := context.WithCancel(ctx)
	defer stop()
	restarted.now = svc.now
	restarted.StartAttachmentCleanup(done, func(err error) { t.Errorf("cleanup: %v", err) })
	require.Eventually(t, func() bool {
		items, _, err := store.SearchCity311StagedAttachments(ctx, st, composeTypes.City311StagedAttachmentFilter{})
		return err == nil && len(items) == 0
	}, time.Second, time.Millisecond*10)
}

func TestTwoServicesCannotConsumeOneStagedAttachmentTwice(t *testing.T) {
	svc, st := attachmentTestService(t)
	ctx := context.Background()
	upload, err := svc.StageAttachment(ctx, 0, "once.txt", "text/plain", []byte("once"))
	require.NoError(t, err)
	other := New(st)
	other.now = svc.now
	var id atomic.Uint64
	id.Store(10000)
	svc.nextID, other.nextID = func() uint64 { return id.Add(1) }, func() uint64 { return id.Add(1) }
	options := SubmissionOptions{AttachmentTokens: []string{upload.AttachmentToken}}
	var success atomic.Int32
	var wg sync.WaitGroup
	for _, instance := range []*Service{svc, other} {
		wg.Add(1)
		go func(instance *Service) {
			defer wg.Done()
			_, _, err := instance.Submit(ctx, validSubmission(), "", options)
			if err == nil {
				success.Add(1)
			}
		}(instance)
	}
	wg.Wait()
	// SQLite can reject both conflicting transactions; retry has to be safe.
	if success.Load() == 0 {
		_, _, err = svc.Submit(ctx, validSubmission(), "", options)
		require.NoError(t, err)
		success.Add(1)
	}
	require.Equal(t, int32(1), success.Load())
	items, _, err := store.SearchCity311RequestAttachments(ctx, st, composeTypes.City311RequestAttachmentFilter{})
	require.NoError(t, err)
	require.Len(t, items, 1)
}

func TestAttachmentCleanupTraversesMixedRawPages(t *testing.T) {
	svc, st := attachmentTestService(t)
	ctx := context.Background()
	now := svc.now()
	// Submitted bytes are outside the staging lifecycle, even at an old timestamp.
	in := validSubmission()
	in.Attachments = []contract.AttachmentInput{{Filename: "kept.txt", MediaType: "text/plain", ContentBase64: base64.StdEncoding.EncodeToString([]byte("submitted"))}}
	created, _, err := svc.Submit(ctx, in, "cleanup-submitted", SubmissionOptions{})
	require.NoError(t, err)
	liveIDs := make(map[uint64]bool)
	for i := 0; i < 512; i++ {
		// More than a full live page first, followed by interleaved expired/live
		// stages. Identifier order deliberately does not imply expiry order.
		expires := now.Add(time.Hour)
		if i >= 101 && i%2 == 1 {
			expires = now.Add(-time.Minute)
		}
		if i == 102 {
			expires = now // Inclusive expiry boundary.
		}
		item := &composeTypes.City311StagedAttachment{
			ID: uint64(800000 + i), TokenHash: hashKey(fmt.Sprintf("mixed-%d", i)),
			Filename: "note.txt", MediaType: "text/plain", Content: []byte("staged"),
			CreatedAt: expires.Add(-time.Hour), ExpiresAt: expires,
		}
		require.NoError(t, store.CreateCity311StagedAttachment(ctx, st, item))
		if expires.After(now) {
			liveIDs[item.ID] = true
		}
	}
	// Repeated runs at the same benchmark clock must finish past live-only pages.
	for i := 0; i < 2; i++ {
		require.NoError(t, svc.CleanupExpiredAttachments(ctx))
		items, _, err := store.SearchCity311StagedAttachments(ctx, st, composeTypes.City311StagedAttachmentFilter{})
		require.NoError(t, err)
		require.Len(t, items, len(liveIDs))
		for _, item := range items {
			require.True(t, liveIDs[item.ID])
			require.Equal(t, []byte("staged"), item.Content)
		}
	}
	attachmentID, err := strconv.ParseUint(created.Attachments[0].AttachmentID, 10, 64)
	require.NoError(t, err)
	download, err := svc.DownloadAttachment(ctx, contract.Actor{ID: 1, Roles: []contract.ApplicationRole{contract.ApplicationRolePlatformAdministrator}}, attachmentID)
	require.NoError(t, err)
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte("submitted")), download.Body)
}

func TestAttachmentDownloadUsesCurrentRequestScopeAndPreservesBinary(t *testing.T) {
	svc, st := attachmentTestService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	user, err := store.LookupUserByHandle(ctx, st, "city311-constituent")
	require.NoError(t, err)
	data := []byte{0, 255, 128, 195, 40}
	upload, err := svc.StageAttachment(ctx, user.ID, "binary.png", "image/png", data)
	require.NoError(t, err)
	created, _, err := svc.Submit(ctx, validSubmission(), "binary", SubmissionOptions{SourceChannel: contract.SourceChannelPortalAuthenticated, ActorID: user.ID, AttachmentTokens: []string{upload.AttachmentToken}})
	require.NoError(t, err)
	requestID, _ := strconv.ParseUint(created.RequestID, 10, 64)
	items, err := svc.attachmentMetadata(ctx, requestID)
	require.NoError(t, err)
	require.Len(t, items, 1)
	attachmentID, _ := strconv.ParseUint(items[0].AttachmentID, 10, 64)
	owner, err := svc.FindActor(ctx, user.ID)
	require.NoError(t, err)
	download, err := New(st).DownloadAttachment(ctx, owner, attachmentID)
	require.NoError(t, err)
	decoded, err := base64.StdEncoding.DecodeString(download.Body)
	require.NoError(t, err)
	require.Equal(t, data, decoded)
	require.Equal(t, "base64", download.BodyEncoding)
	require.Contains(t, download.ContentDisposition, "attachment;")
	stranger := owner
	stranger.ID++
	_, err = svc.DownloadAttachment(ctx, stranger, attachmentID)
	requireIdentityError(t, err, 403, contract.ErrorForbidden)
	_, err = svc.DownloadAttachment(ctx, contract.Actor{}, attachmentID)
	requireIdentityError(t, err, 401, contract.ErrorUnauthenticated)
	staff := contract.Actor{ID: 456, Roles: []contract.ApplicationRole{contract.ApplicationRoleServiceAgent}, Department: contract.DepartmentStreets, Districts: []contract.DistrictCode{contract.DistrictNorth}}
	_, err = svc.DownloadAttachment(ctx, staff, attachmentID)
	require.NoError(t, err)
	request, err := store.LookupCity311ServiceRequestByID(ctx, st, requestID)
	require.NoError(t, err)
	request.CouncilDistrict = contract.DistrictSouth
	require.NoError(t, store.UpdateCity311ServiceRequest(ctx, st, request))
	_, err = svc.DownloadAttachment(ctx, staff, attachmentID)
	requireIdentityError(t, err, 403, contract.ErrorForbidden)
	staff.Roles = []contract.ApplicationRole{contract.ApplicationRoleDepartmentManager}
	_, err = svc.DownloadAttachment(ctx, staff, attachmentID)
	require.NoError(t, err)
	staff.Department = contract.DepartmentSanitation
	_, err = svc.DownloadAttachment(ctx, staff, attachmentID)
	requireIdentityError(t, err, 403, contract.ErrorForbidden)
	_, err = svc.DownloadAttachment(ctx, owner, 999)
	requireIdentityError(t, err, 404, contract.ErrorNotFound)
}
