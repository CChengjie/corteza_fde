package city311

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	city311Service "github.com/cortezaproject/corteza/server/compose/service/city311"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/auth"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth"
)

const (
	maximumJSONBody               = 72 << 20
	serviceRequestsRoute          = "/service-requests"
	sessionRoute                  = "/session"
	invalidFieldsMessage          = "The request contains invalid fields."
	authenticationRequiredMessage = "Authentication is required."
)

var strongVersionPattern = regexp.MustCompile(`^"[1-9][0-9]*"$`)

type handler struct {
	service  *city311Service.Service
	identity *city311Service.IdentityService
}

type identitySessionContextKey struct{}

type stringList []string

func (values *stringList) UnmarshalJSON(data []byte) error {
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		*values = list
		return nil
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*values = []string{value}
	return nil
}

type staffQueueFilters struct {
	Status         stringList `json:"status"`
	ServiceType    stringList `json:"service_type"`
	Department     stringList `json:"department"`
	District       stringList `json:"district"`
	OriginClass    stringList `json:"origin_class"`
	SourceChannel  stringList `json:"source_channel"`
	Assignee       stringList `json:"assignee"`
	Collaborator   stringList `json:"collaborator"`
	Category       stringList `json:"category"`
	CreatedFrom    string     `json:"created_from"`
	CreatedTo      string     `json:"created_to"`
	DuplicateGroup stringList `json:"duplicate_group"`
}

type constituentLinkWrite struct {
	ConstituentID    string                    `json:"constituent_id"`
	RelationshipType contract.RelationshipType `json:"relationship_type"`
	PortalVisible    *bool                     `json:"portal_visible"`
	NotifyStatus     *bool                     `json:"notify_status"`
}

type auditFilterInput struct {
	RequestID     stringList `json:"request_id"`
	EntityType    stringList `json:"entity_type"`
	EntityID      stringList `json:"entity_id"`
	EventType     stringList `json:"event_type"`
	ActorType     stringList `json:"actor_type"`
	ActorID       stringList `json:"actor_id"`
	SourceChannel stringList `json:"source_channel"`
	OccurredFrom  string     `json:"occurred_from"`
	OccurredTo    string     `json:"occurred_to"`
}

type auditExportWrite struct {
	Filters *auditFilterInput `json:"filters"`
}

func MountRoutes() func(chi.Router) {
	return MountRoutesWithServices(city311Service.Default, city311Service.DefaultIdentity)
}

func MountRoutesWithService(service *city311Service.Service) func(chi.Router) {
	return MountRoutesWithServices(service, city311Service.NewIdentity(service.Store(), city311Service.IdentityOptions{}))
}

func MountRoutesWithServices(service *city311Service.Service, identity *city311Service.IdentityService) func(chi.Router) {
	return func(r chi.Router) {
		h := &handler{service: service, identity: identity}
		r.Use(h.optionalIdentitySession)
		r.Post("/public/service-request-status", h.publicStatusLookup)
		r.Post("/accounts", h.accountRegister)
		r.Get(sessionRoute, h.sessionCurrent)
		r.Post(sessionRoute, h.sessionSignIn)
		r.Delete(sessionRoute, h.sessionSignOut)
		r.Post("/auth/password-reset/request", h.passwordResetRequest)
		r.Post("/auth/password-reset/confirm", h.passwordResetConfirm)
		r.Patch("/preferences/language", h.languageUpdate)
		r.With(requireIdentity).Get("/operations/{operation_id}", h.operationGet)
		r.With(requireIdentity).Get("/operations/{operation_id}/result", h.operationResult)
		r.Route("/account", func(r chi.Router) {
			r.Use(requireCityIdentitySession)
			r.With(requireProfileConstituent).Get("/profile", h.profileGet)
			r.With(requireProfileConstituent).Patch("/profile", h.profileUpdate)
			r.Post("/password", h.passwordChange)
			r.Post("/login-identifier", h.loginIdentifierChange)
		})
		r.With(requireScope(contract.ScopeRequestWrite)).Post(serviceRequestsRoute, h.integrationSubmit)
		r.Post("/portal/service-requests", h.portalSubmit)
		r.Post("/portal/attachments", h.attachmentUpload)
		r.With(requireIdentity).Get("/attachments/{attachment_id}", h.attachmentDownload)
		r.With(requireConstituentIdentitySession).Post("/portal/service-request-drafts", h.draftCreate)
		r.With(requireConstituentIdentitySession).Get("/portal/service-request-drafts/{request_id}", h.draftGet)
		r.With(requireConstituentIdentitySession).Patch("/portal/service-request-drafts/{request_id}", h.draftUpdate)
		r.With(requireConstituentIdentitySession).Delete("/portal/service-request-drafts/{request_id}", h.draftDelete)
		r.With(requireConstituentIdentitySession).Post("/portal/service-request-drafts/{request_id}/submit", h.draftSubmit)
		r.With(requireConstituentSession).Get("/portal/service-requests", h.portalMyRequests)
		r.With(requireConstituentSession).Post("/portal/service-requests/link", h.portalLinkAnonymousRequest)
		r.With(requireConstituentSession).Post("/portal/service-requests/{request_id}/notes", h.portalNoteCreate)
		r.With(requireConstituentSession).Post("/portal/service-requests/{request_id}/reopen", h.portalReopenRequest)
		r.Route("/staff", func(r chi.Router) {
			r.Use(requireIdentity)
			r.Get("/audit-events", h.staffAuditList)
			r.Post("/audit-events/export", h.staffAuditExport)
			r.Post(serviceRequestsRoute, h.staffSubmit)
			r.Post("/service-requests/bulk", h.staffBulk)
			r.Get(serviceRequestsRoute, h.staffList)
			r.Get("/service-requests/{request_id}", h.staffDetail)
			r.Post("/service-requests/{request_id}/transitions", h.staffTransition)
			r.Post("/service-requests/{request_id}/origin-class", h.staffOriginOverride)
			r.Post("/service-requests/{request_id}/scope-override", h.staffScopeOverride)
			r.Post("/service-requests/{request_id}/assignment", h.staffReassign)
			r.Post("/service-requests/{request_id}/duplicate-group", h.staffDuplicateGroupConfirm)
			r.Delete("/service-requests/{request_id}/duplicate-group", h.staffDuplicateGroupRemove)
			r.Put("/service-requests/{request_id}/collaborators/{staff_id}", h.staffCollaboratorAdd)
			r.Delete("/service-requests/{request_id}/collaborators/{staff_id}", h.staffCollaboratorRemove)
			r.Post("/service-requests/{request_id}/reminders", h.staffReminderCreate)
			r.Post("/reminders/{reminder_id}/{action}", h.staffReminderAction)
			r.Post("/service-requests/{request_id}/notes", h.staffNoteCreate)
			r.Post("/service-requests/{request_id}/reopen/approve", h.staffReopenApprove)
			r.Post("/service-requests/{request_id}/constituents", h.staffConstituentLink)
			r.Delete("/service-requests/{request_id}/constituents/{constituent_id}", h.staffConstituentUnlink)
		})
	}
}

