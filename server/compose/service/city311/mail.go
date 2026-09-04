package city311

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/smtp"
	"net/textproto"
	"os"
	"strconv"
	"strings"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/errors"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/microcosm-cc/bluemonday"
)

const (
	mailOperationName         = "mail_send"
	mailOperationKind         = "MAIL_DELIVERY"
	mailStatusPending         = "PENDING"
	mailStatusDelivered       = "DELIVERED"
	mailStatusFailed          = "TERMINAL_FAILURE"
	maximumMailAttachmentSize = 5 << 20
)

var mailRetryDelays = []time.Duration{0, time.Second, 5 * time.Second}

type MailMessage struct {
	From        string
	To          []string
	Subject     string
	Text        string
	HTML        string
	Attachments []validatedAttachment
}

type MailSender interface {
	Send(context.Context, MailMessage, string) (int, error)
}

type smtpClient interface {
	StartTLS(*tls.Config) error
	Auth(smtp.Auth) error
	Mail(string) error
	Rcpt(string) error
	Data() (io.WriteCloser, error)
	Quit() error
	Close() error
}

type smtpDialer func(string) (smtpClient, error)

type smtpMailSender struct {
	dial     smtpDialer
	host     string
	port     string
	username string
	password string
}

type mailTemplate struct {
	From          string
	SubjectPrefix string
}

var mailTemplates = map[string]mailTemplate{
	"city311-standard":       {From: "noreply@city.example"},
	"service-request-update": {From: "updates@city.example", SubjectPrefix: "[City 311] "},
}

func (svc *Service) SetMailSender(sender MailSender) {
	if sender != nil {
		svc.runtimeMu.Lock()
		defer svc.runtimeMu.Unlock()
		svc.mailSender = sender
	}
}

func (svc *Service) PreviewMail(actor contract.Actor, input contract.MailCompose) (*contract.MailPreview, error) {
	if !isStaff(actor) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, "A staff role is required.")
	}
	prepared, err := prepareMail(input)
	if err != nil {
		return nil, err
	}
	return &contract.MailPreview{Subject: prepared.Subject, Text: prepared.Text, HTML: prepared.HTML}, nil
}

func (svc *Service) SendMail(ctx context.Context, actor contract.Actor, idempotencyKey string, input contract.MailCompose) (*contract.MailDelivery, error) {
	if !isStaff(actor) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, "A staff role is required.")
	}
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if idempotencyKey == "" {
		return nil, validationError(contract.FieldError{Field: "/headers/Idempotency-Key", Code: contract.ValidationRequired})
	}
	prepared, err := prepareMail(input)
	if err != nil {
		return nil, err
	}
	requestHash, err := hashJSON(input)
	if err != nil {
		return nil, err
	}

	svc.mailMu.Lock()
	defer svc.mailMu.Unlock()
	delivery := &contract.MailDelivery{}
	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		replayed, replayErr := svc.replayMail(ctx, tx, idempotencyKey, requestHash, delivery)
		if replayErr != nil || replayed {
			return replayErr
		}
		now := svc.now()
		operation := &composeTypes.City311Operation{
			ID: svc.nextID(), Kind: mailOperationKind, Status: mailStatusPending, ActorID: actor.ID,
			Result: composeTypes.City311JSON{}, Error: composeTypes.City311JSON{}, CreatedAt: now, UpdatedAt: now,
		}
		if createErr := store.CreateCity311Operation(ctx, tx, operation); createErr != nil {
			return createErr
		}
		deliveryKey := "city311-mail:" + hashKey(idempotencyKey)
		status, attempts, sendErr := svc.deliverMail(ctx, prepared, deliveryKey)
		operation.Status = status
		operation.Progress = attempts
		operation.UpdatedAt = svc.now()
		if sendErr != nil {
			operation.Error = composeTypes.City311JSON{
				"error": string(contract.ErrorOperationFailed), "message": "Mail delivery failed permanently.", "retryable": false,
			}
		}
		if updateErr := store.UpdateCity311Operation(ctx, tx, operation); updateErr != nil {
			return updateErr
		}
		*delivery = *toMailDelivery(operation)
		body, mapErr := mapFrom(delivery)
		if mapErr != nil {
			return mapErr
		}
		if persistErr := store.CreateCity311IdempotencyRecord(ctx, tx, &composeTypes.City311IdempotencyRecord{
			ID: svc.nextID(), Operation: mailOperationName, KeyHash: hashKey(idempotencyKey), RequestHash: requestHash,
			ResponseStatus: http.StatusAccepted, ResponseBody: body, RequestID: operation.ID,
			CreatedAt: now, ExpiresAt: now.Add(idempotencyLifetime),
		}); persistErr != nil {
			return persistErr
		}
		eventType := "EMAIL_SENT"
		if status == mailStatusFailed {
			eventType = "EMAIL_DELIVERY_FAILED"
		}
		return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
			ID: svc.nextID(), EntityType: "mail_delivery", EntityID: publicMailDeliveryID(operation.ID), EventType: eventType,
			ActorType: contract.AuditActorStaff, ActorID: actor.ID, SourceChannel: contract.SourceChannelStaffInPerson,
			Before: composeTypes.City311JSON{}, After: composeTypes.City311JSON{
				"department_code": optionalActorDepartment(actor), "status": status, "attempts": attempts,
				"recipient_count": len(prepared.To), "delivery_key_hash": hashKey(deliveryKey),
			}, CreatedAt: operation.UpdatedAt,
		})
	})
	if err != nil {
		return nil, err
	}
	return delivery, nil
}

