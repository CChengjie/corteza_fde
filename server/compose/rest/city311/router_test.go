package city311

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	city311Service "github.com/cortezaproject/corteza/server/compose/service/city311"
	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/auth"
	"github.com/cortezaproject/corteza/server/pkg/id"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/cortezaproject/corteza/server/store/adapters/rdbms/drivers/sqlite"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/jwtauth"
	"github.com/lestrrat-go/jwx/jwt"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestMain(m *testing.M) {
	ctx, cancel := context.WithCancel(context.Background())
	id.Init(ctx)
	code := m.Run()
	cancel()
	os.Exit(code)
}

func testRouter(t *testing.T) (http.Handler, store.Storer, *city311Service.Service) {
	t.Helper()
	ctx := context.Background()
	dsn := fmt.Sprintf("sqlite3://file:%s?mode=memory&cache=shared", t.Name())
	st, err := sqlite.Connect(ctx, dsn)
	require.NoError(t, err)
	require.NoError(t, store.Upgrade(ctx, zap.NewNop(), st))
	svc := city311Service.New(st)
	require.NoError(t, svc.Seed(ctx, time.Date(2026, 2, 3, 15, 4, 5, 0, time.UTC)))
	router := chi.NewRouter()
	router.Route("/api/v1", MountRoutesWithService(svc))
	return router, st, svc
}

