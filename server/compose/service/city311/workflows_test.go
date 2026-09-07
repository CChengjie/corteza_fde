package city311

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

type workflowClientStub struct {
	status int
	err    error
	keys   []string
	inputs []contract.WorkflowActionRequest
}

func (stub *workflowClientStub) Execute(_ context.Context, input contract.WorkflowActionRequest, key string) (int, error) {
	stub.keys = append(stub.keys, key)
	stub.inputs = append(stub.inputs, input)
	return stub.status, stub.err
}

func workflowDesignerActor() contract.Actor {
	return contract.Actor{ID: 700, Roles: []contract.ApplicationRole{contract.ApplicationRoleWorkflowDesigner, contract.ApplicationRolePlatformAdministrator}}
}

func validWorkflowDefinition(id string, actions ...map[string]any) contract.WorkflowDefinition {
	return contract.WorkflowDefinition{
		WorkflowID: id, Name: "Escalate matching requests", Trigger: WorkflowTriggerCreated,
		Conditions: []map[string]any{{"field": "service_type", "operator": "EQUALS", "value": "POTHOLE"}},
		Actions:    actions,
	}
}

func TestWorkflowDefinitionLifecyclePaginationAndAudit(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	actor := workflowDesignerActor()

	first, err := svc.CreateWorkflow(ctx, actor, validWorkflowDefinition("wf-escalate", map[string]any{
		"type": "FIELD_UPDATE", "field": "custom_fields.priority", "value": "HIGH",
	}))
	require.NoError(t, err)
	require.False(t, first.Active)
	require.Equal(t, uint64(1), first.Version)

	_, err = svc.CreateWorkflow(ctx, actor, validWorkflowDefinition("wf-escalate", map[string]any{
		"type": "FIELD_UPDATE", "field": "custom_fields.priority", "value": "HIGH",
	}))
	requireServiceError(t, err, http.StatusUnprocessableEntity, contract.ErrorValidation)
	_, err = svc.CreateWorkflow(ctx, contract.Actor{Roles: []contract.ApplicationRole{contract.ApplicationRoleServiceAgent}}, validWorkflowDefinition("wf-forbidden", map[string]any{
		"type": "FIELD_UPDATE", "field": "custom_fields.priority", "value": "HIGH",
	}))
	requireServiceError(t, err, http.StatusForbidden, contract.ErrorForbidden)

	activated, err := svc.SetWorkflowActive(ctx, actor, first.WorkflowID, first.Version, true)
	require.NoError(t, err)
	require.True(t, activated.Active)
	require.Equal(t, uint64(2), activated.Version)
	_, err = svc.SetWorkflowActive(ctx, actor, first.WorkflowID, first.Version, false)
	requireServiceError(t, err, http.StatusConflict, contract.ErrorVersionConflict)

	updatedInput := *activated
	updatedInput.Name = "Updated workflow name"
	updatedInput.Trigger = WorkflowTriggerStatusChanged
	updatedInput.Conditions = []map[string]any{{"type": "ACTOR_ROLE", "value": "service_agent"}}
	updatedInput.Actions = []map[string]any{{"type": "AUTHENTICATED_HTTP", "action": "notify_dispatch", "payload": map[string]any{"priority": "HIGH"}}}
	updated, err := svc.UpdateWorkflow(ctx, actor, activated.WorkflowID, activated.Version, updatedInput)
	require.NoError(t, err)
	require.True(t, updated.Active)
	require.Equal(t, uint64(3), updated.Version)

	second, err := svc.CreateWorkflow(ctx, actor, validWorkflowDefinition("wf-second", map[string]any{
		"type": "FIELD_UPDATE", "field": "description", "value": "Updated by the second workflow action.",
	}))
	require.NoError(t, err)
	active := true
	page, err := svc.ListWorkflows(ctx, actor, WorkflowListQuery{Active: &active, PageSize: 1})
	require.NoError(t, err)
	require.Equal(t, 1, page.TotalCount)
	require.Len(t, page.Items, 1)
	require.Nil(t, page.NextPageToken)
	allPage, err := svc.ListWorkflows(ctx, actor, WorkflowListQuery{PageSize: 1})
	require.NoError(t, err)
	require.Equal(t, 2, allPage.TotalCount)
	require.NotNil(t, allPage.NextPageToken)
	next, err := svc.ListWorkflows(ctx, actor, WorkflowListQuery{PageSize: 1, PageToken: *allPage.NextPageToken})
	require.NoError(t, err)
	require.Len(t, next.Items, 1)
	require.NotEqual(t, allPage.Items[0].WorkflowID, next.Items[0].WorkflowID)
	_, err = svc.ListWorkflows(ctx, actor, WorkflowListQuery{PageToken: "invalid"})
	requireServiceError(t, err, http.StatusBadRequest, contract.ErrorInvalidPageToken)

	fetched, err := svc.GetWorkflow(ctx, actor, second.WorkflowID)
	require.NoError(t, err)
	require.Equal(t, second, fetched)
	deactivated, err := svc.SetWorkflowActive(ctx, actor, updated.WorkflowID, updated.Version, false)
	require.NoError(t, err)
	require.False(t, deactivated.Active)

	audits, _, err := store.SearchCity311AuditEvents(ctx, st, composeTypes.City311AuditEventFilter{EntityType: "workflow", EntityID: first.WorkflowID})
	require.NoError(t, err)
	require.Len(t, audits, 4)
	require.Equal(t, "WORKFLOW_DEACTIVATED", audits[len(audits)-1].EventType)
}

