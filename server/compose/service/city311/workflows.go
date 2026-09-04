package city311

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/errors"
	"github.com/cortezaproject/corteza/server/store"
)

const (
	WorkflowTriggerCreated       = "SERVICE_REQUEST_CREATED"
	WorkflowTriggerStatusChanged = "SERVICE_REQUEST_STATUS_CHANGED"
	workflowTestOperationKind    = "WORKFLOW_TEST"
	workflowListSort             = "-updated_at"
)

type WorkflowListQuery struct {
	Trigger   string
	Active    *bool
	PageSize  uint
	PageToken string
}

type WorkflowExecutionQuery struct {
	WorkflowID string
	RequestID  uint64
	Succeeded  *bool
	PageSize   uint
	PageToken  string
}

type WorkflowHTTPClient interface {
	Execute(context.Context, contract.WorkflowActionRequest, string) (int, error)
}

type workflowHTTPError struct {
	status int
	body   contract.APIError
}

func (e *workflowHTTPError) Error() string { return e.body.Message }

type oauthWorkflowHTTPClient struct {
	tokenURL     string
	baseURL      string
	clientID     string
	clientSecret string
	client       *http.Client
	now          func() time.Time

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

type workflowTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

func ValidateWorkflowEnvironment() error {
	_, err := workflowEnvironment()
	return err
}

func NewWorkflowHTTPClientFromEnvironment(client *http.Client) (WorkflowHTTPClient, error) {
	configuration, err := workflowEnvironment()
	if err != nil {
		return nil, err
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &oauthWorkflowHTTPClient{
		tokenURL: configuration[0], baseURL: configuration[1], clientID: configuration[2], clientSecret: configuration[3],
		client: client, now: func() time.Time { return time.Now().UTC() },
	}, nil
}

func workflowEnvironment() ([4]string, error) {
	keys := [...]string{"WORKFLOW_OAUTH_TOKEN_URL", "WORKFLOW_API_BASE_URL", "WORKFLOW_CLIENT_ID", "WORKFLOW_CLIENT_SECRET"}
	values := [4]string{}
	for index, key := range keys {
		values[index] = strings.TrimSpace(os.Getenv(key))
		if values[index] == "" {
			return values, fmt.Errorf("%s is required", key)
		}
	}
	for index, key := range keys[:2] {
		parsed, err := url.Parse(values[index])
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return values, fmt.Errorf("%s must be an absolute HTTP or HTTPS URL", key)
		}
	}
	return values, nil
}

func (client *oauthWorkflowHTTPClient) Execute(ctx context.Context, action contract.WorkflowActionRequest, idempotencyKey string) (int, error) {
	token, status, err := client.token(ctx)
	if err != nil {
		return status, err
	}
	body, err := json.Marshal(action)
	if err != nil {
		return 0, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(client.baseURL, "/")+"/api/v1/actions", bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set(contract.IdempotencyHeader, idempotencyKey)
	response, err := client.client.Do(request)
	if err != nil {
		return 0, &workflowHTTPError{status: http.StatusServiceUnavailable, body: contract.MockWorkflowFailure(contract.ErrorTemporarilyUnavailable, true)}
	}
	defer response.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if readErr != nil {
		return response.StatusCode, &workflowHTTPError{status: response.StatusCode, body: contract.MockWorkflowFailure(contract.ErrorTemporarilyUnavailable, true)}
	}
	if response.StatusCode != http.StatusAccepted {
		failure := contract.MockWorkflowFailure(contract.ErrorOperationFailed, false)
		if json.Unmarshal(responseBody, &failure) != nil || failure.Error == "" {
			failure = contract.MockWorkflowFailure(contract.ErrorOperationFailed, response.StatusCode >= 500)
		}
		failure.Message = "The authenticated workflow action failed."
		return response.StatusCode, &workflowHTTPError{status: response.StatusCode, body: failure}
	}
	accepted := contract.WorkflowActionAccepted{}
	if json.Unmarshal(responseBody, &accepted) != nil || strings.TrimSpace(accepted.ExecutionID) == "" || accepted.AcceptedAt.IsZero() {
		return response.StatusCode, &workflowHTTPError{status: response.StatusCode, body: contract.MockWorkflowFailure(contract.ErrorOperationFailed, false)}
	}
	return response.StatusCode, nil
}

func (client *oauthWorkflowHTTPClient) token(ctx context.Context) (string, int, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	now := client.now()
	if client.accessToken != "" && now.Add(time.Second).Before(client.expiresAt) {
		return client.accessToken, 0, nil
	}
	form := url.Values{"grant_type": {"client_credentials"}, "scope": {contract.ScopeWorkflowExecute}}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.SetBasicAuth(client.clientID, client.clientSecret)
	response, err := client.client.Do(request)
	if err != nil {
		return "", http.StatusServiceUnavailable, &workflowHTTPError{status: http.StatusServiceUnavailable, body: contract.MockWorkflowFailure(contract.ErrorTemporarilyUnavailable, true)}
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if readErr != nil {
		return "", response.StatusCode, readErr
	}
	if response.StatusCode != http.StatusOK {
		failure := contract.MockWorkflowFailure(contract.ErrorInvalidClient, false)
		if json.Unmarshal(body, &failure) != nil || failure.Error == "" {
			failure = contract.MockWorkflowFailure(contract.ErrorInvalidClient, false)
		}
		failure.Message = "The authenticated workflow action failed."
		return "", response.StatusCode, &workflowHTTPError{status: response.StatusCode, body: failure}
	}
	token := workflowTokenResponse{}
	if json.Unmarshal(body, &token) != nil || token.AccessToken == "" || !strings.EqualFold(token.TokenType, "Bearer") || token.ExpiresIn <= 0 || !scopeContains(token.Scope, contract.ScopeWorkflowExecute) {
		return "", response.StatusCode, &workflowHTTPError{status: response.StatusCode, body: contract.MockWorkflowFailure(contract.ErrorInvalidToken, false)}
	}
	client.accessToken = token.AccessToken
	client.expiresAt = now.Add(time.Duration(token.ExpiresIn) * time.Second)
	return client.accessToken, 0, nil
}

func scopeContains(raw, expected string) bool {
	for _, value := range strings.Fields(raw) {
		if value == expected {
			return true
		}
	}
	return false
}

func (svc *Service) SetWorkflowHTTPClient(client WorkflowHTTPClient) {
	svc.workflowHTTP = client
	svc.workflowConfig = nil
}

func (svc *Service) CreateWorkflow(ctx context.Context, actor contract.Actor, input contract.WorkflowDefinition) (*contract.WorkflowDefinition, error) {
	if err := requireWorkflowDesigner(actor); err != nil {
		return nil, err
	}
	if err := validateWorkflowDefinition(input, true); err != nil {
		return nil, err
	}
	svc.workflowMu.Lock()
	defer svc.workflowMu.Unlock()
	now := svc.now()
	stored := &composeTypes.City311WorkflowDefinition{
		ID: svc.nextID(), WorkflowID: strings.TrimSpace(input.WorkflowID), Name: strings.TrimSpace(input.Name), Trigger: input.Trigger,
		Active: false, Definition: workflowDefinitionPayload(input), Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	err := store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		if _, lookupErr := store.LookupCity311WorkflowDefinitionByWorkflowID(ctx, tx, stored.WorkflowID); lookupErr == nil {
			return validationError(contract.FieldError{Field: "/workflow_id", Code: contract.ValidationInvalidValue})
		} else if !errors.IsNotFound(lookupErr) {
			return lookupErr
		}
		if createErr := store.CreateCity311WorkflowDefinition(ctx, tx, stored); createErr != nil {
			return createErr
		}
		return svc.auditWorkflow(ctx, tx, actor, stored, "WORKFLOW_CREATED", nil)
	})
	if err != nil {
		return nil, err
	}
	return workflowDefinitionFromStored(stored), nil
}

func (svc *Service) GetWorkflow(ctx context.Context, actor contract.Actor, workflowID string) (*contract.WorkflowDefinition, error) {
	if err := requireWorkflowDesigner(actor); err != nil {
		return nil, err
	}
	stored, err := svc.lookupWorkflow(ctx, svc.store, workflowID)
	if err != nil {
		return nil, err
	}
	return workflowDefinitionFromStored(stored), nil
}

func (svc *Service) ListWorkflows(ctx context.Context, actor contract.Actor, query WorkflowListQuery) (*contract.WorkflowDefinitionList, error) {
	if err := requireWorkflowDesigner(actor); err != nil {
		return nil, err
	}
	if query.Trigger != "" && query.Trigger != WorkflowTriggerCreated && query.Trigger != WorkflowTriggerStatusChanged {
		return nil, validationError(contract.FieldError{Field: "/query/filters/trigger", Code: contract.ValidationInvalidValue})
	}
	if query.PageSize == 0 {
		query.PageSize = 50
	}
	if query.PageSize > 100 {
		return nil, validationError(contract.FieldError{Field: "/query/page_size", Code: contract.ValidationOutOfRange})
	}
	offset, err := decodePageToken(query.PageToken, []string{workflowListSort})
	if err != nil {
		return nil, invalidPageToken()
	}
	set, _, err := store.SearchCity311WorkflowDefinitions(ctx, svc.store, composeTypes.City311WorkflowDefinitionFilter{})
	if err != nil {
		return nil, err
	}
	matching := make(composeTypes.City311WorkflowDefinitionSet, 0, len(set))
	for _, item := range set {
		if query.Trigger != "" && item.Trigger != query.Trigger {
			continue
		}
		if query.Active != nil && item.Active != *query.Active {
			continue
		}
		matching = append(matching, item)
	}
	sort.Slice(matching, func(i, j int) bool {
		if matching[i].UpdatedAt.Equal(matching[j].UpdatedAt) {
			return matching[i].WorkflowID < matching[j].WorkflowID
		}
		return matching[i].UpdatedAt.After(matching[j].UpdatedAt)
	})
	if offset > len(matching) {
		return nil, invalidPageToken()
	}
	end := offset + int(query.PageSize)
	if end > len(matching) {
		end = len(matching)
	}
	response := &contract.WorkflowDefinitionList{
		Items: make([]contract.WorkflowDefinition, 0, end-offset), TotalCount: len(matching),
		AppliedFilters: map[string]any{}, Sort: []string{workflowListSort},
	}
	if query.Trigger != "" {
		response.AppliedFilters["trigger"] = query.Trigger
	}
	if query.Active != nil {
		response.AppliedFilters["active"] = *query.Active
	}
	for _, item := range matching[offset:end] {
		response.Items = append(response.Items, *workflowDefinitionFromStored(item))
	}
	if end < len(matching) {
		next, encodeErr := encodePageToken(end, []string{workflowListSort})
		if encodeErr != nil {
			return nil, encodeErr
		}
		response.NextPageToken = &next
	}
	return response, nil
}

func (svc *Service) UpdateWorkflow(ctx context.Context, actor contract.Actor, workflowID string, expectedVersion uint64, input contract.WorkflowDefinition) (*contract.WorkflowDefinition, error) {
	if err := requireWorkflowDesigner(actor); err != nil {
		return nil, err
	}
	if expectedVersion == 0 {
		return nil, expectedVersionRequired()
	}
	input.WorkflowID = strings.TrimSpace(workflowID)
	if err := validateWorkflowDefinition(input, false); err != nil {
		return nil, err
	}
	svc.workflowMu.Lock()
	defer svc.workflowMu.Unlock()
	var stored *composeTypes.City311WorkflowDefinition
	err := store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		current, lookupErr := svc.lookupWorkflow(ctx, tx, workflowID)
		if lookupErr != nil {
			return lookupErr
		}
		if uint64(current.Version) != expectedVersion {
			return versionConflict(current.Version)
		}
		before := workflowDefinitionFromStored(current)
		current.Name = strings.TrimSpace(input.Name)
		current.Trigger = input.Trigger
		current.Definition = workflowDefinitionPayload(input)
		current.Version++
		current.UpdatedAt = svc.now()
		if updateErr := store.UpdateCity311WorkflowDefinition(ctx, tx, current); updateErr != nil {
			return updateErr
		}
		stored = current
		return svc.auditWorkflow(ctx, tx, actor, current, "WORKFLOW_UPDATED", before)
	})
	if err != nil {
		return nil, err
	}
	return workflowDefinitionFromStored(stored), nil
}

