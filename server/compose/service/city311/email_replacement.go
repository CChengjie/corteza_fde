package city311

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/errors"
	"github.com/cortezaproject/corteza/server/store"
)

const (
	emailReplacementLifetime = 30 * time.Minute
	emailReplacementKind     = "EMAIL_REPLACEMENT_VERIFICATION"
)

// RequestEmailReplacement starts the proof-of-control flow required by 9.1.2(b).
// A syntactically valid unavailable address receives the same acknowledgement as
// an available address, but no verification token is issued.
func (svc *IdentityService) RequestEmailReplacement(ctx context.Context, resolved *ResolvedSession, input contract.EmailReplacementRequest) (*contract.EmailReplacementAcknowledgement, error) {
	if svc.configErr != nil {
		return nil, svc.configurationUnavailable()
	}
	if err := requireConstituentSession(resolved); err != nil {
		return nil, err
	}
	email := normalizeEmail(input.Email)
	if !validEmail(email) || utf8.RuneCountInString(email) > 254 {
		return nil, validationError(contract.FieldError{Field: "/email", Code: contract.ValidationInvalidFormat})
	}

	acknowledgement := &contract.EmailReplacementAcknowledgement{Accepted: true}
	created := false
	err := store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		if err := store.LockCity311LocalAccount(ctx, tx, resolved.Record.UserID); err != nil {
			return err
		}
		account, err := store.LookupCity311LocalAccountByID(ctx, tx, resolved.Record.UserID)
		if err != nil {
			return err
		}
		now := svc.now()
		if err = svc.invalidateEmailReplacementTokens(ctx, tx, account.ID, now); err != nil {
			return err
		}
		if err = svc.cancelPendingEmailReplacementNotifications(ctx, tx, account.ID, now); err != nil {
			return err
		}
		available, err := verifiedEmailAvailable(ctx, tx, email)
		if err != nil {
			return err
		}
		if !available || normalizeEmail(account.VerifiedEmail) == email {
			return nil
		}

		rawToken, err := randomToken(svc.random)
		if err != nil {
			return err
		}
		token := &composeTypes.City311EmailReplacementToken{
			ID: svc.nextID(), TokenHash: svc.hashToken(rawToken), UserID: account.ID,
			PendingEmail: email, CreatedAt: now, ExpiresAt: now.Add(emailReplacementLifetime),
		}
		if err = store.CreateCity311EmailReplacementToken(ctx, tx, token); err != nil {
			return err
		}
		deliveryKey := "email-replacement-verification:" + strconv.FormatUint(token.ID, 10)
		sealedToken, err := svc.sealNotificationSecret(rawToken, deliveryKey)
		if err != nil {
			return err
		}
		notification := svc.newNotification(account.ID, emailReplacementKind, email, deliveryKey,
			map[string]any{"token_id": strconv.FormatUint(token.ID, 10), "sealed_token": sealedToken})
		if err = store.CreateCity311IdentityNotification(ctx, tx, notification); err != nil {
			return err
		}
		if err = svc.createIdentityAudit(ctx, tx, account.ID, "EMAIL_REPLACEMENT_REQUESTED",
			map[string]any{"verified_email": account.VerifiedEmail}, map[string]any{"pending_email": email}); err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if created {
		svc.wakeNotificationWorker()
	}
	return acknowledgement, nil
}

