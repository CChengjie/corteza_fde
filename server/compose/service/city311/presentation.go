package city311

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
)

const (
	configurationBranding = "BRANDING"
	configurationContent  = "CONTENT"
	configurationHelp     = "HELP"
	brandingResourceKey   = "default"
	presentationListSort  = "-version"
)

var publicContentKeys = []string{"HOME", "SERVICE_CATALOGUE", "HELP", "FOOTER", "TERMS"}

var initialHelpContent = map[string]string{
	"public.request.submit":     "Describe the issue, choose its type, and provide the location where City service is needed.",
	"public.request.lookup":     "Enter the request number and the email used when the request was submitted.",
	"staff.request.triage":      "Confirm the service type, owning department, location, priority, and possible duplicate group.",
	"staff.request.reassign":    "Choose an authorised primary assignee and explain why responsibility is changing.",
	"staff.request.bulk-update": "Review the selected requests and confirm that the same change should apply to every eligible record.",
	"admin.workflow.author":     "Choose a trigger, add conditions and actions, test the workflow, then activate it.",
	"staff.report.create":       "Choose a report, apply filters, grouping, and sorting, then save or export the result.",
	"admin.branding.publish":    "Preview mobile, tablet, and desktop views before publishing the new brand version.",
}

var initialPublicContent = map[string]string{
	"HOME":              "<p>Welcome to City 311.</p>",
	"SERVICE_CATALOGUE": "<p>Browse City services and choose the service that best matches your request.</p>",
	"HELP":              "<p>Use City 311 to submit a request, check its status, or contact City staff for help.</p>",
	"FOOTER":            "<p>City services, one place.</p>",
	"TERMS":             "<p>Use this service only for non-emergency City service requests.</p>",
}

type PresentationListQuery struct {
	PageSize  uint
	PageToken string
}

func (svc *Service) seedPresentation(ctx context.Context, tx store.Storer, benchmarkNow time.Time) error {
	branding := contract.Branding{
		OrganisationName: "City 311", LogoURL: stringPointer("/assets/city-logo.svg"), FaviconURL: stringPointer("/assets/favicon.ico"),
		PortalWallpaperURL: stringPointer("/assets/portal-wallpaper.jpg"), LoginHeader: "City services, one place",
		PublicHeader: "City 311", PublicFooter: "City services", PrimaryColour: "#005EA8", AccentColour: "#FFB81C", FontFamily: "System Sans",
	}
	payload, _ := mapFrom(branding)
	if err := svc.ensureInitialRevision(ctx, tx, configurationBranding, brandingResourceKey, "", payload, benchmarkNow); err != nil {
		return err
	}
	for _, key := range publicContentKeys {
		if err := svc.ensureInitialRevision(ctx, tx, configurationContent, key, "", composeTypes.City311JSON{"body": sanitizeMailHTML(initialPublicContent[key])}, benchmarkNow); err != nil {
			return err
		}
	}
	keys := make([]string, 0, len(initialHelpContent))
	for key := range initialHelpContent {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		body := sanitizeMailHTML("<p>" + initialHelpContent[key] + "</p>")
		if err := svc.ensureInitialRevision(ctx, tx, configurationHelp, key, string(contract.LanguageEN), composeTypes.City311JSON{"body": body}, benchmarkNow); err != nil {
			return err
		}
	}
	return nil
}

func (svc *Service) ensureInitialRevision(ctx context.Context, tx store.Storer, resourceType, resourceKey, language string, payload composeTypes.City311JSON, createdAt time.Time) error {
	set, _, err := store.SearchCity311ConfigurationRevisions(ctx, tx, composeTypes.City311ConfigurationRevisionFilter{ResourceType: resourceType, ResourceKey: resourceKey})
	if err != nil || len(set) > 0 {
		return err
	}
	return store.CreateCity311ConfigurationRevision(ctx, tx, &composeTypes.City311ConfigurationRevision{
		ID: svc.nextID(), ResourceType: resourceType, ResourceKey: resourceKey, Language: language,
		Payload: payload, Version: 1, Published: true, CreatedAt: createdAt.UTC(),
	})
}

func (svc *Service) PublicBranding(ctx context.Context) (*contract.Branding, error) {
	revision, err := svc.latestConfigurationRevision(ctx, svc.store, configurationBranding, brandingResourceKey, "", true)
	if err != nil {
		return nil, err
	}
	return brandingFromRevision(revision), nil
}

