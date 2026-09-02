package city311

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/auth"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func uploadRequest(t *testing.T, router http.Handler, filename, mediaType string, data []byte, userID uint64) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	file, err := form.CreateFormFile("file", filename)
	require.NoError(t, err)
	_, err = file.Write(data)
	require.NoError(t, err)
	require.NoError(t, form.WriteField("filename", filename))
	require.NoError(t, form.WriteField("media_type", mediaType))
	require.NoError(t, form.Close())
	r := httptest.NewRequest(http.MethodPost, "/api/v1/portal/attachments", &body)
	r.Header.Set("Content-Type", form.FormDataContentType())
	if userID != 0 {
		r = r.WithContext(auth.SetIdentityToContext(r.Context(), auth.Authenticated(userID)))
	} else {
		r.AddCookie(&http.Cookie{Name: "city311_session", Value: "invalid"})
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestAttachmentHTTPUploadSubmitReplayDownload(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	user, err := store.LookupUserByHandle(ctx, st, "city311-constituent")
	require.NoError(t, err)
	content := []byte{0, 255, 128, 1}
	w := uploadRequest(t, router, "photo.png", "image/png", content, user.ID)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var receipt contract.PortalAttachment
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &receipt))
	body := validPortalBody()
	body["attachment_tokens"] = []string{receipt.AttachmentToken}
	headers := map[string]string{contract.IdempotencyHeader: "http-file"}
	created := executeJSON(t, router, http.MethodPost, "/api/v1/portal/service-requests", body, headers, user.ID)
	require.Equal(t, 201, created.Code, created.Body.String())
	replayed := executeJSON(t, router, http.MethodPost, "/api/v1/portal/service-requests", body, headers, user.ID)
	require.Equal(t, 201, replayed.Code, replayed.Body.String())
	require.JSONEq(t, created.Body.String(), replayed.Body.String())
	var response contract.ServiceRequestResponse
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &response))
	require.Len(t, response.Attachments, 1)
	requestID, _ := strconv.ParseUint(response.RequestID, 10, 64)
	items, _, err := store.SearchCity311RequestAttachments(ctx, st, composeTypes.City311RequestAttachmentFilter{RequestID: requestID})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, strconv.FormatUint(items[0].ID, 10), response.Attachments[0].AttachmentID)
	path := "/api/v1/attachments/" + strconv.FormatUint(items[0].ID, 10)
	w = executeJSON(t, router, http.MethodGet, path, nil, nil, user.ID)
	require.Equal(t, 200, w.Code, w.Body.String())
	require.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	var download contract.BinaryAttachment
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &download))
	require.Equal(t, "base64", download.BodyEncoding)
	decoded, err := base64.StdEncoding.DecodeString(download.Body)
	require.NoError(t, err)
	require.Equal(t, content, decoded)
	w = executeJSON(t, router, http.MethodGet, path, nil, nil, 0)
	require.Equal(t, 401, w.Code)
	stranger, err := store.LookupUserByHandle(ctx, st, "city311-constituent-two")
	require.NoError(t, err)
	w = executeJSON(t, router, http.MethodGet, path, nil, nil, stranger.ID)
	require.Equal(t, 403, w.Code)
	w = executeJSON(t, router, http.MethodGet, "/api/v1/attachments/"+receipt.AttachmentToken, nil, nil, user.ID)
	require.Equal(t, 404, w.Code)
	staff, err := store.LookupUserByHandle(ctx, st, "city311-service-agent")
	require.NoError(t, err)
	w = executeJSON(t, router, http.MethodGet, "/api/v1/staff/service-requests/"+response.RequestID, nil, nil, staff.ID)
	require.Equal(t, 200, w.Code, w.Body.String())
	var detail contract.StaffServiceRequestDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &detail))
	require.Len(t, detail.Request.Attachments, 1)
	require.Equal(t, strconv.FormatUint(items[0].ID, 10), detail.Request.Attachments[0].AttachmentID)
	require.NotContains(t, w.Body.String(), receipt.AttachmentToken)
}

func TestAttachmentHTTPMultipartRejectionAndAnonymousStaging(t *testing.T) {
	router, st, _ := testRouter(t)
	for _, data := range [][]byte{nil, make([]byte, (10<<20)+1)} {
		w := uploadRequest(t, router, "note.txt", "text/plain", data, 0)
		require.Equal(t, 422, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), "/file")
	}
	w := executeJSON(t, router, http.MethodPost, "/api/v1/portal/attachments", map[string]string{"file": "not multipart"}, nil, 0)
	require.Equal(t, 422, w.Code)
	w = uploadRequest(t, router, "evil.html", "text/html", []byte("invalid"), 0)
	require.Equal(t, 422, w.Code)
	items, _, err := store.SearchCity311StagedAttachments(context.Background(), st, composeTypes.City311StagedAttachmentFilter{})
	require.NoError(t, err)
	require.Empty(t, items)
	w = uploadRequest(t, router, "note.txt", "text/plain", []byte("anonymous"), 0)
	require.Equal(t, 201, w.Code, w.Body.String())
	var receipt contract.PortalAttachment
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &receipt))
	body := validPortalBody()
	body["attachment_tokens"] = []string{receipt.AttachmentToken}
	w = executeJSON(t, router, http.MethodPost, "/api/v1/portal/service-requests", body, map[string]string{contract.IdempotencyHeader: "anonymous-file"}, 0)
	require.Equal(t, 201, w.Code, w.Body.String())
}

