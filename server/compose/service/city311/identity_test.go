package city311

import (
	"context"
	"errors"
	"io"
	"net/textproto"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	authSettings "github.com/cortezaproject/corteza/server/auth/settings"
	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/filter"
	"github.com/cortezaproject/corteza/server/pkg/id"
	"github.com/cortezaproject/corteza/server/store"
	systemTypes "github.com/cortezaproject/corteza/server/system/types"
	"github.com/stretchr/testify/require"
)

type identityNotifierCapture struct {
	resetTokens          []string
	resetRecipients      []string
	resetDeliveryKeys    []string
	securityNotices      []string
	securityDeliveryKeys []string
}

type failingIdentityNotifier struct{ err error }

type sequencedIdentityNotifier struct {
	failures     []error
	tokens       []string
	deliveryKeys []string
	accepted     int
}

type blockingIdentityNotifier struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	calls   atomic.Int32
}

func (notifier failingIdentityNotifier) PasswordReset(context.Context, string, string, string) error {
	return notifier.err
}

func (notifier failingIdentityNotifier) SecurityNotice(context.Context, string, string, string, string) error {
	return notifier.err
}

func (notifier *sequencedIdentityNotifier) PasswordReset(_ context.Context, _ string, token, deliveryKey string) error {
	notifier.tokens = append(notifier.tokens, token)
	notifier.deliveryKeys = append(notifier.deliveryKeys, deliveryKey)
	index := len(notifier.tokens) - 1
	if index < len(notifier.failures) {
		return notifier.failures[index]
	}
	notifier.accepted++
	return nil
}

func (notifier *sequencedIdentityNotifier) SecurityNotice(context.Context, string, string, string, string) error {
	return nil
}

func (notifier *blockingIdentityNotifier) PasswordReset(ctx context.Context, _, _, _ string) error {
	notifier.calls.Add(1)
	notifier.once.Do(func() { close(notifier.started) })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-notifier.release:
		return nil
	}
}

func (notifier *blockingIdentityNotifier) SecurityNotice(context.Context, string, string, string, string) error {
	return nil
}

type failingIdentityReader struct{ err error }

func (reader failingIdentityReader) Read([]byte) (int, error) { return 0, reader.err }

func (capture *identityNotifierCapture) PasswordReset(_ context.Context, recipient, token, deliveryKey string) error {
	capture.resetRecipients = append(capture.resetRecipients, recipient)
	capture.resetTokens = append(capture.resetTokens, token)
	capture.resetDeliveryKeys = append(capture.resetDeliveryKeys, deliveryKey)
	return nil
}

func (capture *identityNotifierCapture) SecurityNotice(_ context.Context, recipient, subject, _ string, deliveryKey string) error {
	capture.securityNotices = append(capture.securityNotices, recipient+":"+subject)
	capture.securityDeliveryKeys = append(capture.securityDeliveryKeys, deliveryKey)
	return nil
}

func testIdentityService(t *testing.T) (*IdentityService, store.Storer, *identityNotifierCapture, *time.Time) {
	t.Helper()
	id.Init(context.Background())
	base, st := testService(t)
	ctx := context.Background()
	require.NoError(t, base.Seed(ctx, base.now()))
	now := base.now()
	next := uint64(910_000_000_000_000_000)
	notifier := &identityNotifierCapture{}
	identity := NewIdentity(st, IdentityOptions{
		Secret: []byte("test-only-city311-session-secret"), Now: func() time.Time { return now },
		NextID: func() uint64 { next++; return next }, Notifier: notifier,
	})
	return identity, st, notifier, &now
}

func validAccountRegistration(identifier, email string) contract.AccountRegistration {
	return contract.AccountRegistration{
		DisplayName: "Alex Resident", Email: email, LoginIdentifier: identifier,
		Password: "StrongPassword1!", PreferredLanguage: contract.LanguageEN,
	}
}

func requireIdentityError(t *testing.T, err error, status int, code contract.ErrorCode) *ServiceError {
	t.Helper()
	var serviceErr *ServiceError
	require.ErrorAs(t, err, &serviceErr)
	require.Equal(t, status, serviceErr.Status)
	require.Equal(t, code, serviceErr.Payload.Error)
	return serviceErr
}

