package city311

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/errors"
	"github.com/cortezaproject/corteza/server/store"
)

const (
	maximumDuplicateGroupIDLength = 64
	maximumBulkPriorityLength     = 64
	maximumBulkStaffNoteLength    = 2000
	bulkOperationName             = "staff_request_bulk"
	duplicateCandidateWindow      = 24 * time.Hour
	duplicateCandidateRadius      = 50.0
)

type parsedBulkChanges struct {
	primaryAssigneeSet bool
	primaryAssigneeID  uint64
	prioritySet        bool
	priority           string
	statusSet          bool
	status             contract.ServiceRequestStatus
	staffNoteSet       bool
	staffNote          string
}

type preparedBulkItem struct {
	request *composeTypes.City311ServiceRequest
}

// qualifyDuplicateGroup applies the frozen same-issue criteria before a new
// request is persisted. Existing manual groups are authoritative; an eligible
// ungrouped request joins the first stable group and never merges two groups.
func (svc *Service) qualifyDuplicateGroup(ctx context.Context, tx store.Storer, request *composeTypes.City311ServiceRequest, submittedAt time.Time) error {
	latitude, longitude, ok := requestCoordinates(request)
	if !ok {
		return nil
	}
	candidates, _, err := store.SearchCity311ServiceRequests(ctx, tx, composeTypes.City311ServiceRequestFilter{
		Check: func(candidate *composeTypes.City311ServiceRequest) (bool, error) {
			if candidate.ID == request.ID || candidate.Status == contract.ServiceRequestStatusDraft || candidate.ServiceType != request.ServiceType {
				return false, nil
			}
			age := submittedAt.Sub(candidate.CreatedAt)
			if age < 0 {
				age = -age
			}
			if age > duplicateCandidateWindow {
				return false, nil
			}
			candidateLatitude, candidateLongitude, hasCoordinates := requestCoordinates(candidate)
			return hasCoordinates && distanceMetres(latitude, longitude, candidateLatitude, candidateLongitude) <= duplicateCandidateRadius, nil
		},
	})
	if err != nil || len(candidates) == 0 {
		return err
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	groupID := ""
	for _, candidate := range candidates {
		if candidate.DuplicateGroupID != "" {
			groupID = candidate.DuplicateGroupID
			break
		}
	}
	if groupID == "" {
		groupID = "DG-" + strconv.FormatUint(candidates[0].ID, 10)
	}
	request.DuplicateGroupID = groupID
	for _, candidate := range candidates {
		if candidate.DuplicateGroupID != "" {
			continue
		}
		candidate.DuplicateGroupID = groupID
		candidate.Version++
		candidate.UpdatedAt = submittedAt
		if err = store.UpdateCity311ServiceRequest(ctx, tx, candidate); err != nil {
			return err
		}
		if err = store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
			ID: svc.nextID(), RequestID: candidate.ID, EntityType: "service_request", EntityID: strconv.FormatUint(candidate.ID, 10),
			EventType: "DUPLICATE_GROUP_AUTO_QUALIFIED", ActorType: contract.AuditActorSystem,
			SourceChannel: candidate.SourceChannel, Before: map[string]any{"duplicate_group_id": nil},
			After: map[string]any{
				"duplicate_group_id": groupID, "service_type": request.ServiceType,
				"window_hours": int(duplicateCandidateWindow.Hours()), "radius_metres": int(duplicateCandidateRadius),
			},
			CreatedAt: submittedAt,
		}); err != nil {
			return err
		}
	}
	return nil
}

func requestCoordinates(request *composeTypes.City311ServiceRequest) (float64, float64, bool) {
	latitude, latitudeOK := numericCoordinate(request.Location["latitude"])
	longitude, longitudeOK := numericCoordinate(request.Location["longitude"])
	return latitude, longitude, latitudeOK && longitudeOK && latitude >= -90 && latitude <= 90 && longitude >= -180 && longitude <= 180
}

func numericCoordinate(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		parsed, err := typed.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}