func (svc *Service) GetMailDelivery(ctx context.Context, actor contract.Actor, rawID string) (*contract.MailDelivery, error) {
	id, err := parseMailDeliveryID(rawID)
	if err != nil {
		return nil, validationError(contract.FieldError{Field: "/path/delivery_id", Code: contract.ValidationInvalidFormat})
	}
	operation, err := store.LookupCity311OperationByID(ctx, svc.store, id)
	if errors.IsNotFound(err) || (err == nil && operation.Kind != mailOperationKind) {
		return nil, apiError(http.StatusNotFound, contract.ErrorNotFound, "The mail delivery was not found.")
	}
	if err != nil {
		return nil, err
	}
	if operation.ActorID != actor.ID && !hasRole(actor, contract.ApplicationRolePlatformAdministrator) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, "The mail delivery belongs to another actor.")
	}
	return toMailDelivery(operation), nil
}

func prepareMail(input contract.MailCompose) (MailMessage, error) {
	var fields []contract.FieldError
	if len(input.To) == 0 {
		fields = append(fields, contract.FieldError{Field: "/to", Code: contract.ValidationRequired})
	}
	to := make([]string, 0, len(input.To))
	for index, recipient := range input.To {
		recipient = normalizeEmail(recipient)
		if !validEmail(recipient) {
			fields = append(fields, contract.FieldError{Field: fmt.Sprintf("/to/%d", index), Code: contract.ValidationInvalidFormat})
		} else {
			to = append(to, recipient)
		}
	}
	subject := strings.TrimSpace(input.Subject)
	if subject == "" {
		fields = append(fields, contract.FieldError{Field: "/subject", Code: contract.ValidationRequired})
	} else if strings.ContainsAny(subject, "\r\n") {
		fields = append(fields, contract.FieldError{Field: "/subject", Code: contract.ValidationInvalidFormat})
	}
	textBody := strings.TrimSpace(input.Text)
	if textBody == "" {
		fields = append(fields, contract.FieldError{Field: "/text", Code: contract.ValidationRequired})
	}
	if len(input.Attachments) > 3 {
		fields = append(fields, contract.FieldError{Field: "/attachments", Code: contract.ValidationTooManyItems})
	}
	attachments, attachmentErr := validateInlineAttachments(input.Attachments)
	if attachmentErr != nil {
		fields = append(fields, attachmentErr.Payload.Errors...)
	}
	for index, attachment := range attachments {
		if len(attachment.Content) > maximumMailAttachmentSize {
			fields = append(fields, contract.FieldError{Field: fmt.Sprintf("/attachments/%d/content_base64", index), Code: contract.ValidationOutOfRange})
		}
	}
	template := mailTemplates["city311-standard"]
	if input.TemplateID != nil {
		templateID := strings.TrimSpace(*input.TemplateID)
		var found bool
		template, found = mailTemplates[templateID]
		if !found {
			return MailMessage{}, apiError(http.StatusNotFound, contract.ErrorNotFound, "The mail template was not found.")
		}
	}
	if len(fields) > 0 {
		return MailMessage{}, validationError(fields...)
	}
	return MailMessage{
		From: template.From, To: to, Subject: template.SubjectPrefix + subject, Text: textBody,
		HTML: sanitizeMailHTML(input.HTML), Attachments: attachments,
	}, nil
}

