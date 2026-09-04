package city311

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

func testProfileService(t *testing.T) (*IdentityService, store.Storer, *ResolvedSession, string) {
	t.Helper()
	svc, st, _, _ := testIdentityService(t)
	// Enable real SQL transactions for rollback tests; the baseline SQLite
	// driver otherwise bypasses them. This is not a PostgreSQL concurrency test.
	database := st.(*rdbms.Store)
	database.TxRetryLimit = 0
	database.DB.(*sqlx.DB).SetMaxOpenConns(1)
	input := validAccountRegistration("profile.user", "profile@example.invalid")
	_, err := svc.Register(context.Background(), input)
	require.NoError(t, err)
	token, resolved, err := svc.SignIn(context.Background(), contract.LocalSignIn{LoginIdentifier: input.LoginIdentifier, Password: input.Password})
	require.NoError(t, err)
	return svc, st, resolved, token
}

func decodeProfilePatch(t *testing.T, raw string) contract.ProfileUpdate {
	t.Helper()
	var input contract.ProfileUpdate
	require.NoError(t, json.Unmarshal([]byte(raw), &input))
	return input
}

func updateTestProfile(t *testing.T, svc *IdentityService, ctx context.Context, resolved *ResolvedSession, input contract.ProfileUpdate) (*contract.Constituent, error) {
	t.Helper()
	version := uint64(1)
	if current, err := svc.GetProfileSnapshot(ctx, resolved); err == nil {
		version = current.Version
	}
	result, err := svc.UpdateProfile(ctx, resolved, version, input)
	if err != nil {
		return nil, err
	}
	return result.Constituent, nil
}

func TestProfileUpdatePersistsProjectionsAndPreservesHistory(t *testing.T) {
	svc, st, resolved, token := testProfileService(t)
	ctx := context.Background()
	profile, err := svc.GetProfile(ctx, resolved)
	require.NoError(t, err)
	// Keep a historical snapshot whose phone/address values must not be rewritten.
	base := New(st)
	request, _, err := base.Submit(ctx, validSubmission(), "profile-history", SubmissionOptions{})
	require.NoError(t, err)
	oldRequests, _, err := store.SearchCity311ServiceRequests(ctx, st, composeTypes.City311ServiceRequestFilter{RequestNumber: request.RequestNumber})
	require.NoError(t, err)
	require.Len(t, oldRequests, 1)
	snapshot, err := json.Marshal(oldRequests[0].PrimaryRequester)
	require.NoError(t, err)
	beforeTime := svc.now()
	svc.now = func() time.Time { return beforeTime.Add(time.Minute) }
	input := decodeProfilePatch(t, `{"display_name":"  Nguyễn Resident  ","phone_numbers":[{"label":"MOBILE","value":"+17165550100"},{"label":"HOME","value":"+17165550101"},{"label":"WORK","value":"+17165550102"}],"addresses":[{"line1":"1 Main St","city":"Buffalo","region":"NY","postal_code":"14201","country":"US","primary":true}],"primary_category":"VETERAN","preferred_language":"VI"}`)
	updated, err := updateTestProfile(t, svc, ctx, resolved, input)
	require.NoError(t, err)
	require.Equal(t, profile.ConstituentID, updated.ConstituentID)
	require.Equal(t, profile.Emails, updated.Emails)
	require.Equal(t, profile.LoginIdentifier, updated.LoginIdentifier)
	require.Equal(t, profile.EmailOptOut, updated.EmailOptOut)
	require.Equal(t, "Nguyễn Resident", updated.DisplayName)
	require.Len(t, updated.PhoneNumbers, 3)
	require.Len(t, updated.Addresses, 1)
	require.Equal(t, contract.LanguageVI, updated.PreferredLanguage)
	current, err := svc.Resolve(ctx, token)
	require.NoError(t, err)
	require.Equal(t, updated.DisplayName, current.Actor.DisplayName)
	require.Equal(t, updated.PreferredLanguage, svc.Session(current).PreferredLanguage)
	require.Equal(t, resolved.Record.ID, current.Record.ID)
	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EntityID: profile.ConstituentID, EventType: "PROFILE_UPDATED"})
	require.NoError(t, err)
	require.Len(t, audits, 1)
	require.Equal(t, resolved.Record.UserID, audits[0].ActorID)
	require.Equal(t, "constituent", audits[0].EntityType)
	require.Equal(t, profile.DisplayName, audits[0].Before["display_name"])
	require.Equal(t, updated.DisplayName, audits[0].After["display_name"])
	require.Equal(t, svc.now(), audits[0].CreatedAt)
	require.NotContains(t, audits[0].After, "emails")
	storedRequest, err := store.LookupCity311ServiceRequestByID(ctx, st, oldRequests[0].ID)
	require.NoError(t, err)
	persistedSnapshot, err := json.Marshal(storedRequest.PrimaryRequester)
	require.NoError(t, err)
	require.Equal(t, snapshot, persistedSnapshot)
	require.NoError(t, store.Upgrade(ctx, zap.NewNop(), st))
	restarted := NewIdentity(st, IdentityOptions{Secret: svc.secret, Now: svc.now})
	current, err = restarted.Resolve(ctx, token)
	require.NoError(t, err)
	loaded, err := restarted.GetProfile(ctx, current)
	require.NoError(t, err)
	require.Equal(t, updated, loaded)
	_, err = updateTestProfile(t, restarted, ctx, current, input)
	require.NoError(t, err)
	audits, _, err = store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EntityID: profile.ConstituentID, EventType: "PROFILE_UPDATED"})
	require.NoError(t, err)
	require.Len(t, audits, 1, "equivalent retry must not duplicate audit state")
	cleared, err := updateTestProfile(t, restarted, ctx, current, decodeProfilePatch(t, `{"phone_numbers":[],"addresses":[]}`))
	require.NoError(t, err)
	require.Empty(t, cleared.PhoneNumbers)
	require.Empty(t, cleared.Addresses)
	require.Equal(t, updated.DisplayName, cleared.DisplayName)
}