func TestIdentityRegistrationIsValidatedPrivateAndFullyProvisioned(t *testing.T) {
	identity, st, _, _ := testIdentityService(t)
	ctx := context.Background()

	invalid := validAccountRegistration("invalid identifier", "not-an-email")
	invalid.Password = "weak"
	invalid.PreferredLanguage = contract.Language("FR")
	_, err := identity.Register(ctx, invalid)
	validation := requireIdentityError(t, err, 422, contract.ErrorValidation)
	require.Len(t, validation.Payload.Errors, 4)

	input := validAccountRegistration("alex.resident", "alex@example.invalid")
	acknowledgement, err := identity.Register(ctx, input)
	require.NoError(t, err)
	require.True(t, acknowledgement.Accepted)

	account, err := store.LookupCity311LocalAccountByLoginIdentifier(ctx, st, input.LoginIdentifier)
	require.NoError(t, err)
	require.Equal(t, input.Email, account.VerifiedEmail)
	user, err := store.LookupUserByID(ctx, st, account.ID)
	require.NoError(t, err)
	require.Equal(t, input.DisplayName, user.Name)
	profile, err := store.LookupCity311ActorProfileByID(ctx, st, account.ID)
	require.NoError(t, err)
	require.Equal(t, composeTypes.City311ApplicationRoleSet{contract.ApplicationRoleConstituent}, profile.ApplicationRoles)
	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{
		EntityType: "account", EntityID: strconv.FormatUint(account.ID, 10), EventType: "ACCOUNT_REGISTERED",
	})
	require.NoError(t, err)
	require.Len(t, audits, 1)

	// Existing identifiers and email addresses remain indistinguishable to callers.
	duplicate := validAccountRegistration(input.LoginIdentifier, "other@example.invalid")
	acknowledgement, err = identity.Register(ctx, duplicate)
	require.NoError(t, err)
	require.True(t, acknowledgement.Accepted)
	fetched, err := store.LookupCity311LocalAccountByLoginIdentifier(ctx, st, input.LoginIdentifier)
	require.NoError(t, err)
	require.Equal(t, input.Email, fetched.VerifiedEmail)
}

func TestDefaultIdentityRequiresRuntimeSecretAndAbsoluteBaseURL(t *testing.T) {
	_, st := testService(t)
	t.Setenv("SESSION_SECRET", "")
	_, err := NewDefaultIdentity(st, time.Now)
	require.EqualError(t, err, "SESSION_SECRET is required for City 311 sessions")

	t.Setenv("SESSION_SECRET", "runtime-provided-secret")
	t.Setenv("APP_BASE_URL", "relative-url")
	_, err = NewDefaultIdentity(st, time.Now)
	require.EqualError(t, err, "APP_BASE_URL must be an absolute HTTP or HTTPS URL")
	t.Setenv("APP_BASE_URL", "https://city311.example.invalid")
	t.Setenv(seedConstituentPasswordEnv, "")
	_, err = NewDefaultIdentity(st, time.Now)
	require.EqualError(t, err, seedConstituentPasswordEnv+" is required for the City 311 public seed")
	t.Setenv(seedConstituentPasswordEnv, "SeedConstituentPassword1!")
	_, err = NewDefaultIdentity(st, time.Now)
	require.NoError(t, err)
}

func TestInitializeBuildsIdentityFromRuntimeConfiguration(t *testing.T) {
	_, st := testService(t)
	id.Init(context.Background())
	previousDefault, previousIdentity := Default, DefaultIdentity
	t.Cleanup(func() {
		Default = previousDefault
		DefaultIdentity = previousIdentity
	})
	t.Setenv("BENCHMARK_NOW", "2026-02-03T15:04:05Z")
	t.Setenv("SESSION_SECRET", "runtime-provided-initialize-secret")
	t.Setenv("APP_BASE_URL", "https://city311.example.invalid")
	require.NoError(t, Initialize(context.Background(), st))
	require.NotNil(t, Default)
	require.NotNil(t, DefaultIdentity)
	require.NoError(t, DefaultIdentity.configErr)

	t.Setenv("SESSION_SECRET", "")
	require.NoError(t, Initialize(context.Background(), st))
	require.NotNil(t, DefaultIdentity)
	require.EqualError(t, DefaultIdentity.configErr, "SESSION_SECRET is required for City 311 sessions")
	_, _, err := DefaultIdentity.SignIn(context.Background(), contract.LocalSignIn{
		LoginIdentifier: "city311-constituent", Password: "SeedConstituentPassword1!",
	})
	unavailable := requireIdentityError(t, err, 503, contract.ErrorTemporarilyUnavailable)
	require.Equal(t, "City 311 identity configuration is unavailable.", unavailable.Payload.Message)
	require.NotContains(t, unavailable.Payload.Message, "SESSION_SECRET")
}

