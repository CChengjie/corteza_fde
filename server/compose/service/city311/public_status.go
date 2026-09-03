package city311

import (
	"context"
	"regexp"
	"sort"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/errors"
	"github.com/cortezaproject/corteza/server/pkg/filter"
	"github.com/cortezaproject/corteza/server/store"
)

var publicRequestNumber = regexp.MustCompile(`^SR-[0-9]{4}-[0-9]{5}$`)

// LookupPublicStatus is a credential-pair lookup, not a public list/search API.
// Only the submitted snapshot's email unlocks the minimal public projection.
func (svc *Service) LookupPublicStatus(ctx context.Context, input contract.AnonymousStatusLookupRequest) (*contract.AnonymousStatusLookupResponse, error) {
	result := &contract.AnonymousStatusLookupResponse{}
	email := normalizeEmail(input.Email)
	if !publicRequestNumber.MatchString(input.RequestNumber) || !validEmail(email) {
		return result, nil
	}
	request, err := store.LookupCity311ServiceRequestByRequestNumber(ctx, svc.store, input.RequestNumber)
	if errors.IsNotFound(err) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	if request.Status == contract.ServiceRequestStatusDraft || normalizeEmail(requesterInput(request.PrimaryRequester).Email) != email {
		return result, nil
	}
	history, err := svc.publicStatusHistory(ctx, request.ID)
	if err != nil {
		return nil, err
	}
	result.RequestDetail = &contract.PublicServiceRequestDetail{
		RequestNumber: request.RequestNumber, Summary: request.Summary,
		ServiceType: request.ServiceType, Status: request.Status,
		OwningDepartment: request.OwningDepartment, UpdatedAt: request.UpdatedAt,
		History: history,
	}
	return result, nil
}

func (svc *Service) publicStatusHistory(ctx context.Context, requestID uint64) ([]contract.PublicHistoryItem, error) {
	query := composeTypes.City311PublicHistoryItemFilter{RequestID: requestID, Paging: filter.Paging{Limit: 100}}
	var records composeTypes.City311PublicHistoryItemSet
	for {
		page, next, err := store.SearchCity311PublicHistoryItems(ctx, svc.store, query)
		if err != nil {
			return nil, err
		}
		records = append(records, page...)
		if next.NextPage == nil {
			break
		}
		query.PageCursor = next.NextPage
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].OccurredAt.Equal(records[j].OccurredAt) {
			return records[i].ID < records[j].ID
		}
		return records[i].OccurredAt.Before(records[j].OccurredAt)
	})
	result := make([]contract.PublicHistoryItem, 0, len(records))
	for _, item := range records {
		result = append(result, contract.PublicHistoryItem{Action: item.Action, OccurredAt: item.OccurredAt, ResponsibleDepartment: string(item.ResponsibleDepartment)})
	}
	return result, nil
}
