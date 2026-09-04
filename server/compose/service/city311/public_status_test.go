package city311

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
	"github.com/stretchr/testify/require"
)

func TestPublicStatusUsesSubmittedEmailAndMinimalProjection(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	created, _, err := svc.Submit(ctx, validSubmission(), "public-lookup", SubmissionOptions{})
	require.NoError(t, err)
	input := contract.AnonymousStatusLookupRequest{RequestNumber: created.RequestNumber, Email: "ALEX@EXAMPLE.INVALID"}
	result, err := svc.LookupPublicStatus(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, result.RequestDetail)
	require.Equal(t, contract.ServiceRequestStatusSubmitted, result.RequestDetail.Status)
	require.Len(t, result.RequestDetail.History, 1)
	encoded, err := json.Marshal(result)
	require.NoError(t, err)
	for _, private := range []string{"primary_requester", "emails", "description", "phone", "audit", "assignee", "location", "custom_fields"} {
		require.NotContains(t, string(encoded), `"`+private+`"`)
	}
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, created.RequestNumber)
	require.NoError(t, err)
	constituent, err := store.LookupCity311ConstituentByConstituentID(ctx, st, fmt.Sprint(request.PrimaryRequester["constituent_id"]))
	require.NoError(t, err)
	constituent.Profile["emails"] = []string{"new@example.invalid"}
	require.NoError(t, store.UpdateCity311Constituent(ctx, st, constituent))
	for _, pair := range []contract.AnonymousStatusLookupRequest{
		{RequestNumber: created.RequestNumber, Email: "new@example.invalid"},
		{RequestNumber: created.RequestNumber, Email: "wrong@example.invalid"},
		{RequestNumber: "SR-2026-99999", Email: input.Email},
		{RequestNumber: " " + created.RequestNumber, Email: input.Email},
		{RequestNumber: created.RequestNumber, Email: "invalid"}, {},
	} {
		missing, err := svc.LookupPublicStatus(ctx, pair)
		require.NoError(t, err)
		require.Nil(t, missing.RequestDetail)
	}
	result, err = New(st).LookupPublicStatus(ctx, input)
	require.NoError(t, err)
	require.NotNil(t, result.RequestDetail, "profile edits must not change the submitted lookup credential")
	request.Status = contract.ServiceRequestStatusDraft
	require.NoError(t, store.UpdateCity311ServiceRequest(ctx, st, request))
	result, err = svc.LookupPublicStatus(ctx, input)
	require.NoError(t, err)
	require.Nil(t, result.RequestDetail, "drafts are not public even if given a request number")
}

func TestPublicStatusIncludesEveryHistoryPageInChronologicalOrder(t *testing.T) {
	svc, st := testService(t)
	ctx := context.Background()
	created, _, err := svc.Submit(ctx, validSubmission(), "public-history", SubmissionOptions{})
	require.NoError(t, err)
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, st, created.RequestNumber)
	require.NoError(t, err)
	for i := 0; i < 251; i++ {
		require.NoError(t, store.CreateCity311PublicHistoryItem(ctx, st, &composeTypes.City311PublicHistoryItem{
			ID: svc.nextID(), RequestID: request.ID, Action: fmt.Sprintf("PUBLIC_%03d", i),
			ResponsibleDepartment: request.OwningDepartment, OccurredAt: svc.now().Add(time.Duration(251-i) * time.Minute),
		}))
	}
	result, err := svc.LookupPublicStatus(ctx, contract.AnonymousStatusLookupRequest{RequestNumber: created.RequestNumber, Email: "alex@example.invalid"})
	require.NoError(t, err)
	require.Len(t, result.RequestDetail.History, 252)
	require.Equal(t, "PUBLIC_250", result.RequestDetail.History[1].Action)
	require.Equal(t, "PUBLIC_000", result.RequestDetail.History[251].Action)
	for i := 1; i < len(result.RequestDetail.History); i++ {
		require.False(t, result.RequestDetail.History[i].OccurredAt.Before(result.RequestDetail.History[i-1].OccurredAt))
	}
}