func distanceMetres(latitudeA, longitudeA, latitudeB, longitudeB float64) float64 {
	const earthRadiusMetres = 6_371_000.0
	toRadians := math.Pi / 180
	latitudeDelta := (latitudeB - latitudeA) * toRadians
	longitudeDelta := (longitudeB - longitudeA) * toRadians
	a := math.Sin(latitudeDelta/2)*math.Sin(latitudeDelta/2) +
		math.Cos(latitudeA*toRadians)*math.Cos(latitudeB*toRadians)*math.Sin(longitudeDelta/2)*math.Sin(longitudeDelta/2)
	return earthRadiusMetres * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func (svc *Service) ConfirmDuplicateGroup(ctx context.Context, actor contract.Actor, requestID, expectedVersion uint64, input contract.DuplicateGroupChange) (*contract.StaffServiceRequestDetail, error) {
	groupID := strings.TrimSpace(input.DuplicateGroupID)
	reason := strings.TrimSpace(input.Reason)
	if err := validateDuplicateGroupMutation(actor, expectedVersion, groupID, reason, true); err != nil {
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
		before := map[string]any{"duplicate_group_id": optionalString(request.DuplicateGroupID)}
		request.DuplicateGroupID = groupID
		request.Version++
		request.UpdatedAt = svc.now()
		if err = store.UpdateCity311ServiceRequest(ctx, tx, request); err != nil {
			return err
		}
		return svc.persistBulkAudit(ctx, tx, actor.ID, request, "DUPLICATE_GROUP_CONFIRMED", before, map[string]any{
			"duplicate_group_id": groupID, "reason": reason,
		})
	})
	if err != nil {
		return nil, err
	}
	return svc.Find(ctx, actor, requestID)
}

func (svc *Service) RemoveDuplicateGroup(ctx context.Context, actor contract.Actor, requestID, expectedVersion uint64, input contract.Reason) (*contract.StaffServiceRequestDetail, error) {
	reason := strings.TrimSpace(input.Reason)
	if err := validateDuplicateGroupMutation(actor, expectedVersion, "", reason, false); err != nil {
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
		if request.DuplicateGroupID == "" {
			return apiError(http.StatusNotFound, contract.ErrorNotFound, "The service request is not assigned to a duplicate group.")
		}
		before := map[string]any{"duplicate_group_id": request.DuplicateGroupID}
		request.DuplicateGroupID = ""
		request.Version++
		request.UpdatedAt = svc.now()
		if err = store.UpdateCity311ServiceRequest(ctx, tx, request); err != nil {
			return err
		}
		return svc.persistBulkAudit(ctx, tx, actor.ID, request, "DUPLICATE_GROUP_REMOVED", before, map[string]any{
			"duplicate_group_id": nil, "reason": reason,
		})
	})
	if err != nil {
		return nil, err
	}
	return svc.Find(ctx, actor, requestID)
}

func validateDuplicateGroupMutation(actor contract.Actor, expectedVersion uint64, groupID, reason string, confirming bool) error {
	if !hasRole(actor, contract.ApplicationRoleSupervisor) {
		return apiError(http.StatusForbidden, contract.ErrorForbidden, "A supervisor role is required.")
	}
	if expectedVersion == 0 {
		return apiError(http.StatusPreconditionRequired, contract.ErrorExpectedVersionRequired, "If-Match must identify the expected record version.")
	}
	var fields []contract.FieldError
	if confirming {
		length := utf8.RuneCountInString(groupID)
		if length == 0 {
			fields = append(fields, contract.FieldError{Field: "/duplicate_group_id", Code: contract.ValidationRequired})
		} else if length > maximumDuplicateGroupIDLength {
			fields = append(fields, contract.FieldError{Field: "/duplicate_group_id", Code: contract.ValidationTooLong})
		}
	}
	fields = append(fields, validateBoundedText(reason, "/reason", 1, maximumAssignmentReasonLength)...)
	if len(fields) > 0 {
		return validationError(fields...)
	}
	return nil
}