func TestProfileValidationLeavesAllStateUnchanged(t *testing.T) {
	svc, st, resolved, _ := testProfileService(t)
	ctx := context.Background()
	original, err := svc.GetProfile(ctx, resolved)
	require.NoError(t, err)
	for _, tc := range []struct{ body, field string }{
		{`{"display_name":" "}`, "/display_name"},
		{`{"display_name":"` + strings.Repeat("界", 121) + `"}`, "/display_name"},
		{`{"phone_numbers":[{"label":"FAX","value":"123"}]}`, "/phone_numbers/0/label"},
		{`{"phone_numbers":[{"label":"HOME","value":"123"}]}`, "/phone_numbers/0/value"},
		{`{"phone_numbers":[{},{},{},{}]}`, "/phone_numbers"},
		{`{"addresses":[{}]}`, "/addresses/0/line1"},
		{`{"addresses":[{"country":" US "}]}`, "/addresses/0/country"},
		{`{"addresses":[{"line1":"` + strings.Repeat(" ", 200) + `X"}]}`, "/addresses/0/line1"},
		{`{"addresses":[{},{},{},{},{},{}]}`, "/addresses"},
		{`{"addresses":[{"primary":true},{"primary":true}]}`, "/addresses"},
		{`{"primary_category":"UNKNOWN"}`, "/primary_category"},
		{`{"preferred_language":"FR"}`, "/preferred_language"},
	} {
		t.Run(tc.body, func(t *testing.T) {
			_, err := updateTestProfile(t, svc, ctx, resolved, decodeProfilePatch(t, tc.body))
			failure := requireIdentityError(t, err, 422, contract.ErrorValidation)
			found := false
			for _, item := range failure.Payload.Errors {
				found = found || item.Field == tc.field
			}
			require.True(t, found, "%s: %#v", tc.field, failure.Payload.Errors)
			current, err := svc.GetProfile(ctx, resolved)
			require.NoError(t, err)
			require.Equal(t, original, current)
		})
	}
	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: "PROFILE_UPDATED"})
	require.NoError(t, err)
	require.Empty(t, audits)
}