func (svc *Service) AdminBranding(ctx context.Context, actor contract.Actor) (*contract.Branding, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	revision, err := svc.latestConfigurationRevision(ctx, svc.store, configurationBranding, brandingResourceKey, "", false)
	if err != nil {
		return nil, err
	}
	return brandingFromRevision(revision), nil
}

func (svc *Service) PreviewBranding(ctx context.Context, actor contract.Actor, input contract.BrandingWrite) (*contract.Branding, error) {
	current, err := svc.AdminBranding(ctx, actor)
	if err != nil {
		return nil, err
	}
	preview, err := mergeBranding(*current, input)
	if err != nil {
		return nil, err
	}
	preview.Version = current.Version + 1
	preview.UpdatedAt = svc.now()
	preview.Published = false
	return &preview, nil
}

func (svc *Service) UpdateBranding(ctx context.Context, actor contract.Actor, expectedVersion uint64, input contract.BrandingWrite) (*contract.Branding, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	if expectedVersion == 0 {
		return nil, expectedVersionRequired()
	}
	if !brandingWritePresent(input) {
		return nil, validationError(contract.FieldError{Field: "/", Code: contract.ValidationRequired})
	}
	svc.presentationMu.Lock()
	defer svc.presentationMu.Unlock()
	current, err := svc.latestConfigurationRevision(ctx, svc.store, configurationBranding, brandingResourceKey, "", false)
	if err != nil {
		return nil, err
	}
	if uint64(current.Version) != expectedVersion {
		return nil, versionConflict(current.Version)
	}
	next, err := mergeBranding(*brandingFromRevision(current), input)
	if err != nil {
		return nil, err
	}
	revision, err := svc.createConfigurationRevision(ctx, actor, current, configurationBranding, brandingResourceKey, "", next, false, "BRANDING_UPDATED")
	if err != nil {
		return nil, err
	}
	return brandingFromRevision(revision), nil
}

func (svc *Service) PublishBranding(ctx context.Context, actor contract.Actor, expectedVersion uint64) (*contract.Branding, error) {
	return svc.publishBrandingRevision(ctx, actor, expectedVersion, 0)
}

func (svc *Service) RollbackBranding(ctx context.Context, actor contract.Actor, expectedVersion, targetVersion uint64) (*contract.Branding, error) {
	if targetVersion == 0 {
		return nil, validationError(contract.FieldError{Field: "/target_version", Code: contract.ValidationOutOfRange})
	}
	return svc.publishBrandingRevision(ctx, actor, expectedVersion, targetVersion)
}

func (svc *Service) publishBrandingRevision(ctx context.Context, actor contract.Actor, expectedVersion, targetVersion uint64) (*contract.Branding, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	if expectedVersion == 0 {
		return nil, expectedVersionRequired()
	}
	svc.presentationMu.Lock()
	defer svc.presentationMu.Unlock()
	set, err := svc.configurationRevisions(ctx, svc.store, configurationBranding, brandingResourceKey)
	if err != nil || len(set) == 0 {
		return nil, presentationLookupError(err, "The branding configuration was not found.")
	}
	current := set[len(set)-1]
	if uint64(current.Version) != expectedVersion {
		return nil, versionConflict(current.Version)
	}
	source := current
	eventType := "BRANDING_PUBLISHED"
	if targetVersion != 0 {
		published := make(composeTypes.City311ConfigurationRevisionSet, 0, len(set))
		for _, item := range set {
			if item.Published {
				published = append(published, item)
			}
		}
		if len(published) < 2 || uint64(published[len(published)-2].Version) != targetVersion {
			return nil, validationError(contract.FieldError{Field: "/target_version", Code: contract.ValidationInvalidValue})
		}
		source = published[len(published)-2]
		eventType = "BRANDING_ROLLED_BACK"
	}
	value := *brandingFromRevision(source)
	if err = validateBranding(value); err != nil {
		return nil, err
	}
	revision, err := svc.createConfigurationRevision(ctx, actor, current, configurationBranding, brandingResourceKey, "", value, true, eventType)
	if err != nil {
		return nil, err
	}
	return brandingFromRevision(revision), nil
}

