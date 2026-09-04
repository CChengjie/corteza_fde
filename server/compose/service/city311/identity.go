package city311

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	stderrors "errors"
	"fmt"
	"io"
	netmail "net/mail"
	"net/textproto"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	authRequest "github.com/cortezaproject/corteza/server/auth/request"
	authSettings "github.com/cortezaproject/corteza/server/auth/settings"
	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/errors"
	"github.com/cortezaproject/corteza/server/pkg/filter"
	"github.com/cortezaproject/corteza/server/pkg/id"
	mailService "github.com/cortezaproject/corteza/server/pkg/mail"
	"github.com/cortezaproject/corteza/server/store"
	systemTypes "github.com/cortezaproject/corteza/server/system/types"
	"golang.org/x/crypto/bcrypt"
)

const (
	IdentitySessionCookie = contract.SessionCookieName

	identitySessionIdleLifetime     = 30 * time.Minute
	identitySessionAbsoluteLifetime = 8 * time.Hour
	passwordResetLifetime           = 15 * time.Minute
	passwordCredentialKind          = "password"

	notificationPending = "PENDING"
	notificationSent    = "SENT"
	notificationFailed  = "FAILED"
	passwordResetKind   = "PASSWORD_RESET"
	securityNoticeKind  = "SECURITY_NOTICE"

	seedConstituentPasswordEnv    = "CITY311_SEED_CONSTITUENT_PASSWORD"
	seedConstituentTwoPasswordEnv = "CITY311_SEED_CONSTITUENT_TWO_PASSWORD"
	loginIdentifierField          = "/login_identifier"
	invalidResetTokenMessage      = "The reset token is invalid."
	authenticationRequiredMessage = "Authentication is required."
	identityRecoveryBatchSize     = 1000
	identityNotificationQueueSize = 1
	identityNotificationPoll      = 30 * time.Second
)

var (
	localIdentifierPattern  = regexp.MustCompile(`^[a-z0-9._-]{3,64}$`)
	notificationRetryDelays = []time.Duration{0, time.Second, 5 * time.Second}
)

type (
	IdentityNotifier interface {
		PasswordReset(context.Context, string, string, string) error
		SecurityNotice(context.Context, string, string, string, string) error
	}

	IdentityOptions struct {
		Secret             []byte
		Now                func() time.Time
		NextID             func() uint64
		Random             io.Reader
		Runtime            *IdentityRuntimeConfiguration
		Federation         FederationProvider
		Notifier           IdentityNotifier
		Wait               func(context.Context, time.Duration) error
		ConfigurationError error
		MFASettings        *authSettings.Settings
		NotificationPoll   time.Duration
		WorkerError        func(error)
	}

	IdentityService struct {
		store      store.Storer
		secret     []byte
		now        func() time.Time
		nextID     func() uint64
		random     io.Reader
		runtime    IdentityRuntimeConfiguration
		federation FederationProvider
		notifier   IdentityNotifier
		wait       func(context.Context, time.Duration) error
		configErr  error

		mfaMu        sync.RWMutex
		runtimeMu    sync.RWMutex
		mfaSettings  authSettings.Settings
		resetMu      sync.Mutex
		federationMu sync.Mutex

		notificationWake chan struct{}
		notificationPoll time.Duration
		workerOnce       sync.Once
		workerError      func(error)
	}

	ResolvedSession struct {
		Record  *composeTypes.City311IdentitySession
		Account *composeTypes.City311LocalAccount
		User    *systemTypes.User
		Actor   *contract.CurrentActor
	}

	defaultIdentityNotifier struct{ baseURL string }
)

var DefaultIdentity *IdentityService

func NewIdentity(s store.Storer, options IdentityOptions) *IdentityService {
	if options.Now == nil {
		options.Now = func() time.Time { return time.Now().UTC().Round(time.Second) }
	}
	if options.NextID == nil {
		options.NextID = id.Next
	}
	if options.Random == nil {
		options.Random = cryptorand.Reader
	}
	if options.Notifier == nil {
		options.Notifier = defaultIdentityNotifier{baseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("APP_BASE_URL")), "/")}
	}
	if options.Wait == nil {
		options.Wait = waitForIdentityRetry
	}
	if len(options.Secret) == 0 {
		options.Secret = make([]byte, 32)
		if _, err := io.ReadFull(options.Random, options.Secret); err != nil {
			panic(fmt.Sprintf("city311: generate ephemeral session secret: %v", err))
		}
	}
	if options.NotificationPoll <= 0 {
		options.NotificationPoll = identityNotificationPoll
	}
	service := &IdentityService{
		store: s, secret: append([]byte(nil), options.Secret...), now: options.Now,
		nextID: options.NextID, random: options.Random, notifier: options.Notifier,
		wait: options.Wait, configErr: options.ConfigurationError,
		notificationWake: make(chan struct{}, identityNotificationQueueSize),
		notificationPoll: options.NotificationPoll, workerError: options.WorkerError,
	}
	if options.Runtime == nil {
		service.runtime = IdentityRuntimeFromEnvironment()
	} else {
		service.runtime = *options.Runtime
	}
	if options.Federation == nil {
		service.federation = NewRuntimeFederationProvider(service.runtime, nil, service.now)
	} else {
		service.federation = options.Federation
	}
	service.UpdateMFASettings(options.MFASettings)
	return service
}

func (svc *IdentityService) UpdateMFASettings(settings *authSettings.Settings) {
	svc.mfaMu.Lock()
	defer svc.mfaMu.Unlock()
	svc.mfaSettings = authSettings.Settings{}
	if settings != nil {
		svc.mfaSettings.MultiFactor = settings.MultiFactor
	}
}

func (svc *IdentityService) ConfigurationError() error {
	return svc.configErr
}

// SetFederationRuntime atomically replaces the live identity-provider
// connection without changing local-account or local-session behavior.
func (svc *IdentityService) SetFederationRuntime(runtime IdentityRuntimeConfiguration) {
	svc.runtimeMu.Lock()
	defer svc.runtimeMu.Unlock()
	svc.runtime = runtime
	svc.federation = NewRuntimeFederationProvider(runtime, nil, svc.now)
}

func (svc *IdentityService) federationRuntime() (IdentityRuntimeConfiguration, FederationProvider) {
	svc.runtimeMu.RLock()
	defer svc.runtimeMu.RUnlock()
	return svc.runtime, svc.federation
}