func (svc *Service) SetWorkflowActive(ctx context.Context, actor contract.Actor, workflowID string, expectedVersion uint64, active bool) (*contract.WorkflowDefinition, error) {
	if err := requireWorkflowDesigner(actor); err != nil {
		return nil, err
	}
	if expectedVersion == 0 {
		return nil, expectedVersionRequired()
	}
	svc.workflowMu.Lock()
	defer svc.workflowMu.Unlock()
	var stored *composeTypes.City311WorkflowDefinition
	err := store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		current, lookupErr := svc.lookupWorkflow(ctx, tx, workflowID)
		if lookupErr != nil {
			return lookupErr
		}
		if uint64(current.Version) != expectedVersion {
			return versionConflict(current.Version)
		}
		before := workflowDefinitionFromStored(current)
		current.Active = active
		current.Version++
		current.UpdatedAt = svc.now()
		if updateErr := store.UpdateCity311WorkflowDefinition(ctx, tx, current); updateErr != nil {
			return updateErr
		}
		stored = current
		eventType := "WORKFLOW_DEACTIVATED"
		if active {
			eventType = "WORKFLOW_ACTIVATED"
		}
		return svc.auditWorkflow(ctx, tx, actor, current, eventType, before)
	})
	if err != nil {
		return nil, err
	}
	return workflowDefinitionFromStored(stored), nil
}