func (svc *Service) BrandingVersions(ctx context.Context, actor contract.Actor, query PresentationListQuery) (*contract.BrandingList, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	set, err := svc.configurationRevisions(ctx, svc.store, configurationBranding, brandingResourceKey)
	if err != nil {
		return nil, err
	}
	start, end, next, err := presentationPage(query, len(set), "branding")
	if err != nil {
		return nil, err
	}
	items := make([]contract.Branding, 0, end-start)
	for index := len(set) - 1 - start; index >= len(set)-end; index-- {
		items = append(items, *brandingFromRevision(set[index]))
	}
	return &contract.BrandingList{Items: items, NextPageToken: next, TotalCount: len(set), Sort: []string{presentationListSort}}, nil
}

func (svc *Service) PublicContent(ctx context.Context, contentKey string) (*contract.ContentObject, error) {
	contentKey, err := validateContentKey(contentKey)
	if err != nil {
		return nil, apiError(http.StatusNotFound, contract.ErrorNotFound, "The published content was not found.")
	}
	revision, err := svc.latestConfigurationRevision(ctx, svc.store, configurationContent, contentKey, "", true)
	if err != nil {
		return nil, presentationLookupError(err, "The published content was not found.")
	}
	return contentFromRevision(revision), nil
}

func (svc *Service) AdminContent(ctx context.Context, actor contract.Actor, contentKey string) (*contract.ContentObject, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	contentKey, err := validateContentKey(contentKey)
	if err != nil {
		return nil, err
	}
	revision, err := svc.latestConfigurationRevision(ctx, svc.store, configurationContent, contentKey, "", false)
	if err != nil {
		return nil, presentationLookupError(err, "The content object was not found.")
	}
	return contentFromRevision(revision), nil
}

func (svc *Service) ListContent(ctx context.Context, actor contract.Actor, query PresentationListQuery) (*contract.ContentList, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	items := make([]contract.ContentObject, 0, len(publicContentKeys))
	for _, key := range publicContentKeys {
		value, err := svc.AdminContent(ctx, actor, key)
		if err != nil {
			return nil, err
		}
		items = append(items, *value)
	}
	start, end, next, err := presentationPage(query, len(items), "content")
	if err != nil {
		return nil, err
	}
	return &contract.ContentList{Items: items[start:end], NextPageToken: next, TotalCount: len(items), Sort: []string{"content_key"}}, nil
}

func (svc *Service) PreviewContent(ctx context.Context, actor contract.Actor, contentKey string, input contract.ContentWrite) (*contract.ContentObject, error) {
	current, err := svc.AdminContent(ctx, actor, contentKey)
	if err != nil {
		return nil, err
	}
	body, err := validatePresentationHTML(input.Body, "/body")
	if err != nil {
		return nil, err
	}
	return &contract.ContentObject{ContentKey: current.ContentKey, Body: body, State: "DRAFT", Published: false, Version: current.Version + 1, UpdatedAt: svc.now()}, nil
}

func (svc *Service) UpdateContent(ctx context.Context, actor contract.Actor, contentKey string, expectedVersion uint64, input contract.ContentWrite) (*contract.ContentObject, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	if expectedVersion == 0 {
		return nil, expectedVersionRequired()
	}
	contentKey, err := validateContentKey(contentKey)
	if err != nil {
		return nil, err
	}
	body, err := validatePresentationHTML(input.Body, "/body")
	if err != nil {
		return nil, err
	}
	svc.presentationMu.Lock()
	defer svc.presentationMu.Unlock()
	current, err := svc.latestConfigurationRevision(ctx, svc.store, configurationContent, contentKey, "", false)
	if err != nil {
		return nil, presentationLookupError(err, "The content object was not found.")
	}
	if uint64(current.Version) != expectedVersion {
		return nil, versionConflict(current.Version)
	}
	value := contract.ContentObject{ContentKey: contentKey, Body: body, State: "DRAFT"}
	revision, err := svc.createConfigurationRevision(ctx, actor, current, configurationContent, contentKey, "", value, false, "CONTENT_UPDATED")
	if err != nil {
		return nil, err
	}
	return contentFromRevision(revision), nil
}

func (svc *Service) PublishContent(ctx context.Context, actor contract.Actor, contentKey string, expectedVersion uint64) (*contract.ContentObject, error) {
	return svc.publishContentRevision(ctx, actor, contentKey, expectedVersion, 0)
}