func NewDefaultIdentity(s store.Storer, now func() time.Time) (*IdentityService, error) {
	if err := ValidateIdentityEnvironment(); err != nil {
		return nil, err
	}
	secret := strings.TrimSpace(os.Getenv("SESSION_SECRET"))
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_BASE_URL")), "/")
	return NewIdentity(s, IdentityOptions{
		Secret: []byte(secret), Now: now,
		Notifier: defaultIdentityNotifier{baseURL: baseURL},
	}), nil
}

func ValidateIdentityEnvironment() error {
	if strings.TrimSpace(os.Getenv("SESSION_SECRET")) == "" {
		return fmt.Errorf("SESSION_SECRET is required for City 311 sessions")
	}
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("APP_BASE_URL")), "/")
	parsedBaseURL, err := url.Parse(baseURL)
	if err != nil || parsedBaseURL.Host == "" || (parsedBaseURL.Scheme != "http" && parsedBaseURL.Scheme != "https") {
		return fmt.Errorf("APP_BASE_URL must be an absolute HTTP or HTTPS URL")
	}
	runtime := IdentityRuntimeFromEnvironment()
	for _, input := range []struct{ key, value string }{
		{"OIDC_STAFF_CLIENT_ID", runtime.OIDCStaffClientID}, {"OIDC_PUBLIC_CLIENT_ID", runtime.OIDCPublicClientID},
		{"OIDC_CLIENT_SECRET", runtime.OIDCClientSecret},
	} {
		if input.value == "" {
			return fmt.Errorf("%s is required for City 311 federated identity", input.key)
		}
	}
	for _, input := range []struct{ key, value string }{
		{"OIDC_ISSUER_URL", runtime.OIDCIssuerURL}, {"SAML_METADATA_URL", runtime.SAMLMetadataURL},
		{"SAML_SP_ENTITY_ID", runtime.SAMLServiceProvider},
	} {
		parsed, parseErr := url.Parse(input.value)
		if parseErr != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return fmt.Errorf("%s must be an absolute HTTP or HTTPS URL", input.key)
		}
	}
	for _, key := range []string{seedConstituentPasswordEnv, seedConstituentTwoPasswordEnv} {
		if value := os.Getenv(key); value == "" {
			return fmt.Errorf("%s is required for the City 311 public seed", key)
		} else if len(validatePassword(value, "/"+strings.ToLower(key))) > 0 {
			return fmt.Errorf("%s must satisfy the City 311 password policy", key)
		}
	}
	return nil
}

func waitForIdentityRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (notifier defaultIdentityNotifier) PasswordReset(ctx context.Context, recipient, token, deliveryKey string) error {
	message := mailService.New()
	message.SetHeader("To", recipient)
	message.SetHeader("Subject", "Reset your City 311 password")
	message.SetHeader("X-City311-Delivery-Key", deliveryKey)
	message.SetBody("text/plain", fmt.Sprintf("Use this link within 15 minutes: %s/reset-password?token=%s", notifier.baseURL, token))
	return mailService.Send(message)
}

func (defaultIdentityNotifier) SecurityNotice(ctx context.Context, recipient, subject, body, deliveryKey string) error {
	message := mailService.New()
	message.SetHeader("To", recipient)
	message.SetHeader("Subject", subject)
	message.SetHeader("X-City311-Delivery-Key", deliveryKey)
	message.SetBody("text/plain", body)
	return mailService.Send(message)
}

func (svc *IdentityService) Register(ctx context.Context, input contract.AccountRegistration) (*contract.AccountRegistrationAcknowledgement, error) {
	if svc.configErr != nil {
		return nil, svc.configurationUnavailable()
	}
	input.LoginIdentifier = normalizeIdentifier(input.LoginIdentifier)
	input.Email = normalizeEmail(input.Email)
	if fields := validateRegistration(input); len(fields) > 0 {
		return nil, validationError(fields...)
	}

	acknowledgement := &contract.AccountRegistrationAcknowledgement{Accepted: true}
	exists, err := svc.localAccountExists(ctx, input.LoginIdentifier, input.Email)
	if err != nil {
		return nil, err
	}
	if exists {
		return acknowledgement, nil
	}

	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		exists, err := svc.localAccountExistsInStore(ctx, tx, input.LoginIdentifier, input.Email)
		if err != nil {
			return err
		}
		if exists {
			return nil
		}
		now := svc.now()
		userID := svc.nextID()
		user := &systemTypes.User{
			ID: userID, Handle: "city311-account-" + strconv.FormatUint(userID, 10),
			Username: input.LoginIdentifier, Email: input.Email, Name: strings.TrimSpace(input.DisplayName),
			EmailConfirmed: true, Meta: &systemTypes.UserMeta{PreferredLanguage: strings.ToLower(string(input.PreferredLanguage))}, CreatedAt: now,
		}
		if err := store.CreateUser(ctx, tx, user); err != nil {
			return err
		}
		if err := svc.setPassword(ctx, tx, userID, input.Password, now); err != nil {
			return err
		}
		account := &composeTypes.City311LocalAccount{
			ID: userID, LoginIdentifier: input.LoginIdentifier, VerifiedEmail: input.Email,
			PreferredLanguage: string(input.PreferredLanguage), CreatedAt: now, UpdatedAt: now,
		}
		if err := store.CreateCity311LocalAccount(ctx, tx, account); err != nil {
			return err
		}
		if err := svc.createConstituentAccount(ctx, tx, user, account, now); err != nil {
			return err
		}
		if err := svc.assignConstituentRole(ctx, tx, userID); err != nil {
			return err
		}
		return svc.createIdentityAudit(ctx, tx, userID, "ACCOUNT_REGISTERED", nil, map[string]any{
			"login_identifier": input.LoginIdentifier, "verified_email": input.Email,
		})
	})
	if err != nil {
		// A concurrent registration collision remains privacy-indistinguishable.
		if exists, lookupErr := svc.localAccountExists(ctx, input.LoginIdentifier, input.Email); lookupErr == nil && exists {
			return acknowledgement, nil
		}
		return nil, err
	}
	return acknowledgement, nil
}