func (h *handler) optionalIdentitySession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.identity == nil {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie(city311Service.IdentitySessionCookie)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		resolved, err := h.identity.Resolve(r.Context(), cookie.Value)
		if err != nil {
			writeResult(w, 0, nil, err)
			return
		}
		if resolved == nil {
			h.expireIdentityCookie(w)
			next.ServeHTTP(w, r)
			return
		}
		ctx := auth.SetIdentityToContext(r.Context(), resolved.User)
		ctx = context.WithValue(ctx, identitySessionContextKey{}, resolved)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func identitySessionFromContext(ctx context.Context) *city311Service.ResolvedSession {
	resolved, _ := ctx.Value(identitySessionContextKey{}).(*city311Service.ResolvedSession)
	return resolved
}

func requireCityIdentitySession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if identitySessionFromContext(r.Context()) == nil {
			writeJSON(w, http.StatusUnauthorized, contract.APIError{Error: contract.ErrorUnauthenticated, Message: authenticationRequiredMessage, Retryable: false})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requireConstituentSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		resolved := identitySessionFromContext(r.Context())
		if resolved == nil || resolved.User == nil {
			writeJSON(w, http.StatusUnauthorized, contract.APIError{Error: contract.ErrorUnauthenticated, Message: authenticationRequiredMessage, Retryable: false})
			return
		}
		if resolved.Actor != nil {
			for _, role := range resolved.Actor.ApplicationRoles {
				if role == contract.ApplicationRoleConstituent {
					next.ServeHTTP(w, r)
					return
				}
			}
		}
		writeJSON(w, http.StatusForbidden, contract.APIError{Error: contract.ErrorForbidden, Message: "A constituent account is required.", Retryable: false})
	})
}

func (h *handler) accountRegister(w http.ResponseWriter, r *http.Request) {
	input := contract.AccountRegistration{}
	if !decodeJSON(w, r, &input) {
		return
	}
	response, err := h.identity.Register(r.Context(), input)
	writeResult(w, http.StatusAccepted, response, err)
}

func (h *handler) sessionCurrent(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.identity.Session(identitySessionFromContext(r.Context())))
}

func (h *handler) sessionSignIn(w http.ResponseWriter, r *http.Request) {
	input := contract.LocalSignIn{}
	if !decodeJSON(w, r, &input) {
		return
	}
	token, resolved, err := h.identity.SignIn(r.Context(), input)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	h.setIdentityCookie(w, token)
	writeJSON(w, http.StatusOK, h.identity.Session(resolved))
}

func (h *handler) sessionSignOut(w http.ResponseWriter, r *http.Request) {
	resolved := identitySessionFromContext(r.Context())
	if resolved == nil {
		writeJSON(w, http.StatusUnauthorized, contract.APIError{Error: contract.ErrorUnauthenticated, Message: authenticationRequiredMessage, Retryable: false})
		return
	}
	if err := h.identity.SignOut(r.Context(), resolved); err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	h.expireIdentityCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) passwordResetRequest(w http.ResponseWriter, r *http.Request) {
	input := contract.PasswordResetRequest{}
	if !decodeJSON(w, r, &input) {
		return
	}
	writeJSON(w, http.StatusAccepted, h.identity.RequestPasswordReset(r.Context(), input.Email))
}