func TestProfileHistoricalContactsAndAuditRemainUnchanged(t *testing.T) {
	svc, st, resolved, _ := testProfileService(t)
	ctx := context.Background()
	profile, err := updateTestProfile(t, svc, ctx, resolved, decodeProfilePatch(t, `{"phone_numbers":[{"label":"HOME","value":"+17165550101"}],"addresses":[{"line1":"1 Old St","city":"Buffalo","region":"NY","postal_code":"14201","country":"US"}]}`))
	require.NoError(t, err)
	base := New(st)
	request, _, err := base.Submit(ctx, validSubmission(), "linked-profile-history", SubmissionOptions{
		ExistingConstituentID: profile.ConstituentID,
		StaffActor:            &contract.Actor{ID: 77, Roles: []contract.ApplicationRole{contract.ApplicationRolePlatformAdministrator}},
	})
	require.NoError(t, err)
	stored, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, request.RequestNumber)
	require.NoError(t, err)
	require.Equal(t, profile.ConstituentID, stored.PrimaryRequester["constituent_id"])
	snapshot, err := json.Marshal(stored.PrimaryRequester)
	require.NoError(t, err)
	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EntityID: profile.ConstituentID, EventType: "PROFILE_UPDATED"})
	require.NoError(t, err)
	require.Len(t, audits, 1)
	auditBefore, err := json.Marshal(audits[0])
	require.NoError(t, err)
	_, err = updateTestProfile(t, svc, ctx, resolved, decodeProfilePatch(t, `{"phone_numbers":[],"addresses":[]}`))
	require.NoError(t, err)
	stored, err = store.LookupCity311ServiceRequestByID(ctx, st, stored.ID)
	require.NoError(t, err)
	after, err := json.Marshal(stored.PrimaryRequester)
	require.NoError(t, err)
	require.Equal(t, snapshot, after)
	audit, err := store.LookupCity311AuditEventByID(ctx, st, audits[0].ID)
	require.NoError(t, err)
	auditAfter, err := json.Marshal(audit)
	require.NoError(t, err)
	require.Equal(t, auditBefore, auditAfter)
}

func TestProfileAuditFailureRollsBackUserAccountAndConstituent(t *testing.T) {
	svc, st, resolved, token := testProfileService(t)
	ctx := context.Background()
	original, err := svc.GetProfile(ctx, resolved)
	require.NoError(t, err)
	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{})
	require.NoError(t, err)
	require.NotEmpty(t, audits)
	svc.nextID = func() uint64 { return audits[0].ID }
	_, err = updateTestProfile(t, svc, ctx, resolved, decodeProfilePatch(t, `{"display_name":"Must roll back","preferred_language":"ES"}`))
	require.Error(t, err)
	current, err := svc.Resolve(ctx, token)
	require.NoError(t, err)
	profile, err := svc.GetProfile(ctx, current)
	require.NoError(t, err)
	require.Equal(t, original, profile)
	require.Equal(t, original.DisplayName, current.Actor.DisplayName)
	require.Equal(t, original.PreferredLanguage, svc.Session(current).PreferredLanguage)
}