func (svc *Service) TestWorkflow(ctx context.Context, actor contract.Actor, workflowID, requestID string) (*contract.Operation, error) {
	if err := requireWorkflowDesigner(actor); err != nil {
		return nil, err
	}
	definition, err := svc.lookupWorkflow(ctx, svc.store, workflowID)
	if err != nil {
		return nil, err
	}
	parsedRequestID, err := strconv.ParseUint(strings.TrimSpace(requestID), 10, 64)
	if err != nil || parsedRequestID == 0 {
		return nil, validationError(contract.FieldError{Field: "/request_id", Code: contract.ValidationInvalidFormat})
	}
	request, err := store.LookupCity311ServiceRequestByID(ctx, svc.store, parsedRequestID)
	if errors.IsNotFound(err) {
		return nil, apiError(http.StatusNotFound, contract.ErrorNotFound, "The service request was not found.")
	}
	if err != nil {
		return nil, err
	}
	if !canRead(actor, request) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, requestScopeDeniedMessage)
	}

	svc.workflowMu.Lock()
	defer svc.workflowMu.Unlock()
	now := svc.now()
	operation := &composeTypes.City311Operation{
		ID: svc.nextID(), Kind: workflowTestOperationKind, Status: string(contract.OperationStatusPending), ActorID: actor.ID,
		Result: composeTypes.City311JSON{}, Error: composeTypes.City311JSON{}, CreatedAt: now, UpdatedAt: now,
	}
	pending := toOperation(operation)
	if err = store.CreateCity311Operation(ctx, svc.store, operation); err != nil {
		return nil, err
	}
	execution, executionErr := svc.executeWorkflow(ctx, actor, definition, request, "TEST")
	completedAt := svc.now()
	operation.Progress = 100
	operation.UpdatedAt = completedAt
	operation.CompletedAt = &completedAt
	if execution != nil {
		operation.Result = composeTypes.City311JSON{"execution_id": execution.ExecutionID}
	}
	if executionErr != nil {
		operation.Status = string(contract.OperationStatusFailed)
		operation.Error = composeTypes.City311JSON{"error": string(contract.ErrorOperationFailed), "message": "The workflow test failed.", "retryable": false}
	} else {
		operation.Status = string(contract.OperationStatusSucceeded)
	}
	if updateErr := store.UpdateCity311Operation(ctx, svc.store, operation); updateErr != nil {
		return nil, updateErr
	}
	return pending, nil
}