func (svc *Service) Bulk(ctx context.Context, actor contract.Actor, input contract.BulkRequest, idempotencyKey string) (*contract.BulkResult, error) {
	items, changes, requestHash, err := prepareBulk(actor, input, idempotencyKey)
	if err != nil {
		return nil, err
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	result := &contract.BulkResult{}
	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		replayed, replayErr := svc.replayBulk(ctx, tx, idempotencyKey, requestHash, result)
		if replayErr != nil || replayed {
			return replayErr
		}
		prepared, prepareErr := prepareBulkRecords(ctx, tx, actor, items, input.Action, changes)
		if prepareErr != nil {
			return prepareErr
		}
		result.UpdatedRequestIDs = make([]string, 0, len(prepared))
		for _, item := range prepared {
			if applyErr := svc.applyBulkItem(ctx, tx, actor, item.request, input.Action, changes, len(prepared)); applyErr != nil {
				return withFailingRequest(applyErr, item.request.ID)
			}
			result.UpdatedRequestIDs = append(result.UpdatedRequestIDs, strconv.FormatUint(item.request.ID, 10))
		}
		result.UpdatedCount = len(result.UpdatedRequestIDs)
		return svc.persistBulkIdempotency(ctx, tx, idempotencyKey, requestHash, result)
	})
	if err != nil {
		return nil, err
	}
	svc.wakeRequestNotificationWorker()
	return result, nil
}

func prepareBulk(actor contract.Actor, input contract.BulkRequest, idempotencyKey string) ([]contract.BulkRequestItem, parsedBulkChanges, string, error) {
	if !hasRole(actor, contract.ApplicationRoleSupervisor) && !hasRole(actor, contract.ApplicationRoleDepartmentManager) {
		return nil, parsedBulkChanges{}, "", apiError(http.StatusForbidden, contract.ErrorForbidden, "A supervisor or department manager role is required.")
	}
	if err := validateIdempotencyKey(strings.TrimSpace(idempotencyKey), true); err != nil {
		return nil, parsedBulkChanges{}, "", err
	}
	var fields []contract.FieldError
	if len(input.RequestItems) == 0 {
		fields = append(fields, contract.FieldError{Field: "/request_items", Code: contract.ValidationRequired})
	}
	if input.Action != contract.BulkActionUpdate && input.Action != contract.BulkActionClose {
		fields = append(fields, contract.FieldError{Field: "/action", Code: contract.ValidationInvalidValue})
	}
	seen := map[uint64]bool{}
	for index, item := range input.RequestItems {
		requestID, parseErr := strconv.ParseUint(strings.TrimSpace(item.RequestID), 10, 64)
		if parseErr != nil || requestID == 0 {
			fields = append(fields, contract.FieldError{Field: "/request_items/" + strconv.Itoa(index) + "/request_id", Code: contract.ValidationInvalidFormat})
		} else if seen[requestID] {
			fields = append(fields, contract.FieldError{Field: "/request_items/" + strconv.Itoa(index) + "/request_id", Code: contract.ValidationDuplicate})
		}
		seen[requestID] = true
		if item.ExpectedVersion == 0 {
			fields = append(fields, contract.FieldError{Field: "/request_items/" + strconv.Itoa(index) + "/expected_version", Code: contract.ValidationOutOfRange})
		}
	}
	changeSet := contract.BulkChanges{}
	if input.Changes == nil {
		fields = append(fields, contract.FieldError{Field: "/changes", Code: contract.ValidationRequired})
	} else {
		changeSet = *input.Changes
	}
	changes, changeFields := parseBulkChanges(changeSet)
	fields = append(fields, changeFields...)
	if input.Action == contract.BulkActionUpdate && input.Changes != nil && len(changeSet) == 0 {
		fields = append(fields, contract.FieldError{Field: "/changes", Code: contract.ValidationRequired})
	}
	if input.Action == contract.BulkActionClose && changes.statusSet && changes.status != contract.ServiceRequestStatusClosed {
		fields = append(fields, contract.FieldError{Field: "/changes/status", Code: contract.ValidationInvalidValue})
	}
	if len(fields) > 0 {
		return nil, parsedBulkChanges{}, "", validationError(fields...)
	}
	requestHash, hashErr := hashJSON(input)
	if hashErr != nil {
		return nil, parsedBulkChanges{}, "", hashErr
	}
	return input.RequestItems, changes, requestHash, nil
}