func TestIdentitySignInSessionProjectionAndExpiry(t *testing.T) {
	identity, st, _, now := testIdentityService(t)
	ctx := context.Background()
	input := validAccountRegistration("alex.session", "session@example.invalid")
	_, err := identity.Register(ctx, input)
	require.NoError(t, err)

	_, _, identifierErr := identity.SignIn(ctx, contract.LocalSignIn{LoginIdentifier: "missing", Password: input.Password})
	_, _, passwordErr := identity.SignIn(ctx, contract.LocalSignIn{LoginIdentifier: input.LoginIdentifier, Password: "incorrect"})
	identifierFailure := requireIdentityError(t, identifierErr, 401, contract.ErrorUnauthenticated)
	passwordFailure := requireIdentityError(t, passwordErr, 401, contract.ErrorUnauthenticated)
	require.Equal(t, identifierFailure.Payload.Message, passwordFailure.Payload.Message)

	rawToken, resolved, err := identity.SignIn(ctx, contract.LocalSignIn{LoginIdentifier: input.Email, Password: input.Password})
	require.NoError(t, err)
	session := identity.Session(resolved)
	require.True(t, session.Authenticated)
	require.Equal(t, []contract.ApplicationRole{contract.ApplicationRoleConstituent}, session.Actor.ApplicationRoles)
	require.Contains(t, session.Actor.Scopes, contract.ScopeRequestWrite)
	require.Contains(t, session.Actor.AvailableRoutes, "portal_service_request_submit")
	require.NotContains(t, session.Actor.AvailableRoutes, "staff_service_request_list")

	*now = now.Add(29 * time.Minute)
	refreshed, err := identity.Resolve(ctx, rawToken)
	require.NoError(t, err)
	require.NotNil(t, refreshed)
	require.Equal(t, now.Add(30*time.Minute), refreshed.Record.ExpiresAt)

	*now = resolved.Record.AbsoluteExpiresAt
	expired, err := identity.Resolve(ctx, rawToken)
	require.NoError(t, err)
	require.Nil(t, expired)
	_, err = store.LookupCity311IdentitySessionByID(ctx, st, resolved.Record.ID)
	require.Error(t, err)
}

func TestIdentitySignInDoesNotCreateSessionWhileCortezaMFAIsRequired(t *testing.T) {
	identity, st, _, _ := testIdentityService(t)
	ctx := context.Background()
	input := validAccountRegistration("alex.mfa", "mfa@example.invalid")
	_, err := identity.Register(ctx, input)
	require.NoError(t, err)
	account, err := store.LookupCity311LocalAccountByLoginIdentifier(ctx, st, input.LoginIdentifier)
	require.NoError(t, err)

	cases := []struct {
		name     string
		settings authSettings.MultiFactor
		policy   func(*systemTypes.UserMeta)
	}{
		{
			name:     "user-enforced Email OTP",
			settings: authSettings.MultiFactor{EmailOTP: authSettings.EmailOTP{Enabled: true}},
			policy:   func(meta *systemTypes.UserMeta) { meta.SecurityPolicy.MFA.EnforcedEmailOTP = true },
		},
		{
			name:     "globally enforced Email OTP",
			settings: authSettings.MultiFactor{EmailOTP: authSettings.EmailOTP{Enabled: true, Enforced: true}},
		},
		{
			name:     "user-enforced TOTP",
			settings: authSettings.MultiFactor{TOTP: authSettings.TOTP{Enabled: true}},
			policy:   func(meta *systemTypes.UserMeta) { meta.SecurityPolicy.MFA.EnforcedTOTP = true },
		},
		{
			name:     "globally enforced unconfigured TOTP",
			settings: authSettings.MultiFactor{TOTP: authSettings.TOTP{Enabled: true, Enforced: true}},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			user, lookupErr := store.LookupUserByID(ctx, st, account.ID)
			require.NoError(t, lookupErr)
			user.Meta = &systemTypes.UserMeta{}
			if c.policy != nil {
				c.policy(user.Meta)
			}
			require.NoError(t, store.UpdateUser(ctx, st, user))
			identity.UpdateMFASettings(&authSettings.Settings{MultiFactor: c.settings})

			_, _, signInErr := identity.SignIn(ctx, contract.LocalSignIn{
				LoginIdentifier: input.LoginIdentifier, Password: input.Password,
			})
			failure := requireIdentityError(t, signInErr, 401, contract.ErrorUnauthenticated)
			require.Equal(t, "The login identifier or password is incorrect.", failure.Payload.Message)
			sessions, _, searchErr := store.SearchCity311IdentitySessions(ctx, st, composeTypes.City311IdentitySessionFilter{UserID: account.ID})
			require.NoError(t, searchErr)
			require.Empty(t, sessions)
		})
	}
}

