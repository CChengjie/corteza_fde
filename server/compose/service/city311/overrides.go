package city311

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
)

func (svc *Service) OverrideOrigin(ctx context.Context, actor contract.Actor, requestID, expectedVersion uint64, input contract.OriginOverride) (*contract.StaffServiceRequestDetail, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if err := validateOriginOverride(actor, expectedVersion, input); err != nil {
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
		before := map[string]any{"origin_class": request.OriginClass}
		request.OriginClass = input.OriginClass
		request.Version++
		request.UpdatedAt = svc.now()
		if err = store.UpdateCity311ServiceRequest(ctx, tx, request); err != nil {
			return err
		}
		return svc.persistOverrideAudit(ctx, tx, actor.ID, request, "ORIGIN_CLASS_OVERRIDDEN", before, map[string]any{
			"origin_class": input.OriginClass, "reason": input.Reason,
		})
	})
	if err != nil {
		return nil, err
	}
	return svc.Find(ctx, actor, requestID)
}

func (svc *Service) OverrideScope(ctx context.Context, actor contract.Actor, requestID, expectedVersion uint64, input contract.ScopeOverride) (*contract.StaffServiceRequestDetail, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if err := validateScopeOverride(actor, expectedVersion, input); err != nil {
		return nil, err
	}
	districts := cloneDistricts(input.DistrictCodes)

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
		before := map[string]any{
			"department_code": optionalDepartment(request.ScopeDepartment),
			"district_codes":  append([]contract.DistrictCode(nil), request.ScopeDistricts...),
		}
		department := input.DepartmentCode
		request.ScopeDepartment = &department
		request.ScopeDistricts = composeTypes.City311DistrictCodeSet(districts)
		request.Version++
		request.UpdatedAt = svc.now()
		if err = store.UpdateCity311ServiceRequest(ctx, tx, request); err != nil {
			return err
		}
		return svc.persistOverrideAudit(ctx, tx, actor.ID, request, "REQUEST_SCOPE_OVERRIDDEN", before, map[string]any{
			"department_code": input.DepartmentCode, "district_codes": districts, "reason": input.Reason,
		})
	})
	if err != nil {
		return nil, err
	}
	return svc.Find(ctx, actor, requestID)
}

func validateOriginOverride(actor contract.Actor, expectedVersion uint64, input contract.OriginOverride) error {
	if !canOverrideRequest(actor) {
		return apiError(http.StatusForbidden, contract.ErrorForbidden, "A department manager or platform administrator role is required.")
	}
	if expectedVersion == 0 {
		return apiError(http.StatusPreconditionRequired, contract.ErrorExpectedVersionRequired, "If-Match must identify the expected record version.")
	}
	var fields []contract.FieldError
	if !containsEnums([]contract.OriginClass{input.OriginClass}, contract.OriginClasses) {
		fields = append(fields, contract.FieldError{Field: "/origin_class", Code: contract.ValidationInvalidValue})
	}
	fields = append(fields, validateBoundedText(input.Reason, "/reason", 1, maximumAssignmentReasonLength)...)
	if len(fields) > 0 {
		return validationError(fields...)
	}
	return nil
}

func validateScopeOverride(actor contract.Actor, expectedVersion uint64, input contract.ScopeOverride) error {
	if !canOverrideRequest(actor) {
		return apiError(http.StatusForbidden, contract.ErrorForbidden, "A department manager or platform administrator role is required.")
	}
	if expectedVersion == 0 {
		return apiError(http.StatusPreconditionRequired, contract.ErrorExpectedVersionRequired, "If-Match must identify the expected record version.")
	}
	var fields []contract.FieldError
	if !containsEnums([]contract.DepartmentCode{input.DepartmentCode}, contract.DepartmentCodes) {
		fields = append(fields, contract.FieldError{Field: "/department_code", Code: contract.ValidationInvalidValue})
	}
	if !containsEnums(input.DistrictCodes, contract.DistrictCodes) || hasDuplicateDistrict(input.DistrictCodes) {
		fields = append(fields, contract.FieldError{Field: "/district_codes", Code: contract.ValidationInvalidValue})
	}
	fields = append(fields, validateBoundedText(input.Reason, "/reason", 1, maximumAssignmentReasonLength)...)
	if len(fields) > 0 {
		return validationError(fields...)
	}
	if !hasRole(actor, contract.ApplicationRolePlatformAdministrator) && actor.Department != input.DepartmentCode {
		return apiError(http.StatusForbidden, contract.ErrorForbidden, "A department manager may only override visibility for their own department.")
	}
	return nil
}

func canOverrideRequest(actor contract.Actor) bool {
	return hasRole(actor, contract.ApplicationRoleDepartmentManager) || hasRole(actor, contract.ApplicationRolePlatformAdministrator)
}

func hasDuplicateDistrict(districts []contract.DistrictCode) bool {
	seen := make(map[contract.DistrictCode]bool, len(districts))
	for _, district := range districts {
		if seen[district] {
			return true
		}
		seen[district] = true
	}
	return false
}

func cloneDistricts(districts []contract.DistrictCode) []contract.DistrictCode {
	return append([]contract.DistrictCode(nil), districts...)
}

func optionalDepartment(department *contract.DepartmentCode) any {
	if department == nil {
		return nil
	}
	return *department
}

func (svc *Service) persistOverrideAudit(ctx context.Context, tx store.Storer, actorID uint64, request *composeTypes.City311ServiceRequest, eventType string, before, after map[string]any) error {
	return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
		ID: svc.nextID(), RequestID: request.ID, EntityType: "service_request", EntityID: strconv.FormatUint(request.ID, 10), EventType: eventType,
		ActorType: contract.AuditActorStaff, ActorID: actorID, SourceChannel: contract.SourceChannelStaffInPerson,
		Before: before, After: after, CreatedAt: request.UpdatedAt,
	})
}