func (svc *Service) GetWorkflowExecution(ctx context.Context, actor contract.Actor, executionID string) (*contract.WorkflowExecution, error) {
	if err := requireWorkflowDesigner(actor); err != nil {
		return nil, err
	}
	stored, err := store.LookupCity311WorkflowExecutionByExecutionID(ctx, svc.store, strings.TrimSpace(executionID))
	if errors.IsNotFound(err) {
		return nil, apiError(http.StatusNotFound, contract.ErrorNotFound, "The workflow execution was not found.")
	}
	if err != nil {
		return nil, err
	}
	return workflowExecutionFromStored(stored), nil
}

func (svc *Service) ListWorkflowExecutions(ctx context.Context, actor contract.Actor, query WorkflowExecutionQuery) (*contract.WorkflowExecutionList, error) {
	if err := requireWorkflowDesigner(actor); err != nil {
		return nil, err
	}
	if query.PageSize == 0 {
		query.PageSize = 50
	}
	if query.PageSize > 100 {
		return nil, validationError(contract.FieldError{Field: "/query/page_size", Code: contract.ValidationOutOfRange})
	}
	offset, err := decodePageToken(query.PageToken, []string{workflowListSort})
	if err != nil {
		return nil, invalidPageToken()
	}
	set, _, err := store.SearchCity311WorkflowExecutions(ctx, svc.store, composeTypes.City311WorkflowExecutionFilter{})
	if err != nil {
		return nil, err
	}
	matching := make(composeTypes.City311WorkflowExecutionSet, 0, len(set))
	for _, item := range set {
		if query.WorkflowID != "" && item.WorkflowID != query.WorkflowID {
			continue
		}
		if query.RequestID != 0 && item.RequestID != query.RequestID {
			continue
		}
		if query.Succeeded != nil && item.Succeeded != *query.Succeeded {
			continue
		}
		matching = append(matching, item)
	}
	sort.Slice(matching, func(i, j int) bool {
		if matching[i].OccurredAt.Equal(matching[j].OccurredAt) {
			return matching[i].ExecutionID < matching[j].ExecutionID
		}
		return matching[i].OccurredAt.After(matching[j].OccurredAt)
	})
	if offset > len(matching) {
		return nil, invalidPageToken()
	}
	end := offset + int(query.PageSize)
	if end > len(matching) {
		end = len(matching)
	}
	response := &contract.WorkflowExecutionList{
		Items: make([]contract.WorkflowExecution, 0, end-offset), TotalCount: len(matching), AppliedFilters: map[string]any{}, Sort: []string{workflowListSort},
	}
	for _, item := range matching[offset:end] {
		response.Items = append(response.Items, *workflowExecutionFromStored(item))
	}
	if query.WorkflowID != "" {
		response.AppliedFilters["workflow_id"] = query.WorkflowID
	}
	if query.RequestID != 0 {
		response.AppliedFilters["request_id"] = strconv.FormatUint(query.RequestID, 10)
	}
	if query.Succeeded != nil {
		response.AppliedFilters["succeeded"] = *query.Succeeded
	}
	if end < len(matching) {
		next, encodeErr := encodePageToken(end, []string{workflowListSort})
		if encodeErr != nil {
			return nil, encodeErr
		}
		response.NextPageToken = &next
	}
	return response, nil
}

func (svc *Service) runActiveWorkflows(ctx context.Context, actor contract.Actor, trigger string, request *composeTypes.City311ServiceRequest) {
	set, _, err := store.SearchCity311WorkflowDefinitions(ctx, svc.store, composeTypes.City311WorkflowDefinitionFilter{})
	if err != nil {
		return
	}
	for _, definition := range set {
		if definition.Active && definition.Trigger == trigger {
			_, _ = svc.executeWorkflow(ctx, actor, definition, request, "LIVE")
		}
	}
}