func TestProfileAuthorizationAndEmptyPatch(t *testing.T) {
	svc, st, resolved, _ := testProfileService(t)
	ctx := context.Background()
	_, err := svc.GetProfile(ctx, nil)
	requireIdentityError(t, err, 401, contract.ErrorUnauthenticated)
	_, err = updateTestProfile(t, svc, ctx, nil, contract.ProfileUpdate{})
	requireIdentityError(t, err, 401, contract.ErrorUnauthenticated)
	staff := *resolved
	actor := *resolved.Actor
	actor.ApplicationRoles = []contract.ApplicationRole{contract.ApplicationRolePlatformAdministrator}
	staff.Actor = &actor
	_, err = svc.GetProfile(ctx, &staff)
	requireIdentityError(t, err, 403, contract.ErrorForbidden)
	_, err = updateTestProfile(t, svc, ctx, &staff, contract.ProfileUpdate{})
	requireIdentityError(t, err, 403, contract.ErrorForbidden)
	original, err := svc.GetProfile(ctx, resolved)
	require.NoError(t, err)
	result, err := updateTestProfile(t, svc, ctx, resolved, contract.ProfileUpdate{})
	require.NoError(t, err)
	require.Equal(t, original, result)
	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: "PROFILE_UPDATED"})
	require.NoError(t, err)
	require.Empty(t, audits)
}

func TestProfileUsesFreshProjectionsAcrossServiceInstances(t *testing.T) {
	svc, st, resolved, token := testProfileService(t)
	ctx := context.Background()
	other := NewIdentity(st, IdentityOptions{Secret: svc.secret, Now: svc.now})
	stale, err := other.Resolve(ctx, token)
	require.NoError(t, err)
	user, err := store.LookupUserByID(ctx, st, resolved.Record.UserID)
	require.NoError(t, err)
	user.Meta.AvatarColor = "#123456"
	require.NoError(t, store.UpdateUser(ctx, st, user))
	_, err = updateTestProfile(t, svc, ctx, resolved, decodeProfilePatch(t, `{"display_name":"Current Name","preferred_language":"ES"}`))
	require.NoError(t, err)
	_, err = other.ChangeLoginIdentifier(ctx, stale, contract.LoginIdentifierChange{
		LoginIdentifier: "profile.renamed", CurrentPassword: "StrongPassword1!",
	})
	require.NoError(t, err)
	// The first service still has the original resolved account/user. Its next
	// identifier write must audit the current database value, not that snapshot.
	now := svc.now().Add(time.Minute)
	svc.now = func() time.Time { return now }
	_, err = svc.ChangeLoginIdentifier(ctx, resolved, contract.LoginIdentifierChange{
		LoginIdentifier: "profile.final", CurrentPassword: "StrongPassword1!",
	})
	require.NoError(t, err)
	profile, err := updateTestProfile(t, other, ctx, stale, decodeProfilePatch(t, `{"primary_category":"VETERAN"}`))
	require.NoError(t, err)
	require.Equal(t, "profile.final", profile.LoginIdentifier)
	require.Equal(t, "Current Name", profile.DisplayName)
	require.Equal(t, contract.LanguageES, profile.PreferredLanguage)
	current, err := svc.Resolve(ctx, token)
	require.NoError(t, err)
	require.Equal(t, "Current Name", current.User.Name)
	require.Equal(t, "#123456", current.User.Meta.AvatarColor)
	require.Equal(t, "es", current.User.Meta.PreferredLanguage)
	require.Equal(t, "ES", current.Account.PreferredLanguage)
	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: "LOGIN_IDENTIFIER_CHANGED"})
	require.NoError(t, err)
	require.Len(t, audits, 2)
	for _, audit := range audits {
		if audit.After["login_identifier"] == "profile.final" {
			require.Equal(t, "profile.renamed", audit.Before["login_identifier"])
		}
	}
}

func TestProfileUnavailableAndMissingRecords(t *testing.T) {
	svc, st, resolved, _ := testProfileService(t)
	ctx := context.Background()
	svc.configErr = fmt.Errorf("test configuration failure")
	_, err := svc.GetProfile(ctx, resolved)
	requireIdentityError(t, err, 503, contract.ErrorTemporarilyUnavailable)
	_, err = updateTestProfile(t, svc, ctx, resolved, contract.ProfileUpdate{})
	requireIdentityError(t, err, 503, contract.ErrorTemporarilyUnavailable)
	svc.configErr = nil
	item, err := ownConstituent(ctx, st, resolved.Record.UserID)
	require.NoError(t, err)
	require.NoError(t, store.DeleteCity311Constituent(ctx, st, item))
	_, err = svc.GetProfile(ctx, resolved)
	requireIdentityError(t, err, 404, contract.ErrorNotFound)
	_, err = updateTestProfile(t, svc, ctx, resolved, contract.ProfileUpdate{})
	requireIdentityError(t, err, 404, contract.ErrorNotFound)
	// Unsupported adapters fail closed rather than silently skipping locking.
	require.EqualError(t, store.LockCity311LocalAccount(ctx, struct{ store.Storer }{st}, resolved.Record.UserID), "store does not support City 311 account locking")
}

