package city311

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/errors"
	"github.com/cortezaproject/corteza/server/store"
)

var publicRequestNumberPattern = regexp.MustCompile(`^SR-[0-9]{4}-[0-9]{5}$`)

type PortalRequestFilter struct {
	PageSize  uint
	PageToken string
	Sort      string
}

type RelationshipNotificationEvent string

const (
	RelationshipNotificationSubmitted    RelationshipNotificationEvent = "SUBMITTED"
	RelationshipNotificationStatusChange RelationshipNotificationEvent = "STATUS_CHANGED"
	RelationshipNotificationResolved     RelationshipNotificationEvent = "RESOLVED"
	RelationshipNotificationClosed       RelationshipNotificationEvent = "CLOSED"
	RelationshipNotificationReopened     RelationshipNotificationEvent = "REOPENED"
)

func (svc *Service) LinkConstituent(ctx context.Context, actor contract.Actor, requestID, expectedVersion uint64, input contract.ConstituentLink) (*contract.StaffServiceRequestDetail, error) {
	input.ConstituentID = strings.TrimSpace(input.ConstituentID)
	if err := validateConstituentLink(actor, expectedVersion, input); err != nil {
		return nil, err
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	err := store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		request, err := lookupScopedRequest(ctx, tx, actor, requestID)
		if err != nil {
			return err
		}
		if uint64(request.Version) != expectedVersion {
			return requestVersionConflict(request.Version)
		}
		constituent, err := store.LookupCity311ConstituentByConstituentID(ctx, tx, input.ConstituentID)
		if errors.IsNotFound(err) {
			return apiError(404, contract.ErrorNotFound, "The constituent was not found.")
		}
		if err != nil {
			return err
		}
		if !canReadConstituent(actor, constituent) {
			return apiError(403, contract.ErrorForbidden, "The constituent is outside the actor's record scope.")
		}
		if _, err = lookupRequestRelationship(ctx, tx, request.ID, input.ConstituentID, input.RelationshipType); err == nil {
			return validationError(contract.FieldError{Field: "/constituent_id", Code: contract.ValidationInvalidValue})
		} else if !errors.IsNotFound(err) {
			return err
		}

		now := svc.now()
		before := map[string]any{}
		if input.RelationshipType == contract.RelationshipPrimaryRequester {
			primary, err := requestRelationships(ctx, tx, request.ID, contract.RelationshipPrimaryRequester)
			if err != nil {
				return err
			}
			before["replaced_primary"] = relationshipSetMap(primary)
			if len(primary) > 0 {
				if err = store.DeleteCity311RequestConstituentLink(ctx, tx, primary...); err != nil {
					return err
				}
			}
			request.PrimaryRequester = cloneMap(constituent.Profile)
			request.PrimaryRequester["constituent_id"] = constituent.ConstituentID
		}
		link := &composeTypes.City311RequestConstituent{
			ID: svc.nextID(), RequestID: request.ID, ConstituentID: constituent.ConstituentID,
			RelationshipType: input.RelationshipType, PortalVisible: input.PortalVisible,
			NotifyStatus: input.NotifyStatus, CreatedAt: now, UpdatedAt: now,
		}
		if err = store.CreateCity311RequestConstituentLink(ctx, tx, link); err != nil {
			return err
		}
		request.Version++
		request.UpdatedAt = now
		if err = store.UpdateCity311ServiceRequest(ctx, tx, request); err != nil {
			return err
		}
		return svc.persistRelationshipAudit(ctx, tx, contract.AuditActorStaff, actor.ID, request.ID, "CONSTITUENT_LINKED", before, map[string]any{"link": relationshipMap(link)}, now)
	})
	if err != nil {
		return nil, err
	}
	return svc.Find(ctx, actor, requestID)
}