func (svc *Service) executeWorkflow(ctx context.Context, actor contract.Actor, definition *composeTypes.City311WorkflowDefinition, request *composeTypes.City311ServiceRequest, mode string) (*contract.WorkflowExecution, error) {
	input := workflowDefinitionFromStored(definition)
	executionID := "wfx-" + strconv.FormatUint(svc.nextID(), 10)
	actionsAttempted := make([]string, 0, len(input.Actions))
	outcome := "CONDITIONS_NOT_MATCHED"
	succeeded := true
	responseStatus := 0
	var executionError error

	if workflowConditionsMatch(input.Conditions, actor, request) {
		outcome = "CONDITIONS_MATCHED"
		for index, action := range input.Actions {
			kind := workflowActionKind(action)
			actionsAttempted = append(actionsAttempted, kind)
			status, actionErr := svc.executeWorkflowAction(ctx, actor, definition, request, index, kind, action)
			if status != 0 {
				responseStatus = status
			}
			if actionErr != nil {
				succeeded = false
				outcome = "ACTION_FAILED"
				executionError = actionErr
				break
			}
		}
		if succeeded {
			outcome = "ACTIONS_SUCCEEDED"
		}
	}

	now := svc.now()
	stored := &composeTypes.City311WorkflowExecution{
		ID: svc.nextID(), ExecutionID: executionID, WorkflowID: definition.WorkflowID, WorkflowVersion: definition.Version,
		RequestID: request.ID, Trigger: definition.Trigger, Outcome: outcome,
		ActionsAttempted: composeTypes.City311JSON{"items": actionsAttempted}, Succeeded: succeeded,
		ResponseStatus: responseStatus, Error: composeTypes.City311JSON{}, OccurredAt: now,
	}
	if executionError != nil {
		stored.Error = workflowErrorPayload(executionError)
	}
	persistErr := store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		if err := store.CreateCity311WorkflowExecution(ctx, tx, stored); err != nil {
			return err
		}
		return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
			ID: svc.nextID(), RequestID: request.ID, EntityType: "workflow_execution", EntityID: executionID, EventType: "WORKFLOW_EXECUTED",
			ActorType: contract.AuditActorStaff, ActorID: actor.ID, SourceChannel: contract.SourceChannelStaffInPerson,
			Before: composeTypes.City311JSON{}, After: composeTypes.City311JSON{
				"workflow_id": definition.WorkflowID, "workflow_version": definition.Version, "trigger": definition.Trigger,
				"mode": mode, "outcome": outcome, "succeeded": succeeded,
			}, CreatedAt: now,
		})
	})
	if persistErr != nil {
		return nil, persistErr
	}
	return workflowExecutionFromStored(stored), executionError
}

func (svc *Service) executeWorkflowAction(ctx context.Context, actor contract.Actor, definition *composeTypes.City311WorkflowDefinition, request *composeTypes.City311ServiceRequest, index int, kind string, action map[string]any) (int, error) {
	switch kind {
	case "FIELD_UPDATE":
		return 0, svc.workflowFieldUpdate(ctx, actor, request, action)
	case "ASSIGNMENT":
		return 0, svc.workflowAssignment(ctx, actor, request, action)
	case "NOTIFICATION":
		return 0, svc.workflowNotification(ctx, actor, definition, request, index, action)
	case "AUTHENTICATED_HTTP":
		if svc.workflowConfig != nil || svc.workflowHTTP == nil {
			return http.StatusServiceUnavailable, &workflowHTTPError{status: http.StatusServiceUnavailable, body: contract.MockWorkflowFailure(contract.ErrorTemporarilyUnavailable, true)}
		}
		payload, _ := action["payload"].(map[string]any)
		actionName, _ := stringValue(action, "action")
		key := fmt.Sprintf("workflow:%s:%d:%d:%d", definition.WorkflowID, definition.Version, request.ID, index)
		return svc.workflowHTTP.Execute(ctx, contract.WorkflowActionRequest{Action: actionName, RequestID: strconv.FormatUint(request.ID, 10), Payload: payload}, key)
	default:
		return 0, validationError(contract.FieldError{Field: fmt.Sprintf("/actions/%d/type", index), Code: contract.ValidationInvalidValue})
	}
}

func (svc *Service) workflowFieldUpdate(ctx context.Context, actor contract.Actor, request *composeTypes.City311ServiceRequest, action map[string]any) error {
	field, _ := stringValue(action, "field")
	value := action["value"]
	return store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		current, err := store.LookupCity311ServiceRequestByID(ctx, tx, request.ID)
		if err != nil {
			return err
		}
		before := requestSnapshot(current)
		switch {
		case field == "summary":
			text, ok := value.(string)
			if !ok || len(validateBoundedText(text, "/actions/value", 5, 160)) > 0 {
				return validationError(contract.FieldError{Field: "/actions/value", Code: contract.ValidationInvalidValue})
			}
			current.Summary = strings.TrimSpace(text)
		case field == "description":
			text, ok := value.(string)
			if !ok || len(validateBoundedText(text, "/actions/value", 10, 5000)) > 0 {
				return validationError(contract.FieldError{Field: "/actions/value", Code: contract.ValidationInvalidValue})
			}
			current.Description = strings.TrimSpace(text)
		case strings.HasPrefix(field, "custom_fields.") && len(strings.TrimPrefix(field, "custom_fields.")) > 0:
			key := strings.TrimPrefix(field, "custom_fields.")
			definitions, definitionErr := svc.customFieldDefinitions(ctx, tx, "service_request")
			if definitionErr != nil {
				return definitionErr
			}
			if len(definitions) > 0 {
				var definition *contract.CustomFieldDefinition
				for index := range definitions {
					if definitions[index].Key == key {
						definition = &definitions[index]
						break
					}
				}
				if definition == nil {
					return validationError(contract.FieldError{Field: "/actions/field", Code: contract.ValidationInvalidValue})
				}
				if !definition.Active {
					return validationError(contract.FieldError{Field: "/actions/field", Code: contract.ValidationInactiveValue})
				}
				if code := validateCustomFieldValue(*definition, value); code != "" {
					return validationError(contract.FieldError{Field: "/actions/value", Code: code})
				}
			}
			if current.CustomFields == nil {
				current.CustomFields = composeTypes.City311JSON{}
			}
			current.CustomFields[key] = value
		default:
			return validationError(contract.FieldError{Field: "/actions/field", Code: contract.ValidationInvalidValue})
		}
		current.Version++
		current.UpdatedAt = svc.now()
		if err = store.UpdateCity311ServiceRequest(ctx, tx, current); err != nil {
			return err
		}
		*request = *current
		return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
			ID: svc.nextID(), RequestID: current.ID, EntityType: "service_request", EntityID: strconv.FormatUint(current.ID, 10), EventType: "WORKFLOW_FIELD_UPDATED",
			ActorType: contract.AuditActorStaff, ActorID: actor.ID, SourceChannel: contract.SourceChannelStaffInPerson,
			Before: before, After: requestSnapshot(current), CreatedAt: current.UpdatedAt,
		})
	})
}