func (h *handler) passwordResetConfirm(w http.ResponseWriter, r *http.Request) {
	input := contract.PasswordResetConfirm{}
	if !decodeJSON(w, r, &input) {
		return
	}
	response, err := h.identity.ConfirmPasswordReset(r.Context(), input)
	writeResult(w, http.StatusOK, response, err)
}

func (h *handler) passwordChange(w http.ResponseWriter, r *http.Request) {
	input := contract.PasswordChange{}
	if !decodeJSON(w, r, &input) {
		return
	}
	if err := h.identity.ChangePassword(r.Context(), identitySessionFromContext(r.Context()), input); err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *handler) loginIdentifierChange(w http.ResponseWriter, r *http.Request) {
	input := contract.LoginIdentifierChange{}
	if !decodeJSON(w, r, &input) {
		return
	}
	response, err := h.identity.ChangeLoginIdentifier(r.Context(), identitySessionFromContext(r.Context()), input)
	writeResult(w, http.StatusOK, response, err)
}

func (h *handler) setIdentityCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: city311Service.IdentitySessionCookie, Value: token, Path: "/",
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
}

func (h *handler) expireIdentityCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: city311Service.IdentitySessionCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode,
	})
}

func (h *handler) integrationSubmit(w http.ResponseWriter, r *http.Request) {
	input := contract.ServiceRequestCreate{}
	if !decodeJSON(w, r, &input) {
		return
	}
	response, status, err := h.service.Submit(r.Context(), input, r.Header.Get(contract.IdempotencyHeader), city311Service.SubmissionOptions{
		Operation: "service_request_create", SourceChannel: contract.SourceChannelAPI,
		ActorType: contract.AuditActorIntegrationClient, ActorID: auth.GetIdentityFromContext(r.Context()).Identity(), RequireIdempotency: true,
	})
	writeResult(w, status, response, err)
}

func (h *handler) portalSubmit(w http.ResponseWriter, r *http.Request) {
	input := contract.PortalServiceRequestSubmit{}
	if !decodeJSON(w, r, &input) {
		return
	}
	if len(input.AttachmentTokens) > 5 {
		writeValidation(w, "/attachment_tokens", contract.ValidationTooManyItems)
		return
	}
	identity := auth.GetIdentityFromContext(r.Context())
	source := contract.SourceChannelPortalAnonymous
	actorID := uint64(0)
	if identity.Valid() {
		source = contract.SourceChannelPortalAuthenticated
		actorID = identity.Identity()
	}
	response, status, err := h.service.Submit(r.Context(), contract.ServiceRequestCreate{
		Summary: input.Summary, Description: input.Description, ServiceType: input.ServiceType,
		Requester: input.Requester, Location: input.Location, CustomFields: input.CustomFields,
	}, r.Header.Get(contract.IdempotencyHeader), city311Service.SubmissionOptions{
		Operation: "portal_service_request_submit", SourceChannel: source,
		ActorType: contract.AuditActorConstituent, ActorID: actorID, RequireIdempotency: true,
		AttachmentTokens: input.AttachmentTokens,
	})
	// This endpoint publishes one success status. Replays return the original
	// representation with 201 while the integration endpoint distinguishes 200.
	if status == http.StatusOK {
		status = http.StatusCreated
	}
	writeResult(w, status, response, err)
}

func (h *handler) portalMyRequests(w http.ResponseWriter, r *http.Request) {
	pageSize := uint64(50)
	var err error
	if raw := r.URL.Query().Get("page_size"); raw != "" {
		pageSize, err = strconv.ParseUint(raw, 10, 16)
		if err != nil || pageSize == 0 {
			writeValidation(w, "/query/page_size", contract.ValidationInvalidFormat)
			return
		}
	}
	resolved := identitySessionFromContext(r.Context())
	result, err := h.service.ListPortalRequests(r.Context(), resolved.User.ID, city311Service.PortalRequestFilter{
		PageSize: uint(pageSize), PageToken: r.URL.Query().Get("page_token"), Sort: r.URL.Query().Get("sort"),
	})
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) portalLinkAnonymousRequest(w http.ResponseWriter, r *http.Request) {
	input := contract.AnonymousRequestLink{}
	if !decodeJSON(w, r, &input) {
		return
	}
	resolved := identitySessionFromContext(r.Context())
	result, err := h.service.LinkAnonymousRequest(r.Context(), resolved.User.ID, input)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) portalNoteCreate(w http.ResponseWriter, r *http.Request) {
	requestID, ok := staffRequestID(w, r)
	if !ok {
		return
	}
	input := contract.RequestNoteWrite{}
	if !decodeJSON(w, r, &input) {
		return
	}
	resolved := identitySessionFromContext(r.Context())
	result, err := h.service.CreatePortalNote(r.Context(), resolved.User.ID, requestID, input)
	writeResult(w, http.StatusCreated, result, err)
}