func executeJSON(t *testing.T, router http.Handler, method, path string, body any, headers map[string]string, userID uint64) *httptest.ResponseRecorder {
	t.Helper()
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	if userID != 0 {
		request = request.WithContext(auth.SetIdentityToContext(request.Context(), auth.Authenticated(userID)))
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func validPortalBody() map[string]any {
	return map[string]any{
		"summary": "Pothole on Example Street", "description": "A deep pothole blocks the eastbound traffic lane.",
		"service_type": "POTHOLE", "requester": map[string]any{"display_name": "Alex Resident", "email": "alex@example.invalid"},
		"location": map[string]any{"address": "100 Example Street, Buffalo, NY 14201", "latitude": 42.88645, "longitude": -78.87837},
	}
}

type routerIdentityNotifier struct {
	resetTokens       []string
	replacementTokens []string
	replacementEmails []string
	notices           int
}

func (notifier *routerIdentityNotifier) PasswordReset(_ context.Context, _ string, token, _ string) error {
	notifier.resetTokens = append(notifier.resetTokens, token)
	return nil
}

func (notifier *routerIdentityNotifier) EmailReplacementVerification(_ context.Context, recipient, token, _ string) error {
	notifier.replacementEmails = append(notifier.replacementEmails, recipient)
	notifier.replacementTokens = append(notifier.replacementTokens, token)
	return nil
}

func (notifier *routerIdentityNotifier) SecurityNotice(_ context.Context, _, _, _, _ string) error {
	notifier.notices++
	return nil
}

func testIdentityRouter(t *testing.T) (http.Handler, store.Storer, *routerIdentityNotifier, *city311Service.IdentityService) {
	t.Helper()
	ctx := context.Background()
	dsn := fmt.Sprintf("sqlite3://file:%s-identity?mode=memory&cache=shared", t.Name())
	st, err := sqlite.Connect(ctx, dsn)
	require.NoError(t, err)
	require.NoError(t, store.Upgrade(ctx, zap.NewNop(), st))
	svc := city311Service.New(st)
	require.NoError(t, svc.Seed(ctx, time.Date(2026, 2, 3, 15, 4, 5, 0, time.UTC)))
	notifier := &routerIdentityNotifier{}
	next := uint64(920_000_000_000_000_000)
	identity := city311Service.NewIdentity(st, city311Service.IdentityOptions{
		Secret: []byte("router-test-session-secret"), Notifier: notifier,
		Now:    func() time.Time { return time.Date(2026, 2, 3, 15, 4, 5, 0, time.UTC) },
		NextID: func() uint64 { next++; return next },
	})
	router := chi.NewRouter()
	router.Route("/api/v1", MountRoutesWithServices(svc, identity))
	return router, st, notifier, identity
}

func identityRegistrationBody(identifier, email string) map[string]any {
	return map[string]any{
		"display_name": "Alex Resident", "email": email, "login_identifier": identifier,
		"password": "StrongPassword1!", "preferred_language": "EN",
	}
}

func TestIdentityHTTPContractAndSecureCookieLifecycle(t *testing.T) {
	router, st, notifier, identity := testIdentityRouter(t)
	registration := identityRegistrationBody("alex.http", "alex-http@example.invalid")
	created := executeJSON(t, router, http.MethodPost, "/api/v1/accounts", registration, nil, 0)
	require.Equal(t, http.StatusAccepted, created.Code, created.Body.String())
	require.JSONEq(t, `{"accepted":true}`, created.Body.String())

	// Duplicate registration publishes the same privacy-preserving response.
	duplicate := identityRegistrationBody("alex.http", "different@example.invalid")
	duplicateResponse := executeJSON(t, router, http.MethodPost, "/api/v1/accounts", duplicate, nil, 0)
	require.Equal(t, created.Code, duplicateResponse.Code)
	require.JSONEq(t, created.Body.String(), duplicateResponse.Body.String())

	wrong := executeJSON(t, router, http.MethodPost, "/api/v1/session", map[string]any{
		"login_identifier": "alex.http", "password": "wrong",
	}, nil, 0)
	require.Equal(t, http.StatusUnauthorized, wrong.Code)
	require.Contains(t, wrong.Body.String(), string(contract.ErrorUnauthenticated))

	signedIn := executeJSON(t, router, http.MethodPost, "/api/v1/session", map[string]any{
		"login_identifier": "alex-http@example.invalid", "password": "StrongPassword1!",
	}, nil, 0)
	require.Equal(t, http.StatusOK, signedIn.Code, signedIn.Body.String())
	require.Contains(t, signedIn.Body.String(), `"authenticated":true`)
	cookies := signedIn.Result().Cookies()
	require.Len(t, cookies, 1)
	sessionCookie := cookies[0]
	require.Equal(t, city311Service.IdentitySessionCookie, sessionCookie.Name)
	require.Equal(t, "/", sessionCookie.Path)
	require.True(t, sessionCookie.HttpOnly)
	require.True(t, sessionCookie.Secure)
	require.Equal(t, http.SameSiteLaxMode, sessionCookie.SameSite)
	require.True(t, sessionCookie.Expires.IsZero())

	headers := map[string]string{"Cookie": sessionCookie.Name + "=" + sessionCookie.Value}
	current := executeJSON(t, router, http.MethodGet, "/api/v1/session", nil, headers, 0)
	require.Equal(t, http.StatusOK, current.Code)
	require.Contains(t, current.Body.String(), `"application_roles":["constituent"]`)
	require.Contains(t, current.Body.String(), `"preferred_language":"EN"`)

	unauthenticatedChange := executeJSON(t, router, http.MethodPost, "/api/v1/account/password", map[string]any{
		"current_password": "StrongPassword1!", "new_password": "ChangedPassword2!",
	}, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticatedChange.Code)
	wrongCurrentPassword := executeJSON(t, router, http.MethodPost, "/api/v1/account/password", map[string]any{
		"current_password": "incorrect", "new_password": "ChangedPassword2!",
	}, headers, 0)
	require.Equal(t, http.StatusUnprocessableEntity, wrongCurrentPassword.Code)
	changed := executeJSON(t, router, http.MethodPost, "/api/v1/account/password", map[string]any{
		"current_password": "StrongPassword1!", "new_password": "ChangedPassword2!",
	}, headers, 0)
	require.Equal(t, http.StatusNoContent, changed.Code, changed.Body.String())
	require.Empty(t, changed.Body.String())
	identifierChanged := executeJSON(t, router, http.MethodPost, "/api/v1/account/login-identifier", map[string]any{
		"current_password": "ChangedPassword2!", "login_identifier": "alex.http.updated",
	}, headers, 0)
	require.Equal(t, http.StatusOK, identifierChanged.Code, identifierChanged.Body.String())
	require.Contains(t, identifierChanged.Body.String(), `"authenticated":true`)

	resetUnknown := executeJSON(t, router, http.MethodPost, "/api/v1/auth/password-reset/request", map[string]any{
		"email": "unknown@example.invalid",
	}, nil, 0)
	resetExisting := executeJSON(t, router, http.MethodPost, "/api/v1/auth/password-reset/request", map[string]any{
		"email": "alex-http@example.invalid",
	}, nil, 0)
	require.Equal(t, http.StatusAccepted, resetUnknown.Code)
	require.Equal(t, resetUnknown.Body.String(), resetExisting.Body.String())
	require.NoError(t, identity.RetryPendingNotifications(context.Background()))
	require.Len(t, notifier.resetTokens, 1)
	confirmed := executeJSON(t, router, http.MethodPost, "/api/v1/auth/password-reset/confirm", map[string]any{
		"token": notifier.resetTokens[0], "password": "ResetPassword3!",
	}, nil, 0)
	require.Equal(t, http.StatusOK, confirmed.Code, confirmed.Body.String())
	require.NoError(t, identity.RetryPendingNotifications(context.Background()))
	require.Equal(t, 2, notifier.notices)

	signedOut := executeJSON(t, router, http.MethodDelete, "/api/v1/session", nil, headers, 0)
	// The reset already revoked this session, so the server does not pretend it
	// can log out an authenticated actor.
	require.Equal(t, http.StatusUnauthorized, signedOut.Code)

	account, err := store.LookupCity311LocalAccountByLoginIdentifier(context.Background(), st, "alex.http.updated")
	require.NoError(t, err)
	audits, _, err := store.SearchCity311AuditEvents(context.Background(), st, composeTypes.City311AuditEventFilter{EntityID: strconv.FormatUint(account.ID, 10)})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(audits), 3)
}

func TestIdentitySignOutDeletesServerSessionAndExpiresCookie(t *testing.T) {
	router, _, _, _ := testIdentityRouter(t)
	registration := identityRegistrationBody("alex.logout", "alex-logout@example.invalid")
	require.Equal(t, http.StatusAccepted, executeJSON(t, router, http.MethodPost, "/api/v1/accounts", registration, nil, 0).Code)
	signedIn := executeJSON(t, router, http.MethodPost, "/api/v1/session", map[string]any{
		"login_identifier": "alex.logout", "password": "StrongPassword1!",
	}, nil, 0)
	require.Equal(t, http.StatusOK, signedIn.Code)
	sessionCookie := signedIn.Result().Cookies()[0]
	headers := map[string]string{"Cookie": sessionCookie.Name + "=" + sessionCookie.Value}
	signedOut := executeJSON(t, router, http.MethodDelete, "/api/v1/session", nil, headers, 0)
	require.Equal(t, http.StatusNoContent, signedOut.Code)
	require.Empty(t, signedOut.Body.String())
	expiredCookies := signedOut.Result().Cookies()
	require.Len(t, expiredCookies, 1)
	require.Less(t, expiredCookies[0].MaxAge, 0)
	require.True(t, expiredCookies[0].HttpOnly)
	require.True(t, expiredCookies[0].Secure)

	current := executeJSON(t, router, http.MethodGet, "/api/v1/session", nil, headers, 0)
	require.Equal(t, http.StatusOK, current.Code)
	require.Contains(t, current.Body.String(), `"authenticated":false`)
}

func TestPortalSubmissionUsesSinglePublishedSuccessStatus(t *testing.T) {
	router, _, _ := testRouter(t)
	headers := map[string]string{contract.IdempotencyHeader: "portal-replay-1"}
	first := executeJSON(t, router, http.MethodPost, "/api/v1/portal/service-requests", validPortalBody(), headers, 0)
	require.Equal(t, http.StatusCreated, first.Code, first.Body.String())
	created := contract.ServiceRequestResponse{}
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &created))
	require.Equal(t, "/api/v1/service-requests/"+created.RequestID, created.Links.Self)
	replay := executeJSON(t, router, http.MethodPost, "/api/v1/portal/service-requests", validPortalBody(), headers, 0)
	require.Equal(t, http.StatusCreated, replay.Code, replay.Body.String())
	require.JSONEq(t, first.Body.String(), replay.Body.String())

	conflictBody := validPortalBody()
	conflictBody["summary"] = "A different pothole report"
	conflict := executeJSON(t, router, http.MethodPost, "/api/v1/portal/service-requests", conflictBody, headers, 0)
	require.Equal(t, http.StatusConflict, conflict.Code)
	require.Contains(t, conflict.Body.String(), string(contract.ErrorIdempotencyConflict))
}

