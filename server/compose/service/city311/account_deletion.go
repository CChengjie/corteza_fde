package city311

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/errors"
	"github.com/cortezaproject/corteza/server/store"
	systemTypes "github.com/cortezaproject/corteza/server/system/types"
)

const (
	deletedAccountDisplayName = "Deleted constituent"
	deletedUserDisplayName    = "Deleted user"
	deletedAccountLanguage    = string(contract.LanguageEN)
	deletedAccountError       = "account deleted"
)

// DeleteAccount fulfils the constituent's erasure request without removing
// request records or immutable audit/history evidence. All identity and
// current-profile projections are changed in one transaction.
func (svc *IdentityService) DeleteAccount(ctx context.Context, resolved *ResolvedSession) error {
	if svc.configErr != nil {
		return svc.configurationUnavailable()
	}
	if err := requireConstituentSession(resolved); err != nil {
		return err
	}
	userID := resolved.User.ID
	now := svc.now().UTC()
	return store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		if err := store.LockCity311LocalAccount(ctx, tx, userID); err != nil {
			return err
		}
		account, err := store.LookupCity311LocalAccountByID(ctx, tx, userID)
		if err != nil {
			return err
		}
		user, err := store.LookupUserByID(ctx, tx, userID)
		if err != nil {
			return err
		}
		// A stale resolved session can retry safely after the first committed
		// deletion. Do not rewrite the alias or append another audit event.
		if user.DeletedAt != nil {
			return nil
		}
		constituent, err := store.LookupCity311ConstituentByConstituentID(ctx, tx, "C-"+strconv.FormatUint(userID, 10))
		if err != nil {
			return err
		}
		alias := svc.deletedAccountAlias(userID)
		if err = svc.anonymiseLocalAccount(ctx, tx, account, alias, now); err != nil {
			return err
		}
		if err = svc.anonymiseUser(ctx, tx, user, alias, now); err != nil {
			return err
		}
		if err = anonymiseConstituent(constituent, now); err != nil {
			return err
		}
		if err = store.UpdateCity311Constituent(ctx, tx, constituent); err != nil {
			return err
		}
		if err = svc.revokeAccountCredentials(ctx, tx, userID, now); err != nil {
			return err
		}
		if err = svc.revokeAccountSessions(ctx, tx, userID); err != nil {
			return err
		}
		if err = svc.invalidateAccountResetTokens(ctx, tx, userID, now); err != nil {
			return err
		}
		if err = svc.invalidateEmailReplacementTokens(ctx, tx, userID, now); err != nil {
			return err
		}
		if err = svc.cancelAccountNotifications(ctx, tx, userID, now); err != nil {
			return err
		}
		if err = svc.cancelRequestNotifications(ctx, tx, constituent.ConstituentID, now); err != nil {
			return err
		}
		if err = svc.removeAccountAuthorisation(ctx, tx, userID, now); err != nil {
			return err
		}
		return svc.createIdentityAudit(ctx, tx, userID, "ACCOUNT_DELETED", nil, map[string]any{
			"anonymised": true,
		})
	})
}

func (svc *IdentityService) deletedAccountAlias(userID uint64) string {
	mac := hmac.New(sha256.New, svc.secret)
	_, _ = mac.Write([]byte("city311-account-deletion\x00"))
	_, _ = mac.Write([]byte(strconv.FormatUint(userID, 10)))
	digest := mac.Sum(nil)
	return "deleted-" + hex.EncodeToString(digest[:12])
}

func (svc *IdentityService) anonymiseLocalAccount(ctx context.Context, tx store.Storer, account *composeTypes.City311LocalAccount, alias string, now time.Time) error {
	account.LoginIdentifier = alias
	account.VerifiedEmail = ""
	account.PreferredLanguage = deletedAccountLanguage
	account.UpdatedAt = now
	return store.UpdateCity311LocalAccount(ctx, tx, account)
}

func (svc *IdentityService) anonymiseUser(ctx context.Context, tx store.Storer, user *systemTypes.User, alias string, now time.Time) error {
	user.Username = alias
	user.Handle = alias
	user.Email = alias + "@invalid"
	user.Name = deletedUserDisplayName
	user.Meta = &systemTypes.UserMeta{PreferredLanguage: "en"}
	user.Labels = nil
	user.EmailConfirmed = false
	user.DeletedAt = &now
	user.UpdatedAt = &now
	return store.UpdateUser(ctx, tx, user)
}

func anonymiseConstituent(item *composeTypes.City311Constituent, now time.Time) error {
	item.Profile = cloneMap(item.Profile)
	item.Profile["constituent_id"] = item.ConstituentID
	item.Profile["display_name"] = deletedAccountDisplayName
	item.Profile["emails"] = []string{}
	item.Profile["phone_numbers"] = []any{}
	item.Profile["addresses"] = []any{}
	item.Profile["primary_category"] = string(contract.ContactCategoryOther)
	item.Profile["preferred_language"] = deletedAccountLanguage
	item.Profile["email_opt_out"] = true
	delete(item.Profile, "login_identifier")
	delete(item.Profile, "custom_fields")
	if err := advanceProfileVersion(item); err != nil {
		return err
	}
	item.UpdatedAt = now
	return nil
}

