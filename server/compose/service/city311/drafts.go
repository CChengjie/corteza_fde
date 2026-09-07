package city311

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/errors"
	"github.com/cortezaproject/corteza/server/store"
)

func (svc *Service) CreateDraft(ctx context.Context, ownerID uint64, input contract.PortalDraftWrite) (*contract.ServiceRequest, error) {
	if ownerID == 0 {
		return nil, apiError(401, contract.ErrorUnauthenticated, "Authentication is required.")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()

	var result *contract.ServiceRequest
	err := store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		constituent, err := svc.lookupDraftConstituent(ctx, tx, ownerID)
		if err != nil {
			return err
		}
		now := svc.now()
		requestID := svc.nextID()
		request := &composeTypes.City311ServiceRequest{
			ID: requestID, RequestNumber: internalDraftNumber(requestID),
			SourceChannel: contract.SourceChannelPortalAuthenticated, OriginClass: contract.OriginClassExternal,
			Status: contract.ServiceRequestStatusDraft, PrimaryRequester: cloneMap(constituent.Profile),
			CustomFields: map[string]any{}, CollaboratorIDs: composeTypes.City311Uint64Set{}, Version: 1,
			CreatedAt: now, UpdatedAt: now,
		}
		request.PrimaryRequester["constituent_id"] = constituent.ConstituentID
		if err := validateDraftAttachmentTokens(input); err != nil {
			return err
		}
		applyDraftWrite(request, input)
		if err := validateDraftRecord(request); err != nil {
			return err
		}
		if request.ServiceType != "" {
			request.OwningDepartment, _ = departmentForServiceType(request.ServiceType)
		}
		if err := store.CreateCity311ServiceRequest(ctx, tx, request); err != nil {
			return err
		}
		if err := svc.persistPrimaryRelationship(ctx, tx, request, now); err != nil {
			return err
		}
		if err := persistDraftAudit(ctx, tx, svc.nextID, request.ID, ownerID, "DRAFT_CREATED", map[string]any{}, requestSnapshot(request), now); err != nil {
			return err
		}
		result = ptrServiceRequest(request)
		return nil
	})
	return result, err
}

func (svc *Service) GetDraft(ctx context.Context, ownerID, requestID uint64) (*contract.ServiceRequest, error) {
	if ownerID == 0 {
		return nil, apiError(401, contract.ErrorUnauthenticated, "Authentication is required.")
	}
	request, err := store.LookupCity311ServiceRequestByID(ctx, svc.store, requestID)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, draftNotFound()
		}
		return nil, err
	}
	if !ownedDraft(request, ownerID) {
		return nil, draftNotFound()
	}
	return ptrServiceRequest(request), nil
}

func (svc *Service) UpdateDraft(ctx context.Context, ownerID, requestID, expectedVersion uint64, input contract.PortalDraftWrite) (*contract.ServiceRequest, error) {
	if ownerID == 0 {
		return nil, apiError(401, contract.ErrorUnauthenticated, "Authentication is required.")
	}
	if expectedVersion == 0 {
		return nil, apiError(428, contract.ErrorExpectedVersionRequired, "If-Match must identify the expected draft version.")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()

	var result *contract.ServiceRequest
	err := store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		request, err := store.LookupCity311ServiceRequestByID(ctx, tx, requestID)
		if err != nil {
			if errors.IsNotFound(err) {
				return draftNotFound()
			}
			return err
		}
		if !ownedDraft(request, ownerID) {
			return draftNotFound()
		}
		if request.Version != int(expectedVersion) {
			return draftVersionConflict(request.Version)
		}
		before := requestSnapshot(request)
		if err := validateDraftAttachmentTokens(input); err != nil {
			return err
		}
		applyDraftWrite(request, input)
		if err := validateDraftRecord(request); err != nil {
			return err
		}
		if request.ServiceType != "" {
			request.OwningDepartment, _ = departmentForServiceType(request.ServiceType)
		} else {
			request.OwningDepartment = ""
		}
		request.Version++
		request.UpdatedAt = svc.now()
		if err := store.UpdateCity311ServiceRequest(ctx, tx, request); err != nil {
			return err
		}
		if err := persistDraftAudit(ctx, tx, svc.nextID, request.ID, ownerID, "DRAFT_UPDATED", before, requestSnapshot(request), request.UpdatedAt); err != nil {
			return err
		}
		result = ptrServiceRequest(request)
		return nil
	})
	return result, err
}