func TestIntegrationSubmissionRequiresScopeAndReturnsReplayStatus(t *testing.T) {
	router, _, _ := testRouter(t)
	path := "/api/v1/service-requests"
	headers := map[string]string{contract.IdempotencyHeader: "integration-replay-1"}
	unauthenticated := executeJSON(t, router, http.MethodPost, path, validPortalBody(), headers, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	executeWithScope := func(scope string) *httptest.ResponseRecorder {
		encoded, err := json.Marshal(validPortalBody())
		require.NoError(t, err)
		request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(encoded))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set(contract.IdempotencyHeader, headers[contract.IdempotencyHeader])
		token := jwt.New()
		require.NoError(t, token.Set("scope", scope))
		ctx := jwtauth.NewContext(request.Context(), token, nil)
		ctx = auth.SetIdentityToContext(ctx, auth.Authenticated(77))
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request.WithContext(ctx))
		return response
	}

	forbidden := executeWithScope("profile.read")
	require.Equal(t, http.StatusForbidden, forbidden.Code)
	created := executeWithScope(contract.ScopeRequestWrite)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	createdBody := contract.ServiceRequestResponse{}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &createdBody))
	require.Equal(t, "/api/v1/service-requests/"+createdBody.RequestID, createdBody.Links.Self)
	replayed := executeWithScope(contract.ScopeRequestWrite)
	require.Equal(t, http.StatusOK, replayed.Code, replayed.Body.String())
	require.JSONEq(t, created.Body.String(), replayed.Body.String())
}