func TestIdentityValidationAndSafeFailureBranches(t *testing.T) {
	identity, st, _, now := testIdentityService(t)
	ctx := context.Background()

	require.Equal(t, contract.ValidationRequired, validateDisplayName("  ", "/display_name")[0].Code)
	require.Equal(t, contract.ValidationTooLong, validateDisplayName(string(make([]rune, 121)), "/display_name")[0].Code)
	require.Equal(t, contract.ValidationTooLong, validatePassword(string(make([]rune, 129)), "/password")[0].Code)
	require.Equal(t, contract.ValidationInvalidValue, validatePassword("aaaaaaaaaaaa", "/password")[0].Code)
	_, err := randomToken(failingIdentityReader{err: io.ErrUnexpectedEOF})
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	requireIdentityError(t, identity.SignOut(ctx, nil), 401, contract.ErrorUnauthenticated)
	requireIdentityError(t, requireConstituentSession(nil), 401, contract.ErrorUnauthenticated)

	missing, err := identity.Resolve(ctx, "not-a-persisted-token")
	require.NoError(t, err)
	require.Nil(t, missing)
	rawToken, orphaned, err := identity.createSession(ctx, 999_999)
	require.NoError(t, err)
	missing, err = identity.Resolve(ctx, rawToken)
	require.NoError(t, err)
	require.Nil(t, missing)
	_, err = store.LookupCity311IdentitySessionByID(ctx, st, orphaned.ID)
	require.Error(t, err)

	input := validAccountRegistration("alex.failures", "failures@example.invalid")
	_, err = identity.Register(ctx, input)
	require.NoError(t, err)
	_, resolved, err := identity.SignIn(ctx, contract.LocalSignIn{LoginIdentifier: input.LoginIdentifier, Password: input.Password})
	require.NoError(t, err)
	requireIdentityError(t, identity.ChangePassword(ctx, resolved, contract.PasswordChange{
		CurrentPassword: "incorrect", NewPassword: "ChangedPassword2!",
	}), 422, contract.ErrorValidation)
	requireIdentityError(t, identity.ChangePassword(ctx, resolved, contract.PasswordChange{
		CurrentPassword: input.Password, NewPassword: "weak",
	}), 422, contract.ErrorValidation)
	_, err = identity.ChangeLoginIdentifier(ctx, resolved, contract.LoginIdentifierChange{
		CurrentPassword: "incorrect", LoginIdentifier: "valid.identifier",
	})
	requireIdentityError(t, err, 422, contract.ErrorValidation)
	_, err = identity.ChangeLoginIdentifier(ctx, resolved, contract.LoginIdentifierChange{
		CurrentPassword: input.Password, LoginIdentifier: "invalid identifier",
	})
	requireIdentityError(t, err, 422, contract.ErrorValidation)
	_, err = identity.ConfirmPasswordReset(ctx, contract.PasswordResetConfirm{Token: "unknown", Password: "ChangedPassword2!"})
	requireIdentityError(t, err, 422, contract.ErrorInvalidResetToken)
	_, err = identity.ConfirmPasswordReset(ctx, contract.PasswordResetConfirm{Token: "unknown", Password: "weak"})
	requireIdentityError(t, err, 422, contract.ErrorValidation)

	account, err := store.LookupCity311LocalAccountByID(ctx, st, resolved.User.ID)
	require.NoError(t, err)
	token, _, err := identity.createPasswordReset(ctx, account)
	require.NoError(t, err)
	*now = now.Add(15 * time.Minute)
	_, err = identity.ConfirmPasswordReset(ctx, contract.PasswordResetConfirm{Token: token, Password: "ChangedPassword2!"})
	requireIdentityError(t, err, 422, contract.ErrorExpiredResetToken)
}