func (svc *Service) UnlinkConstituent(ctx context.Context, actor contract.Actor, requestID, expectedVersion uint64, constituentID string, input contract.ConstituentUnlink) (*contract.StaffServiceRequestDetail, error) {
	constituentID = strings.TrimSpace(constituentID)
	if !hasRole(actor, contract.ApplicationRoleServiceAgent) {
		return nil, apiError(403, contract.ErrorForbidden, "A service agent role is required.")
	}
	if expectedVersion == 0 {
		return nil, apiError(428, contract.ErrorExpectedVersionRequired, "If-Match must identify the expected record version.")
	}
	if constituentID == "" {
		return nil, validationError(contract.FieldError{Field: "/path/constituent_id", Code: contract.ValidationRequired})
	}
	if input.Reason == nil {
		return nil, validationError(contract.FieldError{Field: "/reason", Code: contract.ValidationRequired})
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	err := store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		request, err := lookupScopedRequest(ctx, tx, actor, requestID)
		if err != nil {
			return err
		}
		if uint64(request.Version) != expectedVersion {
			return requestVersionConflict(request.Version)
		}
		links, _, err := store.SearchCity311RequestConstituentLinks(ctx, tx, composeTypes.City311RequestConstituentFilter{
			RequestID: request.ID, ConstituentID: constituentID,
		})
		if err != nil {
			return err
		}
		if len(links) == 0 {
			return apiError(404, contract.ErrorNotFound, "The constituent relationship was not found.")
		}
		for _, link := range links {
			if link.RelationshipType == contract.RelationshipPrimaryRequester {
				return validationError(contract.FieldError{Field: "/path/constituent_id", Code: contract.ValidationInvalidValue})
			}
		}
		now := svc.now()
		if err = store.DeleteCity311RequestConstituentLink(ctx, tx, links...); err != nil {
			return err
		}
		request.Version++
		request.UpdatedAt = now
		if err = store.UpdateCity311ServiceRequest(ctx, tx, request); err != nil {
			return err
		}
		return svc.persistRelationshipAudit(ctx, tx, contract.AuditActorStaff, actor.ID, request.ID, "CONSTITUENT_UNLINKED",
			map[string]any{"links": relationshipSetMap(links)}, map[string]any{"reason": *input.Reason}, now)
	})
	if err != nil {
		return nil, err
	}
	return svc.Find(ctx, actor, requestID)
}

func (svc *Service) LinkAnonymousRequest(ctx context.Context, ownerID uint64, input contract.AnonymousRequestLink) (*contract.ServiceRequest, error) {
	input.RequestNumber = strings.TrimSpace(input.RequestNumber)
	input.Email = normalizeEmail(input.Email)
	if ownerID == 0 {
		return nil, apiError(401, contract.ErrorUnauthenticated, "Authentication is required.")
	}
	var fields []contract.FieldError
	if !publicRequestNumberPattern.MatchString(input.RequestNumber) {
		fields = append(fields, contract.FieldError{Field: "/request_number", Code: contract.ValidationInvalidFormat})
	}
	if !validEmail(input.Email) {
		fields = append(fields, contract.FieldError{Field: "/email", Code: contract.ValidationInvalidFormat})
	}
	if len(fields) > 0 {
		return nil, validationError(fields...)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	var result *contract.ServiceRequest
	err := store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		account, err := store.LookupCity311LocalAccountByID(ctx, tx, ownerID)
		if errors.IsNotFound(err) {
			return apiError(403, contract.ErrorForbidden, "A constituent account is required.")
		}
		if err != nil {
			return err
		}
		if normalizeEmail(account.VerifiedEmail) != input.Email {
			return anonymousLinkNotFound()
		}
		request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, tx, input.RequestNumber)
		if err != nil {
			if errors.IsNotFound(err) {
				return anonymousLinkNotFound()
			}
			return err
		}
		if request.SourceChannel != contract.SourceChannelPortalAnonymous || normalizeEmail(requesterInput(request.PrimaryRequester).Email) != input.Email {
			return anonymousLinkNotFound()
		}
		constituentID := "C-" + strconv.FormatUint(ownerID, 10)
		constituent, err := store.LookupCity311ConstituentByConstituentID(ctx, tx, constituentID)
		if errors.IsNotFound(err) {
			return apiError(403, contract.ErrorForbidden, "A constituent profile is required.")
		}
		if err != nil {
			return err
		}
		primary, err := requestRelationships(ctx, tx, request.ID, contract.RelationshipPrimaryRequester)
		if err != nil {
			return err
		}
		for _, link := range primary {
			if link.ConstituentID == constituentID && link.PortalVisible && link.NotifyStatus {
				value := toContract(request)
				result = &value
				return nil
			}
		}
		now := svc.now()
		if len(primary) > 0 {
			if err = store.DeleteCity311RequestConstituentLink(ctx, tx, primary...); err != nil {
				return err
			}
		}
		link := &composeTypes.City311RequestConstituent{
			ID: svc.nextID(), RequestID: request.ID, ConstituentID: constituentID,
			RelationshipType: contract.RelationshipPrimaryRequester, PortalVisible: true, NotifyStatus: true,
			CreatedAt: now, UpdatedAt: now,
		}
		if err = store.CreateCity311RequestConstituentLink(ctx, tx, link); err != nil {
			return err
		}
		request.PrimaryRequester = cloneMap(constituent.Profile)
		request.PrimaryRequester["constituent_id"] = constituentID
		request.Version++
		request.UpdatedAt = now
		if err = store.UpdateCity311ServiceRequest(ctx, tx, request); err != nil {
			return err
		}
		if err = svc.persistRelationshipAudit(ctx, tx, contract.AuditActorConstituent, ownerID, request.ID, "ANONYMOUS_REQUEST_LINKED",
			map[string]any{"replaced_primary": relationshipSetMap(primary)}, map[string]any{"link": relationshipMap(link)}, now); err != nil {
			return err
		}
		value := toContract(request)
		result = &value
		return nil
	})
	return result, err
}