func (svc *Service) workflowAssignment(ctx context.Context, actor contract.Actor, request *composeTypes.City311ServiceRequest, action map[string]any) error {
	raw, _ := stringValue(action, "assignee_id")
	assigneeID, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || assigneeID == 0 {
		return validationError(contract.FieldError{Field: "/actions/assignee_id", Code: contract.ValidationInvalidFormat})
	}
	target, err := svc.FindActor(ctx, assigneeID)
	if err != nil || !canOperateRequest(target) || !canRead(target, request) {
		return apiError(http.StatusForbidden, contract.ErrorForbidden, "The workflow assignment target is not authorised for the request.")
	}
	return store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		current, lookupErr := store.LookupCity311ServiceRequestByID(ctx, tx, request.ID)
		if lookupErr != nil {
			return lookupErr
		}
		before := requestSnapshot(current)
		current.PrimaryAssigneeID = assigneeID
		current.Version++
		current.UpdatedAt = svc.now()
		if updateErr := store.UpdateCity311ServiceRequest(ctx, tx, current); updateErr != nil {
			return updateErr
		}
		*request = *current
		return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
			ID: svc.nextID(), RequestID: current.ID, EntityType: "service_request", EntityID: strconv.FormatUint(current.ID, 10), EventType: "WORKFLOW_ASSIGNED",
			ActorType: contract.AuditActorStaff, ActorID: actor.ID, SourceChannel: contract.SourceChannelStaffInPerson,
			Before: before, After: requestSnapshot(current), CreatedAt: current.UpdatedAt,
		})
	})
}

func (svc *Service) workflowNotification(ctx context.Context, actor contract.Actor, definition *composeTypes.City311WorkflowDefinition, request *composeTypes.City311ServiceRequest, index int, action map[string]any) error {
	to, _ := stringValue(action, "to")
	subject, _ := stringValue(action, "subject")
	text, _ := stringValue(action, "text")
	htmlBody, _ := stringValue(action, "html")
	templateID, _ := stringValue(action, "template_id")
	input := contract.MailCompose{To: []string{to}, Subject: subject, Text: text, HTML: htmlBody}
	if templateID != "" {
		input.TemplateID = &templateID
	}
	key := fmt.Sprintf("workflow-mail:%s:%d:%d:%d", definition.WorkflowID, definition.Version, request.ID, index)
	_, err := svc.SendMail(ctx, actor, key, input)
	return err
}