func (h *handler) portalReopenRequest(w http.ResponseWriter, r *http.Request) {
	requestID, ok := staffRequestID(w, r)
	if !ok {
		return
	}
	input := contract.ReopenApproval{}
	if !decodeJSON(w, r, &input) {
		return
	}
	resolved := identitySessionFromContext(r.Context())
	result, err := h.service.RequestReopen(r.Context(), resolved.User.ID, requestID, input)
	writeResult(w, http.StatusAccepted, result, err)
}

func (h *handler) staffSubmit(w http.ResponseWriter, r *http.Request) {
	input := contract.StaffServiceRequestCreate{}
	if !decodeJSON(w, r, &input) {
		return
	}
	identity := auth.GetIdentityFromContext(r.Context())
	actor, err := h.service.FindActor(r.Context(), identity.Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	if !actorCanCreate(actor) {
		writeResult(w, 0, nil, &city311Service.ServiceError{Status: 403, Payload: contract.APIError{Error: contract.ErrorForbidden, Message: "The actor cannot create staff service requests.", Retryable: false}})
		return
	}
	if len(input.Request.AttachmentTokens) > 5 {
		writeValidation(w, "/request/attachment_tokens", contract.ValidationTooManyItems)
		return
	}
	requester, existingConstituentID, err := staffRequester(input.Constituent)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	response, _, err := h.service.Submit(r.Context(), contract.ServiceRequestCreate{
		Summary: input.Request.Summary, Description: input.Request.Description, ServiceType: input.Request.ServiceType,
		Requester: requester, Location: input.Request.Location, CustomFields: input.Request.CustomFields,
	}, "", city311Service.SubmissionOptions{
		Operation: "staff_service_request_create", SourceChannel: contract.SourceChannelStaffInPerson,
		ActorType: contract.AuditActorStaff, ActorID: actor.ID, StaffActor: &actor, ExistingConstituentID: existingConstituentID,
		AttachmentTokens: input.Request.AttachmentTokens, AttachmentField: "/request/attachment_tokens",
	})
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	requestID, _ := strconv.ParseUint(response.RequestID, 10, 64)
	detail, err := h.service.Find(r.Context(), actor, requestID)
	writeResult(w, http.StatusCreated, detail, err)
}

func staffRequester(value contract.StaffConstituentInput) (contract.RequesterInput, string, error) {
	hasReference := value.ConstituentID != nil
	hasDisplayName := value.DisplayName != nil
	hasEmail := value.Email != nil
	if hasReference && (hasDisplayName || hasEmail) {
		return contract.RequesterInput{}, "", &city311Service.ServiceError{Status: 422, Payload: contract.APIError{
			Error: contract.ErrorValidation, Message: invalidFieldsMessage, Retryable: false,
			Errors: []contract.FieldError{{Field: "/constituent", Code: contract.ValidationInvalidValue}},
		}}
	}
	if hasReference {
		constituentID := strings.TrimSpace(*value.ConstituentID)
		if constituentID == "" {
			return contract.RequesterInput{}, "", &city311Service.ServiceError{Status: 422, Payload: contract.APIError{
				Error: contract.ErrorValidation, Message: invalidFieldsMessage, Retryable: false,
				Errors: []contract.FieldError{{Field: "/constituent/constituent_id", Code: contract.ValidationRequired}},
			}}
		}
		return contract.RequesterInput{}, constituentID, nil
	}
	if !hasDisplayName || !hasEmail {
		return contract.RequesterInput{}, "", &city311Service.ServiceError{Status: 422, Payload: contract.APIError{
			Error: contract.ErrorValidation, Message: invalidFieldsMessage, Retryable: false,
			Errors: []contract.FieldError{{Field: "/constituent", Code: contract.ValidationRequired}},
		}}
	}
	return contract.RequesterInput{DisplayName: *value.DisplayName, Email: *value.Email}, "", nil
}

func (h *handler) staffList(w http.ResponseWriter, r *http.Request) {
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	pageSize := uint64(50)
	if raw := r.URL.Query().Get("page_size"); raw != "" {
		pageSize, err = strconv.ParseUint(raw, 10, 16)
		if err != nil || pageSize == 0 {
			writeValidation(w, "/query/page_size", contract.ValidationInvalidFormat)
			return
		}
	}
	requestFilter, err := parseStaffQueueFilters(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	requestFilter.PageSize = uint(pageSize)
	requestFilter.PageToken = r.URL.Query().Get("page_token")
	requestFilter.Sort = r.URL.Query().Get("sort")
	result, err := h.service.List(r.Context(), actor, requestFilter)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) staffAuditList(w http.ResponseWriter, r *http.Request) {
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	filters, err := parseStaffAuditFilters(r)
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	pageSize := uint64(50)
	if raw := r.URL.Query().Get("page_size"); raw != "" {
		pageSize, err = strconv.ParseUint(raw, 10, 16)
		if err != nil || pageSize == 0 {
			writeValidation(w, "/query/page_size", contract.ValidationInvalidFormat)
			return
		}
	}
	result, err := h.service.ListAuditEvents(r.Context(), actor, contract.AuditQuery{
		Filters: filters, PageSize: uint(pageSize), PageToken: r.URL.Query().Get("page_token"), Sort: r.URL.Query().Get("sort"),
	})
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) staffAuditExport(w http.ResponseWriter, r *http.Request) {
	input := auditExportWrite{}
	if !decodeJSON(w, r, &input) {
		return
	}
	if input.Filters == nil {
		writeValidation(w, "/filters", contract.ValidationRequired)
		return
	}
	filters, err := auditFilterFromInput(*input.Filters, "/filters")
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.StartAuditExport(r.Context(), actor, contract.AuditExport{Filters: filters})
	writeResult(w, http.StatusAccepted, result, err)
}

func (h *handler) operationGet(w http.ResponseWriter, r *http.Request) {
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.GetOperation(r.Context(), actor, strings.TrimSpace(chi.URLParam(r, "operation_id")))
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) operationResult(w http.ResponseWriter, r *http.Request) {
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.DownloadOperation(r.Context(), actor, strings.TrimSpace(chi.URLParam(r, "operation_id")))
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", result.ContentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": result.Filename}))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Content)
}