func parseBulkChanges(input contract.BulkChanges) (parsedBulkChanges, []contract.FieldError) {
	out := parsedBulkChanges{}
	var fields []contract.FieldError
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		raw := input[key]
		field := "/changes/" + escapeJSONPointer(key)
		switch key {
		case "primary_assignee_id":
			out.primaryAssigneeSet = true
			if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
				continue
			}
			var value string
			if json.Unmarshal(raw, &value) != nil {
				fields = append(fields, contract.FieldError{Field: field, Code: contract.ValidationInvalidFormat})
				continue
			}
			parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
			if err != nil || parsed == 0 {
				fields = append(fields, contract.FieldError{Field: field, Code: contract.ValidationInvalidFormat})
				continue
			}
			out.primaryAssigneeID = parsed
		case "priority":
			out.prioritySet = true
			if json.Unmarshal(raw, &out.priority) != nil {
				fields = append(fields, contract.FieldError{Field: field, Code: contract.ValidationInvalidFormat})
				continue
			}
			out.priority = strings.TrimSpace(out.priority)
			length := utf8.RuneCountInString(out.priority)
			if length == 0 {
				fields = append(fields, contract.FieldError{Field: field, Code: contract.ValidationRequired})
			} else if length > maximumBulkPriorityLength {
				fields = append(fields, contract.FieldError{Field: field, Code: contract.ValidationTooLong})
			}
		case "status":
			out.statusSet = true
			if json.Unmarshal(raw, &out.status) != nil || !containsEnums([]contract.ServiceRequestStatus{out.status}, contract.ServiceRequestStatuses) {
				fields = append(fields, contract.FieldError{Field: field, Code: contract.ValidationInvalidValue})
			}
		case "staff_note":
			out.staffNoteSet = true
			if json.Unmarshal(raw, &out.staffNote) != nil {
				fields = append(fields, contract.FieldError{Field: field, Code: contract.ValidationInvalidFormat})
				continue
			}
			out.staffNote = strings.TrimSpace(out.staffNote)
			fields = append(fields, validateBoundedText(out.staffNote, field, 1, maximumBulkStaffNoteLength)...)
		default:
			fields = append(fields, contract.FieldError{Field: field, Code: contract.ValidationInvalidValue})
		}
	}
	return out, fields
}