func (svc *Service) RollbackContent(ctx context.Context, actor contract.Actor, contentKey string, expectedVersion, targetVersion uint64) (*contract.ContentObject, error) {
	if targetVersion == 0 {
		return nil, validationError(contract.FieldError{Field: "/target_version", Code: contract.ValidationOutOfRange})
	}
	return svc.publishContentRevision(ctx, actor, contentKey, expectedVersion, targetVersion)
}

func (svc *Service) publishContentRevision(ctx context.Context, actor contract.Actor, contentKey string, expectedVersion, targetVersion uint64) (*contract.ContentObject, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	if expectedVersion == 0 {
		return nil, expectedVersionRequired()
	}
	contentKey, err := validateContentKey(contentKey)
	if err != nil {
		return nil, err
	}
	svc.presentationMu.Lock()
	defer svc.presentationMu.Unlock()
	set, err := svc.configurationRevisions(ctx, svc.store, configurationContent, contentKey)
	if err != nil || len(set) == 0 {
		return nil, presentationLookupError(err, "The content object was not found.")
	}
	current := set[len(set)-1]
	if uint64(current.Version) != expectedVersion {
		return nil, versionConflict(current.Version)
	}
	source := current
	eventType := "CONTENT_PUBLISHED"
	if targetVersion != 0 {
		source = nil
		for _, item := range set {
			if item.Published && uint64(item.Version) == targetVersion {
				source = item
				break
			}
		}
		if source == nil {
			return nil, validationError(contract.FieldError{Field: "/target_version", Code: contract.ValidationInvalidValue})
		}
		eventType = "CONTENT_ROLLED_BACK"
	}
	value := *contentFromRevision(source)
	revision, err := svc.createConfigurationRevision(ctx, actor, current, configurationContent, contentKey, "", value, true, eventType)
	if err != nil {
		return nil, err
	}
	return contentFromRevision(revision), nil
}

func (svc *Service) ContentVersions(ctx context.Context, actor contract.Actor, contentKey string, query PresentationListQuery) (*contract.ContentList, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	contentKey, err := validateContentKey(contentKey)
	if err != nil {
		return nil, err
	}
	set, err := svc.configurationRevisions(ctx, svc.store, configurationContent, contentKey)
	if err != nil {
		return nil, err
	}
	start, end, next, err := presentationPage(query, len(set), "content:"+contentKey)
	if err != nil {
		return nil, err
	}
	items := make([]contract.ContentObject, 0, end-start)
	for index := len(set) - 1 - start; index >= len(set)-end; index-- {
		items = append(items, *contentFromRevision(set[index]))
	}
	return &contract.ContentList{Items: items, NextPageToken: next, TotalCount: len(set), Sort: []string{presentationListSort}}, nil
}

func (svc *Service) PublicHelp(ctx context.Context, helpKey string, preferredLanguages []contract.Language) (*contract.HelpContent, error) {
	if err := validateHelpKey(helpKey); err != nil {
		return nil, apiError(http.StatusNotFound, contract.ErrorNotFound, "The contextual help content was not found.")
	}
	languages := append(append([]contract.Language{}, preferredLanguages...), contract.LanguageEN)
	for _, language := range languages {
		if !validLanguage(language) {
			continue
		}
		revision, err := svc.latestConfigurationRevision(ctx, svc.store, configurationHelp, helpKey, string(language), false)
		if err == nil {
			return helpFromRevision(revision), nil
		}
	}
	return nil, apiError(http.StatusNotFound, contract.ErrorNotFound, "The contextual help content was not found.")
}