func parseStaffAuditFilters(r *http.Request) (contract.AuditFilter, error) {
	input := auditFilterInput{}
	raw := r.URL.Query().Get("filters")
	if raw != "" {
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			return contract.AuditFilter{}, queueFilterError("filters", contract.ValidationInvalidFormat)
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return contract.AuditFilter{}, queueFilterError("filters", contract.ValidationInvalidFormat)
		}
	} else {
		query := r.URL.Query()
		input.RequestID = queryValues(query, "request_id")
		input.EntityType = queryValues(query, "entity_type")
		input.EntityID = queryValues(query, "entity_id")
		input.EventType = queryValues(query, "event_type")
		input.ActorType = queryValues(query, "actor_type")
		input.ActorID = queryValues(query, "actor_id")
		input.SourceChannel = queryValues(query, "source_channel")
		input.OccurredFrom = firstQueryValue(queryValues(query, "occurred_from"))
		input.OccurredTo = firstQueryValue(queryValues(query, "occurred_to"))
	}
	return auditFilterFromInput(input, "/query/filters")
}

func auditFilterFromInput(input auditFilterInput, prefix string) (contract.AuditFilter, error) {
	from, err := parseAuditTime(input.OccurredFrom, prefix+"/occurred_from")
	if err != nil {
		return contract.AuditFilter{}, err
	}
	to, err := parseAuditTime(input.OccurredTo, prefix+"/occurred_to")
	if err != nil {
		return contract.AuditFilter{}, err
	}
	return contract.AuditFilter{
		RequestIDs: trimStringList(input.RequestID), EntityTypes: trimStringList(input.EntityType), EntityIDs: trimStringList(input.EntityID),
		EventTypes: trimStringList(input.EventType), ActorTypes: convertStrings(trimStringList(input.ActorType), contract.AuditActorType("")),
		ActorIDs: trimStringList(input.ActorID), SourceChannels: convertStrings(trimStringList(input.SourceChannel), contract.SourceChannel("")),
		OccurredFrom: from, OccurredTo: to,
	}, nil
}