func TestIdentityRoleProjectionAndNotificationRecoveryState(t *testing.T) {
	identity, st, _, _ := testIdentityService(t)
	ctx := context.Background()
	administrator, err := store.LookupUserByEmail(ctx, st, "platform-admin@city311.example.invalid")
	require.NoError(t, err)
	account, err := store.LookupCity311LocalAccountByID(ctx, st, administrator.ID)
	require.NoError(t, err)
	actor, err := identity.currentActor(ctx, administrator, account)
	require.NoError(t, err)
	require.ElementsMatch(t, contract.DepartmentCodes, actor.DepartmentCodes)
	require.ElementsMatch(t, contract.DistrictCodes, actor.DistrictCodes)
	require.Contains(t, actor.Scopes, contract.ScopeCRMExport)
	require.Contains(t, actor.AvailableRoutes, "staff_request_queue")
	require.True(t, hasStaffRole([]contract.ApplicationRole{contract.ApplicationRoleWorkflowDesigner}))
	require.False(t, hasStaffRole([]contract.ApplicationRole{contract.ApplicationRoleConstituent}))
	require.False(t, roleAllowsEndpoint(actor.ApplicationRoles, contract.AuthenticationContract{ActorClass: "unsupported"}))

	now := time.Date(2026, 2, 3, 15, 4, 5, 0, time.UTC)
	next := uint64(930_000_000_000_000_000)
	failing := NewIdentity(st, IdentityOptions{
		Secret: []byte("failing-notifier-session-secret"), Now: func() time.Time { return now },
		NextID:   func() uint64 { next++; return next },
		Notifier: failingIdentityNotifier{err: errors.New("mail fixture unavailable")},
		Wait:     func(context.Context, time.Duration) error { return nil },
	})
	input := validAccountRegistration("alex.outbox", "outbox@example.invalid")
	_, err = failing.Register(ctx, input)
	require.NoError(t, err)
	failing.RequestPasswordReset(ctx, input.Email)
	pending, _, err := store.SearchCity311IdentityNotifications(ctx, st, composeTypes.City311IdentityNotificationFilter{
		Status: notificationPending,
	})
	require.NoError(t, err)
	require.Len(t, pending, 1)
	require.Zero(t, pending[0].Attempts)
	require.NoError(t, failing.RetryPendingNotifications(ctx))
	created, err := store.LookupCity311LocalAccountByLoginIdentifier(ctx, st, input.LoginIdentifier)
	require.NoError(t, err)
	notifications, _, err := store.SearchCity311IdentityNotifications(ctx, st, composeTypes.City311IdentityNotificationFilter{
		UserID: created.ID, Status: notificationFailed,
	})
	require.NoError(t, err)
	require.Len(t, notifications, 1)
	require.Equal(t, 3, notifications[0].Attempts)
	require.Equal(t, "mail fixture unavailable", notifications[0].LastError)
}

func TestPasswordResetRequestsReturnWhileBoundedWorkerDeliveryIsBlocked(t *testing.T) {
	_, st, _, _ := testIdentityService(t)
	ctx := context.Background()
	notifier := &blockingIdentityNotifier{started: make(chan struct{}), release: make(chan struct{})}
	var nextID atomic.Uint64
	nextID.Store(960_000_000_000_000_000)
	identity := NewIdentity(st, IdentityOptions{
		Secret: []byte("nonblocking-password-reset-secret"), Notifier: notifier,
		NotificationPoll: time.Hour, NextID: func() uint64 { return nextID.Add(1) },
	})
	input := validAccountRegistration("alex.nonblocking", "nonblocking@example.invalid")
	_, err := identity.Register(ctx, input)
	require.NoError(t, err)

	workerCtx, cancelWorker := context.WithCancel(ctx)
	identity.StartNotificationWorker(workerCtx)
	t.Cleanup(func() {
		cancelWorker()
		select {
		case <-notifier.release:
		default:
			close(notifier.release)
		}
	})

	const requestCount = 8
	start := make(chan struct{})
	responses := make(chan *contract.PasswordResetResponse, requestCount)
	for index := 0; index < requestCount; index++ {
		go func() {
			<-start
			responses <- identity.RequestPasswordReset(ctx, input.Email)
		}()
	}
	close(start)

	select {
	case <-notifier.started:
	case <-time.After(5 * time.Second):
		t.Fatal("notification worker did not start delivery")
	}
	for index := 0; index < requestCount; index++ {
		select {
		case response := <-responses:
			require.Equal(t, "If the account is eligible, a reset link has been sent.", response.Message)
		case <-time.After(5 * time.Second):
			t.Fatal("password-reset request waited for notification delivery")
		}
	}
	require.Equal(t, int32(1), notifier.calls.Load(), "one bounded worker must serialize delivery")
}

