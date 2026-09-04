package city311

const SessionCookieName = "city311_session"

func completeProfileContract(document *ContractDocument) {
	for _, name := range []string{"profile_get", "profile_update"} {
		endpoint := document.Endpoints[name]
		endpoint.ResponseHeaders = map[string]string{"ETag": "success response: quoted decimal profile revision; send unchanged as If-Match on profile_update"}
		document.Endpoints[name] = endpoint
	}
	concurrency := document.Protocol["optimistic_concurrency"].(map[string]interface{})
	concurrency["applies_to"] = append(concurrency["applies_to"].([]string), "profile")
	version := uint64(2)
	document.Mocks["profile_version_conflict"] = MockContract{Endpoint: "profile_update", Role: "response", HTTPStatus: 409, Body: APIError{
		Error: ErrorVersionConflict, Message: "The profile has changed. Reload before saving.", CurrentVersion: &version,
	}}
	document.Mocks["profile_version_required"] = MockContract{Endpoint: "profile_update", Role: "response", HTTPStatus: 428, Body: APIError{
		Error: ErrorExpectedVersionRequired, Message: "A quoted current profile version is required.",
	}}
	profile := Constituent{ConstituentID: "C-1", DisplayName: "Alex Resident", LoginIdentifier: "alex.resident", Emails: []string{"alex@example.invalid"}, PhoneNumbers: []PhoneNumber{}, Addresses: []Address{}, PrimaryCategory: ContactCategoryResident, PreferredLanguage: LanguageEN}
	document.Mocks["profile_loaded"] = MockContract{Endpoint: "profile_get", Role: "response", HTTPStatus: 200, Headers: map[string]string{"ETag": `"1"`}, Body: profile}
	profile.PreferredLanguage = LanguageES
	document.Mocks["profile_saved"] = MockContract{Endpoint: "profile_update", Role: "response", HTTPStatus: 200, Headers: map[string]string{"ETag": `"2"`}, Body: profile}
}