func parseAuditTime(value, field string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, &city311Service.ServiceError{Status: http.StatusUnprocessableEntity, Payload: contract.APIError{
			Error: contract.ErrorValidation, Message: invalidFieldsMessage, Retryable: false,
			Errors: []contract.FieldError{{Field: field, Code: contract.ValidationInvalidFormat}},
		}}
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func trimStringList(values stringList) stringList {
	out := make(stringList, 0, len(values))
	for _, value := range values {
		out = append(out, strings.TrimSpace(value))
	}
	return out
}

func parseStaffQueueFilters(r *http.Request) (city311Service.RequestFilter, error) {
	input := staffQueueFilters{}
	raw := r.URL.Query().Get("filters")
	if raw != "" {
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			return city311Service.RequestFilter{}, queueFilterError("filters", contract.ValidationInvalidFormat)
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return city311Service.RequestFilter{}, queueFilterError("filters", contract.ValidationInvalidFormat)
		}
	} else {
		// Accept OpenAPI form-style exploded objects in addition to the JSON object
		// representation used by the frozen examples and frontend mocks.
		query := r.URL.Query()
		input.Status = queryValues(query, "status")
		input.ServiceType = queryValues(query, "service_type")
		input.Department = queryValues(query, "department")
		input.District = queryValues(query, "district")
		input.OriginClass = queryValues(query, "origin_class")
		input.SourceChannel = queryValues(query, "source_channel")
		input.Assignee = queryValues(query, "assignee")
		input.Collaborator = queryValues(query, "collaborator")
		input.Category = queryValues(query, "category")
		input.CreatedFrom = firstQueryValue(queryValues(query, "created_from"))
		input.CreatedTo = firstQueryValue(queryValues(query, "created_to"))
		input.DuplicateGroup = queryValues(query, "duplicate_group")
	}

	assignees, err := parseFilterIDs(input.Assignee, "assignee")
	if err != nil {
		return city311Service.RequestFilter{}, err
	}
	collaborators, err := parseFilterIDs(input.Collaborator, "collaborator")
	if err != nil {
		return city311Service.RequestFilter{}, err
	}
	createdFrom, err := parseFilterTime(input.CreatedFrom, "created_from")
	if err != nil {
		return city311Service.RequestFilter{}, err
	}
	createdTo, err := parseFilterTime(input.CreatedTo, "created_to")
	if err != nil {
		return city311Service.RequestFilter{}, err
	}

	return city311Service.RequestFilter{
		Statuses:           convertStrings(input.Status, contract.ServiceRequestStatus("")),
		ServiceTypes:       convertStrings(input.ServiceType, contract.ServiceType("")),
		OwningDepartments:  convertStrings(input.Department, contract.DepartmentCode("")),
		CouncilDistricts:   convertStrings(input.District, contract.DistrictCode("")),
		OriginClasses:      convertStrings(input.OriginClass, contract.OriginClass("")),
		SourceChannels:     convertStrings(input.SourceChannel, contract.SourceChannel("")),
		PrimaryAssigneeIDs: assignees,
		CollaboratorIDs:    collaborators,
		Categories:         convertStrings(input.Category, contract.ContactCategory("")),
		CreatedFrom:        createdFrom,
		CreatedTo:          createdTo,
		DuplicateGroups:    []string(input.DuplicateGroup),
	}, nil
}

func queryValues(query map[string][]string, field string) stringList {
	values := query[field]
	if len(values) == 0 {
		values = query["filters["+field+"]"]
	}
	out := make(stringList, 0, len(values))
	for _, value := range values {
		for _, item := range strings.Split(value, ",") {
			if item = strings.TrimSpace(item); item != "" {
				out = append(out, item)
			}
		}
	}
	return out
}

func firstQueryValue(values stringList) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func parseFilterIDs(values stringList, field string) ([]uint64, error) {
	out := make([]uint64, 0, len(values))
	for _, value := range values {
		parsed, err := strconv.ParseUint(value, 10, 64)
		if err != nil || parsed == 0 {
			return nil, queueFilterError(field, contract.ValidationInvalidFormat)
		}
		out = append(out, parsed)
	}
	return out, nil
}

func parseFilterTime(value, field string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, queueFilterError(field, contract.ValidationInvalidFormat)
	}
	parsed = parsed.UTC()
	return &parsed, nil
}

func queueFilterError(field string, code contract.ValidationCode) *city311Service.ServiceError {
	return &city311Service.ServiceError{Status: 422, Payload: contract.APIError{
		Error: contract.ErrorValidation, Message: invalidFieldsMessage, Retryable: false,
		Errors: []contract.FieldError{{Field: "/query/filters/" + field, Code: code}},
	}}
}

func convertStrings[T ~string](values stringList, _ T) []T {
	out := make([]T, 0, len(values))
	for _, value := range values {
		out = append(out, T(value))
	}
	return out
}