func (svc *Service) UpdateHelp(ctx context.Context, actor contract.Actor, helpKey string, expectedVersion uint64, input contract.HelpWrite) (*contract.HelpContent, error) {
	if err := requirePlatformAdministrator(actor); err != nil {
		return nil, err
	}
	if expectedVersion == 0 {
		return nil, expectedVersionRequired()
	}
	if err := validateHelpKey(helpKey); err != nil {
		return nil, err
	}
	if !validLanguage(input.Language) {
		return nil, validationError(contract.FieldError{Field: "/language", Code: contract.ValidationInvalidValue})
	}
	body, err := validatePresentationHTML(input.Body, "/body")
	if err != nil {
		return nil, err
	}
	svc.presentationMu.Lock()
	defer svc.presentationMu.Unlock()
	current, err := svc.latestConfigurationRevision(ctx, svc.store, configurationHelp, helpKey, string(input.Language), false)
	if err != nil {
		current, err = svc.latestConfigurationRevision(ctx, svc.store, configurationHelp, helpKey, string(contract.LanguageEN), false)
	}
	if err != nil {
		return nil, presentationLookupError(err, "The contextual help content was not found.")
	}
	if uint64(current.Version) != expectedVersion {
		return nil, versionConflict(current.Version)
	}
	value := contract.HelpContent{HelpKey: helpKey, Language: input.Language, Body: body}
	revision, err := svc.createConfigurationRevision(ctx, actor, current, configurationHelp, helpKey, string(input.Language), value, true, "HELP_UPDATED")
	if err != nil {
		return nil, err
	}
	return helpFromRevision(revision), nil
}

func (svc *Service) createConfigurationRevision(ctx context.Context, actor contract.Actor, current *composeTypes.City311ConfigurationRevision, resourceType, resourceKey, language string, value any, published bool, eventType string) (*composeTypes.City311ConfigurationRevision, error) {
	payload, err := mapFrom(value)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"version", "updated_at", "published", "state", "content_key", "help_key", "language"} {
		delete(payload, key)
	}
	now := svc.now().UTC()
	revision := &composeTypes.City311ConfigurationRevision{
		ID: svc.nextID(), ResourceType: resourceType, ResourceKey: resourceKey, Language: language,
		Payload: payload, Version: current.Version + 1, Published: published, CreatedAt: now,
	}
	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		if err := store.CreateCity311ConfigurationRevision(ctx, tx, revision); err != nil {
			return err
		}
		before := cloneMap(current.Payload)
		after := cloneMap(revision.Payload)
		after["version"] = revision.Version
		after["published"] = revision.Published
		return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
			ID: svc.nextID(), EntityType: strings.ToLower(resourceType), EntityID: resourceKey, EventType: eventType,
			ActorType: contract.AuditActorStaff, ActorID: actor.ID, SourceChannel: contract.SourceChannelStaffInPerson,
			Before: before, After: after, CreatedAt: now,
		})
	})
	return revision, err
}

func (svc *Service) configurationRevisions(ctx context.Context, st store.Storer, resourceType, resourceKey string) (composeTypes.City311ConfigurationRevisionSet, error) {
	set, _, err := store.SearchCity311ConfigurationRevisions(ctx, st, composeTypes.City311ConfigurationRevisionFilter{ResourceType: resourceType, ResourceKey: resourceKey})
	if err != nil {
		return nil, err
	}
	matching := make(composeTypes.City311ConfigurationRevisionSet, 0, len(set))
	for _, item := range set {
		if item.ResourceType == resourceType && item.ResourceKey == resourceKey {
			matching = append(matching, item)
		}
	}
	sort.Slice(matching, func(i, j int) bool { return matching[i].Version < matching[j].Version })
	return matching, nil
}

func (svc *Service) latestConfigurationRevision(ctx context.Context, st store.Storer, resourceType, resourceKey, language string, publishedOnly bool) (*composeTypes.City311ConfigurationRevision, error) {
	set, err := svc.configurationRevisions(ctx, st, resourceType, resourceKey)
	if err != nil {
		return nil, err
	}
	for index := len(set) - 1; index >= 0; index-- {
		if language != "" && set[index].Language != language {
			continue
		}
		if publishedOnly && !set[index].Published {
			continue
		}
		return set[index], nil
	}
	return nil, apiError(http.StatusNotFound, contract.ErrorNotFound, "The configuration revision was not found.")
}

func brandingFromRevision(revision *composeTypes.City311ConfigurationRevision) *contract.Branding {
	value := &contract.Branding{}
	decodeConfigurationPayload(revision.Payload, value)
	value.Published = revision.Published
	value.Version = uint64(revision.Version)
	value.UpdatedAt = revision.CreatedAt
	return value
}