func workflowConditionsMatch(conditions []map[string]any, actor contract.Actor, request *composeTypes.City311ServiceRequest) bool {
	for _, condition := range conditions {
		kind := strings.ToUpper(strings.TrimSpace(anyString(condition["type"])))
		if kind == "ACTOR_ROLE" || condition["actor_role"] != nil {
			role := anyString(condition["actor_role"])
			if role == "" {
				role = anyString(condition["value"])
			}
			if !hasRole(actor, contract.ApplicationRole(role)) {
				return false
			}
			continue
		}
		field := anyString(condition["field"])
		actual, present := workflowFieldValue(field, request)
		operator := strings.ToUpper(strings.TrimSpace(anyString(condition["operator"])))
		if operator == "" {
			operator = "EQUALS"
		}
		switch operator {
		case "EQ", "EQUALS":
			if !workflowValuesEqual(actual, condition["value"]) {
				return false
			}
		case "NE", "NOT_EQUALS":
			if workflowValuesEqual(actual, condition["value"]) {
				return false
			}
		case "IN":
			values := workflowConditionValues(condition["values"])
			if len(values) == 0 {
				values = workflowConditionValues(condition["value"])
			}
			matched := false
			for _, value := range values {
				matched = matched || workflowValuesEqual(actual, value)
			}
			if !matched {
				return false
			}
		case "EXISTS":
			if !present {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func workflowConditionValues(value any) []any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	values := []any{}
	if json.Unmarshal(encoded, &values) != nil {
		return nil
	}
	return values
}

func workflowFieldValue(field string, request *composeTypes.City311ServiceRequest) (any, bool) {
	switch field {
	case "summary":
		return request.Summary, true
	case "description":
		return request.Description, true
	case "status":
		return string(request.Status), true
	case "service_type":
		return string(request.ServiceType), true
	case "department", "owning_department":
		return string(request.OwningDepartment), true
	case "district", "council_district":
		return string(request.CouncilDistrict), request.CouncilDistrict != ""
	case "source_channel":
		return string(request.SourceChannel), true
	case "origin_class":
		return string(request.OriginClass), true
	default:
		if strings.HasPrefix(field, "custom_fields.") {
			value, present := request.CustomFields[strings.TrimPrefix(field, "custom_fields.")]
			return value, present
		}
		return nil, false
	}
}

func workflowValuesEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func validateWorkflowDefinition(input contract.WorkflowDefinition, creating bool) error {
	var fields []contract.FieldError
	if !validResourceKey(input.WorkflowID) {
		fields = append(fields, contract.FieldError{Field: "/workflow_id", Code: contract.ValidationInvalidFormat})
	}
	if length := utf8.RuneCountInString(strings.TrimSpace(input.Name)); length < 1 || length > 120 {
		fields = append(fields, contract.FieldError{Field: "/name", Code: contract.ValidationInvalidValue})
	}
	if input.Trigger != WorkflowTriggerCreated && input.Trigger != WorkflowTriggerStatusChanged {
		fields = append(fields, contract.FieldError{Field: "/trigger", Code: contract.ValidationInvalidValue})
	}
	if creating && input.Active {
		fields = append(fields, contract.FieldError{Field: "/active", Code: contract.ValidationInvalidValue})
	}
	if len(input.Conditions) > 20 {
		fields = append(fields, contract.FieldError{Field: "/conditions", Code: contract.ValidationTooManyItems})
	}
	if len(input.Actions) == 0 {
		fields = append(fields, contract.FieldError{Field: "/actions", Code: contract.ValidationRequired})
	} else if len(input.Actions) > 20 {
		fields = append(fields, contract.FieldError{Field: "/actions", Code: contract.ValidationTooManyItems})
	}
	for index, condition := range input.Conditions {
		if !validWorkflowCondition(condition) {
			fields = append(fields, contract.FieldError{Field: fmt.Sprintf("/conditions/%d", index), Code: contract.ValidationInvalidValue})
		}
	}
	for index, action := range input.Actions {
		if !validWorkflowAction(action) {
			fields = append(fields, contract.FieldError{Field: fmt.Sprintf("/actions/%d", index), Code: contract.ValidationInvalidValue})
		}
	}
	if len(fields) > 0 {
		return validationError(fields...)
	}
	return nil
}

func validWorkflowCondition(condition map[string]any) bool {
	kind := strings.ToUpper(strings.TrimSpace(anyString(condition["type"])))
	if kind == "ACTOR_ROLE" || condition["actor_role"] != nil {
		role := anyString(condition["actor_role"])
		if role == "" {
			role = anyString(condition["value"])
		}
		return validApplicationRole(contract.ApplicationRole(role))
	}
	field := anyString(condition["field"])
	if !validWorkflowField(field) {
		return false
	}
	operator := strings.ToUpper(strings.TrimSpace(anyString(condition["operator"])))
	if operator == "" {
		operator = "EQUALS"
	}
	return operator == "EQ" || operator == "EQUALS" || operator == "NE" || operator == "NOT_EQUALS" || operator == "IN" || operator == "EXISTS"
}

func validWorkflowAction(action map[string]any) bool {
	switch workflowActionKind(action) {
	case "FIELD_UPDATE":
		return validWorkflowField(anyString(action["field"])) && action["value"] != nil
	case "ASSIGNMENT":
		value, ok := stringValue(action, "assignee_id")
		parsed, err := strconv.ParseUint(value, 10, 64)
		return ok && err == nil && parsed > 0
	case "NOTIFICATION":
		to, hasTo := stringValue(action, "to")
		subject, hasSubject := stringValue(action, "subject")
		text, hasText := stringValue(action, "text")
		return hasTo && hasSubject && hasText && to != "" && subject != "" && text != ""
	case "AUTHENTICATED_HTTP":
		actionName, ok := stringValue(action, "action")
		_, payloadOK := action["payload"].(map[string]any)
		return ok && actionName != "" && payloadOK
	default:
		return false
	}
}

func workflowActionKind(action map[string]any) string {
	value := anyString(action["type"])
	if value == "" {
		value = anyString(action["kind"])
	}
	value = strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(value), "-", "_"), " ", "_"))
	switch value {
	case "HTTP", "OAUTH_HTTP", "AUTHENTICATED_HTTP_ACTION":
		return "AUTHENTICATED_HTTP"
	case "ASSIGN", "ASSIGNEE":
		return "ASSIGNMENT"
	case "NOTIFY", "EMAIL":
		return "NOTIFICATION"
	case "UPDATE_FIELD":
		return "FIELD_UPDATE"
	default:
		return value
	}
}