func TestAttachmentHTTPStaffSubmission(t *testing.T) {
	router, st, _ := testRouter(t)
	user, err := store.LookupUserByHandle(context.Background(), st, "city311-service-agent")
	require.NoError(t, err)
	w := uploadRequest(t, router, "note.txt", "text/plain", []byte("staff"), user.ID)
	require.Equal(t, 201, w.Code)
	var receipt contract.PortalAttachment
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &receipt))
	body := validPortalBody()
	body["attachment_tokens"] = []string{receipt.AttachmentToken}
	input := map[string]any{"request": body, "constituent": map[string]string{"constituent_id": "C-1"}}
	w = executeJSON(t, router, http.MethodPost, "/api/v1/staff/service-requests", input, nil, user.ID)
	require.Equal(t, 201, w.Code, w.Body.String())
	var detail contract.StaffServiceRequestDetail
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &detail))
	require.Len(t, detail.Request.Attachments, 1)
	require.Equal(t, contract.SourceChannelStaffInPerson, detail.Request.SourceChannel)
}

func TestAttachmentHTTPRequiresCRMRecordRoleAndScope(t *testing.T) {
	router, st, _ := testRouter(t)
	ctx := context.Background()
	uploaded := uploadRequest(t, router, "private.txt", "text/plain", []byte("private request bytes"), 0)
	require.Equal(t, http.StatusCreated, uploaded.Code)
	var receipt contract.PortalAttachment
	require.NoError(t, json.Unmarshal(uploaded.Body.Bytes(), &receipt))
	body := validPortalBody()
	body["service_type"] = "GENERAL_INQUIRY"
	body["attachment_tokens"] = []string{receipt.AttachmentToken}
	created := executeJSON(t, router, http.MethodPost, "/api/v1/portal/service-requests", body, map[string]string{contract.IdempotencyHeader: "workflow-record-role"}, 0)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var response contract.ServiceRequestResponse
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &response))
	requestID, err := strconv.ParseUint(response.RequestID, 10, 64)
	require.NoError(t, err)
	request, err := store.LookupCity311ServiceRequestByID(ctx, st, requestID)
	require.NoError(t, err)
	request.CouncilDistrict = contract.DistrictCentral
	require.NoError(t, store.UpdateCity311ServiceRequest(ctx, st, request))
	user, err := store.LookupUserByHandle(ctx, st, "city311-workflow-designer")
	require.NoError(t, err)
	profile, err := store.LookupCity311ActorProfileByID(ctx, st, user.ID)
	require.NoError(t, err)
	for _, tc := range []struct {
		name       string
		roles      []contract.ApplicationRole
		department contract.DepartmentCode
		districts  []contract.DistrictCode
		status     int
	}{
		{"workflow only", []contract.ApplicationRole{contract.ApplicationRoleWorkflowDesigner}, contract.DepartmentGeneralServices, []contract.DistrictCode{contract.DistrictCentral}, http.StatusForbidden},
		{"cumulative record role", []contract.ApplicationRole{contract.ApplicationRoleWorkflowDesigner, contract.ApplicationRoleServiceAgent}, contract.DepartmentGeneralServices, []contract.DistrictCode{contract.DistrictCentral}, http.StatusOK},
		{"wrong district", []contract.ApplicationRole{contract.ApplicationRoleWorkflowDesigner, contract.ApplicationRoleServiceAgent}, contract.DepartmentGeneralServices, nil, http.StatusForbidden},
		{"wrong department", []contract.ApplicationRole{contract.ApplicationRoleWorkflowDesigner, contract.ApplicationRoleServiceAgent}, contract.DepartmentPublicWorks, []contract.DistrictCode{contract.DistrictCentral}, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile.ApplicationRoles, profile.Department, profile.Districts = tc.roles, tc.department, tc.districts
			require.NoError(t, store.UpdateCity311ActorProfile(ctx, st, profile))
			for _, path := range []string{"/api/v1/attachments/" + response.Attachments[0].AttachmentID, "/api/v1/staff/service-requests/" + response.RequestID} {
				result := executeJSON(t, router, http.MethodGet, path, nil, nil, user.ID)
				require.Equal(t, tc.status, result.Code, "%s: %s", path, result.Body.String())
			}
		})
	}
}