func TestWorkflowTestExecutesFieldAssignmentNotificationAndHTTPActions(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	actor := workflowDesignerActor()
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	agent, err := store.LookupUserByEmail(ctx, st, "service-agent@city311.example.invalid")
	require.NoError(t, err)
	client := &workflowClientStub{status: http.StatusAccepted}
	svc.SetWorkflowHTTPClient(client)
	mailSender := &scriptedMailSender{}
	svc.SetMailSender(mailSender)

	definition := validWorkflowDefinition("wf-multi-action",
		map[string]any{"type": "FIELD_UPDATE", "field": "custom_fields.priority", "value": "HIGH"},
		map[string]any{"type": "ASSIGNMENT", "assignee_id": strconv.FormatUint(agent.ID, 10)},
		map[string]any{"type": "NOTIFICATION", "to": "dispatch@example.invalid", "subject": "Dispatch", "text": "A request needs attention."},
		map[string]any{"type": "AUTHENTICATED_HTTP", "action": "dispatch", "payload": map[string]any{"priority": "HIGH"}},
	)
	created, err := svc.CreateWorkflow(ctx, actor, definition)
	require.NoError(t, err)
	pending, err := svc.TestWorkflow(ctx, actor, created.WorkflowID, strconv.FormatUint(request.ID, 10))
	require.NoError(t, err)
	require.Equal(t, contract.OperationStatusPending, pending.Status)

	operation, err := svc.GetOperation(ctx, actor, pending.OperationID)
	require.NoError(t, err)
	require.Equal(t, contract.OperationStatusSucceeded, operation.Status)
	executionID, ok := operation.Result["execution_id"].(string)
	require.True(t, ok)
	execution, err := svc.GetWorkflowExecution(ctx, actor, executionID)
	require.NoError(t, err)
	require.True(t, execution.Succeeded)
	require.Equal(t, "ACTIONS_SUCCEEDED", execution.Outcome)
	require.Equal(t, []string{"FIELD_UPDATE", "ASSIGNMENT", "NOTIFICATION", "AUTHENTICATED_HTTP"}, execution.ActionsAttempted)
	require.NotNil(t, execution.ResponseStatus)
	require.Equal(t, http.StatusAccepted, *execution.ResponseStatus)

	updated, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, "HIGH", updated.CustomFields["priority"])
	require.Equal(t, agent.ID, updated.PrimaryAssigneeID)
	require.Len(t, mailSender.messages, 1)
	require.Equal(t, []string{"dispatch@example.invalid"}, mailSender.messages[0].To)
	require.Len(t, client.inputs, 1)
	require.Equal(t, strconv.FormatUint(request.ID, 10), client.inputs[0].RequestID)
	require.Contains(t, client.keys[0], "wf-multi-action")

	listed, err := svc.ListWorkflowExecutions(ctx, actor, WorkflowExecutionQuery{WorkflowID: created.WorkflowID, RequestID: request.ID, PageSize: 1})
	require.NoError(t, err)
	require.Equal(t, 1, listed.TotalCount)
	require.Equal(t, execution.ExecutionID, listed.Items[0].ExecutionID)
}