func TestPasswordResetNotificationRetryScheduleAndRestartRecovery(t *testing.T) {
	identity, st, _, now := testIdentityService(t)
	ctx := context.Background()
	input := validAccountRegistration("alex.retry", "retry@example.invalid")
	_, err := identity.Register(ctx, input)
	require.NoError(t, err)
	account, err := store.LookupCity311LocalAccountByLoginIdentifier(ctx, st, input.LoginIdentifier)
	require.NoError(t, err)

	sequence := &sequencedIdentityNotifier{failures: []error{errors.New("first outage"), errors.New("second outage")}}
	var waits []time.Duration
	retrying := NewIdentity(st, IdentityOptions{
		Secret: []byte("test-only-city311-session-secret"), Now: func() time.Time { return *now },
		Notifier: sequence, Wait: func(_ context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			return nil
		},
	})
	rawToken, notification, err := retrying.createPasswordReset(ctx, account)
	require.NoError(t, err)
	require.NoError(t, retrying.deliverNotification(ctx, notification))
	require.Equal(t, []time.Duration{time.Second, 5 * time.Second}, waits)
	require.Equal(t, []string{rawToken, rawToken, rawToken}, sequence.tokens)
	require.Equal(t, []string{notification.DeliveryKey, notification.DeliveryKey, notification.DeliveryKey}, sequence.deliveryKeys)
	require.Equal(t, 1, sequence.accepted)
	require.Equal(t, notificationSent, notification.Status)
	require.Equal(t, 3, notification.Attempts)

	restartInput := validAccountRegistration("alex.restart", "restart@example.invalid")
	_, err = identity.Register(ctx, restartInput)
	require.NoError(t, err)
	restartAccount, err := store.LookupCity311LocalAccountByLoginIdentifier(ctx, st, restartInput.LoginIdentifier)
	require.NoError(t, err)
	restartToken, pending, err := retrying.createPasswordReset(ctx, restartAccount)
	require.NoError(t, err)
	failedOnce := &sequencedIdentityNotifier{failures: []error{errors.New("application stopped")}}
	beforeRestart := NewIdentity(st, IdentityOptions{
		Secret: []byte("test-only-city311-session-secret"), Now: func() time.Time { return *now }, Notifier: failedOnce,
	})
	require.NoError(t, beforeRestart.attemptNotificationOnce(ctx, pending))
	require.Equal(t, notificationPending, pending.Status)

	recovered := &sequencedIdentityNotifier{}
	var recoveryWaits []time.Duration
	afterRestart := NewIdentity(st, IdentityOptions{
		Secret: []byte("test-only-city311-session-secret"), Now: func() time.Time { return *now },
		Notifier: recovered, Wait: func(_ context.Context, delay time.Duration) error {
			recoveryWaits = append(recoveryWaits, delay)
			return nil
		},
	})
	require.NoError(t, afterRestart.RetryPendingNotifications(ctx))
	require.Equal(t, []time.Duration{time.Second}, recoveryWaits)
	require.Equal(t, []string{restartToken}, recovered.tokens)
	require.Equal(t, []string{pending.DeliveryKey}, recovered.deliveryKeys)
	reloaded, err := store.LookupCity311IdentityNotificationByID(ctx, st, pending.ID)
	require.NoError(t, err)
	require.Equal(t, notificationSent, reloaded.Status)
	require.Equal(t, 2, reloaded.Attempts)
}

func TestIdentityNotificationSMTPFailureClassification(t *testing.T) {
	require.True(t, retryableIdentityDelivery(&textproto.Error{Code: 421, Msg: "service unavailable"}))
	require.True(t, retryableIdentityDelivery(&textproto.Error{Code: 451, Msg: "local processing error"}))
	require.False(t, retryableIdentityDelivery(&textproto.Error{Code: 550, Msg: "mailbox unavailable"}))
	require.False(t, retryableIdentityDelivery(&textproto.Error{Code: 553, Msg: "mailbox name invalid"}))
}

func TestIdentityNotificationRecoveryDrainsEveryBatch(t *testing.T) {
	_, st := testService(t)
	ctx := context.Background()
	now := time.Date(2026, 2, 3, 15, 4, 5, 0, time.UTC)
	set := make(composeTypes.City311IdentityNotificationSet, identityRecoveryBatchSize+1)
	for index := range set {
		deliveryKey := "batch-recovery-" + strconv.Itoa(index)
		set[index] = &composeTypes.City311IdentityNotification{
			ID: uint64(950_000_000_000_000_000 + index), Kind: securityNoticeKind,
			Recipient: "resident@example.invalid", DeliveryKey: deliveryKey,
			Payload: composeTypes.City311JSON{"subject": "Security notice", "body": "Recovery regression"},
			Status:  notificationPending, CreatedAt: now, UpdatedAt: now,
		}
	}
	require.NoError(t, store.CreateCity311IdentityNotification(ctx, st, set...))

	notifier := &identityNotifierCapture{}
	recovering := NewIdentity(st, IdentityOptions{
		Secret: []byte("batch-recovery-session-secret"), Now: func() time.Time { return now }, Notifier: notifier,
	})
	require.NoError(t, recovering.RetryPendingNotifications(ctx))

	pending, _, err := store.SearchCity311IdentityNotifications(ctx, st, composeTypes.City311IdentityNotificationFilter{
		Status: notificationPending, Paging: filter.Paging{Limit: identityRecoveryBatchSize + 1},
	})
	require.NoError(t, err)
	require.Empty(t, pending)
	require.Len(t, notifier.securityDeliveryKeys, identityRecoveryBatchSize+1)
	acceptances := make(map[string]int, len(notifier.securityDeliveryKeys))
	for _, deliveryKey := range notifier.securityDeliveryKeys {
		acceptances[deliveryKey]++
	}
	require.Len(t, acceptances, identityRecoveryBatchSize+1)
	for _, count := range acceptances {
		require.Equal(t, 1, count)
	}
}