func verifiedEmailAvailable(ctx context.Context, s store.Storer, email string) (bool, error) {
	_, err := store.LookupCity311LocalAccountByVerifiedEmail(ctx, s, email)
	if errors.IsNotFound(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, nil
}

func (svc *IdentityService) invalidateEmailReplacementTokens(ctx context.Context, tx store.Storer, userID uint64, now time.Time) error {
	tokens, _, err := store.SearchCity311EmailReplacementTokens(ctx, tx, composeTypes.City311EmailReplacementTokenFilter{UserID: userID})
	if err != nil {
		return err
	}
	changed := make(composeTypes.City311EmailReplacementTokenSet, 0, len(tokens))
	for _, token := range tokens {
		if token.UsedAt == nil {
			token.UsedAt = &now
			changed = append(changed, token)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	return store.UpdateCity311EmailReplacementToken(ctx, tx, changed...)
}

func (svc *IdentityService) cancelPendingEmailReplacementNotifications(ctx context.Context, tx store.Storer, userID uint64, now time.Time) error {
	notifications, _, err := store.SearchCity311IdentityNotifications(ctx, tx, composeTypes.City311IdentityNotificationFilter{
		UserID: userID,
		Status: notificationPending,
	})
	if err != nil {
		return err
	}
	cancelled := make(composeTypes.City311IdentityNotificationSet, 0, len(notifications))
	for _, notification := range notifications {
		if notification.Kind != emailReplacementKind {
			continue
		}
		notification.Status = notificationFailed
		notification.LastError = "superseded by a newer email replacement request"
		notification.Recipient = ""
		notification.Payload = composeTypes.City311JSON{}
		notification.UpdatedAt = now
		cancelled = append(cancelled, notification)
	}
	if len(cancelled) == 0 {
		return nil
	}
	return store.UpdateCity311IdentityNotification(ctx, tx, cancelled...)
}

// ConfirmEmailReplacement proves control of the replacement address and changes
// every current identity/profile projection atomically. The token is single use.
func (svc *IdentityService) ConfirmEmailReplacement(ctx context.Context, input contract.EmailReplacementConfirm) (*contract.EmailReplacementResult, error) {
	if svc.configErr != nil {
		return nil, svc.configurationUnavailable()
	}
	rawToken := strings.TrimSpace(input.Token)
	if rawToken == "" {
		return nil, validationError(contract.FieldError{Field: "/token", Code: contract.ValidationRequired})
	}
	if utf8.RuneCountInString(rawToken) > 512 {
		return nil, validationError(contract.FieldError{Field: "/token", Code: contract.ValidationTooLong})
	}
	token, err := store.LookupCity311EmailReplacementTokenByTokenHash(ctx, svc.store, svc.hashToken(rawToken))
	if err = validateEmailReplacementToken(token, err, svc.now()); err != nil {
		return nil, err
	}

	var result *contract.EmailReplacementResult
	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		if err := store.LockCity311LocalAccount(ctx, tx, token.UserID); err != nil {
			return err
		}
		current, err := store.LookupCity311EmailReplacementTokenByID(ctx, tx, token.ID)
		if err = validateEmailReplacementToken(current, err, svc.now()); err != nil {
			return err
		}
		result, err = svc.persistEmailReplacement(ctx, tx, current)
		return err
	})
	if err != nil {
		// A different account can claim the address between request and
		// confirmation. Preserve the non-disclosing invalid-token outcome.
		if existing, lookupErr := store.LookupCity311LocalAccountByVerifiedEmail(ctx, svc.store, token.PendingEmail); lookupErr == nil && existing.ID != token.UserID {
			return nil, invalidEmailVerificationTokenError()
		}
		return nil, err
	}
	svc.wakeNotificationWorker()
	return result, nil
}

func validateEmailReplacementToken(token *composeTypes.City311EmailReplacementToken, lookupErr error, now time.Time) error {
	if lookupErr != nil || token == nil || token.UsedAt != nil {
		return invalidEmailVerificationTokenError()
	}
	if !now.Before(token.ExpiresAt) {
		return apiError(422, contract.ErrorExpiredEmailVerificationToken, "The email verification token has expired.")
	}
	return nil
}

func invalidEmailVerificationTokenError() *ServiceError {
	return apiError(422, contract.ErrorInvalidEmailVerificationToken, "The email verification token is invalid.")
}

func (svc *IdentityService) persistEmailReplacement(ctx context.Context, tx store.Storer, token *composeTypes.City311EmailReplacementToken) (*contract.EmailReplacementResult, error) {
	account, err := store.LookupCity311LocalAccountByID(ctx, tx, token.UserID)
	if err != nil || account.VerifiedEmail == "" {
		return nil, invalidEmailVerificationTokenError()
	}
	available, err := verifiedEmailAvailable(ctx, tx, token.PendingEmail)
	if err != nil {
		return nil, err
	}
	if !available {
		return nil, invalidEmailVerificationTokenError()
	}
	user, err := store.LookupUserByID(ctx, tx, account.ID)
	if err != nil || user.DeletedAt != nil {
		return nil, invalidEmailVerificationTokenError()
	}
	constituent, err := store.LookupCity311ConstituentByConstituentID(ctx, tx, "C-"+strconv.FormatUint(account.ID, 10))
	if err != nil {
		return nil, err
	}

	now := svc.now()
	oldEmail := normalizeEmail(account.VerifiedEmail)
	newEmail := normalizeEmail(token.PendingEmail)
	account.VerifiedEmail = newEmail
	account.UpdatedAt = now
	if err = store.UpdateCity311LocalAccount(ctx, tx, account); err != nil {
		return nil, err
	}
	user.Email = newEmail
	user.EmailConfirmed = true
	user.UpdatedAt = &now
	if err = store.UpdateUser(ctx, tx, user); err != nil {
		return nil, err
	}
	constituent.Profile = cloneMap(constituent.Profile)
	constituent.Profile["emails"] = replaceVerifiedProfileEmail(constituent.Profile["emails"], oldEmail, newEmail)
	if err = advanceProfileVersion(constituent); err != nil {
		return nil, err
	}
	constituent.UpdatedAt = now
	if err = store.UpdateCity311Constituent(ctx, tx, constituent); err != nil {
		return nil, err
	}
	if err = svc.invalidateEmailReplacementTokens(ctx, tx, account.ID, now); err != nil {
		return nil, err
	}
	if err = svc.createIdentityAudit(ctx, tx, account.ID, "VERIFIED_EMAIL_CHANGED",
		map[string]any{"verified_email": oldEmail}, map[string]any{"verified_email": newEmail}); err != nil {
		return nil, err
	}
	notification := svc.newNotification(account.ID, securityNoticeKind, oldEmail,
		"email-replacement-security:"+strconv.FormatUint(token.ID, 10), map[string]any{
			"subject": "Your City 311 email address changed",
			"body":    fmt.Sprintf("Your City 311 verified email address changed from %s to %s.", oldEmail, newEmail),
		})
	if err = store.CreateCity311IdentityNotification(ctx, tx, notification); err != nil {
		return nil, err
	}
	return &contract.EmailReplacementResult{VerifiedEmail: newEmail}, nil
}

func replaceVerifiedProfileEmail(value any, oldEmail, newEmail string) []string {
	result := []string{newEmail}
	seen := map[string]bool{newEmail: true}
	appendEmail := func(value string) {
		normalized := normalizeEmail(value)
		if normalized == "" || normalized == oldEmail || seen[normalized] {
			return
		}
		seen[normalized] = true
		result = append(result, normalized)
	}
	switch emails := value.(type) {
	case []string:
		for _, email := range emails {
			appendEmail(email)
		}
	case []any:
		for _, email := range emails {
			if text, ok := email.(string); ok {
				appendEmail(text)
			}
		}
	}
	return result
}