func TestSubmissionRejectsUnresolvedAttachmentTokensWithoutPersistence(t *testing.T) {
	router, st, _ := testRouter(t)
	before, _, err := store.SearchCity311ServiceRequests(context.Background(), st, composeTypes.City311ServiceRequestFilter{})
	require.NoError(t, err)
	body := validPortalBody()
	body["attachment_tokens"] = []string{"1", "2", "3", "4", "5", "6"}
	response := executeJSON(t, router, http.MethodPost, "/api/v1/portal/service-requests", body, map[string]string{contract.IdempotencyHeader: "too-many-attachments"}, 0)
	require.Equal(t, http.StatusUnprocessableEntity, response.Code)
	require.Contains(t, response.Body.String(), "/attachment_tokens")

	body = validPortalBody()
	body["attachment_tokens"] = []string{"upload-00031"}
	response = executeJSON(t, router, http.MethodPost, "/api/v1/portal/service-requests", body, map[string]string{contract.IdempotencyHeader: "unresolved-portal-attachment"}, 0)
	require.Equal(t, http.StatusUnprocessableEntity, response.Code)
	require.Contains(t, response.Body.String(), string(contract.ValidationInvalidValue))

	user, err := store.LookupUserByEmail(context.Background(), st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	staffBody := map[string]any{"request": body, "constituent": map[string]any{"constituent_id": "C-1"}}
	response = executeJSON(t, router, http.MethodPost, "/api/v1/staff/service-requests", staffBody, nil, user.ID)
	require.Equal(t, http.StatusUnprocessableEntity, response.Code)
	require.Contains(t, response.Body.String(), "/request/attachment_tokens")

	after, _, err := store.SearchCity311ServiceRequests(context.Background(), st, composeTypes.City311ServiceRequestFilter{})
	require.NoError(t, err)
	require.Len(t, after, len(before))
}

func TestStaffRoutesEnforceIdentityScopeAndVersion(t *testing.T) {
	router, st, _ := testRouter(t)
	user, err := store.LookupUserByEmail(context.Background(), st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	request, err := store.LookupCity311ServiceRequestByRequestNumber(context.Background(), st, "SR-2026-00034")
	require.NoError(t, err)

	unauthenticated := executeJSON(t, router, http.MethodGet, "/api/v1/staff/service-requests", nil, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)

	queueFilters := url.QueryEscape(`{"status":["SUBMITTED"]}`)
	queue := executeJSON(t, router, http.MethodGet, "/api/v1/staff/service-requests?filters="+queueFilters+"&page_size=10", nil, nil, user.ID)
	require.Equal(t, http.StatusOK, queue.Code, queue.Body.String())
	require.Contains(t, queue.Body.String(), "SR-2026-00034")
	require.NotContains(t, queue.Body.String(), "SR-2026-00035")

	path := fmt.Sprintf("/api/v1/staff/service-requests/%d/transitions", request.ID)
	for _, invalid := range []string{`W/"1"`, "1", `"0"`, `"1","2"`} {
		response := executeJSON(t, router, http.MethodPost, path, map[string]any{"to_status": "TRIAGED"}, map[string]string{contract.IfMatchHeader: invalid}, user.ID)
		require.Equal(t, http.StatusPreconditionRequired, response.Code, invalid+": "+response.Body.String())
		require.Contains(t, response.Body.String(), string(contract.ErrorExpectedVersionRequired))
	}
	transition := executeJSON(t, router, http.MethodPost, path, map[string]any{"to_status": "TRIAGED", "reason": "Validated and routed"}, map[string]string{contract.IfMatchHeader: `"1"`}, user.ID)
	require.Equal(t, http.StatusOK, transition.Code, transition.Body.String())
	require.Contains(t, transition.Body.String(), `"version":2`)

	stale := executeJSON(t, router, http.MethodPost, path, map[string]any{"to_status": "ASSIGNED"}, map[string]string{contract.IfMatchHeader: `"1"`}, user.ID)
	require.Equal(t, http.StatusConflict, stale.Code)
	require.Contains(t, stale.Body.String(), `"current_version":2`)

	missingVersion := executeJSON(t, router, http.MethodPost, path, map[string]any{"to_status": "ASSIGNED"}, nil, user.ID)
	require.Equal(t, http.StatusPreconditionRequired, missingVersion.Code)
}

func TestStaffCreateSupportsConstituentReferenceAndRejectsMixedUnion(t *testing.T) {
	router, st, _ := testRouter(t)
	user, err := store.LookupUserByEmail(context.Background(), st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	body := map[string]any{"request": validPortalBody(), "constituent": map[string]any{"constituent_id": "C-1"}}
	created := executeJSON(t, router, http.MethodPost, "/api/v1/staff/service-requests", body, nil, user.ID)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	require.Contains(t, created.Body.String(), "constituent1@city311.example.invalid")
	require.Contains(t, created.Body.String(), `"origin_class":"INTERNAL"`)
	detail := contract.StaffServiceRequestDetail{}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &detail))
	require.Equal(t, "C-1", detail.Request.PrimaryRequester.ConstituentID)

	newBody := map[string]any{
		"request":     validPortalBody(),
		"constituent": map[string]any{"display_name": "New Staff Constituent", "email": "new-staff@example.invalid"},
	}
	created = executeJSON(t, router, http.MethodPost, "/api/v1/staff/service-requests", newBody, nil, user.ID)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	detail = contract.StaffServiceRequestDetail{}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &detail))
	require.NotEmpty(t, detail.Request.PrimaryRequester.ConstituentID)
	persisted, err := store.LookupCity311ConstituentByConstituentID(context.Background(), st, detail.Request.PrimaryRequester.ConstituentID)
	require.NoError(t, err)
	require.Equal(t, "New Staff Constituent", persisted.Profile["display_name"])

	body["constituent"] = map[string]any{"constituent_id": "C-1", "display_name": "Mixed", "email": "mixed@example.invalid"}
	mixed := executeJSON(t, router, http.MethodPost, "/api/v1/staff/service-requests", body, nil, user.ID)
	require.Equal(t, http.StatusUnprocessableEntity, mixed.Code)
}