func TestWorkflowExecutionRecordsConditionMissAndHTTPFailure(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	actor := workflowDesignerActor()
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)

	miss := validWorkflowDefinition("wf-miss", map[string]any{"type": "FIELD_UPDATE", "field": "custom_fields.priority", "value": "HIGH"})
	miss.Conditions = []map[string]any{{"field": "status", "operator": "IN", "values": []any{"CLOSED", "RESOLVED"}}}
	created, err := svc.CreateWorkflow(ctx, actor, miss)
	require.NoError(t, err)
	pending, err := svc.TestWorkflow(ctx, actor, created.WorkflowID, strconv.FormatUint(request.ID, 10))
	require.NoError(t, err)
	operation, err := svc.GetOperation(ctx, actor, pending.OperationID)
	require.NoError(t, err)
	execution, err := svc.GetWorkflowExecution(ctx, actor, operation.Result["execution_id"].(string))
	require.NoError(t, err)
	require.True(t, execution.Succeeded)
	require.Equal(t, "CONDITIONS_NOT_MATCHED", execution.Outcome)
	require.Empty(t, execution.ActionsAttempted)

	failure := &workflowHTTPError{status: http.StatusForbidden, body: contract.MockWorkflowFailure(contract.ErrorInsufficientScope, false)}
	svc.SetWorkflowHTTPClient(&workflowClientStub{status: http.StatusForbidden, err: failure})
	failing := validWorkflowDefinition("wf-http-failure", map[string]any{"type": "AUTHENTICATED_HTTP", "action": "dispatch", "payload": map[string]any{}})
	failing.Conditions = nil
	created, err = svc.CreateWorkflow(ctx, actor, failing)
	require.NoError(t, err)
	pending, err = svc.TestWorkflow(ctx, actor, created.WorkflowID, strconv.FormatUint(request.ID, 10))
	require.NoError(t, err)
	operation, err = svc.GetOperation(ctx, actor, pending.OperationID)
	require.NoError(t, err)
	require.Equal(t, contract.OperationStatusFailed, operation.Status)
	execution, err = svc.GetWorkflowExecution(ctx, actor, operation.Result["execution_id"].(string))
	require.NoError(t, err)
	require.False(t, execution.Succeeded)
	require.Equal(t, "ACTION_FAILED", execution.Outcome)
	require.Equal(t, contract.ErrorInsufficientScope, execution.Error.Error)
	require.Equal(t, "The authenticated workflow action failed.", execution.Error.Message)
}

func TestWorkflowConditionInAcceptsTypedSlices(t *testing.T) {
	request := &composeTypes.City311ServiceRequest{Status: contract.ServiceRequestStatusSubmitted}
	require.True(t, workflowConditionsMatch([]map[string]any{{
		"field": "status", "operator": "IN", "values": []string{"DRAFT", "SUBMITTED"},
	}}, contract.Actor{}, request))
	require.False(t, workflowConditionsMatch([]map[string]any{{
		"field": "status", "operator": "IN", "values": []string{"CLOSED", "RESOLVED"},
	}}, contract.Actor{}, request))
}