func contentFromRevision(revision *composeTypes.City311ConfigurationRevision) *contract.ContentObject {
	value := &contract.ContentObject{ContentKey: revision.ResourceKey}
	decodeConfigurationPayload(revision.Payload, value)
	value.Published = revision.Published
	value.State = "DRAFT"
	if revision.Published {
		value.State = "PUBLISHED"
	}
	value.Version = uint64(revision.Version)
	value.UpdatedAt = revision.CreatedAt
	return value
}

func helpFromRevision(revision *composeTypes.City311ConfigurationRevision) *contract.HelpContent {
	value := &contract.HelpContent{HelpKey: revision.ResourceKey, Language: contract.Language(revision.Language)}
	decodeConfigurationPayload(revision.Payload, value)
	value.Version = uint64(revision.Version)
	value.UpdatedAt = revision.CreatedAt
	return value
}

func decodeConfigurationPayload(payload composeTypes.City311JSON, target any) {
	encoded, _ := json.Marshal(payload)
	_ = json.Unmarshal(encoded, target)
}

func mergeBranding(current contract.Branding, input contract.BrandingWrite) (contract.Branding, error) {
	if input.OrganisationName != nil {
		current.OrganisationName = strings.TrimSpace(*input.OrganisationName)
	}
	if input.LogoURL != nil {
		current.LogoURL = trimmedStringPointer(*input.LogoURL)
	}
	if input.FaviconURL != nil {
		current.FaviconURL = trimmedStringPointer(*input.FaviconURL)
	}
	if input.PortalWallpaperURL != nil {
		current.PortalWallpaperURL = trimmedStringPointer(*input.PortalWallpaperURL)
	}
	if input.LoginHeader != nil {
		current.LoginHeader = strings.TrimSpace(*input.LoginHeader)
	}
	if input.PublicHeader != nil {
		current.PublicHeader = strings.TrimSpace(*input.PublicHeader)
	}
	if input.PublicFooter != nil {
		current.PublicFooter = strings.TrimSpace(*input.PublicFooter)
	}
	if input.PrimaryColour != nil {
		current.PrimaryColour = strings.ToUpper(strings.TrimSpace(*input.PrimaryColour))
	}
	if input.AccentColour != nil {
		current.AccentColour = strings.ToUpper(strings.TrimSpace(*input.AccentColour))
	}
	if input.FontFamily != nil {
		current.FontFamily = strings.TrimSpace(*input.FontFamily)
	}
	current.Published = false
	if err := validateBranding(current); err != nil {
		return contract.Branding{}, err
	}
	return current, nil
}

func validateBranding(value contract.Branding) error {
	fields := []contract.FieldError{}
	if size := utf8.RuneCountInString(value.OrganisationName); size < 1 || size > 120 {
		fields = append(fields, contract.FieldError{Field: "/organisation_name", Code: contract.ValidationInvalidValue})
	}
	for field, text := range map[string]string{"/login_header": value.LoginHeader, "/public_header": value.PublicHeader, "/public_footer": value.PublicFooter} {
		if utf8.RuneCountInString(text) > 500 {
			fields = append(fields, contract.FieldError{Field: field, Code: contract.ValidationInvalidValue})
		}
	}
	for field, raw := range map[string]*string{"/logo_url": value.LogoURL, "/favicon_url": value.FaviconURL, "/portal_wallpaper_url": value.PortalWallpaperURL} {
		if raw != nil && !validPresentationURL(*raw) {
			fields = append(fields, contract.FieldError{Field: field, Code: contract.ValidationInvalidFormat})
		}
	}
	primary, primaryOK := parseHexColour(value.PrimaryColour)
	accent, accentOK := parseHexColour(value.AccentColour)
	if !primaryOK || contrastRatio(primary, [3]float64{1, 1, 1}) < 4.5 {
		fields = append(fields, contract.FieldError{Field: "/primary_colour", Code: contract.ValidationInvalidValue})
	}
	if !accentOK || (primaryOK && contrastRatio(primary, accent) < 3) {
		fields = append(fields, contract.FieldError{Field: "/accent_colour", Code: contract.ValidationInvalidValue})
	}
	fonts := map[string]bool{"System Sans": true, "Inter": true, "Source Sans 3": true, "Arial": true}
	if !fonts[value.FontFamily] {
		fields = append(fields, contract.FieldError{Field: "/font_family", Code: contract.ValidationInvalidValue})
	}
	if len(fields) > 0 {
		return validationError(fields...)
	}
	return nil
}