func (svc *Service) ListPortalRequests(ctx context.Context, ownerID uint64, requested PortalRequestFilter) (*contract.PortalRequestList, error) {
	if ownerID == 0 {
		return nil, apiError(401, contract.ErrorUnauthenticated, "Authentication is required.")
	}
	if requested.PageSize == 0 {
		requested.PageSize = 50
	}
	if requested.PageSize > 100 {
		return nil, validationError(contract.FieldError{Field: "/query/page_size", Code: contract.ValidationOutOfRange})
	}
	publishedSort, err := normalizeSort(requested.Sort)
	if err != nil {
		return nil, validationError(contract.FieldError{Field: "/query/sort", Code: contract.ValidationInvalidFormat})
	}
	offset, err := decodePageToken(requested.PageToken, publishedSort)
	if err != nil {
		return nil, validationError(contract.FieldError{Field: "/query/page_token", Code: contract.ValidationInvalidFormat})
	}
	constituentID := "C-" + strconv.FormatUint(ownerID, 10)
	if _, err = store.LookupCity311ConstituentByConstituentID(ctx, svc.store, constituentID); err != nil {
		if errors.IsNotFound(err) {
			return nil, apiError(403, contract.ErrorForbidden, "A constituent profile is required.")
		}
		return nil, err
	}
	links, _, err := store.SearchCity311RequestConstituentLinks(ctx, svc.store, composeTypes.City311RequestConstituentFilter{
		ConstituentID: constituentID,
		Check: func(link *composeTypes.City311RequestConstituent) (bool, error) {
			return relationshipGrantsPortalView(link), nil
		},
	})
	if err != nil {
		return nil, err
	}
	seen := make(map[uint64]bool, len(links))
	requests := make(composeTypes.City311ServiceRequestSet, 0, len(links))
	for _, link := range links {
		if seen[link.RequestID] {
			continue
		}
		request, lookupErr := store.LookupCity311ServiceRequestByID(ctx, svc.store, link.RequestID)
		if errors.IsNotFound(lookupErr) {
			continue
		}
		if lookupErr != nil {
			return nil, lookupErr
		}
		if request.Status == contract.ServiceRequestStatusDraft {
			continue
		}
		seen[link.RequestID] = true
		requests = append(requests, request)
	}
	sortServiceRequests(requests, publishedSort)
	if offset > len(requests) {
		return nil, validationError(contract.FieldError{Field: "/query/page_token", Code: contract.ValidationInvalidFormat})
	}
	end := offset + int(requested.PageSize)
	if end > len(requests) {
		end = len(requests)
	}
	response := &contract.PortalRequestList{
		Items: make([]contract.PortalRequestSummary, 0, end-offset), TotalCount: len(requests),
		AppliedFilters: map[string]any{}, Sort: publishedSort,
	}
	for _, request := range requests[offset:end] {
		response.Items = append(response.Items, portalRequestSummary(request))
	}
	if end < len(requests) {
		next, err := encodePageToken(end, publishedSort)
		if err != nil {
			return nil, err
		}
		response.NextPageToken = &next
	}
	return response, nil
}

