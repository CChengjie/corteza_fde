package city311

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/errors"
	"github.com/cortezaproject/corteza/server/store"
	systemTypes "github.com/cortezaproject/corteza/server/system/types"
)

const profileRevisionKey = "_profile_version"

type ProfileSnapshot struct {
	Constituent *contract.Constituent
	Version     uint64
}

func (svc *IdentityService) GetProfile(ctx context.Context, resolved *ResolvedSession) (*contract.Constituent, error) {
	result, err := svc.GetProfileSnapshot(ctx, resolved)
	if err != nil {
		return nil, err
	}
	return result.Constituent, nil
}

func (svc *IdentityService) GetProfileSnapshot(ctx context.Context, resolved *ResolvedSession) (*ProfileSnapshot, error) {
	if svc.configErr != nil {
		return nil, svc.configurationUnavailable()
	}
	if err := requireConstituentSession(resolved); err != nil {
		return nil, err
	}
	item, err := ownConstituent(ctx, svc.store, resolved.Record.UserID)
	if err != nil {
		return nil, err
	}
	return profileSnapshot(item)
}

func profileVersion(item *composeTypes.City311Constituent) (uint64, error) {
	if item.Profile[profileRevisionKey] == nil {
		return 1, nil
	}
	value, err := strconv.ParseUint(fmt.Sprint(item.Profile[profileRevisionKey]), 10, 64)
	if err != nil || value == 0 {
		return 0, fmt.Errorf("invalid stored profile revision")
	}
	return value, nil
}

func advanceProfileVersion(item *composeTypes.City311Constituent) error {
	value, err := profileVersion(item)
	if err != nil {
		return err
	}
	if value == ^uint64(0) {
		return fmt.Errorf("profile revision exhausted")
	}
	item.Profile[profileRevisionKey] = strconv.FormatUint(value+1, 10)
	return nil
}

func profileSnapshot(item *composeTypes.City311Constituent) (*ProfileSnapshot, error) {
	version, err := profileVersion(item)
	if err != nil {
		return nil, err
	}
	profile, err := constituentProfile(item)
	if err != nil {
		return nil, err
	}
	return &ProfileSnapshot{Constituent: profile, Version: version}, nil
}

func ownConstituent(ctx context.Context, s store.Storer, userID uint64) (*composeTypes.City311Constituent, error) {
	item, err := store.LookupCity311ConstituentByConstituentID(ctx, s, "C-"+strconv.FormatUint(userID, 10))
	if errors.IsNotFound(err) {
		return nil, apiError(404, contract.ErrorNotFound, "The constituent profile was not found.")
	}
	return item, err
}

func constituentProfile(item *composeTypes.City311Constituent) (*contract.Constituent, error) {
	var result contract.Constituent
	raw, err := json.Marshal(item.Profile)
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(raw, &result); err != nil {
		return nil, err
	}
	result.ConstituentID = item.ConstituentID
	return &result, nil
}

// UpdateProfile changes only the editable profile fields (9.1.2). Historical
// request snapshots, verified email, login identifier, opt-out and scope are not
// writable here. Profile/account/user projections and the audit share one tx.
func (svc *IdentityService) UpdateProfile(ctx context.Context, resolved *ResolvedSession, expectedVersion uint64, input contract.ProfileUpdate) (*ProfileSnapshot, error) {
	return svc.updateProfile(ctx, resolved, &expectedVersion, input)
}

// The optional-session language endpoint has no If-Match precondition in the
// published contract, but still advances the profile revision atomically.
func (svc *IdentityService) SetPreferredLanguage(ctx context.Context, resolved *ResolvedSession, language contract.Language) error {
	_, err := svc.updateProfile(ctx, resolved, nil, contract.ProfileUpdate{PreferredLanguage: &language})
	return err
}

func (svc *IdentityService) updateProfile(ctx context.Context, resolved *ResolvedSession, expectedVersion *uint64, input contract.ProfileUpdate) (*ProfileSnapshot, error) {
	if svc.configErr != nil {
		return nil, svc.configurationUnavailable()
	}
	if err := requireConstituentSession(resolved); err != nil {
		return nil, err
	}
	if expectedVersion != nil && *expectedVersion == 0 {
		return nil, apiError(428, contract.ErrorExpectedVersionRequired, "A quoted current profile version is required.")
	}
	patch, err := profilePatch(input)
	if err != nil {
		return nil, err
	}
	var result *ProfileSnapshot
	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		result, err = svc.persistProfilePatch(ctx, tx, resolved.Record.UserID, expectedVersion, patch)
		return err
	})
	return result, err
}

func (svc *IdentityService) persistProfilePatch(ctx context.Context, tx store.Storer, userID uint64, expectedVersion *uint64, patch map[string]any) (*ProfileSnapshot, error) {
	if err := store.LockCity311LocalAccount(ctx, tx, userID); err != nil {
		return nil, err
	}
	item, err := ownConstituent(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	version, err := profileVersion(item)
	if err != nil {
		return nil, err
	}
	if expectedVersion != nil && *expectedVersion != version {
		return nil, &ServiceError{Status: 409, Payload: contract.APIError{Error: contract.ErrorVersionConflict, Message: "The profile has changed. Reload before saving.", CurrentVersion: &version}}
	}
	before, after := changedProfileValues(item.Profile, patch)
	if len(after) == 0 {
		return profileSnapshot(item)
	}
	item.Profile = cloneMap(item.Profile)
	for field, value := range after {
		item.Profile[field] = value
	}
	if err = advanceProfileVersion(item); err != nil {
		return nil, err
	}
	item.UpdatedAt = svc.now()
	if err = store.UpdateCity311Constituent(ctx, tx, item); err != nil {
		return nil, err
	}
	if err = svc.updateProfileIdentity(ctx, tx, userID, after); err != nil {
		return nil, err
	}
	if err = store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
		ID: svc.nextID(), EntityType: "constituent", EntityID: item.ConstituentID,
		EventType: "PROFILE_UPDATED", ActorType: contract.AuditActorConstituent, ActorID: userID,
		SourceChannel: contract.SourceChannelPortalAuthenticated, Before: before, After: after, CreatedAt: svc.now(),
	}); err != nil {
		return nil, err
	}
	return profileSnapshot(item)
}

