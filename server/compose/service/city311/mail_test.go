package city311

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"io"
	"net/smtp"
	"net/textproto"
	"strings"
	"testing"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

type scriptedMailSender struct {
	codes        []int
	messages     []MailMessage
	deliveryKeys []string
}

type smtpTestClient struct {
	failAt string
	body   bytes.Buffer
}

func (client *smtpTestClient) fail(step string) error {
	if client.failAt == step {
		return &textproto.Error{Code: 451, Msg: step}
	}
	return nil
}

func (client *smtpTestClient) StartTLS(*tls.Config) error { return client.fail("starttls") }
func (client *smtpTestClient) Auth(smtp.Auth) error       { return client.fail("auth") }
func (client *smtpTestClient) Mail(string) error          { return client.fail("mail") }
func (client *smtpTestClient) Rcpt(string) error          { return client.fail("rcpt") }
func (client *smtpTestClient) Quit() error                { return client.fail("quit") }
func (client *smtpTestClient) Close() error               { return nil }
func (client *smtpTestClient) Data() (io.WriteCloser, error) {
	if err := client.fail("data"); err != nil {
		return nil, err
	}
	return &smtpTestWriter{client: client, failAt: client.failAt}, nil
}

type smtpTestWriter struct {
	client *smtpTestClient
	failAt string
}

func (writer *smtpTestWriter) Write(value []byte) (int, error) {
	if writer.failAt == "write" {
		return 0, &textproto.Error{Code: 451, Msg: "write"}
	}
	return writer.client.body.Write(value)
}

func (writer *smtpTestWriter) Close() error {
	if writer.failAt == "close-data" {
		return &textproto.Error{Code: 451, Msg: "close-data"}
	}
	return nil
}

func (sender *scriptedMailSender) Send(_ context.Context, message MailMessage, deliveryKey string) (int, error) {
	sender.messages = append(sender.messages, message)
	sender.deliveryKeys = append(sender.deliveryKeys, deliveryKey)
	code := 250
	if len(sender.codes) > 0 {
		code = sender.codes[0]
		sender.codes = sender.codes[1:]
	}
	if code >= 200 && code < 300 {
		return code, nil
	}
	return code, &textproto.Error{Code: code, Msg: "fixture rejection"}
}

func TestMailPreviewSanitizesHTMLAndAppliesTemplate(t *testing.T) {
	svc, _ := testService(t)
	templateID := "service-request-update"
	preview, err := svc.PreviewMail(contract.Actor{Roles: []contract.ApplicationRole{contract.ApplicationRoleServiceAgent}}, contract.MailCompose{
		TemplateID: &templateID, To: []string{" Resident@Example.Invalid "}, Subject: "Request updated", Text: "Your request changed.",
		HTML: `<p onclick="bad()"><strong>Updated</strong><script>alert(1)</script><a href="javascript:bad()" title="safe">details</a></p>`,
	})
	require.NoError(t, err)
	require.Equal(t, "[City 311] Request updated", preview.Subject)
	require.Equal(t, "Your request changed.", preview.Text)
	require.Contains(t, preview.HTML, "<strong>Updated</strong>")
	require.NotContains(t, preview.HTML, "script")
	require.NotContains(t, preview.HTML, "onclick")
	require.NotContains(t, preview.HTML, "javascript")

	unknown := "unknown"
	_, err = svc.PreviewMail(contract.Actor{Roles: []contract.ApplicationRole{contract.ApplicationRoleServiceAgent}}, contract.MailCompose{
		TemplateID: &unknown, To: []string{"resident@example.invalid"}, Subject: "Subject", Text: "Body",
	})
	requireServiceError(t, err, 404, contract.ErrorNotFound)
	_, err = svc.PreviewMail(contract.Actor{Roles: []contract.ApplicationRole{contract.ApplicationRoleConstituent}}, contract.MailCompose{})
	requireServiceError(t, err, 403, contract.ErrorForbidden)
}