func escapeJSONPointer(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func prepareBulkRecords(ctx context.Context, tx store.Storer, actor contract.Actor, items []contract.BulkRequestItem, action contract.BulkAction, changes parsedBulkChanges) ([]preparedBulkItem, error) {
	prepared := make([]preparedBulkItem, 0, len(items))
	var department contract.DepartmentCode
	groupID := ""
	for index, item := range items {
		requestID, _ := strconv.ParseUint(strings.TrimSpace(item.RequestID), 10, 64)
		request, err := lookupScopedRequest(ctx, tx, actor, requestID)
		if err != nil {
			return nil, withFailingRequest(err, requestID)
		}
		if uint64(request.Version) != item.ExpectedVersion {
			return nil, withFailingRequest(requestVersionConflict(request.Version), requestID)
		}
		if index == 0 {
			department, groupID = request.OwningDepartment, request.DuplicateGroupID
			if groupID == "" {
				return nil, withFailingRequest(validationError(contract.FieldError{Field: "/request_items/0/request_id", Code: contract.ValidationInvalidValue}), requestID)
			}
		} else if request.OwningDepartment != department || request.DuplicateGroupID != groupID {
			return nil, withFailingRequest(validationError(contract.FieldError{Field: "/request_items/" + strconv.Itoa(index) + "/request_id", Code: contract.ValidationConflict}), requestID)
		}
		if action == contract.BulkActionClose && request.Status != contract.ServiceRequestStatusResolved {
			return nil, withFailingRequest(invalidStatusTransition("Bulk close requires every selected service request to be RESOLVED."), requestID)
		}
		targetStatus := changes.status
		if action == contract.BulkActionClose {
			targetStatus = contract.ServiceRequestStatusClosed
		}
		if targetStatus != "" && request.Status != targetStatus {
			if targetStatus == contract.ServiceRequestStatusReopened || !transitionAllowed(request.Status, targetStatus) {
				return nil, withFailingRequest(invalidStatusTransition("The requested bulk status transition is not allowed."), requestID)
			}
		}
		if changes.primaryAssigneeSet && changes.primaryAssigneeID != 0 {
			if _, err = lookupAssignmentTarget(ctx, tx, changes.primaryAssigneeID, request); err != nil {
				return nil, withFailingRequest(err, requestID)
			}
		}
		prepared = append(prepared, preparedBulkItem{request: request})
	}
	return prepared, nil
}

func (svc *Service) applyBulkItem(ctx context.Context, tx store.Storer, actor contract.Actor, request *composeTypes.City311ServiceRequest, action contract.BulkAction, changes parsedBulkChanges, selectedCount int) error {
	now := svc.now()
	before, after := bulkAuditValues(request, action, changes, selectedCount)
	previousAssignee := request.PrimaryAssigneeID
	if changes.primaryAssigneeSet {
		request.PrimaryAssigneeID = changes.primaryAssigneeID
		request.CollaboratorIDs = removeStaffID(request.CollaboratorIDs, changes.primaryAssigneeID)
	}
	if changes.prioritySet {
		request.CustomFields = cloneMap(request.CustomFields)
		request.CustomFields["priority"] = changes.priority
	}
	previousStatus := request.Status
	if action == contract.BulkActionClose {
		request.Status = contract.ServiceRequestStatusClosed
	} else if changes.statusSet {
		request.Status = changes.status
	}
	request.Version++
	request.UpdatedAt = now
	if err := store.UpdateCity311ServiceRequest(ctx, tx, request); err != nil {
		return err
	}
	if changes.staffNoteSet {
		if err := store.CreateCity311RequestNote(ctx, tx, &composeTypes.City311RequestNote{
			ID: svc.nextID(), RequestID: request.ID, AuthorType: contract.AuditActorStaff,
			AuthorID: actor.ID, Body: changes.staffNote, PortalVisible: false, CreatedAt: now,
		}); err != nil {
			return err
		}
	}
	if changes.primaryAssigneeSet && previousAssignee != request.PrimaryAssigneeID {
		if request.PrimaryAssigneeID != 0 {
			if err := svc.persistAssignmentNotification(ctx, tx, actor.ID, request.PrimaryAssigneeID, request, "You are now the primary assignee."); err != nil {
				return err
			}
		}
		if previousAssignee != 0 {
			if err := svc.persistAssignmentNotification(ctx, tx, actor.ID, previousAssignee, request, "You are no longer the primary assignee."); err != nil {
				return err
			}
		}
	}
	if previousStatus != request.Status {
		if err := store.CreateCity311PublicHistoryItem(ctx, tx, &composeTypes.City311PublicHistoryItem{
			ID: svc.nextID(), RequestID: request.ID, Action: string(request.Status),
			ResponsibleDepartment: request.OwningDepartment, OccurredAt: now,
		}); err != nil {
			return err
		}
	}
	eventType := "SERVICE_REQUEST_BULK_UPDATED"
	if action == contract.BulkActionClose {
		eventType = "SERVICE_REQUEST_BULK_CLOSED"
	}
	if err := svc.persistBulkAudit(ctx, tx, actor.ID, request, eventType, before, after); err != nil {
		return err
	}
	if previousStatus == request.Status {
		return nil
	}
	return svc.enqueueRelationshipNotifications(ctx, tx, request, previousStatus, relationshipNotificationEvent(request.Status), actor.ID, contract.SourceChannelStaffInPerson)
}

func bulkAuditValues(request *composeTypes.City311ServiceRequest, action contract.BulkAction, changes parsedBulkChanges, selectedCount int) (map[string]any, map[string]any) {
	before := map[string]any{}
	after := map[string]any{"bulk_action": action, "selected_count": selectedCount}
	if changes.primaryAssigneeSet {
		before["primary_assignee_id"] = optionalAuditID(request.PrimaryAssigneeID)
		after["primary_assignee_id"] = optionalAuditID(changes.primaryAssigneeID)
	}
	if changes.prioritySet {
		before["priority"] = request.CustomFields["priority"]
		after["priority"] = changes.priority
	}
	if changes.statusSet || action == contract.BulkActionClose {
		before["status"] = request.Status
		target := changes.status
		if action == contract.BulkActionClose {
			target = contract.ServiceRequestStatusClosed
		}
		after["status"] = target
	}
	if changes.staffNoteSet {
		before["staff_note"] = nil
		after["staff_note"] = changes.staffNote
	}
	return before, after
}

func (svc *Service) persistBulkAudit(ctx context.Context, tx store.Storer, actorID uint64, request *composeTypes.City311ServiceRequest, eventType string, before, after map[string]any) error {
	return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
		ID: svc.nextID(), RequestID: request.ID, EntityType: "service_request", EntityID: strconv.FormatUint(request.ID, 10),
		EventType: eventType, ActorType: contract.AuditActorStaff, ActorID: actorID,
		SourceChannel: contract.SourceChannelStaffInPerson, Before: before, After: after, CreatedAt: request.UpdatedAt,
	})
}