func TestIdentityNotificationRecoveryPreservesCancellationAndExhaustion(t *testing.T) {
	identity, st, _, now := testIdentityService(t)
	ctx := context.Background()
	pending := identity.newNotification(123, securityNoticeKind, "resident@example.invalid", "cancelled-recovery",
		map[string]any{"subject": "Security notice", "body": "Recovery regression"})
	pending.Attempts = 1
	require.NoError(t, store.CreateCity311IdentityNotification(ctx, st, pending))
	identity.wait = func(context.Context, time.Duration) error { return context.Canceled }
	require.ErrorIs(t, identity.RetryPendingNotifications(ctx), context.Canceled)
	reloadedPending, err := store.LookupCity311IdentityNotificationByID(ctx, st, pending.ID)
	require.NoError(t, err)
	require.Equal(t, notificationPending, reloadedPending.Status)
	require.Equal(t, 1, reloadedPending.Attempts)

	exhausted := identity.newNotification(124, securityNoticeKind, "resident@example.invalid", "exhausted-recovery",
		map[string]any{"subject": "Security notice", "body": "Recovery regression"})
	exhausted.Attempts = len(notificationRetryDelays)
	exhausted.UpdatedAt = *now
	require.NoError(t, store.CreateCity311IdentityNotification(ctx, st, exhausted))
	require.NoError(t, identity.deliverNotification(ctx, exhausted))
	require.Equal(t, notificationFailed, exhausted.Status)
	require.Equal(t, "notification retry budget exhausted", exhausted.LastError)
}

func TestPasswordResetInvalidatesOlderTokenAndAllSessions(t *testing.T) {
	identity, st, notifier, now := testIdentityService(t)
	ctx := context.Background()
	input := validAccountRegistration("alex.reset", "reset@example.invalid")
	_, err := identity.Register(ctx, input)
	require.NoError(t, err)
	_, firstSession, err := identity.SignIn(ctx, contract.LocalSignIn{LoginIdentifier: input.LoginIdentifier, Password: input.Password})
	require.NoError(t, err)
	_, _, err = identity.SignIn(ctx, contract.LocalSignIn{LoginIdentifier: input.LoginIdentifier, Password: input.Password})
	require.NoError(t, err)

	firstResponse := identity.RequestPasswordReset(ctx, input.Email)
	unknownResponse := identity.RequestPasswordReset(ctx, "unknown@example.invalid")
	require.Equal(t, firstResponse, unknownResponse)
	require.NoError(t, identity.RetryPendingNotifications(ctx))
	require.Len(t, notifier.resetTokens, 1)
	firstToken := notifier.resetTokens[0]
	identity.RequestPasswordReset(ctx, input.Email)
	require.NoError(t, identity.RetryPendingNotifications(ctx))
	require.Len(t, notifier.resetTokens, 2)
	secondToken := notifier.resetTokens[1]

	_, err = identity.ConfirmPasswordReset(ctx, contract.PasswordResetConfirm{Token: firstToken, Password: "AnotherStrong2!"})
	requireIdentityError(t, err, 422, contract.ErrorInvalidResetToken)
	response, err := identity.ConfirmPasswordReset(ctx, contract.PasswordResetConfirm{Token: secondToken, Password: "AnotherStrong2!"})
	require.NoError(t, err)
	require.Equal(t, "Your password has been reset.", response.Message)
	require.NoError(t, identity.RetryPendingNotifications(ctx))
	require.Len(t, notifier.securityNotices, 1)

	sessions, _, err := store.SearchCity311IdentitySessions(ctx, st, composeTypes.City311IdentitySessionFilter{UserID: firstSession.User.ID})
	require.NoError(t, err)
	require.Empty(t, sessions)
	_, _, err = identity.SignIn(ctx, contract.LocalSignIn{LoginIdentifier: input.LoginIdentifier, Password: input.Password})
	requireIdentityError(t, err, 401, contract.ErrorUnauthenticated)
	_, _, err = identity.SignIn(ctx, contract.LocalSignIn{LoginIdentifier: input.LoginIdentifier, Password: "AnotherStrong2!"})
	require.NoError(t, err)
	_, err = identity.ConfirmPasswordReset(ctx, contract.PasswordResetConfirm{Token: secondToken, Password: "ThirdStrongPass3!"})
	requireIdentityError(t, err, 422, contract.ErrorInvalidResetToken)

	identity.RequestPasswordReset(ctx, input.Email)
	require.NoError(t, identity.RetryPendingNotifications(ctx))
	expiringToken := notifier.resetTokens[len(notifier.resetTokens)-1]
	*now = now.Add(16 * time.Minute)
	_, err = identity.ConfirmPasswordReset(ctx, contract.PasswordResetConfirm{Token: expiringToken, Password: "ThirdStrongPass3!"})
	requireIdentityError(t, err, 422, contract.ErrorExpiredResetToken)
}

