package city311

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"mime"
	"path"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/errors"
	"github.com/cortezaproject/corteza/server/pkg/filter"
	"github.com/cortezaproject/corteza/server/store"
)

const stagedAttachmentLifetime = time.Hour

// StageAttachment validates the full file before persisting anything (9.4.1–3).
// The receipt is an opaque capability; only its SHA-256 digest is stored.
func (svc *Service) StageAttachment(ctx context.Context, ownerID uint64, filename, mediaType string, content []byte) (*contract.PortalAttachment, error) {
	filename = path.Base(strings.ReplaceAll(strings.TrimSpace(filename), `\`, "/"))
	mediaType = strings.TrimSpace(mediaType)
	fields := validateBoundedText(filename, "/filename", 1, 120)
	if filename == "." || filename == ".." || filename == "/" || strings.IndexFunc(filename, unicode.IsControl) >= 0 || !utf8.ValidString(filename) {
		fields = append(fields, contract.FieldError{Field: "/filename", Code: contract.ValidationInvalidValue})
	}
	if !allowedAttachmentMediaTypes[mediaType] {
		fields = append(fields, contract.FieldError{Field: "/media_type", Code: contract.ValidationInvalidValue})
	}
	if len(content) == 0 || len(content) > maximumAttachmentSize {
		fields = append(fields, contract.FieldError{Field: "/file", Code: contract.ValidationOutOfRange})
	}
	if len(fields) > 0 {
		return nil, validationError(fields...)
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, err
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	svc.mu.Lock()
	defer svc.mu.Unlock()
	now := svc.now()
	staged := &composeTypes.City311StagedAttachment{
		ID: svc.nextID(), TokenHash: hashKey(token), OwnerID: ownerID,
		Filename: filename, MediaType: mediaType, Content: content,
		CreatedAt: now, ExpiresAt: now.Add(stagedAttachmentLifetime),
	}
	if err := store.CreateCity311StagedAttachment(ctx, svc.store, staged); err != nil {
		return nil, err
	}
	return &contract.PortalAttachment{AttachmentToken: token, Filename: filename, MediaType: mediaType, Size: uint64(len(content)), ExpiresAt: staged.ExpiresAt}, nil
}

func attachmentField(options SubmissionOptions) string {
	if options.AttachmentField != "" {
		return options.AttachmentField
	}
	return "/attachment_tokens"
}

func (svc *Service) resolveStagedAttachments(ctx context.Context, tx store.Storer, options SubmissionOptions, now time.Time) ([]validatedAttachment, error) {
	result := make([]validatedAttachment, 0, len(options.AttachmentTokens))
	seen := make(map[string]bool)
	for index, token := range options.AttachmentTokens {
		invalid := validationError(contract.FieldError{Field: fmt.Sprintf("%s/%d", attachmentField(options), index), Code: contract.ValidationInvalidValue})
		if len(token) != 43 || seen[token] {
			return nil, invalid
		}
		seen[token] = true
		item, err := store.LookupCity311StagedAttachmentByTokenHash(ctx, tx, hashKey(token))
		if errors.IsNotFound(err) {
			return nil, invalid
		}
		if err != nil {
			return nil, err
		}
		if !item.ExpiresAt.After(now) || (item.OwnerID != 0 && item.OwnerID != options.ActorID) {
			return nil, invalid
		}
		// The final attachment retains this ID. Its primary-key constraint is
		// the database-backed single-consumption guard across service instances.
		result = append(result, validatedAttachment{ID: item.ID, Filename: item.Filename, MediaType: item.MediaType, Content: item.Content})
	}
	return result, nil
}

func (svc *Service) persistAttachmentEvents(ctx context.Context, tx store.Storer, requestID uint64, attachments []validatedAttachment, options SubmissionOptions, now time.Time) error {
	for _, item := range attachments {
		err := store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
			ID: svc.nextID(), RequestID: requestID, EntityType: "attachment", EntityID: strconv.FormatUint(item.ID, 10),
			EventType: "ATTACHMENT_ADDED", ActorType: options.ActorType, ActorID: options.ActorID, SourceChannel: options.SourceChannel,
			Before: composeTypes.City311JSON{}, After: composeTypes.City311JSON{"filename": item.Filename, "media_type": item.MediaType, "size": len(item.Content)}, CreatedAt: now,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// CleanupExpiredAttachments removes abandoned bytes, never submitted files.
// It drains multiple bounded batches and also runs after application restart.
func (svc *Service) CleanupExpiredAttachments(ctx context.Context) error {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	cutoff := svc.now()
	page := composeTypes.City311StagedAttachmentFilter{Paging: filter.Paging{Limit: 100}}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Page the raw rows: a live-only page is not the end of the table.
		// Capture the cursor before deletion; it contains ordering values and
		// remains valid even when the last row of this page is removed.
		items, result, err := store.SearchCity311StagedAttachments(ctx, svc.store, page)
		if err != nil {
			return err
		}
		expired := expiredAttachmentBatch(items, cutoff)
		if len(expired) > 0 {
			if err = store.DeleteCity311StagedAttachment(ctx, svc.store, expired...); err != nil {
				return err
			}
		}
		if result.NextPage == nil {
			return nil
		}
		page.PageCursor = result.NextPage
	}
}

func expiredAttachmentBatch(items composeTypes.City311StagedAttachmentSet, cutoff time.Time) composeTypes.City311StagedAttachmentSet {
	expired := make(composeTypes.City311StagedAttachmentSet, 0, len(items))
	for _, item := range items {
		if !item.ExpiresAt.After(cutoff) {
			expired = append(expired, item)
		}
	}
	return expired
}

func (svc *Service) StartAttachmentCleanup(ctx context.Context, report func(error)) {
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			if err := svc.CleanupExpiredAttachments(ctx); err != nil && ctx.Err() == nil && report != nil {
				report(err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (svc *Service) attachmentMetadata(ctx context.Context, requestID uint64) ([]contract.AttachmentMetadata, error) {
	items, _, err := store.SearchCity311RequestAttachments(ctx, svc.store, composeTypes.City311RequestAttachmentFilter{RequestID: requestID})
	if err != nil {
		return nil, err
	}
	result := make([]contract.AttachmentMetadata, 0, len(items))
	for _, item := range items {
		result = append(result, contract.AttachmentMetadata{AttachmentID: strconv.FormatUint(item.ID, 10), Filename: item.Filename, MediaType: item.MediaType, Size: item.Size})
	}
	return result, nil
}

// DownloadAttachment applies request-view permission before returning bytes.
// An upload receipt cannot be used here, even by its original uploader.
func (svc *Service) DownloadAttachment(ctx context.Context, actor contract.Actor, attachmentID uint64) (*contract.BinaryAttachment, error) {
	if actor.ID == 0 {
		return nil, apiError(401, contract.ErrorUnauthenticated, "Authentication is required.")
	}
	item, err := store.LookupCity311RequestAttachmentByID(ctx, svc.store, attachmentID)
	if errors.IsNotFound(err) {
		return nil, apiError(404, contract.ErrorNotFound, "The attachment was not found.")
	}
	if err != nil {
		return nil, err
	}
	request, err := store.LookupCity311ServiceRequestByID(ctx, svc.store, item.RequestID)
	if err != nil {
		return nil, err
	}
	if !canRead(actor, request) && !isPrimaryRequester(actor, request) {
		return nil, apiError(403, contract.ErrorForbidden, requestScopeDeniedMessage)
	}
	return &contract.BinaryAttachment{
		ContentType: item.MediaType, ContentDisposition: mime.FormatMediaType("attachment", map[string]string{"filename": item.Filename}),
		Body: base64.StdEncoding.EncodeToString(item.Content), BodyEncoding: "base64",
	}, nil
}

func isPrimaryRequester(actor contract.Actor, request *composeTypes.City311ServiceRequest) bool {
	return hasRole(actor, contract.ApplicationRoleConstituent) && request.PrimaryRequester["constituent_id"] == "C-"+strconv.FormatUint(actor.ID, 10)
}