func TestStaffCreateAuthorizesDerivedScopeBeforePersistence(t *testing.T) {
	router, st, _ := testRouter(t)
	user, err := store.LookupUserByEmail(context.Background(), st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	before, _, err := store.SearchCity311ServiceRequests(context.Background(), st, composeTypes.City311ServiceRequestFilter{})
	require.NoError(t, err)
	request := validPortalBody()
	request["service_type"] = string(contract.ServiceTypeMissedTrash)
	body := map[string]any{"request": request, "constituent": map[string]any{"constituent_id": "C-1"}}
	response := executeJSON(t, router, http.MethodPost, "/api/v1/staff/service-requests", body, nil, user.ID)
	require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
	after, _, err := store.SearchCity311ServiceRequests(context.Background(), st, composeTypes.City311ServiceRequestFilter{})
	require.NoError(t, err)
	require.Len(t, after, len(before))
}

func TestStaffCreateEnforcesConstituentDepartmentAndDistrictScopeBeforePersistence(t *testing.T) {
	router, st, _ := testRouter(t)
	user, err := store.LookupUserByEmail(context.Background(), st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	before, _, err := store.SearchCity311ServiceRequests(context.Background(), st, composeTypes.City311ServiceRequestFilter{})
	require.NoError(t, err)
	now := time.Date(2026, 2, 3, 15, 4, 5, 0, time.UTC)
	constituents := []*composeTypes.City311Constituent{
		{
			ID: id.Next(), ConstituentID: "C-private-sanitation",
			Profile:          composeTypes.City311JSON{"constituent_id": "C-private-sanitation", "display_name": "Private Sanitation Resident", "emails": []string{"private-sanitation@example.invalid"}},
			OwningDepartment: contract.DepartmentSanitation, CouncilDistrict: contract.DistrictNorth, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: id.Next(), ConstituentID: "C-private-south",
			Profile:          composeTypes.City311JSON{"constituent_id": "C-private-south", "display_name": "Private South Resident", "emails": []string{"private-south@example.invalid"}},
			OwningDepartment: contract.DepartmentStreets, CouncilDistrict: contract.DistrictSouth, CreatedAt: now, UpdatedAt: now,
		},
	}
	require.NoError(t, store.CreateCity311Constituent(context.Background(), st, constituents...))
	for _, constituent := range constituents {
		body := map[string]any{"request": validPortalBody(), "constituent": map[string]any{"constituent_id": constituent.ConstituentID}}
		response := executeJSON(t, router, http.MethodPost, "/api/v1/staff/service-requests", body, nil, user.ID)
		require.Equal(t, http.StatusForbidden, response.Code, response.Body.String())
		require.NotContains(t, response.Body.String(), fmt.Sprint(constituent.Profile["display_name"]))
	}
	after, _, err := store.SearchCity311ServiceRequests(context.Background(), st, composeTypes.City311ServiceRequestFilter{})
	require.NoError(t, err)
	require.Len(t, after, len(before))
}

func TestStaffQueueDecodesAndEchoesCompleteFrozenFilterObject(t *testing.T) {
	router, st, _ := testRouter(t)
	user, err := store.LookupUserByEmail(context.Background(), st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	firstRequest, err := store.LookupCity311ServiceRequestByRequestNumber(context.Background(), st, "SR-2026-00034")
	require.NoError(t, err)
	secondRequest, err := store.LookupCity311ServiceRequestByRequestNumber(context.Background(), st, "SR-2026-00035")
	require.NoError(t, err)
	for _, request := range []*composeTypes.City311ServiceRequest{firstRequest, secondRequest} {
		request.PrimaryAssigneeID = user.ID
		request.CollaboratorIDs = composeTypes.City311Uint64Set{user.ID}
		request.PrimaryRequester["primary_category"] = string(contract.ContactCategoryResident)
		request.DuplicateGroupID = "duplicate-queue-test"
		require.NoError(t, store.UpdateCity311ServiceRequest(context.Background(), st, request))
	}

	filters := map[string]any{
		"status": []string{string(firstRequest.Status), string(secondRequest.Status)}, "service_type": []string{string(firstRequest.ServiceType)},
		"department": []string{string(firstRequest.OwningDepartment)}, "district": []string{string(firstRequest.CouncilDistrict)},
		"origin_class": []string{string(firstRequest.OriginClass)}, "source_channel": []string{string(firstRequest.SourceChannel)},
		"assignee": []string{strconv.FormatUint(user.ID, 10)}, "collaborator": []string{strconv.FormatUint(user.ID, 10)},
		"category":        []string{string(contract.ContactCategoryResident)},
		"created_from":    firstRequest.CreatedAt.Add(-time.Minute).Format(time.RFC3339),
		"created_to":      secondRequest.CreatedAt.Add(time.Minute).Format(time.RFC3339),
		"duplicate_group": []string{"duplicate-queue-test"},
	}
	encoded, err := json.Marshal(filters)
	require.NoError(t, err)
	path := "/api/v1/staff/service-requests?filters=" + url.QueryEscape(string(encoded)) + "&page_size=1&sort=-updated_at"
	response := executeJSON(t, router, http.MethodGet, path, nil, nil, user.ID)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	firstPage := contract.ListResponse{}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &firstPage))
	require.Len(t, firstPage.Items, 1)
	require.Equal(t, 2, firstPage.TotalCount)
	require.NotNil(t, firstPage.NextPageToken)
	require.Len(t, firstPage.AppliedFilters, 12)
	require.Equal(t, []string{"-updated_at"}, firstPage.Sort)

	path += "&page_token=" + url.QueryEscape(*firstPage.NextPageToken)
	response = executeJSON(t, router, http.MethodGet, path, nil, nil, user.ID)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	secondPage := contract.ListResponse{}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &secondPage))
	require.Len(t, secondPage.Items, 1)
	require.Equal(t, 2, secondPage.TotalCount)
	require.NotEqual(t, firstPage.Items[0].RequestID, secondPage.Items[0].RequestID)
}