func (svc *Service) RelationshipNotificationRecipients(ctx context.Context, requestID uint64, event RelationshipNotificationEvent) ([]string, error) {
	return relationshipNotificationRecipients(ctx, svc.store, requestID, event)
}

func relationshipNotificationRecipients(ctx context.Context, st store.Storer, requestID uint64, event RelationshipNotificationEvent) ([]string, error) {
	if !validRelationshipNotificationEvent(event) {
		return nil, validationError(contract.FieldError{Field: "/event", Code: contract.ValidationInvalidValue})
	}
	links, _, err := store.SearchCity311RequestConstituentLinks(ctx, st, composeTypes.City311RequestConstituentFilter{RequestID: requestID})
	if err != nil {
		return nil, err
	}
	recipients := make([]string, 0, len(links))
	seen := map[string]bool{}
	for _, link := range links {
		eligible := link.RelationshipType == contract.RelationshipPrimaryRequester ||
			(event != RelationshipNotificationSubmitted &&
				(link.RelationshipType == contract.RelationshipAffectedResident || link.RelationshipType == contract.RelationshipReporter) &&
				link.PortalVisible && link.NotifyStatus)
		if eligible && !seen[link.ConstituentID] {
			seen[link.ConstituentID] = true
			recipients = append(recipients, link.ConstituentID)
		}
	}
	sort.Strings(recipients)
	return recipients, nil
}

func validateConstituentLink(actor contract.Actor, expectedVersion uint64, input contract.ConstituentLink) error {
	if !hasRole(actor, contract.ApplicationRoleServiceAgent) {
		return apiError(403, contract.ErrorForbidden, "A service agent role is required.")
	}
	if expectedVersion == 0 {
		return apiError(428, contract.ErrorExpectedVersionRequired, "If-Match must identify the expected record version.")
	}
	var fields []contract.FieldError
	if input.ConstituentID == "" {
		fields = append(fields, contract.FieldError{Field: "/constituent_id", Code: contract.ValidationRequired})
	}
	if !containsEnums([]contract.RelationshipType{input.RelationshipType}, contract.RelationshipTypes) {
		fields = append(fields, contract.FieldError{Field: "/relationship_type", Code: contract.ValidationInvalidValue})
	}
	if len(fields) > 0 {
		return validationError(fields...)
	}
	return nil
}

func lookupScopedRequest(ctx context.Context, tx store.Storer, actor contract.Actor, requestID uint64) (*composeTypes.City311ServiceRequest, error) {
	request, err := store.LookupCity311ServiceRequestByID(ctx, tx, requestID)
	if errors.IsNotFound(err) {
		return nil, apiError(404, contract.ErrorNotFound, "The service request was not found.")
	}
	if err != nil {
		return nil, err
	}
	if !canRead(actor, request) {
		return nil, apiError(403, contract.ErrorForbidden, requestScopeDeniedMessage)
	}
	return request, nil
}

func requestVersionConflict(current int) *ServiceError {
	version := uint64(current)
	return &ServiceError{Status: 409, Payload: contract.APIError{
		Error: contract.ErrorVersionConflict, Message: "The service request was updated by another operation.",
		Retryable: false, CurrentVersion: &version,
	}}
}

func requestRelationships(ctx context.Context, st store.Storer, requestID uint64, relationshipType contract.RelationshipType) (composeTypes.City311RequestConstituentSet, error) {
	set, _, err := store.SearchCity311RequestConstituentLinks(ctx, st, composeTypes.City311RequestConstituentFilter{
		RequestID: requestID, RelationshipType: string(relationshipType),
	})
	return set, err
}