func (svc *Service) DeleteDraft(ctx context.Context, ownerID, requestID, expectedVersion uint64) error {
	if ownerID == 0 {
		return apiError(401, contract.ErrorUnauthenticated, "Authentication is required.")
	}
	if expectedVersion == 0 {
		return apiError(428, contract.ErrorExpectedVersionRequired, "If-Match must identify the expected draft version.")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	return store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		request, err := store.LookupCity311ServiceRequestByID(ctx, tx, requestID)
		if err != nil {
			if errors.IsNotFound(err) {
				return draftNotFound()
			}
			return err
		}
		if !ownedDraft(request, ownerID) {
			return draftNotFound()
		}
		if request.Version != int(expectedVersion) {
			return draftVersionConflict(request.Version)
		}
		if err := persistDraftAudit(ctx, tx, svc.nextID, request.ID, ownerID, "DRAFT_DELETED", requestSnapshot(request), map[string]any{}, svc.now()); err != nil {
			return err
		}
		links, _, err := store.SearchCity311RequestConstituentLinks(ctx, tx, composeTypes.City311RequestConstituentFilter{RequestID: request.ID})
		if err != nil {
			return err
		}
		if len(links) > 0 {
			if err = store.DeleteCity311RequestConstituentLink(ctx, tx, links...); err != nil {
				return err
			}
		}
		return store.DeleteCity311ServiceRequest(ctx, tx, request)
	})
}

func (svc *Service) SubmitDraft(ctx context.Context, ownerID, requestID, expectedVersion uint64) (*contract.ServiceRequestResponse, error) {
	if ownerID == 0 {
		return nil, apiError(401, contract.ErrorUnauthenticated, "Authentication is required.")
	}
	if expectedVersion == 0 {
		return nil, apiError(428, contract.ErrorExpectedVersionRequired, "If-Match must identify the expected draft version.")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()

	var result *contract.ServiceRequestResponse
	err := store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		request, err := store.LookupCity311ServiceRequestByID(ctx, tx, requestID)
		if err != nil {
			if errors.IsNotFound(err) {
				return draftNotFound()
			}
			return err
		}
		if !ownedDraft(request, ownerID) {
			return draftNotFound()
		}
		if request.Status != contract.ServiceRequestStatusDraft {
			return apiError(409, contract.ErrorVersionConflict, "The draft has already been submitted or deleted.")
		}
		if request.Version != int(expectedVersion) {
			return draftVersionConflict(request.Version)
		}
		input := draftSubmissionInput(request)
		if validationErr := validateWrite(input); validationErr != nil {
			return validationErr
		}
		before := requestSnapshot(request)
		year, number, err := allocateRequestNumber(ctx, tx, svc.now())
		if err != nil {
			return err
		}
		request.RequestNumber = fmt.Sprintf("SR-%04d-%05d", year, number)
		request.Status = contract.ServiceRequestStatusSubmitted
		request.Version++
		request.UpdatedAt = svc.now()
		if err := store.UpdateCity311ServiceRequest(ctx, tx, request); err != nil {
			return err
		}
		if err := persistDraftAudit(ctx, tx, svc.nextID, request.ID, ownerID, "DRAFT_SUBMITTED", before, requestSnapshot(request), request.UpdatedAt); err != nil {
			return err
		}
		if err := store.CreateCity311PublicHistoryItem(ctx, tx, &composeTypes.City311PublicHistoryItem{
			ID: svc.nextID(), RequestID: request.ID, Action: string(request.Status),
			ResponsibleDepartment: request.OwningDepartment, OccurredAt: request.UpdatedAt,
		}); err != nil {
			return err
		}
		result = responseFor(request)
		return nil
	})
	return result, err
}

func (svc *Service) lookupDraftConstituent(ctx context.Context, tx store.Storer, ownerID uint64) (*composeTypes.City311Constituent, error) {
	constituent, err := store.LookupCity311ConstituentByConstituentID(ctx, tx, "C-"+strconv.FormatUint(ownerID, 10))
	if errors.IsNotFound(err) {
		return nil, apiError(403, contract.ErrorForbidden, "A constituent profile is required for drafts.")
	}
	return constituent, err
}

func ownedDraft(request *composeTypes.City311ServiceRequest, ownerID uint64) bool {
	if request == nil || request.Status != contract.ServiceRequestStatusDraft {
		return false
	}
	return fmt.Sprint(request.PrimaryRequester["constituent_id"]) == "C-"+strconv.FormatUint(ownerID, 10)
}

func draftNotFound() *ServiceError {
	return apiError(404, contract.ErrorNotFound, "The draft was not found.")
}

func draftVersionConflict(current int) *ServiceError {
	version := uint64(current)
	return &ServiceError{Status: 409, Payload: contract.APIError{
		Error: contract.ErrorVersionConflict, Message: "The draft was updated by another operation.",
		Retryable: false, CurrentVersion: &version,
	}}
}

func internalDraftNumber(requestID uint64) string {
	return "D" + strconv.FormatUint(requestID, 36)
}

func ptrServiceRequest(request *composeTypes.City311ServiceRequest) *contract.ServiceRequest {
	value := toContract(request)
	return &value
}