func validateRegistration(input contract.AccountRegistration) []contract.FieldError {
	fields := validateDisplayName(input.DisplayName, "/display_name")
	if !validEmail(input.Email) {
		fields = append(fields, contract.FieldError{Field: "/email", Code: contract.ValidationInvalidFormat})
	}
	if !localIdentifierPattern.MatchString(input.LoginIdentifier) {
		fields = append(fields, contract.FieldError{Field: loginIdentifierField, Code: contract.ValidationInvalidFormat})
	}
	fields = append(fields, validatePassword(input.Password, "/password")...)
	if !containsLanguage(input.PreferredLanguage) {
		fields = append(fields, contract.FieldError{Field: "/preferred_language", Code: contract.ValidationInvalidValue})
	}
	return fields
}

func validateDisplayName(value, field string) []contract.FieldError {
	length := utf8.RuneCountInString(strings.TrimSpace(value))
	if length == 0 {
		return []contract.FieldError{{Field: field, Code: contract.ValidationRequired}}
	}
	if length > 120 {
		return []contract.FieldError{{Field: field, Code: contract.ValidationTooLong}}
	}
	return nil
}

func validatePassword(value, field string) []contract.FieldError {
	length := utf8.RuneCountInString(value)
	if length < 12 {
		return []contract.FieldError{{Field: field, Code: contract.ValidationTooShort}}
	}
	if length > 128 {
		return []contract.FieldError{{Field: field, Code: contract.ValidationTooLong}}
	}
	classes := 0
	var upper, lower, digit, symbol bool
	for _, char := range value {
		upper = upper || unicode.IsUpper(char)
		lower = lower || unicode.IsLower(char)
		digit = digit || unicode.IsDigit(char)
		symbol = symbol || (!unicode.IsLetter(char) && !unicode.IsDigit(char))
	}
	for _, present := range []bool{upper, lower, digit, symbol} {
		if present {
			classes++
		}
	}
	if classes < 3 {
		return []contract.FieldError{{Field: field, Code: contract.ValidationInvalidValue}}
	}
	return nil
}

func containsLanguage(value contract.Language) bool {
	for _, language := range contract.Languages {
		if language == value {
			return true
		}
	}
	return false
}

func validEmail(value string) bool {
	parsed, err := netmail.ParseAddress(value)
	return err == nil && parsed.Address == value && parsed.Name == ""
}