func TestPasswordAndIdentifierChangesPreserveOnlyCurrentSession(t *testing.T) {
	identity, st, notifier, _ := testIdentityService(t)
	ctx := context.Background()
	first := validAccountRegistration("alex.changes", "changes@example.invalid")
	second := validAccountRegistration("taken.identifier", "taken@example.invalid")
	_, err := identity.Register(ctx, first)
	require.NoError(t, err)
	_, err = identity.Register(ctx, second)
	require.NoError(t, err)
	currentToken, current, err := identity.SignIn(ctx, contract.LocalSignIn{LoginIdentifier: first.LoginIdentifier, Password: first.Password})
	require.NoError(t, err)
	otherToken, _, err := identity.SignIn(ctx, contract.LocalSignIn{LoginIdentifier: first.LoginIdentifier, Password: first.Password})
	require.NoError(t, err)

	_, err = identity.ChangeLoginIdentifier(ctx, current, contract.LoginIdentifierChange{
		CurrentPassword: first.Password, LoginIdentifier: second.LoginIdentifier,
	})
	collision := requireIdentityError(t, err, 422, contract.ErrorValidation)
	require.Equal(t, contract.ValidationDuplicate, collision.Payload.Errors[0].Code)
	require.NotNil(t, mustResolve(t, identity, currentToken))
	require.NotNil(t, mustResolve(t, identity, otherToken))
	account, err := store.LookupCity311LocalAccountByID(ctx, st, current.User.ID)
	require.NoError(t, err)
	require.Equal(t, first.LoginIdentifier, account.LoginIdentifier)
	user, err := store.LookupUserByID(ctx, st, current.User.ID)
	require.NoError(t, err)
	require.Equal(t, first.LoginIdentifier, user.Username)
	constituentID := "C-" + strconv.FormatUint(current.User.ID, 10)
	constituent, err := store.LookupCity311ConstituentByConstituentID(ctx, st, constituentID)
	require.NoError(t, err)
	require.Equal(t, first.LoginIdentifier, constituent.Profile["login_identifier"])

	session, err := identity.ChangeLoginIdentifier(ctx, current, contract.LoginIdentifierChange{
		CurrentPassword: first.Password, LoginIdentifier: "alex.updated",
	})
	require.NoError(t, err)
	require.True(t, session.Authenticated)
	require.NotNil(t, mustResolve(t, identity, currentToken))
	require.Nil(t, mustResolve(t, identity, otherToken))
	require.NoError(t, identity.RetryPendingNotifications(ctx))
	require.Len(t, notifier.securityNotices, 1)
	account, err = store.LookupCity311LocalAccountByID(ctx, st, current.User.ID)
	require.NoError(t, err)
	require.Equal(t, "alex.updated", account.LoginIdentifier)
	user, err = store.LookupUserByID(ctx, st, current.User.ID)
	require.NoError(t, err)
	require.Equal(t, "alex.updated", user.Username)
	constituent, err = store.LookupCity311ConstituentByConstituentID(ctx, st, constituentID)
	require.NoError(t, err)
	require.Equal(t, "alex.updated", constituent.Profile["login_identifier"])

	secondToken, secondSession, err := identity.SignIn(ctx, contract.LocalSignIn{LoginIdentifier: "alex.updated", Password: first.Password})
	require.NoError(t, err)
	err = identity.ChangePassword(ctx, current, contract.PasswordChange{CurrentPassword: first.Password, NewPassword: "ChangedPassword4!"})
	require.NoError(t, err)
	require.NotNil(t, mustResolve(t, identity, currentToken))
	require.Nil(t, mustResolve(t, identity, secondToken))
	_, _, err = identity.SignIn(ctx, contract.LocalSignIn{LoginIdentifier: "alex.updated", Password: first.Password})
	requireIdentityError(t, err, 401, contract.ErrorUnauthenticated)
	_, _, err = identity.SignIn(ctx, contract.LocalSignIn{LoginIdentifier: "alex.updated", Password: "ChangedPassword4!"})
	require.NoError(t, err)

	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EntityID: strconv.FormatUint(secondSession.User.ID, 10)})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(audits), 3)
}

func mustResolve(t *testing.T, identity *IdentityService, token string) *ResolvedSession {
	t.Helper()
	resolved, err := identity.Resolve(context.Background(), token)
	require.NoError(t, err)
	return resolved
}