func parseHexColour(value string) ([3]float64, bool) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "#")
	if len(value) == 3 {
		value = string([]byte{value[0], value[0], value[1], value[1], value[2], value[2]})
	}
	if len(value) != 6 {
		return [3]float64{}, false
	}
	parsed, err := strconv.ParseUint(value, 16, 24)
	if err != nil {
		return [3]float64{}, false
	}
	return [3]float64{float64(parsed>>16) / 255, float64((parsed>>8)&255) / 255, float64(parsed&255) / 255}, true
}

func contrastRatio(left, right [3]float64) float64 {
	luminance := func(colour [3]float64) float64 {
		for index, component := range colour {
			if component <= 0.04045 {
				colour[index] = component / 12.92
			} else {
				colour[index] = math.Pow((component+0.055)/1.055, 2.4)
			}
		}
		return 0.2126*colour[0] + 0.7152*colour[1] + 0.0722*colour[2]
	}
	first, second := luminance(left), luminance(right)
	if first < second {
		first, second = second, first
	}
	return (first + 0.05) / (second + 0.05)
}

func validPresentationURL(value string) bool {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.User != nil || parsed.Fragment != "" {
		return false
	}
	if parsed.IsAbs() {
		return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
	}
	return strings.HasPrefix(parsed.Path, "/") && parsed.RawQuery == ""
}

func validatePresentationHTML(value, field string) (string, error) {
	sanitized := sanitizeMailHTML(value)
	if strings.TrimSpace(sanitized) == "" || utf8.RuneCountInString(sanitized) > 20000 {
		return "", validationError(contract.FieldError{Field: field, Code: contract.ValidationInvalidValue})
	}
	return sanitized, nil
}

func validateContentKey(value string) (string, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	for _, candidate := range publicContentKeys {
		if value == candidate {
			return value, nil
		}
	}
	return "", validationError(contract.FieldError{Field: "/path/content_key", Code: contract.ValidationInvalidValue})
}

func validateHelpKey(value string) error {
	if _, ok := initialHelpContent[strings.TrimSpace(value)]; !ok {
		return validationError(contract.FieldError{Field: "/path/help_key", Code: contract.ValidationInvalidValue})
	}
	return nil
}

func validLanguage(value contract.Language) bool {
	for _, candidate := range contract.Languages {
		if candidate == value {
			return true
		}
	}
	return false
}

func presentationPage(query PresentationListQuery, total int, binding string) (int, int, *string, error) {
	if query.PageSize == 0 {
		query.PageSize = 50
	}
	if query.PageSize > 100 {
		return 0, 0, nil, validationError(contract.FieldError{Field: "/query/page_size", Code: contract.ValidationOutOfRange})
	}
	offset, err := decodePageToken(query.PageToken, []string{binding})
	if err != nil || offset > total {
		return 0, 0, nil, invalidPageToken()
	}
	end := offset + int(query.PageSize)
	if end > total {
		end = total
	}
	var next *string
	if end < total {
		token, err := encodePageToken(end, []string{binding})
		if err != nil {
			return 0, 0, nil, err
		}
		next = &token
	}
	return offset, end, next, nil
}

func presentationLookupError(err error, message string) error {
	if err == nil {
		return apiError(http.StatusNotFound, contract.ErrorNotFound, message)
	}
	if typed, ok := err.(*ServiceError); ok && typed.Payload.Error == contract.ErrorNotFound {
		return apiError(http.StatusNotFound, contract.ErrorNotFound, message)
	}
	return err
}

func requirePlatformAdministrator(actor contract.Actor) error {
	if !hasRole(actor, contract.ApplicationRolePlatformAdministrator) {
		return apiError(http.StatusForbidden, contract.ErrorForbidden, "A platform administrator role is required.")
	}
	return nil
}

func brandingWritePresent(input contract.BrandingWrite) bool {
	return input.OrganisationName != nil || input.LogoURL != nil || input.FaviconURL != nil || input.PortalWallpaperURL != nil ||
		input.LoginHeader != nil || input.PublicHeader != nil || input.PublicFooter != nil || input.PrimaryColour != nil || input.AccentColour != nil || input.FontFamily != nil
}

func stringPointer(value string) *string { return &value }

func trimmedStringPointer(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}