func TestWorkflowConditionOperatorsFieldsAndAliases(t *testing.T) {
	request := &composeTypes.City311ServiceRequest{
		Summary: "Blocked lane", Description: "A blocked traffic lane.", ServiceType: contract.ServiceTypePothole,
		OwningDepartment: contract.DepartmentStreets, CouncilDistrict: contract.DistrictNorth,
		SourceChannel: contract.SourceChannelAPI, OriginClass: contract.OriginClassExternal,
		Status: contract.ServiceRequestStatusSubmitted, CustomFields: composeTypes.City311JSON{"priority": "HIGH"},
	}
	actor := contract.Actor{Roles: []contract.ApplicationRole{contract.ApplicationRoleServiceAgent}}
	require.True(t, workflowConditionsMatch([]map[string]any{{"type": "ACTOR_ROLE", "value": "service_agent"}}, actor, request))
	require.False(t, workflowConditionsMatch([]map[string]any{{"actor_role": "supervisor"}}, actor, request))
	require.True(t, workflowConditionsMatch([]map[string]any{{"field": "status", "operator": "NOT_EQUALS", "value": "CLOSED"}}, actor, request))
	require.False(t, workflowConditionsMatch([]map[string]any{{"field": "status", "operator": "NE", "value": "SUBMITTED"}}, actor, request))
	require.True(t, workflowConditionsMatch([]map[string]any{{"field": "custom_fields.priority", "operator": "EXISTS"}}, actor, request))
	require.False(t, workflowConditionsMatch([]map[string]any{{"field": "custom_fields.missing", "operator": "EXISTS"}}, actor, request))
	require.False(t, workflowConditionsMatch([]map[string]any{{"field": "status", "operator": "UNKNOWN", "value": "SUBMITTED"}}, actor, request))

	fields := []string{"summary", "description", "status", "service_type", "department", "owning_department", "district", "council_district", "source_channel", "origin_class", "custom_fields.priority"}
	for _, field := range fields {
		_, present := workflowFieldValue(field, request)
		require.True(t, present, field)
	}
	_, present := workflowFieldValue("custom_fields.missing", request)
	require.False(t, present)
	_, present = workflowFieldValue("unknown", request)
	require.False(t, present)

	aliases := map[string]string{
		"http": "AUTHENTICATED_HTTP", "authenticated-http-action": "AUTHENTICATED_HTTP", "assign": "ASSIGNMENT",
		"assignee": "ASSIGNMENT", "notify": "NOTIFICATION", "email": "NOTIFICATION", "update field": "FIELD_UPDATE",
	}
	for input, expected := range aliases {
		require.Equal(t, expected, workflowActionKind(map[string]any{"kind": input}))
	}
	require.Nil(t, workflowConditionValues(make(chan int)))
	require.Empty(t, decodeObjectList(make(chan int)))
	require.Empty(t, decodeObjectList("not-an-object-list"))
	require.Equal(t, "failed", (&workflowHTTPError{body: contract.APIError{Message: "failed"}}).Error())
	require.Equal(t, http.StatusPreconditionRequired, expectedVersionRequired().Status)
}

func TestWorkflowActionValidationAndMutationErrors(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	actor := workflowDesignerActor()

	require.NoError(t, svc.workflowFieldUpdate(ctx, actor, request, map[string]any{"field": "summary", "value": "Updated request summary"}))
	require.NoError(t, svc.workflowFieldUpdate(ctx, actor, request, map[string]any{"field": "description", "value": "Updated request description for workflow coverage."}))
	err = svc.workflowFieldUpdate(ctx, actor, request, map[string]any{"field": "summary", "value": 12})
	requireServiceError(t, err, http.StatusUnprocessableEntity, contract.ErrorValidation)
	err = svc.workflowFieldUpdate(ctx, actor, request, map[string]any{"field": "description", "value": "short"})
	requireServiceError(t, err, http.StatusUnprocessableEntity, contract.ErrorValidation)
	err = svc.workflowFieldUpdate(ctx, actor, request, map[string]any{"field": "status", "value": "CLOSED"})
	requireServiceError(t, err, http.StatusUnprocessableEntity, contract.ErrorValidation)
	err = svc.workflowAssignment(ctx, actor, request, map[string]any{"assignee_id": "bad"})
	requireServiceError(t, err, http.StatusUnprocessableEntity, contract.ErrorValidation)
	err = svc.workflowAssignment(ctx, actor, request, map[string]any{"assignee_id": "999999999"})
	requireServiceError(t, err, http.StatusForbidden, contract.ErrorForbidden)

	definition := &composeTypes.City311WorkflowDefinition{WorkflowID: "wf-action-errors", Version: 1}
	_, err = svc.executeWorkflowAction(ctx, actor, definition, request, 0, "UNKNOWN", map[string]any{})
	requireServiceError(t, err, http.StatusUnprocessableEntity, contract.ErrorValidation)
	_, err = svc.executeWorkflowAction(ctx, actor, definition, request, 0, "AUTHENTICATED_HTTP", map[string]any{"payload": map[string]any{}})
	require.Error(t, err)
}

