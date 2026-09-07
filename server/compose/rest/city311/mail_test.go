package city311

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	city311Service "github.com/cortezaproject/corteza/server/compose/service/city311"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

type acceptingRESTMailSender struct{ calls int }

func (sender *acceptingRESTMailSender) Send(context.Context, city311Service.MailMessage, string) (int, error) {
	sender.calls++
	return 250, nil
}

func TestMailHTTPPreviewSendReplayAndLookup(t *testing.T) {
	router, st, svc := testRouter(t)
	ctx := context.Background()
	agent, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	sender := &acceptingRESTMailSender{}
	svc.SetMailSender(sender)
	body := map[string]any{
		"to": []string{"resident@example.invalid"}, "subject": "Request updated", "text": "Plain update",
		"html": `<p onclick="bad()"><strong>Safe</strong><script>bad()</script></p>`,
	}

	unauthenticated := executeJSON(t, router, http.MethodPost, "/api/v1/staff/mail/preview", body, nil, 0)
	require.Equal(t, http.StatusUnauthorized, unauthenticated.Code)
	preview := executeJSON(t, router, http.MethodPost, "/api/v1/staff/mail/preview", body, nil, agent.ID)
	require.Equal(t, http.StatusOK, preview.Code, preview.Body.String())
	var previewBody contract.MailPreview
	require.NoError(t, json.Unmarshal(preview.Body.Bytes(), &previewBody))
	require.Contains(t, previewBody.HTML, `<strong>Safe</strong>`)
	require.NotContains(t, previewBody.HTML, "script")

	missingKey := executeJSON(t, router, http.MethodPost, "/api/v1/staff/mail", body, nil, agent.ID)
	require.Equal(t, http.StatusUnprocessableEntity, missingKey.Code, missingKey.Body.String())
	headers := map[string]string{contract.IdempotencyHeader: "http-mail-key"}
	accepted := executeJSON(t, router, http.MethodPost, "/api/v1/staff/mail", body, headers, agent.ID)
	require.Equal(t, http.StatusAccepted, accepted.Code, accepted.Body.String())
	var delivery contract.MailDelivery
	require.NoError(t, json.Unmarshal(accepted.Body.Bytes(), &delivery))
	require.Equal(t, "DELIVERED", delivery.Status)
	require.Equal(t, 1, delivery.Attempts)
	require.Equal(t, 1, sender.calls)

	replayed := executeJSON(t, router, http.MethodPost, "/api/v1/staff/mail", body, headers, agent.ID)
	require.Equal(t, http.StatusAccepted, replayed.Code, replayed.Body.String())
	require.JSONEq(t, accepted.Body.String(), replayed.Body.String())
	require.Equal(t, 1, sender.calls)

	fetched := executeJSON(t, router, http.MethodGet, "/api/v1/staff/mail/"+delivery.DeliveryID, nil, nil, agent.ID)
	require.Equal(t, http.StatusOK, fetched.Code, fetched.Body.String())
	require.JSONEq(t, accepted.Body.String(), fetched.Body.String())
	missing := executeJSON(t, router, http.MethodGet, "/api/v1/staff/mail/mail-999999999", nil, nil, agent.ID)
	require.Equal(t, http.StatusNotFound, missing.Code, missing.Body.String())
}