func (svc *Service) persistPrimaryRelationship(ctx context.Context, st store.Storer, request *composeTypes.City311ServiceRequest, now time.Time) error {
	constituentID := strings.TrimSpace(fmt.Sprint(request.PrimaryRequester["constituent_id"]))
	if constituentID == "" || constituentID == "<nil>" {
		return validationError(contract.FieldError{Field: "/requester/constituent_id", Code: contract.ValidationRequired})
	}
	_, err := lookupRequestRelationship(ctx, st, request.ID, constituentID, contract.RelationshipPrimaryRequester)
	if err == nil {
		return nil
	}
	if !errors.IsNotFound(err) {
		return err
	}
	primary, err := requestRelationships(ctx, st, request.ID, contract.RelationshipPrimaryRequester)
	if err != nil {
		return err
	}
	if len(primary) > 0 {
		return fmt.Errorf("service request %d already has a primary constituent relationship", request.ID)
	}
	return store.CreateCity311RequestConstituentLink(ctx, st, &composeTypes.City311RequestConstituent{
		ID: svc.nextID(), RequestID: request.ID, ConstituentID: constituentID,
		RelationshipType: contract.RelationshipPrimaryRequester, PortalVisible: true, NotifyStatus: true,
		CreatedAt: now, UpdatedAt: now,
	})
}

func lookupRequestRelationship(ctx context.Context, st store.Storer, requestID uint64, constituentID string, relationshipType contract.RelationshipType) (*composeTypes.City311RequestConstituent, error) {
	set, _, err := store.SearchCity311RequestConstituentLinks(ctx, st, composeTypes.City311RequestConstituentFilter{
		RequestID: requestID, ConstituentID: constituentID, RelationshipType: string(relationshipType),
	})
	if err != nil {
		return nil, err
	}
	if len(set) == 0 {
		return nil, errors.NotFound("request constituent relationship not found")
	}
	return set[0], nil
}

func relationshipGrantsPortalView(link *composeTypes.City311RequestConstituent) bool {
	if link == nil || !link.PortalVisible {
		return false
	}
	return link.RelationshipType == contract.RelationshipPrimaryRequester ||
		link.RelationshipType == contract.RelationshipAffectedResident ||
		link.RelationshipType == contract.RelationshipReporter
}

func validRelationshipNotificationEvent(event RelationshipNotificationEvent) bool {
	switch event {
	case RelationshipNotificationSubmitted, RelationshipNotificationStatusChange, RelationshipNotificationResolved, RelationshipNotificationClosed, RelationshipNotificationReopened:
		return true
	default:
		return false
	}
}

func anonymousLinkNotFound() *ServiceError {
	return apiError(404, contract.ErrorNotFound, "No matching anonymous request was found.")
}

func portalRequestSummary(request *composeTypes.City311ServiceRequest) contract.PortalRequestSummary {
	return contract.PortalRequestSummary{
		RequestID: strconv.FormatUint(request.ID, 10), RequestNumber: request.RequestNumber,
		Summary: request.Summary, ServiceType: request.ServiceType, Status: request.Status,
		OwningDepartment: request.OwningDepartment, UpdatedAt: request.UpdatedAt,
	}
}

func relationshipMap(link *composeTypes.City311RequestConstituent) map[string]any {
	if link == nil {
		return map[string]any{}
	}
	return map[string]any{
		"constituent_id": link.ConstituentID, "relationship_type": link.RelationshipType,
		"portal_visible": link.PortalVisible, "notify_status": link.NotifyStatus,
	}
}

func relationshipSetMap(set composeTypes.City311RequestConstituentSet) []map[string]any {
	out := make([]map[string]any, 0, len(set))
	for _, link := range set {
		out = append(out, relationshipMap(link))
	}
	return out
}

func (svc *Service) persistRelationshipAudit(ctx context.Context, tx store.Storer, actorType contract.AuditActorType, actorID, requestID uint64, event string, before, after map[string]any, now time.Time) error {
	source := contract.SourceChannelStaffInPerson
	if actorType == contract.AuditActorConstituent {
		source = contract.SourceChannelPortalAuthenticated
	}
	return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
		ID: svc.nextID(), RequestID: requestID, EntityType: "service_request", EntityID: strconv.FormatUint(requestID, 10),
		EventType: event, ActorType: actorType, ActorID: actorID, SourceChannel: source,
		Before: before, After: after, CreatedAt: now,
	})
}
