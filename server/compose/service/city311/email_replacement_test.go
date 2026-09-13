package city311

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func registerEmailReplacementAccount(t *testing.T, identity *IdentityService, identifier, email string) (*ResolvedSession, contract.AccountRegistration) {
	t.Helper()
	input := validAccountRegistration(identifier, email)
	acknowledgement, err := identity.Register(context.Background(), input)
	require.NoError(t, err)
	require.True(t, acknowledgement.Accepted)
	_, resolved, err := identity.SignIn(context.Background(), contract.LocalSignIn{
		LoginIdentifier: identifier,
		Password:        input.Password,
	})
	require.NoError(t, err)
	return resolved, input
}

func TestVerifiedEmailReplacementRetainsOldAddressUntilConfirmation(t *testing.T) {
	identity, st, notifier, _ := testIdentityService(t)
	ctx := context.Background()
	resolved, registration := registerEmailReplacementAccount(t, identity, "replacement.owner", "owner@example.invalid")
	profileBefore, err := identity.GetProfileSnapshot(ctx, resolved)
	require.NoError(t, err)

	acknowledgement, err := identity.RequestEmailReplacement(ctx, resolved, contract.EmailReplacementRequest{Email: "  NEW.Owner@Example.Invalid "})
	require.NoError(t, err)
	require.True(t, acknowledgement.Accepted)

	account, err := store.LookupCity311LocalAccountByID(ctx, st, resolved.User.ID)
	require.NoError(t, err)
	require.Equal(t, registration.Email, account.VerifiedEmail)
	user, err := store.LookupUserByID(ctx, st, resolved.User.ID)
	require.NoError(t, err)
	require.Equal(t, registration.Email, user.Email)
	profile, err := identity.GetProfileSnapshot(ctx, resolved)
	require.NoError(t, err)
	require.Equal(t, profileBefore, profile)

	tokens, _, err := store.SearchCity311EmailReplacementTokens(ctx, st, composeTypes.City311EmailReplacementTokenFilter{UserID: resolved.User.ID})
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	require.NotEmpty(t, tokens[0].TokenHash)
	require.NotEqual(t, "NEW.Owner@Example.Invalid", tokens[0].TokenHash)
	require.Equal(t, "new.owner@example.invalid", tokens[0].PendingEmail)
	require.Equal(t, 30*time.Minute, tokens[0].ExpiresAt.Sub(tokens[0].CreatedAt))
	notifications, _, err := store.SearchCity311IdentityNotifications(ctx, st, composeTypes.City311IdentityNotificationFilter{UserID: resolved.User.ID})
	require.NoError(t, err)
	require.Len(t, notifications, 1)
	require.NotEqual(t, tokens[0].TokenHash, fmt.Sprint(notifications[0].Payload["sealed_token"]))

	require.NoError(t, identity.RetryPendingNotifications(ctx))
	require.Equal(t, []string{"new.owner@example.invalid"}, notifier.replacementRecipients)
	require.Len(t, notifier.replacementTokens, 1)
	require.NotEqual(t, notifier.replacementTokens[0], tokens[0].TokenHash, "raw token must not be stored")
	require.NotEqual(t, notifier.replacementTokens[0], fmt.Sprint(notifications[0].Payload["sealed_token"]), "outbox token must be encrypted")

	result, err := identity.ConfirmEmailReplacement(ctx, contract.EmailReplacementConfirm{Token: notifier.replacementTokens[0]})
	require.NoError(t, err)
	require.Equal(t, "new.owner@example.invalid", result.VerifiedEmail)
	account, err = store.LookupCity311LocalAccountByID(ctx, st, resolved.User.ID)
	require.NoError(t, err)
	require.Equal(t, result.VerifiedEmail, account.VerifiedEmail)
	user, err = store.LookupUserByID(ctx, st, resolved.User.ID)
	require.NoError(t, err)
	require.Equal(t, result.VerifiedEmail, user.Email)
	require.True(t, user.EmailConfirmed)
	constituent := mustConstituent(t, ctx, st, "C-"+strconv.FormatUint(resolved.User.ID, 10))
	require.Equal(t, []any{result.VerifiedEmail}, constituent.Profile["emails"])
	version, err := profileVersion(constituent)
	require.NoError(t, err)
	require.Equal(t, profileBefore.Version+1, version)

	_, err = identity.ConfirmEmailReplacement(ctx, contract.EmailReplacementConfirm{Token: notifier.replacementTokens[0]})
	requireIdentityError(t, err, 422, contract.ErrorInvalidEmailVerificationToken)
	_, _, err = identity.SignIn(ctx, contract.LocalSignIn{LoginIdentifier: registration.Email, Password: registration.Password})
	requireIdentityError(t, err, 401, contract.ErrorUnauthenticated)
	_, _, err = identity.SignIn(ctx, contract.LocalSignIn{LoginIdentifier: result.VerifiedEmail, Password: registration.Password})
	require.NoError(t, err)
	currentAcknowledgement, err := identity.RequestEmailReplacement(ctx, resolved, contract.EmailReplacementRequest{Email: result.VerifiedEmail})
	require.NoError(t, err)
	require.True(t, currentAcknowledgement.Accepted)

	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{
		EntityType: "account", EntityID: strconv.FormatUint(resolved.User.ID, 10),
	})
	require.NoError(t, err)
	requestAudits, changeAudits := 0, 0
	for _, audit := range audits {
		switch audit.EventType {
		case "EMAIL_REPLACEMENT_REQUESTED":
			requestAudits++
		case "VERIFIED_EMAIL_CHANGED":
			changeAudits++
		}
	}
	require.Equal(t, 1, requestAudits)
	require.Equal(t, 1, changeAudits)
	require.NoError(t, identity.RetryPendingNotifications(ctx))
	require.Contains(t, notifier.securityNotices[0], registration.Email)
	require.Contains(t, notifier.securityBodies[0], result.VerifiedEmail)
}