func (svc *IdentityService) revokeAccountCredentials(ctx context.Context, tx store.Storer, userID uint64, now time.Time) error {
	credentials, _, err := store.SearchCredentials(ctx, tx, systemTypes.CredentialFilter{OwnerID: userID, Kind: passwordCredentialKind})
	if err != nil {
		return err
	}
	for _, credential := range credentials {
		credential.DeletedAt = &now
	}
	if len(credentials) == 0 {
		return nil
	}
	return store.UpdateCredential(ctx, tx, credentials...)
}

func (svc *IdentityService) revokeAccountSessions(ctx context.Context, tx store.Storer, userID uint64) error {
	sessions, _, err := store.SearchCity311IdentitySessions(ctx, tx, composeTypes.City311IdentitySessionFilter{UserID: userID})
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if err = store.DeleteCity311IdentitySessionByID(ctx, tx, session.ID); err != nil {
			return err
		}
	}
	return nil
}

func (svc *IdentityService) invalidateAccountResetTokens(ctx context.Context, tx store.Storer, userID uint64, now time.Time) error {
	tokens, _, err := store.SearchCity311PasswordResetTokens(ctx, tx, composeTypes.City311PasswordResetTokenFilter{UserID: userID})
	if err != nil {
		return err
	}
	changed := make(composeTypes.City311PasswordResetTokenSet, 0, len(tokens))
	for _, token := range tokens {
		if token.UsedAt == nil {
			token.UsedAt = &now
			changed = append(changed, token)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	return store.UpdateCity311PasswordResetToken(ctx, tx, changed...)
}

func (svc *IdentityService) cancelAccountNotifications(ctx context.Context, tx store.Storer, userID uint64, now time.Time) error {
	notifications, _, err := store.SearchCity311IdentityNotifications(ctx, tx, composeTypes.City311IdentityNotificationFilter{UserID: userID})
	if err != nil {
		return err
	}
	for _, notification := range notifications {
		notification.Recipient = ""
		notification.DeliveryKey = ""
		notification.Payload = composeTypes.City311JSON{}
		if notification.Status == notificationPending {
			notification.Status = notificationFailed
			notification.LastError = deletedAccountError
		}
		notification.UpdatedAt = now
	}
	if len(notifications) == 0 {
		return nil
	}
	return store.UpdateCity311IdentityNotification(ctx, tx, notifications...)
}

func (svc *IdentityService) cancelRequestNotifications(ctx context.Context, tx store.Storer, constituentID string, now time.Time) error {
	operations, _, err := store.SearchCity311Operations(ctx, tx, composeTypes.City311OperationFilter{Kind: requestNotificationOperationKind})
	if err != nil {
		return err
	}
	for _, operation := range operations {
		payload := requestNotificationPayload{}
		encoded, marshalErr := json.Marshal(operation.Result)
		if marshalErr != nil || json.Unmarshal(encoded, &payload) != nil || payload.ConstituentID != constituentID {
			continue
		}
		payload.Recipient = ""
		payload.DeliveryKey = ""
		payload.DeliveryStatus = mailStatusFailed
		result, mapErr := mapFrom(payload)
		if mapErr != nil {
			return mapErr
		}
		operation.Result = result
		if operation.Status == mailStatusPending {
			operation.Status = mailStatusFailed
			operation.Error = composeTypes.City311JSON{"error": deletedAccountError}
			operation.CompletedAt = &now
		}
		operation.UpdatedAt = now
		if err = store.UpdateCity311Operation(ctx, tx, operation); err != nil {
			return err
		}
	}
	return nil
}

func (svc *IdentityService) removeAccountAuthorisation(ctx context.Context, tx store.Storer, userID uint64, now time.Time) error {
	profile, err := store.LookupCity311ActorProfileByID(ctx, tx, userID)
	if err == nil {
		profile.ApplicationRoles = nil
		profile.Department = ""
		profile.Districts = nil
		profile.UpdatedAt = now
		if err = store.UpdateCity311ActorProfile(ctx, tx, profile); err != nil {
			return err
		}
	} else if !errors.IsNotFound(err) {
		return err
	}
	resource := fmt.Sprintf("corteza::system:user/%d", userID)
	memberships, _, err := store.SearchRoleMembers(ctx, tx, systemTypes.RoleMemberFilter{Resource: resource})
	if err != nil {
		return err
	}
	for _, membership := range memberships {
		if err = store.DeleteRoleMember(ctx, tx, membership); err != nil {
			return err
		}
	}
	return nil
}