func TestMailDeliveryRetriesIdempotencyOwnershipAndAudit(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	agent := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")
	administrator := seededAssignmentActor(t, ctx, svc, st, "platform-admin@city311.example.invalid")
	other := seededAssignmentActor(t, ctx, svc, st, "supervisor@city311.example.invalid")
	sender := &scriptedMailSender{codes: []int{421, 451, 250, 550}}
	svc.mailSender = sender
	var waits []time.Duration
	svc.mailWait = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}
	input := contract.MailCompose{
		To: []string{" Resident@Example.Invalid "}, Subject: "Status update", Text: "Your request is in progress.",
		HTML: `<p>Your request is <em>in progress</em>.</p>`, Attachments: []contract.AttachmentInput{{
			Filename: "update.txt", MediaType: "text/plain", ContentBase64: base64.StdEncoding.EncodeToString([]byte("status")),
		}},
	}
	delivery, err := svc.SendMail(ctx, agent, "mail-key", input)
	require.NoError(t, err)
	require.Equal(t, mailStatusDelivered, delivery.Status)
	require.Equal(t, 3, delivery.Attempts)
	require.Nil(t, delivery.Error)
	require.Equal(t, []time.Duration{time.Second, 5 * time.Second}, waits)
	require.Len(t, sender.messages, 3)
	require.Equal(t, []string{"resident@example.invalid"}, sender.messages[0].To)
	require.Len(t, sender.messages[0].Attachments, 1)
	require.Equal(t, sender.deliveryKeys[0], sender.deliveryKeys[1])
	require.Equal(t, sender.deliveryKeys[1], sender.deliveryKeys[2])

	replayed, err := svc.SendMail(ctx, agent, "mail-key", input)
	require.NoError(t, err)
	require.Equal(t, delivery, replayed)
	require.Len(t, sender.messages, 3)
	input.Subject = "Different content"
	_, err = svc.SendMail(ctx, agent, "mail-key", input)
	requireServiceError(t, err, 409, contract.ErrorIdempotencyConflict)

	owned, err := svc.GetMailDelivery(ctx, agent, delivery.DeliveryID)
	require.NoError(t, err)
	require.Equal(t, delivery, owned)
	_, err = svc.GetMailDelivery(ctx, other, delivery.DeliveryID)
	requireServiceError(t, err, 403, contract.ErrorForbidden)
	_, err = svc.GetMailDelivery(ctx, administrator, delivery.DeliveryID)
	require.NoError(t, err)

	failed, err := svc.SendMail(ctx, agent, "permanent-key", contract.MailCompose{
		To: []string{"resident@example.invalid"}, Subject: "Permanent failure", Text: "Body",
	})
	require.NoError(t, err)
	require.Equal(t, mailStatusFailed, failed.Status)
	require.Equal(t, 1, failed.Attempts)
	require.NotNil(t, failed.Error)
	require.Len(t, sender.messages, 4)

	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{})
	require.NoError(t, err)
	events := map[string]int{}
	for _, event := range audits {
		events[event.EventType]++
	}
	require.Equal(t, 1, events["EMAIL_SENT"])
	require.Equal(t, 1, events["EMAIL_DELIVERY_FAILED"])
}

func TestMailValidationEncodingAndRetryCancellation(t *testing.T) {
	svc, _ := testService(t)
	actor := contract.Actor{ID: 1, Roles: []contract.ApplicationRole{contract.ApplicationRoleServiceAgent}}
	_, err := svc.SendMail(context.Background(), actor, "", contract.MailCompose{})
	requireServiceError(t, err, 422, contract.ErrorValidation)
	for _, input := range []contract.MailCompose{
		{},
		{To: []string{"invalid"}, Subject: "Subject", Text: "Body"},
		{To: []string{"resident@example.invalid"}, Subject: "bad\nsubject", Text: "Body"},
		{To: []string{"resident@example.invalid"}, Subject: "Subject", Text: "Body", Attachments: []contract.AttachmentInput{
			{}, {}, {}, {},
		}},
		{To: []string{"resident@example.invalid"}, Subject: "Subject", Text: "Body", Attachments: []contract.AttachmentInput{{
			Filename: "bad.exe", MediaType: "application/octet-stream", ContentBase64: "not-base64",
		}}},
		{To: []string{"resident@example.invalid"}, Subject: "Subject", Text: "Body", Attachments: []contract.AttachmentInput{{
			Filename: "large.txt", MediaType: "text/plain", ContentBase64: base64.StdEncoding.EncodeToString(make([]byte, maximumMailAttachmentSize+1)),
		}}},
	} {
		_, err = svc.PreviewMail(actor, input)
		requireServiceError(t, err, 422, contract.ErrorValidation)
	}

	message := MailMessage{
		From: "noreply@city.example", To: []string{"resident@example.invalid"}, Subject: "Résumé update", Text: "Plain body", HTML: "<p>HTML body</p>",
		Attachments: []validatedAttachment{{Filename: "note.txt", MediaType: "text/plain", Content: []byte("attachment")}},
	}
	encoded, err := encodeSMTPMessage(message, "delivery-key")
	require.NoError(t, err)
	require.Contains(t, string(encoded), "X-Delivery-Key: delivery-key")
	require.Contains(t, string(encoded), "Content-Disposition: attachment; filename=note.txt")
	require.Contains(t, string(encoded), base64.StdEncoding.EncodeToString([]byte("attachment")))
	require.Equal(t, 451, smtpErrorCode(&textproto.Error{Code: 451, Msg: "retry"}))
	require.Zero(t, smtpErrorCode(errors.New("network")))

	svc.mailSender = &scriptedMailSender{codes: []int{421}}
	svc.mailWait = func(context.Context, time.Duration) error { return context.Canceled }
	status, attempts, err := svc.deliverMail(context.Background(), message, "key")
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, mailStatusFailed, status)
	require.Equal(t, 1, attempts)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, waitForMailRetry(cancelled, time.Hour), context.Canceled)

	_, err = svc.GetMailDelivery(context.Background(), actor, "invalid")
	requireServiceError(t, err, 422, contract.ErrorValidation)
	_, err = svc.GetMailDelivery(context.Background(), actor, "mail-999999999")
	requireServiceError(t, err, 404, contract.ErrorNotFound)
}