func validWorkflowField(field string) bool {
	switch field {
	case "summary", "description", "status", "service_type", "department", "owning_department", "district", "council_district", "source_channel", "origin_class":
		return true
	default:
		return strings.HasPrefix(field, "custom_fields.") && validResourceKey(strings.TrimPrefix(field, "custom_fields."))
	}
}

func validResourceKey(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' {
			return false
		}
	}
	return true
}

func validApplicationRole(role contract.ApplicationRole) bool {
	for _, candidate := range contract.ApplicationRoles {
		if candidate == role {
			return true
		}
	}
	return false
}

func workflowDefinitionPayload(input contract.WorkflowDefinition) composeTypes.City311JSON {
	return composeTypes.City311JSON{"conditions": input.Conditions, "actions": input.Actions}
}

func workflowDefinitionFromStored(stored *composeTypes.City311WorkflowDefinition) *contract.WorkflowDefinition {
	conditions := decodeObjectList(stored.Definition["conditions"])
	actions := decodeObjectList(stored.Definition["actions"])
	return &contract.WorkflowDefinition{
		WorkflowID: stored.WorkflowID, Name: stored.Name, Trigger: stored.Trigger, Active: stored.Active,
		Conditions: conditions, Actions: actions, Version: uint64(stored.Version), UpdatedAt: stored.UpdatedAt,
	}
}

func decodeObjectList(value any) []map[string]any {
	encoded, err := json.Marshal(value)
	if err != nil {
		return []map[string]any{}
	}
	out := []map[string]any{}
	if json.Unmarshal(encoded, &out) != nil {
		return []map[string]any{}
	}
	return out
}

func workflowExecutionFromStored(stored *composeTypes.City311WorkflowExecution) *contract.WorkflowExecution {
	actions := []string{}
	if raw, present := stored.ActionsAttempted["items"]; present {
		encoded, _ := json.Marshal(raw)
		_ = json.Unmarshal(encoded, &actions)
	}
	result := &contract.WorkflowExecution{
		ExecutionID: stored.ExecutionID, WorkflowVersion: uint64(stored.WorkflowVersion), Trigger: stored.Trigger,
		Outcome: stored.Outcome, ActionsAttempted: actions, Succeeded: stored.Succeeded, OccurredAt: stored.OccurredAt,
	}
	if stored.ResponseStatus != 0 {
		status := stored.ResponseStatus
		result.ResponseStatus = &status
	}
	if len(stored.Error) > 0 {
		encoded, _ := json.Marshal(stored.Error)
		failure := &contract.APIError{}
		if json.Unmarshal(encoded, failure) == nil && failure.Error != "" {
			result.Error = failure
		}
	}
	return result
}

func workflowErrorPayload(err error) composeTypes.City311JSON {
	failure := contract.MockWorkflowFailure(contract.ErrorOperationFailed, false)
	if typed, ok := err.(*workflowHTTPError); ok {
		failure = typed.body
	} else if typed, ok := err.(*ServiceError); ok {
		failure = typed.Payload
	}
	encoded, _ := json.Marshal(failure)
	payload := composeTypes.City311JSON{}
	_ = json.Unmarshal(encoded, &payload)
	return payload
}

func (svc *Service) lookupWorkflow(ctx context.Context, st store.Storer, workflowID string) (*composeTypes.City311WorkflowDefinition, error) {
	workflowID = strings.TrimSpace(workflowID)
	if !validResourceKey(workflowID) {
		return nil, validationError(contract.FieldError{Field: "/path/workflow_id", Code: contract.ValidationInvalidFormat})
	}
	stored, err := store.LookupCity311WorkflowDefinitionByWorkflowID(ctx, st, workflowID)
	if errors.IsNotFound(err) {
		return nil, apiError(http.StatusNotFound, contract.ErrorNotFound, "The workflow was not found.")
	}
	return stored, err
}

func (svc *Service) auditWorkflow(ctx context.Context, tx store.Storer, actor contract.Actor, stored *composeTypes.City311WorkflowDefinition, eventType string, before *contract.WorkflowDefinition) error {
	beforeMap := composeTypes.City311JSON{}
	if before != nil {
		beforeMap, _ = mapFrom(before)
	}
	after, err := mapFrom(workflowDefinitionFromStored(stored))
	if err != nil {
		return err
	}
	return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
		ID: svc.nextID(), EntityType: "workflow", EntityID: stored.WorkflowID, EventType: eventType,
		ActorType: contract.AuditActorStaff, ActorID: actor.ID, SourceChannel: contract.SourceChannelStaffInPerson,
		Before: beforeMap, After: after, CreatedAt: stored.UpdatedAt,
	})
}

func requireWorkflowDesigner(actor contract.Actor) error {
	if !hasRole(actor, contract.ApplicationRoleWorkflowDesigner) {
		return apiError(http.StatusForbidden, contract.ErrorForbidden, "A workflow designer role is required.")
	}
	return nil
}

func expectedVersionRequired() *ServiceError {
	return apiError(http.StatusPreconditionRequired, contract.ErrorExpectedVersionRequired, "If-Match is required for this update.")
}

func stringValue(values map[string]any, key string) (string, bool) {
	value, ok := values[key].(string)
	return strings.TrimSpace(value), ok
}

func anyString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}