func sanitizeMailHTML(value string) string {
	policy := bluemonday.NewPolicy()
	policy.AllowElements("p", "br", "strong", "em", "ul", "ol", "li", "a", "table", "thead", "tbody", "tr", "th", "td")
	policy.AllowAttrs("href", "title").OnElements("a")
	policy.AllowAttrs("colspan", "rowspan").OnElements("th", "td")
	policy.AllowURLSchemes("http", "https", "mailto")
	return policy.Sanitize(strings.TrimSpace(value))
}

func (svc *Service) deliverMail(ctx context.Context, message MailMessage, deliveryKey string) (string, int, error) {
	svc.runtimeMu.RLock()
	sender := svc.mailSender
	svc.runtimeMu.RUnlock()
	var lastErr error
	for index, delay := range mailRetryDelays {
		if delay > 0 {
			if err := svc.mailWait(ctx, delay); err != nil {
				return mailStatusFailed, index, err
			}
		}
		code, err := sender.Send(ctx, message, deliveryKey)
		attempts := index + 1
		if err == nil {
			return mailStatusDelivered, attempts, nil
		}
		lastErr = err
		if code != 0 && code != 421 && code != 451 {
			return mailStatusFailed, attempts, err
		}
	}
	return mailStatusFailed, len(mailRetryDelays), lastErr
}

func (svc *Service) replayMail(ctx context.Context, st store.Storer, key, requestHash string, out *contract.MailDelivery) (bool, error) {
	record, err := store.LookupCity311IdempotencyRecordByOperationKeyHash(ctx, st, mailOperationName, hashKey(key))
	if errors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !record.ExpiresAt.After(svc.now()) {
		return false, store.DeleteCity311IdempotencyRecord(ctx, st, record)
	}
	if record.RequestHash != requestHash {
		return false, apiError(http.StatusConflict, contract.ErrorIdempotencyConflict, "The idempotency key was already used with different mail content.")
	}
	encoded, err := json.Marshal(record.ResponseBody)
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal(encoded, out)
}

func toMailDelivery(operation *composeTypes.City311Operation) *contract.MailDelivery {
	result := &contract.MailDelivery{
		DeliveryID: publicMailDeliveryID(operation.ID), Status: operation.Status, Attempts: operation.Progress, UpdatedAt: operation.UpdatedAt,
	}
	if len(operation.Error) > 0 {
		encoded, err := json.Marshal(operation.Error)
		if err == nil {
			result.Error = &contract.APIError{}
			if json.Unmarshal(encoded, result.Error) != nil {
				result.Error = nil
			}
		}
	}
	return result
}

func publicMailDeliveryID(id uint64) string { return "mail-" + strconv.FormatUint(id, 10) }

func parseMailDeliveryID(raw string) (uint64, error) {
	if !strings.HasPrefix(raw, "mail-") {
		return 0, fmt.Errorf("mail delivery id must start with mail-")
	}
	id, err := strconv.ParseUint(strings.TrimPrefix(raw, "mail-"), 10, 64)
	if err != nil || id == 0 {
		return 0, fmt.Errorf("mail delivery id must contain a positive decimal identifier")
	}
	return id, nil
}

func waitForMailRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (sender smtpMailSender) Send(ctx context.Context, message MailMessage, deliveryKey string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	host := strings.TrimSpace(sender.host)
	port := strings.TrimSpace(sender.port)
	username := sender.username
	password := sender.password
	if host == "" && port == "" && username == "" && password == "" {
		host = strings.TrimSpace(os.Getenv("MAIL_SMTP_HOST"))
		port = strings.TrimSpace(os.Getenv("MAIL_SMTP_PORT"))
		username = os.Getenv("MAIL_SMTP_USERNAME")
		password = os.Getenv("MAIL_SMTP_PASSWORD")
	}
	if host == "" || port == "" || username == "" || password == "" {
		return 0, fmt.Errorf("mail SMTP configuration is incomplete")
	}
	address := net.JoinHostPort(host, port)
	dial := sender.dial
	if dial == nil {
		dial = dialSMTP
	}
	client, err := dial(address)
	if err != nil {
		return smtpErrorCode(err), err
	}
	defer client.Close()
	if err = client.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
		return smtpErrorCode(err), err
	}
	if err = client.Auth(smtp.PlainAuth("", username, password, host)); err != nil {
		return smtpErrorCode(err), err
	}
	if err = client.Mail(message.From); err != nil {
		return smtpErrorCode(err), err
	}
	for _, recipient := range message.To {
		if err = client.Rcpt(recipient); err != nil {
			return smtpErrorCode(err), err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return smtpErrorCode(err), err
	}
	payload, err := encodeSMTPMessage(message, deliveryKey)
	if err == nil {
		_, err = writer.Write(payload)
	}
	closeErr := writer.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return smtpErrorCode(err), err
	}
	if err = client.Quit(); err != nil {
		return smtpErrorCode(err), err
	}
	return 250, nil
}

func dialSMTP(address string) (smtpClient, error) { return smtp.Dial(address) }

func encodeSMTPMessage(message MailMessage, deliveryKey string) ([]byte, error) {
	buffer := &bytes.Buffer{}
	mixed := multipart.NewWriter(buffer)
	for _, header := range [][2]string{
		{"From", message.From}, {"To", strings.Join(message.To, ", ")}, {"Subject", mime.QEncoding.Encode("utf-8", message.Subject)},
		{"MIME-Version", "1.0"}, {"Content-Type", "multipart/mixed; boundary=" + strconv.Quote(mixed.Boundary())}, {"X-Delivery-Key", deliveryKey},
	} {
		if _, err := fmt.Fprintf(buffer, "%s: %s\r\n", header[0], header[1]); err != nil {
			return nil, err
		}
	}
	buffer.WriteString("\r\n")
	for _, body := range [][2]string{{"text/plain; charset=utf-8", message.Text}, {"text/html; charset=utf-8", message.HTML}} {
		mediaType, content := body[0], body[1]
		if content == "" {
			continue
		}
		header := textproto.MIMEHeader{"Content-Type": {mediaType}, "Content-Transfer-Encoding": {"quoted-printable"}}
		part, err := mixed.CreatePart(header)
		if err != nil {
			return nil, err
		}
		quoted := quotedprintable.NewWriter(part)
		if _, err = io.WriteString(quoted, content); err != nil {
			return nil, err
		}
		if err = quoted.Close(); err != nil {
			return nil, err
		}
	}
	for _, attachment := range message.Attachments {
		header := textproto.MIMEHeader{
			"Content-Type": {attachment.MediaType}, "Content-Transfer-Encoding": {"base64"},
			"Content-Disposition": {mime.FormatMediaType("attachment", map[string]string{"filename": attachment.Filename})},
		}
		part, err := mixed.CreatePart(header)
		if err != nil {
			return nil, err
		}
		encoder := base64.NewEncoder(base64.StdEncoding, part)
		if _, err = encoder.Write(attachment.Content); err != nil {
			return nil, err
		}
		if err = encoder.Close(); err != nil {
			return nil, err
		}
	}
	if err := mixed.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func smtpErrorCode(err error) int {
	var protocolError *textproto.Error
	if errors.As(err, &protocolError) {
		return protocolError.Code
	}
	return 0
}