func TestParseIfMatchRequiresOneStrongQuotedPositiveVersion(t *testing.T) {
	parsed, err := parseIfMatch(`"12"`)
	require.NoError(t, err)
	require.Equal(t, uint64(12), parsed)
	for _, value := range []string{"", `W/"12"`, "12", `"0"`, `"-1"`, `"1","2"`, `"abc"`} {
		_, err = parseIfMatch(value)
		require.Error(t, err, value)
	}
}

func TestStaffDetailAndExplodedQueueFilterErrors(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	user, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)

	detailPath := fmt.Sprintf("/api/v1/staff/service-requests/%d", request.ID)
	detail := executeJSON(t, router, http.MethodGet, detailPath, nil, nil, user.ID)
	require.Equal(t, http.StatusOK, detail.Code, detail.Body.String())
	require.Contains(t, detail.Body.String(), request.RequestNumber)

	invalidDetail := executeJSON(t, router, http.MethodGet, "/api/v1/staff/service-requests/not-an-id", nil, nil, user.ID)
	require.Equal(t, http.StatusUnprocessableEntity, invalidDetail.Code)
	missingDetail := executeJSON(t, router, http.MethodGet, "/api/v1/staff/service-requests/999999999", nil, nil, user.ID)
	require.Equal(t, http.StatusNotFound, missingDetail.Code)

	query := url.Values{}
	query.Set("filters[status]", "SUBMITTED,CLOSED")
	query.Set("service_type", "POTHOLE")
	query.Set("created_from", request.CreatedAt.Add(-time.Minute).Format(time.RFC3339))
	query.Set("filters[created_to]", request.CreatedAt.Add(time.Minute).Format(time.RFC3339))
	queue := executeJSON(t, router, http.MethodGet, "/api/v1/staff/service-requests?"+query.Encode(), nil, nil, user.ID)
	require.Equal(t, http.StatusOK, queue.Code, queue.Body.String())

	invalidQueries := []string{
		"page_size=0",
		"filters=" + url.QueryEscape(`{"unknown":true}`),
		"filters=" + url.QueryEscape(`{"status":"SUBMITTED"} {}`),
		"assignee=not-an-id",
		"created_from=not-a-time",
	}
	for _, rawQuery := range invalidQueries {
		response := executeJSON(t, router, http.MethodGet, "/api/v1/staff/service-requests?"+rawQuery, nil, nil, user.ID)
		require.Equal(t, http.StatusUnprocessableEntity, response.Code, rawQuery+": "+response.Body.String())
	}
}

