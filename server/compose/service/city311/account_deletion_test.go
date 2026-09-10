package city311

import (
	"context"
	"strconv"
	"testing"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/filter"
	"github.com/cortezaproject/corteza/server/store"
	systemTypes "github.com/cortezaproject/corteza/server/system/types"
	"github.com/stretchr/testify/require"
)

func TestAccountDeletionAnonymisesIdentityAndPreservesRequests(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	identity := NewIdentity(st, IdentityOptions{Secret: []byte("account-deletion-secret"), Now: svc.now, NextID: svc.nextID})
	_, resolved, err := identity.SignIn(ctx, contract.LocalSignIn{LoginIdentifier: "city311-constituent", Password: "SeedConstituentPassword1!"})
	require.NoError(t, err)
	userID := resolved.User.ID
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	original := cloneMap((mustConstituent(t, ctx, st, "C-"+strconv.FormatUint(userID, 10))).Profile)
	// Seed all deletion-owned queues so the test exercises both pending and
	// already-completed cleanup paths.
	now := svc.now()
	token := &composeTypes.City311PasswordResetToken{
		ID: svc.nextID(), TokenHash: "reset-token-hash", UserID: userID,
		CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	require.NoError(t, store.CreateCity311PasswordResetToken(ctx, st, token))
	emailToken := &composeTypes.City311EmailReplacementToken{
		ID: svc.nextID(), TokenHash: "email-replacement-token-hash", UserID: userID,
		PendingEmail: "replacement@example.invalid", CreatedAt: now, ExpiresAt: now.Add(time.Hour),
	}
	require.NoError(t, store.CreateCity311EmailReplacementToken(ctx, st, emailToken))
	pending := &composeTypes.City311IdentityNotification{
		ID: svc.nextID(), UserID: userID, Kind: passwordResetKind, Recipient: "constituent1@city311.example.invalid",
		DeliveryKey: "password-reset:pending", Payload: composeTypes.City311JSON{"token_id": "pending"},
		Status: notificationPending, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, store.CreateCity311IdentityNotification(ctx, st, pending))
	validPayload, err := mapFrom(requestNotificationPayload{
		RequestID: request.ID, RequestNumber: request.RequestNumber, ConstituentID: "C-" + strconv.FormatUint(userID, 10),
		Recipient: "constituent1@city311.example.invalid", DeliveryKey: "request:old", DeliveryStatus: mailStatusPending,
	})
	require.NoError(t, err)
	matchingOperation := &composeTypes.City311Operation{
		ID: svc.nextID(), Kind: requestNotificationOperationKind, Status: mailStatusPending,
		Result: validPayload, Error: composeTypes.City311JSON{}, CreatedAt: now, UpdatedAt: now,
	}
	malformedOperation := &composeTypes.City311Operation{
		ID: svc.nextID(), Kind: requestNotificationOperationKind, Status: mailStatusPending,
		Result: composeTypes.City311JSON{"unexpected": true}, Error: composeTypes.City311JSON{}, CreatedAt: now, UpdatedAt: now,
	}
	require.NoError(t, store.CreateCity311Operation(ctx, st, matchingOperation, malformedOperation))

	require.NoError(t, identity.DeleteAccount(ctx, resolved))

	account, err := store.LookupCity311LocalAccountByID(ctx, st, userID)
	require.NoError(t, err)
	require.NotContains(t, account.LoginIdentifier, "city311-constituent")
	require.Empty(t, account.VerifiedEmail)
	user, err := store.LookupUserByID(ctx, st, userID)
	require.NoError(t, err)
	require.NotNil(t, user.DeletedAt)
	require.NotContains(t, user.Email, "constituent1@city311.example.invalid")
	require.NotContains(t, user.Name, "City 311 Constituent")
	profile := mustConstituent(t, ctx, st, "C-"+strconv.FormatUint(userID, 10))
	require.Equal(t, deletedAccountDisplayName, profile.Profile["display_name"])
	require.Empty(t, profile.Profile["emails"])
	require.Empty(t, profile.Profile["phone_numbers"])
	require.Empty(t, profile.Profile["addresses"])
	require.Equal(t, string(contract.ContactCategoryOther), profile.Profile["primary_category"])
	require.NotContains(t, profile.Profile, "login_identifier")
	require.NotContains(t, profile.Profile, "custom_fields")
	require.NotEqual(t, original["display_name"], profile.Profile["display_name"])

	credentials, _, err := store.SearchCredentials(ctx, st, systemTypes.CredentialFilter{OwnerID: userID, Kind: passwordCredentialKind, Deleted: filter.StateInclusive})
	require.NoError(t, err)
	require.NotEmpty(t, credentials)
	for _, credential := range credentials {
		require.NotNil(t, credential.DeletedAt)
	}
	sessions, _, err := store.SearchCity311IdentitySessions(ctx, st, composeTypes.City311IdentitySessionFilter{UserID: userID})
	require.NoError(t, err)
	require.Empty(t, sessions)
	tokens, _, err := store.SearchCity311PasswordResetTokens(ctx, st, composeTypes.City311PasswordResetTokenFilter{UserID: userID})
	require.NoError(t, err)
	require.NotEmpty(t, tokens)
	for _, token := range tokens {
		require.NotNil(t, token.UsedAt)
	}
	emailTokens, _, err := store.SearchCity311EmailReplacementTokens(ctx, st, composeTypes.City311EmailReplacementTokenFilter{UserID: userID})
	require.NoError(t, err)
	require.NotEmpty(t, emailTokens)
	for _, token := range emailTokens {
		require.NotNil(t, token.UsedAt)
	}
	notifications, _, err := store.SearchCity311IdentityNotifications(ctx, st, composeTypes.City311IdentityNotificationFilter{UserID: userID})
	require.NoError(t, err)
	require.Len(t, notifications, 1)
	for _, notification := range notifications {
		require.Empty(t, notification.Recipient)
		require.Empty(t, notification.DeliveryKey)
		require.Empty(t, notification.Payload)
	}
	operations, _, err := store.SearchCity311Operations(ctx, st, composeTypes.City311OperationFilter{Kind: requestNotificationOperationKind})
	require.NoError(t, err)
	require.Len(t, operations, 2)
	for _, operation := range operations {
		if operation.ID == matchingOperation.ID {
			require.Equal(t, mailStatusFailed, operation.Status)
			require.Empty(t, operation.Result["recipient"])
			require.Empty(t, operation.Result["delivery_key"])
			require.Equal(t, mailStatusFailed, operation.Result["delivery_status"])
		} else {
			require.Equal(t, mailStatusPending, operation.Status)
			require.Equal(t, true, operation.Result["unexpected"])
		}
	}
	memberships, _, err := store.SearchRoleMembers(ctx, st, systemTypes.RoleMemberFilter{Resource: "corteza::system:user/" + strconv.FormatUint(userID, 10)})
	require.NoError(t, err)
	require.Empty(t, memberships)

	require.NoError(t, identity.DeleteAccount(ctx, resolved))
	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EntityType: "account", EntityID: strconv.FormatUint(userID, 10), EventType: "ACCOUNT_DELETED"})
	require.NoError(t, err)
	require.Len(t, audits, 1)
	require.NotContains(t, audits[0].After, "constituent1@city311.example.invalid")
	retained, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, request.ID, retained.ID)
}

func mustConstituent(t *testing.T, ctx context.Context, st store.Storer, id string) *composeTypes.City311Constituent {
	item, err := store.LookupCity311ConstituentByConstituentID(ctx, st, id)
	require.NoError(t, err)
	return item
}