func withFailingRequest(err error, requestID uint64) error {
	var serviceErr *ServiceError
	if errors.As(err, &serviceErr) {
		copy := *serviceErr
		copy.Payload = serviceErr.Payload
		copy.Payload.FailingRequestID = strconv.FormatUint(requestID, 10)
		return &copy
	}
	return err
}

func (svc *Service) replayBulk(ctx context.Context, tx store.Storer, key, requestHash string, result *contract.BulkResult) (bool, error) {
	keyHash := hashKey(strings.TrimSpace(key))
	existing, err := store.LookupCity311IdempotencyRecordByOperationKeyHash(ctx, tx, bulkOperationName, keyHash)
	if errors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !existing.ExpiresAt.After(svc.now()) {
		return false, store.DeleteCity311IdempotencyRecord(ctx, tx, existing)
	}
	if existing.RequestHash != requestHash {
		return false, apiError(http.StatusConflict, contract.ErrorIdempotencyConflict, "The idempotency key was already used with a different request.")
	}
	encoded, err := json.Marshal(existing.ResponseBody)
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(encoded, result)
}

func (svc *Service) persistBulkIdempotency(ctx context.Context, tx store.Storer, key, requestHash string, result *contract.BulkResult) error {
	body, err := mapFrom(result)
	if err != nil {
		return err
	}
	requestID := uint64(0)
	if len(result.UpdatedRequestIDs) > 0 {
		requestID, _ = strconv.ParseUint(result.UpdatedRequestIDs[0], 10, 64)
	}
	now := svc.now()
	return store.CreateCity311IdempotencyRecord(ctx, tx, &composeTypes.City311IdempotencyRecord{
		ID: svc.nextID(), Operation: bulkOperationName, KeyHash: hashKey(strings.TrimSpace(key)), RequestHash: requestHash,
		ResponseStatus: http.StatusOK, ResponseBody: body, RequestID: requestID,
		CreatedAt: now, ExpiresAt: now.Add(idempotencyLifetime),
	})
}