func TestEmailReplacementRejectsUnavailableRuntimeAndInvalidActor(t *testing.T) {
	identity, st, _, now := testIdentityService(t)
	ctx := context.Background()
	resolved, _ := registerEmailReplacementAccount(t, identity, "replacement.boundary", "boundary@example.invalid")

	unavailable := NewIdentity(st, IdentityOptions{
		Secret:             identity.secret,
		Now:                func() time.Time { return *now },
		ConfigurationError: errors.New("identity configuration unavailable"),
	})
	_, err := unavailable.RequestEmailReplacement(ctx, resolved, contract.EmailReplacementRequest{Email: "new-boundary@example.invalid"})
	requireIdentityError(t, err, 503, contract.ErrorTemporarilyUnavailable)
	_, err = unavailable.ConfirmEmailReplacement(ctx, contract.EmailReplacementConfirm{Token: "token"})
	requireIdentityError(t, err, 503, contract.ErrorTemporarilyUnavailable)

	_, err = identity.RequestEmailReplacement(ctx, nil, contract.EmailReplacementRequest{Email: "new-boundary@example.invalid"})
	requireIdentityError(t, err, 401, contract.ErrorUnauthenticated)
	staffResolved := *resolved
	staffActor := *resolved.Actor
	staffActor.ApplicationRoles = []contract.ApplicationRole{contract.ApplicationRoleServiceAgent}
	staffResolved.Actor = &staffActor
	_, err = identity.RequestEmailReplacement(ctx, &staffResolved, contract.EmailReplacementRequest{Email: "new-boundary@example.invalid"})
	requireIdentityError(t, err, 403, contract.ErrorForbidden)
	_, err = identity.ConfirmEmailReplacement(ctx, contract.EmailReplacementConfirm{Token: "  "})
	requireIdentityError(t, err, 422, contract.ErrorValidation)
	_, err = identity.ConfirmEmailReplacement(ctx, contract.EmailReplacementConfirm{Token: strings.Repeat("x", 513)})
	requireIdentityError(t, err, 422, contract.ErrorValidation)

	failingRandom := NewIdentity(st, IdentityOptions{
		Secret: identity.secret, Now: func() time.Time { return *now }, Random: failingIdentityReader{err: errors.New("random unavailable")},
	})
	_, err = failingRandom.RequestEmailReplacement(ctx, resolved, contract.EmailReplacementRequest{Email: "new-boundary@example.invalid"})
	require.EqualError(t, err, "random unavailable")
}

func TestEmailReplacementFailsClosedIfAddressIsClaimedBeforeConfirmation(t *testing.T) {
	identity, st, notifier, _ := testIdentityService(t)
	ctx := context.Background()
	owner, _ := registerEmailReplacementAccount(t, identity, "replacement.race", "race@example.invalid")
	_, err := identity.RequestEmailReplacement(ctx, owner, contract.EmailReplacementRequest{Email: "claimed-during-confirm@example.invalid"})
	require.NoError(t, err)
	require.NoError(t, identity.RetryPendingNotifications(ctx))
	require.Len(t, notifier.replacementTokens, 1)

	_, _ = registerEmailReplacementAccount(t, identity, "replacement.race.winner", "claimed-during-confirm@example.invalid")
	_, err = identity.ConfirmEmailReplacement(ctx, contract.EmailReplacementConfirm{Token: notifier.replacementTokens[0]})
	requireIdentityError(t, err, 422, contract.ErrorInvalidEmailVerificationToken)
	account, err := store.LookupCity311LocalAccountByID(ctx, st, owner.User.ID)
	require.NoError(t, err)
	require.Equal(t, "race@example.invalid", account.VerifiedEmail)
}