func (h *handler) staffDetail(w http.ResponseWriter, r *http.Request) {
	requestID, err := strconv.ParseUint(chi.URLParam(r, "request_id"), 10, 64)
	if err != nil || requestID == 0 {
		writeValidation(w, "/path/request_id", contract.ValidationInvalidFormat)
		return
	}
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.Find(r.Context(), actor, requestID)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) staffTransition(w http.ResponseWriter, r *http.Request) {
	requestID, err := strconv.ParseUint(chi.URLParam(r, "request_id"), 10, 64)
	if err != nil || requestID == 0 {
		writeValidation(w, "/path/request_id", contract.ValidationInvalidFormat)
		return
	}
	expectedVersion, err := parseIfMatch(r.Header.Get(contract.IfMatchHeader))
	if err != nil {
		writeResult(w, 0, nil, &city311Service.ServiceError{Status: 428, Payload: contract.APIError{Error: contract.ErrorExpectedVersionRequired, Message: "If-Match must identify the expected record version.", Retryable: false}})
		return
	}
	input := contract.RequestTransition{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.Transition(r.Context(), actor, requestID, expectedVersion, input)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) staffReassign(w http.ResponseWriter, r *http.Request) {
	requestID, ok := staffRequestID(w, r)
	if !ok {
		return
	}
	expectedVersion, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	input := contract.Reassignment{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.Reassign(r.Context(), actor, requestID, expectedVersion, input)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) staffOriginOverride(w http.ResponseWriter, r *http.Request) {
	requestID, ok := staffRequestID(w, r)
	if !ok {
		return
	}
	expectedVersion, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	input := contract.OriginOverride{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.OverrideOrigin(r.Context(), actor, requestID, expectedVersion, input)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) staffScopeOverride(w http.ResponseWriter, r *http.Request) {
	requestID, ok := staffRequestID(w, r)
	if !ok {
		return
	}
	expectedVersion, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	input := contract.ScopeOverride{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.OverrideScope(r.Context(), actor, requestID, expectedVersion, input)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) staffDuplicateGroupConfirm(w http.ResponseWriter, r *http.Request) {
	requestID, ok := staffRequestID(w, r)
	if !ok {
		return
	}
	expectedVersion, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	input := contract.DuplicateGroupChange{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.ConfirmDuplicateGroup(r.Context(), actor, requestID, expectedVersion, input)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) staffDuplicateGroupRemove(w http.ResponseWriter, r *http.Request) {
	requestID, ok := staffRequestID(w, r)
	if !ok {
		return
	}
	expectedVersion, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	input := contract.Reason{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.RemoveDuplicateGroup(r.Context(), actor, requestID, expectedVersion, input)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) staffBulk(w http.ResponseWriter, r *http.Request) {
	input := contract.BulkRequest{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.Bulk(r.Context(), actor, input, r.Header.Get(contract.IdempotencyHeader))
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) staffCollaboratorAdd(w http.ResponseWriter, r *http.Request) {
	h.staffCollaboratorChange(w, r, true)
}

func (h *handler) staffCollaboratorRemove(w http.ResponseWriter, r *http.Request) {
	h.staffCollaboratorChange(w, r, false)
}

func (h *handler) staffCollaboratorChange(w http.ResponseWriter, r *http.Request, add bool) {
	requestID, ok := staffRequestID(w, r)
	if !ok {
		return
	}
	staffID, err := strconv.ParseUint(chi.URLParam(r, "staff_id"), 10, 64)
	if err != nil || staffID == 0 {
		writeValidation(w, "/path/staff_id", contract.ValidationInvalidFormat)
		return
	}
	expectedVersion, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	input := contract.Reason{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	var result *contract.StaffServiceRequestDetail
	if add {
		result, err = h.service.AddCollaborator(r.Context(), actor, requestID, expectedVersion, staffID, input)
	} else {
		result, err = h.service.RemoveCollaborator(r.Context(), actor, requestID, expectedVersion, staffID, input)
	}
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) staffReminderCreate(w http.ResponseWriter, r *http.Request) {
	requestID, ok := staffRequestID(w, r)
	if !ok {
		return
	}
	input := contract.ReminderWrite{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.CreateReminder(r.Context(), actor, requestID, input)
	writeResult(w, http.StatusCreated, result, err)
}

func (h *handler) staffReminderAction(w http.ResponseWriter, r *http.Request) {
	reminderID, err := strconv.ParseUint(chi.URLParam(r, "reminder_id"), 10, 64)
	if err != nil || reminderID == 0 {
		writeValidation(w, "/path/reminder_id", contract.ValidationInvalidFormat)
		return
	}
	action := contract.ReminderAction(strings.TrimSpace(chi.URLParam(r, "action")))
	input := contract.ReminderActionInput{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.ActionReminder(r.Context(), actor, reminderID, action, input)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) staffNoteCreate(w http.ResponseWriter, r *http.Request) {
	requestID, ok := staffRequestID(w, r)
	if !ok {
		return
	}
	input := contract.RequestNoteWrite{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.CreateStaffNote(r.Context(), actor, requestID, input)
	writeResult(w, http.StatusCreated, result, err)
}

func (h *handler) staffReopenApprove(w http.ResponseWriter, r *http.Request) {
	requestID, ok := staffRequestID(w, r)
	if !ok {
		return
	}
	expectedVersion, err := parseIfMatch(r.Header.Get(contract.IfMatchHeader))
	if err != nil {
		writeResult(w, 0, nil, &city311Service.ServiceError{Status: 428, Payload: contract.APIError{Error: contract.ErrorExpectedVersionRequired, Message: "If-Match must identify the expected record version.", Retryable: false}})
		return
	}
	input := contract.ReopenApproval{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.ApproveReopen(r.Context(), actor, requestID, expectedVersion, input)
	writeResult(w, http.StatusOK, result, err)
}

func (h *handler) staffConstituentLink(w http.ResponseWriter, r *http.Request) {
	requestID, ok := staffRequestID(w, r)
	if !ok {
		return
	}
	expectedVersion, err := parseIfMatch(r.Header.Get(contract.IfMatchHeader))
	if err != nil {
		writeResult(w, 0, nil, &city311Service.ServiceError{Status: 428, Payload: contract.APIError{Error: contract.ErrorExpectedVersionRequired, Message: "If-Match must identify the expected record version.", Retryable: false}})
		return
	}
	input := constituentLinkWrite{}
	if !decodeJSON(w, r, &input) {
		return
	}
	var missing []contract.FieldError
	if input.PortalVisible == nil {
		missing = append(missing, contract.FieldError{Field: "/portal_visible", Code: contract.ValidationRequired})
	}
	if input.NotifyStatus == nil {
		missing = append(missing, contract.FieldError{Field: "/notify_status", Code: contract.ValidationRequired})
	}
	if len(missing) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, contract.APIError{Error: contract.ErrorValidation, Message: invalidFieldsMessage, Retryable: false, Errors: missing})
		return
	}
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.LinkConstituent(r.Context(), actor, requestID, expectedVersion, contract.ConstituentLink{
		ConstituentID: input.ConstituentID, RelationshipType: input.RelationshipType,
		PortalVisible: *input.PortalVisible, NotifyStatus: *input.NotifyStatus,
	})
	writeResult(w, http.StatusCreated, result, err)
}

func (h *handler) staffConstituentUnlink(w http.ResponseWriter, r *http.Request) {
	requestID, ok := staffRequestID(w, r)
	if !ok {
		return
	}
	expectedVersion, err := parseIfMatch(r.Header.Get(contract.IfMatchHeader))
	if err != nil {
		writeResult(w, 0, nil, &city311Service.ServiceError{Status: 428, Payload: contract.APIError{Error: contract.ErrorExpectedVersionRequired, Message: "If-Match must identify the expected record version.", Retryable: false}})
		return
	}
	constituentID := strings.TrimSpace(chi.URLParam(r, "constituent_id"))
	if constituentID == "" {
		writeValidation(w, "/path/constituent_id", contract.ValidationRequired)
		return
	}
	input := contract.ConstituentUnlink{}
	if !decodeJSON(w, r, &input) {
		return
	}
	actor, err := h.service.FindActor(r.Context(), auth.GetIdentityFromContext(r.Context()).Identity())
	if err != nil {
		writeResult(w, 0, nil, err)
		return
	}
	result, err := h.service.UnlinkConstituent(r.Context(), actor, requestID, expectedVersion, constituentID, input)
	writeResult(w, http.StatusOK, result, err)
}

func staffRequestID(w http.ResponseWriter, r *http.Request) (uint64, bool) {
	requestID, err := strconv.ParseUint(chi.URLParam(r, "request_id"), 10, 64)
	if err != nil || requestID == 0 {
		writeValidation(w, "/path/request_id", contract.ValidationInvalidFormat)
		return 0, false
	}
	return requestID, true
}

func parseIfMatch(value string) (uint64, error) {
	value = strings.TrimSpace(value)
	if !strongVersionPattern.MatchString(value) {
		return 0, errors.New("If-Match must be one quoted positive decimal version")
	}
	return strconv.ParseUint(value[1:len(value)-1], 10, 64)
}

func requiredVersion(w http.ResponseWriter, r *http.Request) (uint64, bool) {
	version, err := parseIfMatch(r.Header.Get(contract.IfMatchHeader))
	if err != nil {
		writeResult(w, 0, nil, &city311Service.ServiceError{Status: http.StatusPreconditionRequired, Payload: contract.APIError{
			Error: contract.ErrorExpectedVersionRequired, Message: "If-Match must identify the expected record version.", Retryable: false,
		}})
		return 0, false
	}
	return version, true
}

func requireIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !auth.GetIdentityFromContext(r.Context()).Valid() {
			writeJSON(w, http.StatusUnauthorized, contract.APIError{Error: contract.ErrorUnauthenticated, Message: authenticationRequiredMessage, Retryable: false})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, _, err := jwtauth.FromContext(r.Context())
			if err != nil || token == nil || !auth.GetIdentityFromContext(r.Context()).Valid() {
				writeJSON(w, http.StatusUnauthorized, contract.APIError{Error: contract.ErrorUnauthenticated, Message: "A valid access token is required.", Retryable: false})
				return
			}
			if !auth.CheckJwtScope(token, scope) {
				writeJSON(w, http.StatusForbidden, contract.APIError{Error: contract.ErrorForbidden, Message: "The access token does not grant the required scope.", Retryable: false})
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maximumJSONBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeValidation(w, "/", contract.ValidationInvalidFormat)
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeValidation(w, "/", contract.ValidationInvalidFormat)
		return false
	}
	return true
}

func writeValidation(w http.ResponseWriter, field string, code contract.ValidationCode) {
	writeJSON(w, 422, contract.APIError{Error: contract.ErrorValidation, Message: invalidFieldsMessage, Retryable: false, Errors: []contract.FieldError{{Field: field, Code: code}}})
}

func writeResult(w http.ResponseWriter, status int, value any, err error) {
	if err != nil {
		var serviceErr *city311Service.ServiceError
		if errors.As(err, &serviceErr) {
			writeJSON(w, serviceErr.Status, serviceErr.Payload)
			return
		}
		writeJSON(w, http.StatusInternalServerError, contract.APIError{Error: contract.ErrorOperationFailed, Message: "The operation could not be completed.", Retryable: false})
		return
	}
	writeJSON(w, status, value)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func actorCanCreate(actor contract.Actor) bool {
	for _, role := range actor.Roles {
		switch role {
		case contract.ApplicationRoleServiceAgent, contract.ApplicationRoleSupervisor, contract.ApplicationRoleDepartmentManager, contract.ApplicationRolePlatformAdministrator:
			return true
		}
	}
	return false
}