func TestProfileVersionPreconditionsAcrossServiceInstances(t *testing.T) {
	svc, st, resolved, _ := testProfileService(t)
	ctx := context.Background()
	other := NewIdentity(st, IdentityOptions{Secret: svc.secret, Now: svc.now})
	initial, err := svc.GetProfileSnapshot(ctx, resolved)
	require.NoError(t, err)
	require.Equal(t, uint64(1), initial.Version)
	_, err = svc.UpdateProfile(ctx, resolved, 0, contract.ProfileUpdate{})
	requireIdentityError(t, err, 428, contract.ErrorExpectedVersionRequired)
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, service := range []*IdentityService{svc, other} {
		go func(service *IdentityService) {
			<-start
			name := "Concurrent profile"
			_, err := service.UpdateProfile(ctx, resolved, initial.Version, contract.ProfileUpdate{DisplayName: &name})
			results <- err
		}(service)
	}
	close(start)
	first, second := <-results, <-results
	if first == nil {
		first, second = second, first
	}
	require.NoError(t, second)
	conflict := requireIdentityError(t, first, 409, contract.ErrorVersionConflict)
	require.Equal(t, uint64(2), *conflict.Payload.CurrentVersion)
	current, err := svc.GetProfileSnapshot(ctx, resolved)
	require.NoError(t, err)
	unchanged, err := svc.UpdateProfile(ctx, resolved, current.Version, contract.ProfileUpdate{})
	require.NoError(t, err)
	require.Equal(t, current.Version, unchanged.Version)
	require.NoError(t, other.SetPreferredLanguage(ctx, resolved, contract.LanguageVI))
	_, err = svc.UpdateProfile(ctx, resolved, current.Version, contract.ProfileUpdate{})
	conflict = requireIdentityError(t, err, 409, contract.ErrorVersionConflict)
	require.Equal(t, uint64(3), *conflict.Payload.CurrentVersion)
	_, err = svc.ChangeLoginIdentifier(ctx, resolved, contract.LoginIdentifierChange{LoginIdentifier: "profile.versioned", CurrentPassword: "StrongPassword1!"})
	require.NoError(t, err)
	restarted := NewIdentity(st, IdentityOptions{Secret: svc.secret, Now: svc.now})
	current, err = restarted.GetProfileSnapshot(ctx, resolved)
	require.NoError(t, err)
	require.Equal(t, uint64(4), current.Version)
	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EventType: "PROFILE_UPDATED"})
	require.NoError(t, err)
	require.Len(t, audits, 2, "conflict and no-op must not append audit records")
}

func TestProfileRevisionRejectsCorruptOrExhaustedState(t *testing.T) {
	svc, st, resolved, _ := testProfileService(t)
	ctx := context.Background()
	item, err := ownConstituent(ctx, st, resolved.Record.UserID)
	require.NoError(t, err)
	for _, revision := range []string{"bad", "0", "18446744073709551615"} {
		item.Profile[profileRevisionKey] = revision
		require.NoError(t, store.UpdateCity311Constituent(ctx, st, item))
		require.Error(t, svc.SetPreferredLanguage(ctx, resolved, contract.LanguageES))
		stored, err := ownConstituent(ctx, st, resolved.Record.UserID)
		require.NoError(t, err)
		require.Equal(t, item.Profile, stored.Profile)
	}
}