func TestEmailReplacementPreservesDistinctSecondaryProfileAddresses(t *testing.T) {
	want := []string{"new@example.invalid", "secondary@example.invalid"}
	require.Equal(t, want, replaceVerifiedProfileEmail(
		[]string{"OLD@example.invalid", "secondary@example.invalid", "SECONDARY@example.invalid"},
		"old@example.invalid", "new@example.invalid",
	))
	require.Equal(t, want, replaceVerifiedProfileEmail(
		[]any{"old@example.invalid", 42, "secondary@example.invalid", ""},
		"old@example.invalid", "new@example.invalid",
	))
}

func TestEmailReplacementIsPrivacySafeAndNewestRequestWins(t *testing.T) {
	identity, st, notifier, now := testIdentityService(t)
	ctx := context.Background()
	owner, _ := registerEmailReplacementAccount(t, identity, "replacement.privacy", "privacy@example.invalid")
	_, _ = registerEmailReplacementAccount(t, identity, "replacement.other", "claimed@example.invalid")

	first, err := identity.RequestEmailReplacement(ctx, owner, contract.EmailReplacementRequest{Email: "first@example.invalid"})
	require.NoError(t, err)
	taken, err := identity.RequestEmailReplacement(ctx, owner, contract.EmailReplacementRequest{Email: "claimed@example.invalid"})
	require.NoError(t, err)
	require.Equal(t, first, taken, "claimed addresses must be indistinguishable")

	tokens, _, err := store.SearchCity311EmailReplacementTokens(ctx, st, composeTypes.City311EmailReplacementTokenFilter{UserID: owner.User.ID})
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	require.NotNil(t, tokens[0].UsedAt, "the latest valid request must invalidate older tokens")
	require.NoError(t, identity.RetryPendingNotifications(ctx))
	require.Empty(t, notifier.replacementTokens, "superseded verification must not be delivered")
	notifications, _, err := store.SearchCity311IdentityNotifications(ctx, st, composeTypes.City311IdentityNotificationFilter{UserID: owner.User.ID})
	require.NoError(t, err)
	require.Len(t, notifications, 1)
	require.Equal(t, notificationFailed, notifications[0].Status)
	require.Empty(t, notifications[0].Recipient)
	require.Empty(t, notifications[0].Payload)

	_, err = identity.RequestEmailReplacement(ctx, owner, contract.EmailReplacementRequest{Email: "second@example.invalid"})
	require.NoError(t, err)
	require.NoError(t, identity.RetryPendingNotifications(ctx))
	require.Len(t, notifier.replacementTokens, 1)
	secondToken := notifier.replacementTokens[0]
	_, err = identity.RequestEmailReplacement(ctx, owner, contract.EmailReplacementRequest{Email: "third@example.invalid"})
	require.NoError(t, err)
	require.NoError(t, identity.RetryPendingNotifications(ctx))
	require.Len(t, notifier.replacementTokens, 2)
	_, err = identity.ConfirmEmailReplacement(ctx, contract.EmailReplacementConfirm{Token: secondToken})
	requireIdentityError(t, err, 422, contract.ErrorInvalidEmailVerificationToken)

	*now = now.Add(emailReplacementLifetime)
	_, err = identity.ConfirmEmailReplacement(ctx, contract.EmailReplacementConfirm{Token: notifier.replacementTokens[1]})
	requireIdentityError(t, err, 422, contract.ErrorExpiredEmailVerificationToken)

	invalid, err := identity.RequestEmailReplacement(ctx, owner, contract.EmailReplacementRequest{Email: "not an email"})
	require.Nil(t, invalid)
	requireIdentityError(t, err, 422, contract.ErrorValidation)
}

func TestEmailReplacementNotificationSurvivesRestartWithoutRawTokenPersistence(t *testing.T) {
	identity, st, _, now := testIdentityService(t)
	ctx := context.Background()
	owner, _ := registerEmailReplacementAccount(t, identity, "replacement.restart", "restart@example.invalid")
	_, err := identity.RequestEmailReplacement(ctx, owner, contract.EmailReplacementRequest{Email: "after-restart@example.invalid"})
	require.NoError(t, err)

	recoveredNotifier := &identityNotifierCapture{}
	next := uint64(930_000_000_000_000_000)
	restarted := NewIdentity(st, IdentityOptions{
		Secret:   identity.secret,
		Now:      func() time.Time { return *now },
		NextID:   func() uint64 { next++; return next },
		Notifier: recoveredNotifier,
	})
	require.NoError(t, restarted.RetryPendingNotifications(ctx))
	require.Equal(t, []string{"after-restart@example.invalid"}, recoveredNotifier.replacementRecipients)
	require.Len(t, recoveredNotifier.replacementTokens, 1)
	result, err := restarted.ConfirmEmailReplacement(ctx, contract.EmailReplacementConfirm{Token: recoveredNotifier.replacementTokens[0]})
	require.NoError(t, err)
	require.Equal(t, "after-restart@example.invalid", result.VerifiedEmail)
}