func TestToMailDeliveryIgnoresMalformedError(t *testing.T) {
	now := time.Now().UTC()
	operation := &composeTypes.City311Operation{
		ID: 1, Kind: mailOperationKind, Status: mailStatusFailed, Progress: 1, UpdatedAt: now,
		Error: composeTypes.City311JSON{"retryable": "invalid"},
	}
	require.Nil(t, toMailDelivery(operation).Error)
	operation.Error = composeTypes.City311JSON{"bad": func() {}}
	require.Nil(t, toMailDelivery(operation).Error)
	require.Equal(t, uint64(1), mustMailDeliveryID(t, publicMailDeliveryID(operation.ID)))
}

func mustMailDeliveryID(t *testing.T, raw string) uint64 {
	t.Helper()
	id, err := parseMailDeliveryID(raw)
	require.NoError(t, err)
	return id
}

func TestSMTPMessageContainsNoUnsafeHTML(t *testing.T) {
	require.NotContains(t, sanitizeMailHTML(`<a href="javascript:alert(1)">bad</a><p>safe</p>`), "javascript")
	require.True(t, strings.Contains(sanitizeMailHTML(`<table><tr><td colspan="2">ok</td></tr></table>`), `colspan="2"`))
}

func TestSMTPMailSenderConfigurationSuccessAndFailures(t *testing.T) {
	t.Setenv("MAIL_SMTP_HOST", "smtp.example.invalid")
	t.Setenv("MAIL_SMTP_PORT", "587")
	t.Setenv("MAIL_SMTP_USERNAME", "city311")
	t.Setenv("MAIL_SMTP_PASSWORD", "secret")
	message := MailMessage{
		From: "noreply@city.example", To: []string{"one@example.invalid", "two@example.invalid"},
		Subject: "Subject", Text: "Body",
	}
	client := &smtpTestClient{}
	sender := smtpMailSender{dial: func(address string) (smtpClient, error) {
		require.Equal(t, "smtp.example.invalid:587", address)
		return client, nil
	}}
	status, err := sender.Send(context.Background(), message, "delivery-key")
	require.NoError(t, err)
	require.Equal(t, 250, status)
	require.Contains(t, client.body.String(), "X-Delivery-Key: delivery-key")

	for _, step := range []string{"starttls", "auth", "mail", "rcpt", "data", "write", "close-data", "quit"} {
		t.Run(step, func(t *testing.T) {
			client := &smtpTestClient{failAt: step}
			sender := smtpMailSender{dial: func(string) (smtpClient, error) { return client, nil }}
			status, err := sender.Send(context.Background(), message, "delivery-key")
			require.Error(t, err)
			require.Equal(t, 451, status)
		})
	}
	dialFailure := smtpMailSender{dial: func(string) (smtpClient, error) {
		return nil, &textproto.Error{Code: 421, Msg: "dial"}
	}}
	status, err = dialFailure.Send(context.Background(), message, "delivery-key")
	require.Error(t, err)
	require.Equal(t, 421, status)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	status, err = sender.Send(cancelled, message, "delivery-key")
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, status)

	t.Setenv("MAIL_SMTP_PASSWORD", "")
	status, err = sender.Send(context.Background(), message, "delivery-key")
	require.Error(t, err)
	require.Zero(t, status)
}

func TestSetMailSenderIgnoresNil(t *testing.T) {
	svc, _ := testService(t)
	svc.SetMailSender(nil)
	_, ok := svc.mailSender.(smtpMailSender)
	require.True(t, ok)
	replacement := &scriptedMailSender{}
	svc.SetMailSender(replacement)
	require.Equal(t, replacement, svc.mailSender)
}