func (svc *IdentityService) updateProfileIdentity(ctx context.Context, tx store.Storer, userID uint64, changed map[string]any) error {
	name, nameChanged := changed["display_name"].(string)
	language, languageChanged := changed["preferred_language"].(string)
	if nameChanged || languageChanged {
		if err := svc.updateProfileUser(ctx, tx, userID, name, language); err != nil {
			return err
		}
	}
	if languageChanged {
		account, err := store.LookupCity311LocalAccountByID(ctx, tx, userID)
		if err != nil {
			return err
		}
		account.PreferredLanguage = language
		account.UpdatedAt = svc.now()
		return store.UpdateCity311LocalAccount(ctx, tx, account)
	}
	return nil
}

func (svc *IdentityService) updateProfileUser(ctx context.Context, tx store.Storer, userID uint64, name, language string) error {
	user, err := store.LookupUserByID(ctx, tx, userID)
	if err != nil {
		return err
	}
	// Empty is not a valid display name or language in an accepted patch.
	if name != "" {
		user.Name = name
	}
	if language != "" {
		if user.Meta == nil {
			user.Meta = &systemTypes.UserMeta{}
		}
		user.Meta.PreferredLanguage = strings.ToLower(language)
	}
	now := svc.now()
	user.UpdatedAt = &now
	return store.UpdateUser(ctx, tx, user)
}

func changedProfileValues(profile composeTypes.City311JSON, patch map[string]any) (map[string]any, map[string]any) {
	before, after := map[string]any{}, map[string]any{}
	for field, value := range patch {
		if !reflect.DeepEqual(profile[field], value) {
			before[field], after[field] = profile[field], value
		}
	}
	return before, after
}

func profilePatch(input contract.ProfileUpdate) (map[string]any, error) {
	var fields []contract.FieldError
	if input.DisplayName != nil {
		fields = append(fields, validateBoundedText(*input.DisplayName, "/display_name", 1, 120)...)
	}
	if input.PhoneNumbers != nil {
		fields = append(fields, validateProfilePhones(*input.PhoneNumbers)...)
	}
	if input.Addresses != nil {
		fields = append(fields, validateProfileAddresses(*input.Addresses)...)
	}
	if input.PrimaryCategory != nil && !profileEnumContains(contract.ContactCategories, *input.PrimaryCategory) {
		fields = append(fields, contract.FieldError{Field: "/primary_category", Code: contract.ValidationInvalidValue})
	}
	if input.PreferredLanguage != nil && !profileEnumContains(contract.Languages, *input.PreferredLanguage) {
		fields = append(fields, contract.FieldError{Field: "/preferred_language", Code: contract.ValidationInvalidValue})
	}
	if len(fields) > 0 {
		return nil, validationError(fields...)
	}
	patch, err := mapFrom(input)
	if err == nil && input.DisplayName != nil {
		patch["display_name"] = strings.TrimSpace(*input.DisplayName)
	}
	return patch, err
}

func profileEnumContains[T ~string](values []T, value T) bool {
	for _, candidate := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func validateProfilePhones(phones []contract.PhoneNumber) []contract.FieldError {
	var fields []contract.FieldError
	if phones == nil || len(phones) > 3 {
		return []contract.FieldError{{Field: "/phone_numbers", Code: contract.ValidationInvalidValue}}
	}
	for index, phone := range phones {
		path := fmt.Sprintf("/phone_numbers/%d", index)
		if !profileEnumContains(contract.PhoneLabels, phone.Label) {
			fields = append(fields, contract.FieldError{Field: path + "/label", Code: contract.ValidationInvalidValue})
		}
		if !e164Pattern.MatchString(phone.Value) {
			fields = append(fields, contract.FieldError{Field: path + "/value", Code: contract.ValidationInvalidFormat})
		}
	}
	return fields
}

func validateProfileAddresses(addresses []contract.Address) []contract.FieldError {
	var fields []contract.FieldError
	if addresses == nil || len(addresses) > 5 {
		return []contract.FieldError{{Field: "/addresses", Code: contract.ValidationInvalidValue}}
	}
	primary := 0
	for index, address := range addresses {
		path := fmt.Sprintf("/addresses/%d", index)
		for _, field := range []struct {
			name, value string
			min, max    int
		}{
			{"line1", address.Line1, 1, 200}, {"line2", address.Line2, 0, 200},
			{"city", address.City, 1, 120}, {"region", address.Region, 1, 120},
			{"postal_code", address.PostalCode, 1, 32}, {"country", address.Country, 2, 2},
		} {
			invalid := validateBoundedText(field.value, path+"/"+field.name, field.min, field.max)
			// Address values are stored verbatim; outer whitespace must not
			// allow the persisted response to exceed the schema's length limit.
			if len(invalid) == 0 && utf8.RuneCountInString(field.value) > field.max {
				invalid = []contract.FieldError{{Field: path + "/" + field.name, Code: contract.ValidationTooLong}}
			}
			fields = append(fields, invalid...)
		}
		if address.Primary {
			primary++
		}
	}
	if primary > 1 {
		fields = append(fields, contract.FieldError{Field: "/addresses", Code: contract.ValidationInvalidValue})
	}
	return fields
}