func TestRouterHelperAndAuthorizationErrorPaths(t *testing.T) {
	var values stringList
	require.NoError(t, json.Unmarshal([]byte(`"single"`), &values))
	require.Equal(t, stringList{"single"}, values)
	require.Error(t, json.Unmarshal([]byte(`1`), &values))

	router, st, svc := testRouter(t)
	previousDefault := city311Service.Default
	city311Service.Default = svc
	t.Cleanup(func() { city311Service.Default = previousDefault })
	defaultRouter := chi.NewRouter()
	defaultRouter.Route("/api/v1", MountRoutes())

	authenticatedPortal := executeJSON(t, defaultRouter, http.MethodPost, "/api/v1/portal/service-requests", validPortalBody(), map[string]string{contract.IdempotencyHeader: "authenticated-portal"}, 321)
	require.Equal(t, http.StatusCreated, authenticatedPortal.Code, authenticatedPortal.Body.String())

	malformedRequest := httptest.NewRequest(http.MethodPost, "/api/v1/portal/service-requests", bytes.NewBufferString(`{"summary":`))
	malformedRequest.Header.Set("Content-Type", "application/json")
	malformed := httptest.NewRecorder()
	router.ServeHTTP(malformed, malformedRequest)
	require.Equal(t, http.StatusUnprocessableEntity, malformed.Code)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/portal/service-requests", bytes.NewBufferString(`{} {}`))
	request.Header.Set("Content-Type", "application/json")
	trailing := httptest.NewRecorder()
	router.ServeHTTP(trailing, request)
	require.Equal(t, http.StatusUnprocessableEntity, trailing.Code)

	workflowDesigner, err := store.LookupUserByEmail(context.Background(), st, "workflow-designer@city311.example.invalid")
	require.NoError(t, err)
	staffBody := map[string]any{"request": validPortalBody(), "constituent": map[string]any{"constituent_id": "C-1"}}
	forbidden := executeJSON(t, router, http.MethodPost, "/api/v1/staff/service-requests", staffBody, nil, workflowDesigner.ID)
	require.Equal(t, http.StatusForbidden, forbidden.Code)

	missingActor := executeJSON(t, router, http.MethodGet, "/api/v1/staff/service-requests", nil, nil, 999999999)
	require.Equal(t, http.StatusForbidden, missingActor.Code)
	invalidTransition := executeJSON(t, router, http.MethodPost, "/api/v1/staff/service-requests/not-an-id/transitions", map[string]any{"to_status": "TRIAGED"}, map[string]string{contract.IfMatchHeader: `"1"`}, workflowDesigner.ID)
	require.Equal(t, http.StatusUnprocessableEntity, invalidTransition.Code)

	generic := httptest.NewRecorder()
	writeResult(generic, 0, nil, errors.New("store failure"))
	require.Equal(t, http.StatusInternalServerError, generic.Code)
	require.False(t, actorCanCreate(contract.Actor{Roles: []contract.ApplicationRole{contract.ApplicationRoleWorkflowDesigner}}))
}