func TestCivicWorksStatusCallbackTriggersWorkflowOnce(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	agent := seededAssignmentActor(t, ctx, svc, st, "service-agent@city311.example.invalid")
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, "SR-2026-00034")
	require.NoError(t, err)
	_, err = svc.Transition(ctx, agent, request.ID, 1, contract.RequestTransition{ToStatus: contract.ServiceRequestStatusTriaged})
	require.NoError(t, err)
	svc.SetCivicWorks(&civicWorksStub{}, "webhook-secret")
	_, err = svc.Transition(ctx, agent, request.ID, 2, contract.RequestTransition{ToStatus: contract.ServiceRequestStatusAssigned})
	require.NoError(t, err)

	definition := validWorkflowDefinition("wf-civicworks-callback", map[string]any{
		"type": "FIELD_UPDATE", "field": "custom_fields.callback_workflow", "value": "applied",
	})
	definition.Trigger = WorkflowTriggerStatusChanged
	definition.Conditions = nil
	created, err := svc.CreateWorkflow(ctx, workflowDesignerActor(), definition)
	require.NoError(t, err)
	_, err = svc.SetWorkflowActive(ctx, workflowDesignerActor(), created.WorkflowID, created.Version, true)
	require.NoError(t, err)

	event := contract.CivicWorksEvent{
		EventID: "EVT-WORKFLOW", EventType: "work_order.status_changed", WorkOrderID: "WO-000034",
		SourceCaseID: "city311-case-" + strconv.FormatUint(request.ID, 10), PreviousStatus: contract.CivicWorksStatusAssigned,
		Status: contract.CivicWorksStatusInProgress, Version: 2, OccurredAt: svc.now(),
	}
	body := civicWorksEventBody(t, event)
	signature := civicWorksSignature(body, "webhook-secret")
	require.NoError(t, svc.HandleCivicWorksEvent(ctx, body, event.EventID, signature))
	updated, err := store.LookupCity311ServiceRequestByID(ctx, st, request.ID)
	require.NoError(t, err)
	require.Equal(t, "applied", updated.CustomFields["callback_workflow"])
	executions, _, err := store.SearchCity311WorkflowExecutions(ctx, st, composeTypes.City311WorkflowExecutionFilter{WorkflowID: created.WorkflowID})
	require.NoError(t, err)
	require.Len(t, executions, 1)

	require.NoError(t, svc.HandleCivicWorksEvent(ctx, body, event.EventID, signature))
	executions, _, err = store.SearchCity311WorkflowExecutions(ctx, st, composeTypes.City311WorkflowExecutionFilter{WorkflowID: created.WorkflowID})
	require.NoError(t, err)
	require.Len(t, executions, 1)
}