func applyDraftWrite(request *composeTypes.City311ServiceRequest, input contract.PortalDraftWrite) {
	if input.Summary != nil {
		request.Summary = strings.TrimSpace(*input.Summary)
	}
	if input.Description != nil {
		request.Description = strings.TrimSpace(*input.Description)
	}
	if input.ServiceType != nil {
		request.ServiceType = *input.ServiceType
	}
	if input.Requester != nil {
		mergeDraftRequester(request.PrimaryRequester, *input.Requester)
	}
	if input.Location != nil {
		request.Location = locationMap(input.Location)
	}
	if input.CustomFields != nil {
		request.CustomFields = cloneMap(*input.CustomFields)
	}
}

func validateDraftAttachmentTokens(input contract.PortalDraftWrite) *ServiceError {
	if input.AttachmentTokens != nil && len(*input.AttachmentTokens) > 0 {
		return validationError(contract.FieldError{Field: "/attachment_tokens", Code: contract.ValidationInvalidValue})
	}
	return nil
}

func mergeDraftRequester(profile map[string]any, input contract.RequesterInput) {
	if displayName := strings.TrimSpace(input.DisplayName); displayName != "" {
		profile["display_name"] = displayName
	}
	if email := strings.ToLower(strings.TrimSpace(input.Email)); email != "" {
		profile["emails"] = []string{email}
	}
	if phone := strings.TrimSpace(input.Phone); phone != "" {
		profile["phone_numbers"] = []contract.PhoneNumber{{Label: contract.PhoneLabelMobile, Value: phone}}
	}
}

func validateDraftRecord(request *composeTypes.City311ServiceRequest) *ServiceError {
	var fields []contract.FieldError
	if request.Summary != "" {
		fields = append(fields, validateBoundedText(request.Summary, "/summary", 5, 160)...)
	}
	if request.Description != "" {
		fields = append(fields, validateBoundedText(request.Description, "/description", 10, 5000)...)
	}
	if request.ServiceType != "" {
		if _, ok := departmentForServiceType(request.ServiceType); !ok {
			fields = append(fields, contract.FieldError{Field: "/service_type", Code: contract.ValidationInvalidValue})
		}
	}
	requester := requesterInput(request.PrimaryRequester)
	if requester.DisplayName != "" && len([]rune(strings.TrimSpace(requester.DisplayName))) > 120 {
		fields = append(fields, contract.FieldError{Field: "/requester/display_name", Code: contract.ValidationTooLong})
	}
	if requester.Email != "" && !validEmail(strings.TrimSpace(requester.Email)) {
		fields = append(fields, contract.FieldError{Field: "/requester/email", Code: contract.ValidationInvalidFormat})
	}
	if requester.Phone != "" && !e164Pattern.MatchString(strings.TrimSpace(requester.Phone)) {
		fields = append(fields, contract.FieldError{Field: "/requester/phone", Code: contract.ValidationInvalidFormat})
	}
	if len(request.Location) > 0 {
		location := locationInputFromMap(request.Location)
		if location != nil && location.Latitude != nil && (*location.Latitude < -90 || *location.Latitude > 90) {
			fields = append(fields, contract.FieldError{Field: "/location/latitude", Code: contract.ValidationOutOfRange})
		}
		if location != nil && location.Longitude != nil && (*location.Longitude < -180 || *location.Longitude > 180) {
			fields = append(fields, contract.FieldError{Field: "/location/longitude", Code: contract.ValidationOutOfRange})
		}
	}
	if len(fields) > 0 {
		return validationError(fields...)
	}
	return nil
}

func draftSubmissionInput(request *composeTypes.City311ServiceRequest) contract.ServiceRequestCreate {
	location := locationInputFromMap(request.Location)
	return contract.ServiceRequestCreate{
		Summary: request.Summary, Description: request.Description, ServiceType: request.ServiceType,
		Requester: requesterInput(request.PrimaryRequester), Location: location, CustomFields: cloneMap(request.CustomFields),
	}
}

func locationInputFromMap(value map[string]any) *contract.LocationInput {
	if len(value) == 0 {
		return nil
	}
	stored := contract.ServiceRequestLocation{}
	if err := jsonUnmarshal(value, &stored); err != nil {
		return nil
	}
	return &contract.LocationInput{Address: stored.Address.Line1, Latitude: stored.Latitude, Longitude: stored.Longitude}
}

func persistDraftAudit(ctx context.Context, tx store.Storer, nextID func() uint64, requestID, ownerID uint64, event string, before, after map[string]any, occurredAt time.Time) error {
	return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
		ID: nextID(), RequestID: requestID, EntityType: "service_request", EntityID: strconv.FormatUint(requestID, 10),
		EventType: event, ActorType: contract.AuditActorConstituent, ActorID: ownerID,
		SourceChannel: contract.SourceChannelPortalAuthenticated, Before: before, After: after, CreatedAt: occurredAt,
	})
}

func jsonUnmarshal(value map[string]any, target any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(encoded, target)
}