func normalizeEmail(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func normalizeIdentifier(value string) string { return strings.ToLower(strings.TrimSpace(value)) }

func (svc *IdentityService) localAccountExists(ctx context.Context, loginIdentifier, email string) (bool, error) {
	return svc.localAccountExistsInStore(ctx, svc.store, loginIdentifier, email)
}

func (svc *IdentityService) localAccountExistsInStore(ctx context.Context, s store.Storer, loginIdentifier, email string) (bool, error) {
	if _, err := store.LookupCity311LocalAccountByLoginIdentifier(ctx, s, loginIdentifier); err == nil {
		return true, nil
	} else if !errors.IsNotFound(err) {
		return false, err
	}
	if _, err := store.LookupCity311LocalAccountByVerifiedEmail(ctx, s, email); err == nil {
		return true, nil
	} else if !errors.IsNotFound(err) {
		return false, err
	}
	return false, nil
}

func (svc *IdentityService) createConstituentAccount(ctx context.Context, tx store.Storer, user *systemTypes.User, account *composeTypes.City311LocalAccount, now time.Time) error {
	constituentID := "C-" + strconv.FormatUint(user.ID, 10)
	return store.CreateCity311Constituent(ctx, tx, &composeTypes.City311Constituent{
		ID: svc.nextID(), ConstituentID: constituentID,
		Profile: composeTypes.City311JSON{
			"constituent_id": constituentID, "display_name": user.Name,
			"login_identifier": account.LoginIdentifier, "emails": []string{account.VerifiedEmail},
			"phone_numbers": []any{}, "addresses": []any{}, "primary_category": string(contract.ContactCategoryResident),
			"preferred_language": string(account.PreferredLanguage), "email_opt_out": false,
		},
		CreatedAt: now, UpdatedAt: now,
	})
}

func (svc *IdentityService) assignConstituentRole(ctx context.Context, tx store.Storer, userID uint64) error {
	role, err := store.LookupRoleByHandle(ctx, tx, "city311-"+string(contract.ApplicationRoleConstituent))
	if err != nil {
		return err
	}
	if err = ensureSeedRoleMembership(ctx, tx, "registered constituent", role.ID, userID); err != nil {
		return err
	}
	now := svc.now()
	return store.CreateCity311ActorProfile(ctx, tx, &composeTypes.City311ActorProfile{
		ID: userID, ApplicationRoles: composeTypes.City311ApplicationRoleSet{contract.ApplicationRoleConstituent},
		Districts: composeTypes.City311DistrictCodeSet{}, CreatedAt: now, UpdatedAt: now,
	})
}

func (svc *IdentityService) SignIn(ctx context.Context, input contract.LocalSignIn) (string, *ResolvedSession, error) {
	if svc.configErr != nil {
		return "", nil, svc.configurationUnavailable()
	}
	identifier := normalizeIdentifier(input.LoginIdentifier)
	if identifier == "" || input.Password == "" {
		return "", nil, unauthenticatedLogin()
	}
	account, err := svc.findLocalAccount(ctx, identifier)
	if err != nil {
		if errors.IsNotFound(err) {
			return "", nil, unauthenticatedLogin()
		}
		return "", nil, err
	}
	user, err := store.LookupUserByID(ctx, svc.store, account.ID)
	if err != nil || !user.Valid() || !svc.passwordMatches(ctx, svc.store, user.ID, input.Password) {
		return "", nil, unauthenticatedLogin()
	}
	if svc.requiresMFA(user) {
		return "", nil, unauthenticatedLogin()
	}

	rawToken, record, err := svc.createSession(ctx, user.ID)
	if err != nil {
		return "", nil, err
	}
	actor, err := svc.currentActor(ctx, user, account)
	if err != nil {
		_ = store.DeleteCity311IdentitySessionByID(ctx, svc.store, record.ID)
		return "", nil, err
	}
	return rawToken, &ResolvedSession{Record: record, Account: account, User: user, Actor: actor}, nil
}

func (svc *IdentityService) requiresMFA(user *systemTypes.User) bool {
	svc.mfaMu.RLock()
	settings := svc.mfaSettings
	svc.mfaMu.RUnlock()
	authUser := authRequest.NewAuthUser(&settings, user.Clone(), false)
	return authUser.PendingMFA() || authUser.UnconfiguredTOTP()
}

func unauthenticatedLogin() *ServiceError {
	return apiError(401, contract.ErrorUnauthenticated, "The login identifier or password is incorrect.")
}

func (svc *IdentityService) findLocalAccount(ctx context.Context, identifier string) (*composeTypes.City311LocalAccount, error) {
	if strings.Contains(identifier, "@") {
		return store.LookupCity311LocalAccountByVerifiedEmail(ctx, svc.store, normalizeEmail(identifier))
	}
	return store.LookupCity311LocalAccountByLoginIdentifier(ctx, svc.store, normalizeIdentifier(identifier))
}

func (svc *IdentityService) createSession(ctx context.Context, userID uint64) (string, *composeTypes.City311IdentitySession, error) {
	token, err := randomToken(svc.random)
	if err != nil {
		return "", nil, err
	}
	now := svc.now()
	record := &composeTypes.City311IdentitySession{
		ID: svc.nextID(), TokenHash: svc.hashToken(token), UserID: userID,
		IssuedAt: now, LastSeenAt: now, ExpiresAt: now.Add(identitySessionIdleLifetime),
		AbsoluteExpiresAt: now.Add(identitySessionAbsoluteLifetime),
	}
	if err = store.CreateCity311IdentitySession(ctx, svc.store, record); err != nil {
		return "", nil, err
	}
	return token, record, nil
}

func randomToken(source io.Reader) (string, error) {
	value := make([]byte, 32)
	if _, err := io.ReadFull(source, value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func (svc *IdentityService) hashToken(token string) string {
	mac := hmac.New(sha256.New, svc.secret)
	_, _ = mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

func (svc *IdentityService) Resolve(ctx context.Context, rawToken string) (*ResolvedSession, error) {
	if svc.configErr != nil {
		return nil, svc.configurationUnavailable()
	}
	if strings.TrimSpace(rawToken) == "" {
		return nil, nil
	}
	record, err := svc.lookupIdentitySession(ctx, rawToken)
	if err != nil {
		return nil, err
	}
	if record == nil {
		return nil, nil
	}
	now := svc.now()
	if !now.Before(record.ExpiresAt) || !now.Before(record.AbsoluteExpiresAt) {
		svc.discardIdentitySession(ctx, record.ID)
		return nil, nil
	}
	account, user, err := svc.resolveSessionPrincipals(ctx, record)
	if err != nil {
		return nil, err
	}
	if account == nil || user == nil {
		return nil, nil
	}
	if err = svc.refreshIdentitySession(ctx, record, now); err != nil {
		return nil, err
	}
	actor, err := svc.currentActor(ctx, user, account)
	if err != nil {
		return nil, err
	}
	return &ResolvedSession{Record: record, Account: account, User: user, Actor: actor}, nil
}

func (svc *IdentityService) lookupIdentitySession(ctx context.Context, rawToken string) (*composeTypes.City311IdentitySession, error) {
	record, err := store.LookupCity311IdentitySessionByTokenHash(ctx, svc.store, svc.hashToken(rawToken))
	if errors.IsNotFound(err) {
		return nil, nil
	}
	return record, err
}

func (svc *IdentityService) resolveSessionPrincipals(ctx context.Context, record *composeTypes.City311IdentitySession) (*composeTypes.City311LocalAccount, *systemTypes.User, error) {
	account, err := store.LookupCity311LocalAccountByID(ctx, svc.store, record.UserID)
	if err != nil {
		svc.discardIdentitySession(ctx, record.ID)
		if errors.IsNotFound(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	user, err := store.LookupUserByID(ctx, svc.store, record.UserID)
	if err != nil || !user.Valid() {
		svc.discardIdentitySession(ctx, record.ID)
		if err != nil && !errors.IsNotFound(err) {
			return nil, nil, err
		}
		return nil, nil, nil
	}
	return account, user, nil
}

func (svc *IdentityService) discardIdentitySession(ctx context.Context, sessionID uint64) {
	_ = store.DeleteCity311IdentitySessionByID(ctx, svc.store, sessionID)
}

func (svc *IdentityService) refreshIdentitySession(ctx context.Context, record *composeTypes.City311IdentitySession, now time.Time) error {
	record.LastSeenAt = now
	record.ExpiresAt = now.Add(identitySessionIdleLifetime)
	if record.ExpiresAt.After(record.AbsoluteExpiresAt) {
		record.ExpiresAt = record.AbsoluteExpiresAt
	}
	return store.UpdateCity311IdentitySession(ctx, svc.store, record)
}

func (svc *IdentityService) Session(resolved *ResolvedSession) *contract.Session {
	if resolved == nil {
		return &contract.Session{Authenticated: false, Actor: nil, ExpiresAt: nil, PreferredLanguage: contract.LanguageEN}
	}
	expiresAt := resolved.Record.ExpiresAt
	return &contract.Session{
		Authenticated: true, Actor: resolved.Actor, ExpiresAt: &expiresAt,
		PreferredLanguage: contract.Language(resolved.Account.PreferredLanguage),
	}
}

func (svc *IdentityService) SignOut(ctx context.Context, resolved *ResolvedSession) error {
	if svc.configErr != nil {
		return svc.configurationUnavailable()
	}
	if resolved == nil || resolved.Record == nil {
		return apiError(401, contract.ErrorUnauthenticated, authenticationRequiredMessage)
	}
	return store.DeleteCity311IdentitySessionByID(ctx, svc.store, resolved.Record.ID)
}

func (svc *IdentityService) RequestPasswordReset(ctx context.Context, email string) *contract.PasswordResetResponse {
	response := &contract.PasswordResetResponse{Message: "If the account is eligible, a reset link has been sent."}
	if svc.configErr != nil {
		return response
	}
	email = normalizeEmail(email)
	if !validEmail(email) {
		return response
	}
	account, err := store.LookupCity311LocalAccountByVerifiedEmail(ctx, svc.store, email)
	if err != nil {
		return response
	}
	svc.resetMu.Lock()
	defer svc.resetMu.Unlock()
	_, _, err = svc.createPasswordReset(ctx, account)
	if err != nil {
		return response
	}
	svc.wakeNotificationWorker()
	return response
}

func (svc *IdentityService) createPasswordReset(ctx context.Context, account *composeTypes.City311LocalAccount) (string, *composeTypes.City311IdentityNotification, error) {
	rawToken, err := randomToken(svc.random)
	if err != nil {
		return "", nil, err
	}
	now := svc.now()
	var notification *composeTypes.City311IdentityNotification
	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		if err := svc.invalidatePasswordResetTokens(ctx, tx, account.ID, now); err != nil {
			return err
		}
		tokenID := svc.nextID()
		token := &composeTypes.City311PasswordResetToken{
			ID: tokenID, TokenHash: svc.hashToken(rawToken), UserID: account.ID,
			CreatedAt: now, ExpiresAt: now.Add(passwordResetLifetime),
		}
		if err = store.CreateCity311PasswordResetToken(ctx, tx, token); err != nil {
			return err
		}
		deliveryKey := "password-reset:" + strconv.FormatUint(token.ID, 10)
		sealedToken, err := svc.sealNotificationSecret(rawToken, deliveryKey)
		if err != nil {
			return err
		}
		notification = svc.newNotification(account.ID, passwordResetKind, account.VerifiedEmail, deliveryKey,
			map[string]any{"token_id": strconv.FormatUint(token.ID, 10), "sealed_token": sealedToken})
		return store.CreateCity311IdentityNotification(ctx, tx, notification)
	})
	return rawToken, notification, err
}

func (svc *IdentityService) invalidatePasswordResetTokens(ctx context.Context, tx store.Storer, userID uint64, now time.Time) error {
	tokens, _, err := store.SearchCity311PasswordResetTokens(ctx, tx, composeTypes.City311PasswordResetTokenFilter{UserID: userID})
	if err != nil {
		return err
	}
	for _, token := range tokens {
		if token.UsedAt == nil {
			token.UsedAt = &now
		}
	}
	if len(tokens) == 0 {
		return nil
	}
	return store.UpdateCity311PasswordResetToken(ctx, tx, tokens...)
}

func (svc *IdentityService) ConfirmPasswordReset(ctx context.Context, input contract.PasswordResetConfirm) (*contract.PasswordResetResponse, error) {
	if svc.configErr != nil {
		return nil, svc.configurationUnavailable()
	}
	if fields := validatePassword(input.Password, "/password"); len(fields) > 0 {
		return nil, validationError(fields...)
	}
	token, err := store.LookupCity311PasswordResetTokenByTokenHash(ctx, svc.store, svc.hashToken(strings.TrimSpace(input.Token)))
	now := svc.now()
	if err = validatePasswordResetToken(token, err, now); err != nil {
		return nil, err
	}
	account, err := store.LookupCity311LocalAccountByID(ctx, svc.store, token.UserID)
	if err != nil {
		return nil, invalidResetTokenError()
	}
	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		_, err = svc.applyPasswordReset(ctx, tx, token.ID, account, input.Password, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	svc.wakeNotificationWorker()
	return &contract.PasswordResetResponse{Message: "Your password has been reset."}, nil
}

func validatePasswordResetToken(token *composeTypes.City311PasswordResetToken, lookupErr error, now time.Time) error {
	if lookupErr != nil || token == nil || token.UsedAt != nil {
		return invalidResetTokenError()
	}
	if !now.Before(token.ExpiresAt) {
		return apiError(422, contract.ErrorExpiredResetToken, "The reset token has expired.")
	}
	return nil
}

func invalidResetTokenError() *ServiceError {
	return apiError(422, contract.ErrorInvalidResetToken, invalidResetTokenMessage)
}

func (svc *IdentityService) applyPasswordReset(ctx context.Context, tx store.Storer, tokenID uint64, account *composeTypes.City311LocalAccount, password string, now time.Time) (*composeTypes.City311IdentityNotification, error) {
	current, err := store.LookupCity311PasswordResetTokenByID(ctx, tx, tokenID)
	if validationErr := validatePasswordResetToken(current, err, svc.now()); validationErr != nil {
		return nil, validationErr
	}
	current.UsedAt = &now
	if err = store.UpdateCity311PasswordResetToken(ctx, tx, current); err != nil {
		return nil, err
	}
	if err = svc.setPassword(ctx, tx, account.ID, password, now); err != nil {
		return nil, err
	}
	if err = svc.deleteSessions(ctx, tx, account.ID, 0); err != nil {
		return nil, err
	}
	if err = svc.createIdentityAudit(ctx, tx, account.ID, "PASSWORD_RESET", nil, map[string]any{"sessions_revoked": true}); err != nil {
		return nil, err
	}
	notification := svc.newNotification(account.ID, securityNoticeKind, account.VerifiedEmail,
		"password-reset-security:"+strconv.FormatUint(current.ID, 10), map[string]any{
			"subject": "Your City 311 password was reset", "body": "Your City 311 password was reset and all existing sessions were revoked.",
		})
	return notification, store.CreateCity311IdentityNotification(ctx, tx, notification)
}

func (svc *IdentityService) ChangePassword(ctx context.Context, resolved *ResolvedSession, input contract.PasswordChange) error {
	if svc.configErr != nil {
		return svc.configurationUnavailable()
	}
	if err := requireConstituentSession(resolved); err != nil {
		return err
	}
	if !svc.passwordMatches(ctx, svc.store, resolved.User.ID, input.CurrentPassword) {
		return validationError(contract.FieldError{Field: "/current_password", Code: contract.ValidationInvalidValue})
	}
	if fields := validatePassword(input.NewPassword, "/new_password"); len(fields) > 0 {
		return validationError(fields...)
	}
	now := svc.now()
	return store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		if err := svc.setPassword(ctx, tx, resolved.User.ID, input.NewPassword, now); err != nil {
			return err
		}
		if err := svc.deleteSessions(ctx, tx, resolved.User.ID, resolved.Record.ID); err != nil {
			return err
		}
		return svc.createIdentityAudit(ctx, tx, resolved.User.ID, "PASSWORD_CHANGED", nil, map[string]any{"other_sessions_revoked": true})
	})
}

func (svc *IdentityService) ChangeLoginIdentifier(ctx context.Context, resolved *ResolvedSession, input contract.LoginIdentifierChange) (*contract.Session, error) {
	if svc.configErr != nil {
		return nil, svc.configurationUnavailable()
	}
	if err := requireConstituentSession(resolved); err != nil {
		return nil, err
	}
	identifier, err := svc.validateLoginIdentifierChange(ctx, resolved, input)
	if err != nil {
		return nil, err
	}
	var account *composeTypes.City311LocalAccount
	var user *systemTypes.User
	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		account, user, _, err = svc.persistLoginIdentifierChange(ctx, tx, resolved, identifier)
		return err
	})
	if err != nil {
		return nil, err
	}
	resolved.Account = account
	resolved.User = user
	svc.wakeNotificationWorker()
	actor, err := svc.currentActor(ctx, user, account)
	if err != nil {
		return nil, err
	}
	resolved.Actor = actor
	return svc.Session(resolved), nil
}

func (svc *IdentityService) validateLoginIdentifierChange(ctx context.Context, resolved *ResolvedSession, input contract.LoginIdentifierChange) (string, error) {
	if !svc.passwordMatches(ctx, svc.store, resolved.User.ID, input.CurrentPassword) {
		return "", validationError(contract.FieldError{Field: "/current_password", Code: contract.ValidationInvalidValue})
	}
	identifier := normalizeIdentifier(input.LoginIdentifier)
	if !localIdentifierPattern.MatchString(identifier) {
		return "", validationError(contract.FieldError{Field: loginIdentifierField, Code: contract.ValidationInvalidFormat})
	}
	if err := ensureLoginIdentifierAvailable(ctx, svc.store, identifier, resolved.User.ID); err != nil {
		return "", err
	}
	return identifier, nil
}

func ensureLoginIdentifierAvailable(ctx context.Context, s store.Storer, identifier string, userID uint64) error {
	existing, err := store.LookupCity311LocalAccountByLoginIdentifier(ctx, s, identifier)
	if errors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if existing.ID != userID {
		return validationError(contract.FieldError{Field: loginIdentifierField, Code: contract.ValidationDuplicate})
	}
	return nil
}

func (svc *IdentityService) persistLoginIdentifierChange(ctx context.Context, tx store.Storer, resolved *ResolvedSession, identifier string) (*composeTypes.City311LocalAccount, *systemTypes.User, *composeTypes.City311IdentityNotification, error) {
	if err := store.LockCity311LocalAccount(ctx, tx, resolved.User.ID); err != nil {
		return nil, nil, nil, err
	}
	account, err := store.LookupCity311LocalAccountByID(ctx, tx, resolved.User.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	oldIdentifier := account.LoginIdentifier
	if err = ensureLoginIdentifierAvailable(ctx, tx, identifier, account.ID); err != nil {
		return nil, nil, nil, err
	}
	now := svc.now()
	user, err := svc.updateLoginIdentifierProjections(ctx, tx, account, identifier, now)
	if err != nil {
		return nil, nil, nil, err
	}
	if err = svc.deleteSessions(ctx, tx, account.ID, resolved.Record.ID); err != nil {
		return nil, nil, nil, err
	}
	if err = svc.createIdentityAudit(ctx, tx, account.ID, "LOGIN_IDENTIFIER_CHANGED",
		map[string]any{"login_identifier": oldIdentifier}, map[string]any{"login_identifier": identifier}); err != nil {
		return nil, nil, nil, err
	}
	notification := svc.newNotification(account.ID, securityNoticeKind, account.VerifiedEmail,
		"login-identifier:"+strconv.FormatUint(account.ID, 10)+":"+strconv.FormatInt(now.Unix(), 10), map[string]any{
			"subject": "Your City 311 login identifier changed",
			"body":    fmt.Sprintf("Your City 311 login identifier changed from %s to %s.", oldIdentifier, identifier),
		})
	if err = store.CreateCity311IdentityNotification(ctx, tx, notification); err != nil {
		return nil, nil, nil, err
	}
	return account, user, notification, nil
}

func (svc *IdentityService) updateLoginIdentifierProjections(ctx context.Context, tx store.Storer, account *composeTypes.City311LocalAccount, identifier string, now time.Time) (*systemTypes.User, error) {
	account.LoginIdentifier = identifier
	account.UpdatedAt = now
	if err := store.UpdateCity311LocalAccount(ctx, tx, account); err != nil {
		return nil, err
	}
	user, err := store.LookupUserByID(ctx, tx, account.ID)
	if err != nil {
		return nil, err
	}
	user.Username = identifier
	user.UpdatedAt = &now
	if err = store.UpdateUser(ctx, tx, user); err != nil {
		return nil, err
	}
	constituentID := "C-" + strconv.FormatUint(account.ID, 10)
	constituent, err := store.LookupCity311ConstituentByConstituentID(ctx, tx, constituentID)
	if err != nil {
		return nil, err
	}
	if constituent.Profile["login_identifier"] != identifier {
		if err = advanceProfileVersion(constituent); err != nil {
			return nil, err
		}
	}
	constituent.Profile["login_identifier"] = identifier
	constituent.UpdatedAt = now
	if err = store.UpdateCity311Constituent(ctx, tx, constituent); err != nil {
		return nil, err
	}
	return user, nil
}

func requireConstituentSession(resolved *ResolvedSession) error {
	if resolved == nil || resolved.Record == nil || resolved.Actor == nil {
		return apiError(401, contract.ErrorUnauthenticated, authenticationRequiredMessage)
	}
	for _, role := range resolved.Actor.ApplicationRoles {
		if role == contract.ApplicationRoleConstituent {
			return nil
		}
	}
	return apiError(403, contract.ErrorForbidden, "A constituent account is required.")
}

func (svc *IdentityService) configurationUnavailable() *ServiceError {
	return &ServiceError{Status: 503, Payload: contract.APIError{
		Error: contract.ErrorTemporarilyUnavailable, Message: "City 311 identity configuration is unavailable.", Retryable: true,
	}}
}

func (svc *IdentityService) passwordMatches(ctx context.Context, s store.Storer, userID uint64, password string) bool {
	credentials, _, err := store.SearchCredentials(ctx, s, systemTypes.CredentialFilter{OwnerID: userID, Kind: passwordCredentialKind})
	if err != nil {
		return false
	}
	for _, credential := range credentials {
		if credential.Valid() && bcrypt.CompareHashAndPassword([]byte(credential.Credentials), []byte(password)) == nil {
			return true
		}
	}
	return false
}

func (svc *IdentityService) setPassword(ctx context.Context, tx store.Storer, userID uint64, password string, now time.Time) error {
	credentials, _, err := store.SearchCredentials(ctx, tx, systemTypes.CredentialFilter{OwnerID: userID, Kind: passwordCredentialKind})
	if err != nil {
		return err
	}
	for _, credential := range credentials {
		credential.DeletedAt = &now
	}
	if len(credentials) > 0 {
		if err = store.UpdateCredential(ctx, tx, credentials...); err != nil {
			return err
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return store.CreateCredential(ctx, tx, &systemTypes.Credential{
		ID: svc.nextID(), OwnerID: userID, Kind: passwordCredentialKind, Credentials: string(hash), CreatedAt: now,
	})
}

func (svc *IdentityService) deleteSessions(ctx context.Context, tx store.Storer, userID, keepID uint64) error {
	sessions, _, err := store.SearchCity311IdentitySessions(ctx, tx, composeTypes.City311IdentitySessionFilter{UserID: userID})
	if err != nil {
		return err
	}
	for _, session := range sessions {
		if session.ID != keepID {
			if err = store.DeleteCity311IdentitySessionByID(ctx, tx, session.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (svc *IdentityService) createIdentityAudit(ctx context.Context, tx store.Storer, userID uint64, event string, before, after map[string]any) error {
	if before == nil {
		before = map[string]any{}
	}
	if after == nil {
		after = map[string]any{}
	}
	return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
		ID: svc.nextID(), EntityType: "account", EntityID: strconv.FormatUint(userID, 10),
		EventType: event, ActorType: contract.AuditActorConstituent, ActorID: userID,
		SourceChannel: contract.SourceChannelPortalAuthenticated, Before: before, After: after, CreatedAt: svc.now(),
	})
}

func (svc *IdentityService) newNotification(userID uint64, kind, recipient, deliveryKey string, payload map[string]any) *composeTypes.City311IdentityNotification {
	now := svc.now()
	return &composeTypes.City311IdentityNotification{
		ID: svc.nextID(), UserID: userID, Kind: kind, Recipient: recipient, DeliveryKey: deliveryKey,
		Payload: payload, Status: notificationPending, CreatedAt: now, UpdatedAt: now,
	}
}

func (svc *IdentityService) sealNotificationSecret(value, deliveryKey string) (string, error) {
	block, err := aes.NewCipher(svc.notificationEncryptionKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(svc.random, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(value), []byte(deliveryKey))
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (svc *IdentityService) openNotificationSecret(value, deliveryKey string) (string, error) {
	sealed, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return "", fmt.Errorf("decode notification secret: %w", err)
	}
	block, err := aes.NewCipher(svc.notificationEncryptionKey())
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(sealed) < gcm.NonceSize() {
		return "", fmt.Errorf("notification secret is malformed")
	}
	opened, err := gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], []byte(deliveryKey))
	if err != nil {
		return "", fmt.Errorf("open notification secret: %w", err)
	}
	return string(opened), nil
}

func (svc *IdentityService) notificationEncryptionKey() []byte {
	key := sha256.Sum256(append([]byte("city311-identity-notification-v1\x00"), svc.secret...))
	return key[:]
}

func (svc *IdentityService) RetryPendingNotifications(ctx context.Context) error {
	if svc.configErr != nil {
		return svc.configurationUnavailable()
	}
	for {
		notifications, err := svc.pendingNotificationBatch(ctx)
		if err != nil {
			return err
		}
		if len(notifications) == 0 {
			return nil
		}
		if err = svc.deliverNotificationBatch(ctx, notifications); err != nil {
			return err
		}
	}
}

func (svc *IdentityService) StartNotificationWorker(ctx context.Context) {
	if svc.configErr != nil {
		return
	}
	svc.workerOnce.Do(func() {
		go svc.runNotificationWorker(ctx)
		svc.wakeNotificationWorker()
	})
}

func (svc *IdentityService) SetNotificationWorkerErrorHandler(handler func(error)) {
	svc.workerError = handler
}

func (svc *IdentityService) runNotificationWorker(ctx context.Context) {
	ticker := time.NewTicker(svc.notificationPoll)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-svc.notificationWake:
		case <-ticker.C:
		}
		if err := svc.RetryPendingNotifications(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			if svc.workerError != nil {
				svc.workerError(err)
			}
		}
	}
}

func (svc *IdentityService) wakeNotificationWorker() {
	select {
	case svc.notificationWake <- struct{}{}:
	default:
	}
}

func (svc *IdentityService) pendingNotificationBatch(ctx context.Context) (composeTypes.City311IdentityNotificationSet, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	notifications, _, err := store.SearchCity311IdentityNotifications(ctx, svc.store, composeTypes.City311IdentityNotificationFilter{
		Status: notificationPending, Paging: filter.Paging{Limit: identityRecoveryBatchSize},
	})
	return notifications, err
}

func (svc *IdentityService) deliverNotificationBatch(ctx context.Context, notifications composeTypes.City311IdentityNotificationSet) error {
	for _, notification := range notifications {
		if err := svc.deliverNotification(ctx, notification); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (svc *IdentityService) deliverNotification(ctx context.Context, notification *composeTypes.City311IdentityNotification) error {
	if notification == nil || notification.Status != notificationPending {
		return nil
	}
	if notification.Attempts >= len(notificationRetryDelays) {
		notification.Status = notificationFailed
		if notification.LastError == "" {
			notification.LastError = "notification retry budget exhausted"
		}
		notification.UpdatedAt = svc.now()
		return store.UpdateCity311IdentityNotification(ctx, svc.store, notification)
	}
	for notification.Status == notificationPending && notification.Attempts < len(notificationRetryDelays) {
		delay := notificationRetryDelays[notification.Attempts]
		if remaining := delay - svc.now().Sub(notification.UpdatedAt); remaining > 0 {
			if err := svc.wait(ctx, remaining); err != nil {
				return err
			}
		}
		if err := svc.attemptNotificationOnce(ctx, notification); err != nil {
			return err
		}
	}
	return nil
}

func (svc *IdentityService) attemptNotificationOnce(ctx context.Context, notification *composeTypes.City311IdentityNotification) error {
	deliveryErr := svc.sendIdentityNotification(ctx, notification)
	notification.Attempts++
	notification.UpdatedAt = svc.now()
	switch {
	case deliveryErr == nil:
		notification.Status = notificationSent
		notification.LastError = ""
	case !retryableIdentityDelivery(deliveryErr) || notification.Attempts >= len(notificationRetryDelays):
		notification.Status = notificationFailed
		notification.LastError = deliveryErr.Error()
	default:
		notification.Status = notificationPending
		notification.LastError = deliveryErr.Error()
	}
	return store.UpdateCity311IdentityNotification(ctx, svc.store, notification)
}

func (svc *IdentityService) sendIdentityNotification(ctx context.Context, notification *composeTypes.City311IdentityNotification) error {
	switch notification.Kind {
	case passwordResetKind:
		sealedToken, _ := notification.Payload["sealed_token"].(string)
		token, err := svc.openNotificationSecret(sealedToken, notification.DeliveryKey)
		if err != nil {
			return err
		}
		return svc.notifier.PasswordReset(ctx, notification.Recipient, token, notification.DeliveryKey)
	case securityNoticeKind:
		subject, _ := notification.Payload["subject"].(string)
		body, _ := notification.Payload["body"].(string)
		return svc.notifier.SecurityNotice(ctx, notification.Recipient, subject, body, notification.DeliveryKey)
	default:
		return fmt.Errorf("unsupported identity notification kind %q", notification.Kind)
	}
}

func retryableIdentityDelivery(err error) bool {
	if err == nil {
		return false
	}
	var smtpErr *textproto.Error
	if stderrors.As(err, &smtpErr) {
		return smtpErr.Code == 421 || smtpErr.Code == 451
	}
	return true
}

func (svc *IdentityService) currentActor(ctx context.Context, user *systemTypes.User, account *composeTypes.City311LocalAccount) (*contract.CurrentActor, error) {
	profile, err := store.LookupCity311ActorProfileByID(ctx, svc.store, user.ID)
	if err != nil {
		return nil, err
	}
	roles := append([]contract.ApplicationRole(nil), profile.ApplicationRoles...)
	departmentCodes := []contract.DepartmentCode{}
	districtCodes := append([]contract.DistrictCode(nil), profile.Districts...)
	if profile.Department != "" {
		departmentCodes = append(departmentCodes, profile.Department)
	}
	if identityHasRole(roles, contract.ApplicationRolePlatformAdministrator) {
		departmentCodes = append([]contract.DepartmentCode(nil), contract.DepartmentCodes...)
		districtCodes = append([]contract.DistrictCode(nil), contract.DistrictCodes...)
	}
	capabilities, routes := roleAccess(roles)
	oidcActorType := (*string)(nil)
	if identityHasRole(roles, contract.ApplicationRoleConstituent) {
		value := "constituent"
		oidcActorType = &value
	}
	return &contract.CurrentActor{
		ActorID: strconv.FormatUint(user.ID, 10), DisplayName: user.Name, OIDCActorType: oidcActorType,
		ApplicationRoles: roles, DepartmentCodes: uniqueSorted(departmentCodes), DistrictCodes: uniqueSorted(districtCodes),
		Capabilities: capabilities, Scopes: roleScopes(roles), AvailableRoutes: routes,
	}, nil
}

func roleAccess(roles []contract.ApplicationRole) ([]string, []string) {
	document := contract.NewContractDocument()
	capabilitySet := map[string]bool{}
	routeSet := map[string]bool{
		"session_current": true, "portal_service_request_submit": true, "anonymous_status_lookup": true,
		"public_branding_get": true, "public_content_get": true, "public_help_get": true,
	}
	for name, endpoint := range document.Endpoints {
		if endpoint.Authentication.Mode != "session_cookie" && endpoint.Authentication.Mode != "session_cookie_optional" {
			continue
		}
		if !roleAllowsEndpoint(roles, endpoint.Authentication) {
			continue
		}
		routeSet[name] = true
		if endpoint.RequiredCapability != "" {
			capabilitySet[endpoint.RequiredCapability] = true
		}
	}
	return sortedKeys(capabilitySet), sortedKeys(routeSet)
}

func roleAllowsEndpoint(roles []contract.ApplicationRole, authentication contract.AuthenticationContract) bool {
	switch authentication.ActorClass {
	case "constituent":
		if !identityHasRole(roles, contract.ApplicationRoleConstituent) {
			return false
		}
	case "staff":
		if !hasStaffRole(roles) {
			return false
		}
	case "any_authenticated_actor", "":
	default:
		return false
	}
	if len(authentication.Alternatives) == 0 {
		return true
	}
	for _, alternative := range authentication.Alternatives {
		for _, allowed := range alternative.ApplicationRoles {
			if identityHasRole(roles, contract.ApplicationRole(allowed)) {
				return true
			}
		}
	}
	return false
}

func hasStaffRole(roles []contract.ApplicationRole) bool {
	for _, role := range roles {
		switch role {
		case contract.ApplicationRoleServiceAgent, contract.ApplicationRoleSupervisor,
			contract.ApplicationRoleDepartmentManager, contract.ApplicationRolePlatformAdministrator,
			contract.ApplicationRoleWorkflowDesigner:
			return true
		}
	}
	return false
}

func identityHasRole(roles []contract.ApplicationRole, target contract.ApplicationRole) bool {
	for _, role := range roles {
		if role == target {
			return true
		}
	}
	return false
}

func roleScopes(roles []contract.ApplicationRole) []string {
	set := map[string]bool{}
	for _, role := range roles {
		switch role {
		case contract.ApplicationRoleConstituent, contract.ApplicationRoleServiceAgent, contract.ApplicationRoleSupervisor:
			set[contract.ScopeRequestWrite] = true
		case contract.ApplicationRoleDepartmentManager, contract.ApplicationRolePlatformAdministrator:
			set[contract.ScopeRequestWrite] = true
			set[contract.ScopeCRMExport] = true
		case contract.ApplicationRoleWorkflowDesigner:
			set["workflow.execute"] = true
		}
	}
	return sortedKeys(set)
}

func sortedKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func uniqueSorted[T ~string](values []T) []T {
	set := make(map[T]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	out := make([]T, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