func TestActivatedCreatedWorkflowRunsOnceForNewSubmission(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	require.NoError(t, svc.Seed(ctx, svc.now()))
	actor := workflowDesignerActor()
	definition := validWorkflowDefinition("wf-auto-created", map[string]any{
		"type": "FIELD_UPDATE", "field": "custom_fields.workflow_marker", "value": "applied",
	})
	created, err := svc.CreateWorkflow(ctx, actor, definition)
	require.NoError(t, err)
	_, err = svc.SetWorkflowActive(ctx, actor, created.WorkflowID, created.Version, true)
	require.NoError(t, err)

	response, status, err := svc.Submit(ctx, validSubmission(), "workflow-submit", SubmissionOptions{
		Operation: "workflow_submission", SourceChannel: contract.SourceChannelAPI, ActorType: contract.AuditActorIntegrationClient, ActorID: 99, RequireIdempotency: true,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, status)
	requestID, err := strconv.ParseUint(response.RequestID, 10, 64)
	require.NoError(t, err)
	stored, err := store.LookupCity311ServiceRequestByID(ctx, st, requestID)
	require.NoError(t, err)
	require.Equal(t, "applied", stored.CustomFields["workflow_marker"])

	_, replayStatus, err := svc.Submit(ctx, validSubmission(), "workflow-submit", SubmissionOptions{
		Operation: "workflow_submission", SourceChannel: contract.SourceChannelAPI, ActorType: contract.AuditActorIntegrationClient, ActorID: 99, RequireIdempotency: true,
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, replayStatus)
	executions, _, err := store.SearchCity311WorkflowExecutions(ctx, st, composeTypes.City311WorkflowExecutionFilter{WorkflowID: created.WorkflowID, RequestID: requestID})
	require.NoError(t, err)
	require.Len(t, executions, 1)
}

func TestWorkflowValidationAndLookupErrors(t *testing.T) {
	svc, _ := testService(t)
	actor := workflowDesignerActor()
	ctx := context.Background()
	cases := []contract.WorkflowDefinition{
		{},
		validWorkflowDefinition("bad id!", map[string]any{"type": "FIELD_UPDATE", "field": "summary", "value": "valid summary"}),
		validWorkflowDefinition("wf-bad-action", map[string]any{"type": "UNKNOWN"}),
		validWorkflowDefinition("wf-bad-condition", map[string]any{"type": "FIELD_UPDATE", "field": "summary", "value": "valid summary"}),
	}
	cases[3].Conditions = []map[string]any{{"field": "unknown", "operator": "EQUALS", "value": "x"}}
	for _, input := range cases {
		_, err := svc.CreateWorkflow(ctx, actor, input)
		requireServiceError(t, err, http.StatusUnprocessableEntity, contract.ErrorValidation)
	}
	_, err := svc.GetWorkflow(ctx, actor, "missing")
	requireServiceError(t, err, http.StatusNotFound, contract.ErrorNotFound)
	_, err = svc.ListWorkflows(ctx, actor, WorkflowListQuery{Trigger: "UNKNOWN"})
	requireServiceError(t, err, http.StatusUnprocessableEntity, contract.ErrorValidation)
	_, err = svc.ListWorkflows(ctx, actor, WorkflowListQuery{PageSize: 101})
	requireServiceError(t, err, http.StatusUnprocessableEntity, contract.ErrorValidation)
	_, err = svc.TestWorkflow(ctx, actor, "missing", "bad")
	requireServiceError(t, err, http.StatusNotFound, contract.ErrorNotFound)
}

func TestOAuthWorkflowHTTPClientTokenCachingHeadersAndExpiry(t *testing.T) {
	var mu sync.Mutex
	tokenCalls := 0
	actionCalls := 0
	keys := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch r.URL.Path {
		case "/oauth/token":
			tokenCalls++
			clientID, secret, ok := r.BasicAuth()
			require.True(t, ok)
			require.Equal(t, "workflow-client", clientID)
			require.Equal(t, "workflow-secret", secret)
			require.NoError(t, r.ParseForm())
			require.Equal(t, "client_credentials", r.Form.Get("grant_type"))
			require.Equal(t, contract.ScopeWorkflowExecute, r.Form.Get("scope"))
			writeWorkflowJSON(t, w, http.StatusOK, map[string]any{"access_token": fmt.Sprintf("token-%d", tokenCalls), "token_type": "Bearer", "expires_in": 60, "scope": contract.ScopeWorkflowExecute})
		case "/api/v1/actions":
			actionCalls++
			require.Equal(t, fmt.Sprintf("Bearer token-%d", tokenCalls), r.Header.Get("Authorization"))
			require.Equal(t, "application/json", r.Header.Get("Content-Type"))
			keys = append(keys, r.Header.Get(contract.IdempotencyHeader))
			input := contract.WorkflowActionRequest{}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&input))
			require.Equal(t, "dispatch", input.Action)
			writeWorkflowJSON(t, w, http.StatusAccepted, contract.WorkflowActionAccepted{ExecutionID: fmt.Sprintf("fixture-%d", actionCalls), AcceptedAt: time.Date(2026, 2, 3, 15, 4, 5, 0, time.UTC)})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	setWorkflowEnvironment(t, server.URL)
	client, err := NewWorkflowHTTPClientFromEnvironment(server.Client())
	require.NoError(t, err)
	oauthClient := client.(*oauthWorkflowHTTPClient)
	now := time.Date(2026, 2, 3, 15, 4, 5, 0, time.UTC)
	oauthClient.now = func() time.Time { return now }
	input := contract.WorkflowActionRequest{Action: "dispatch", RequestID: "123", Payload: map[string]any{"priority": "HIGH"}}
	status, err := client.Execute(context.Background(), input, "workflow-key")
	require.NoError(t, err)
	require.Equal(t, http.StatusAccepted, status)
	_, err = client.Execute(context.Background(), input, "workflow-key")
	require.NoError(t, err)
	require.Equal(t, 1, tokenCalls)
	require.Equal(t, 2, actionCalls)
	require.Equal(t, []string{"workflow-key", "workflow-key"}, keys)

	now = now.Add(61 * time.Second)
	_, err = client.Execute(context.Background(), input, "workflow-key-2")
	require.NoError(t, err)
	require.Equal(t, 2, tokenCalls)
}

func TestOAuthWorkflowHTTPClientDeterministicFailuresAndEnvironmentValidation(t *testing.T) {
	t.Setenv("WORKFLOW_OAUTH_TOKEN_URL", "")
	_, err := NewWorkflowHTTPClientFromEnvironment(nil)
	require.ErrorContains(t, err, "WORKFLOW_OAUTH_TOKEN_URL is required")
	t.Setenv("WORKFLOW_OAUTH_TOKEN_URL", "not-a-url")
	t.Setenv("WORKFLOW_API_BASE_URL", "https://workflow.example.invalid")
	t.Setenv("WORKFLOW_CLIENT_ID", "client")
	t.Setenv("WORKFLOW_CLIENT_SECRET", "secret")
	require.ErrorContains(t, ValidateWorkflowEnvironment(), "absolute HTTP")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			writeWorkflowJSON(t, w, http.StatusUnauthorized, contract.MockWorkflowFailure(contract.ErrorInvalidClient, false))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	setWorkflowEnvironment(t, server.URL)
	client, err := NewWorkflowHTTPClientFromEnvironment(server.Client())
	require.NoError(t, err)
	status, err := client.Execute(context.Background(), contract.WorkflowActionRequest{Action: "dispatch", RequestID: "1", Payload: map[string]any{}}, "key")
	require.Error(t, err)
	require.Equal(t, http.StatusUnauthorized, status)
	typed := err.(*workflowHTTPError)
	require.Equal(t, contract.ErrorInvalidClient, typed.body.Error)
	require.Equal(t, "The authenticated workflow action failed.", typed.body.Message)

	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			writeWorkflowJSON(t, w, http.StatusOK, map[string]any{"access_token": "bad", "token_type": "Bearer", "expires_in": 60, "scope": "wrong.scope"})
			return
		}
	})
	client, err = NewWorkflowHTTPClientFromEnvironment(server.Client())
	require.NoError(t, err)
	_, err = client.Execute(context.Background(), contract.WorkflowActionRequest{}, "key")
	require.Error(t, err)
	require.Equal(t, contract.ErrorInvalidToken, err.(*workflowHTTPError).body.Error)
}

func setWorkflowEnvironment(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("WORKFLOW_OAUTH_TOKEN_URL", baseURL+"/oauth/token")
	t.Setenv("WORKFLOW_API_BASE_URL", baseURL)
	t.Setenv("WORKFLOW_CLIENT_ID", "workflow-client")
	t.Setenv("WORKFLOW_CLIENT_SECRET", "workflow-secret")
}

func writeWorkflowJSON(t *testing.T, w http.ResponseWriter, status int, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	require.NoError(t, json.NewEncoder(w).Encode(value))
}

func TestScopeContains(t *testing.T) {
	require.True(t, scopeContains("openid workflow.execute crm.export", contract.ScopeWorkflowExecute))
	require.False(t, scopeContains("workflow.read", contract.ScopeWorkflowExecute))
}
