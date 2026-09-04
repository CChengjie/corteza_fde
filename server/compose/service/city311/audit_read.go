package city311

import (
	"context"
	"strconv"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/errors"
	"github.com/cortezaproject/corteza/server/pkg/filter"
	"github.com/cortezaproject/corteza/server/store"
)

func (svc *Service) ListAuditEvents(ctx context.Context, actor contract.Actor, query RecordReadQuery) (*RecordReadPage, error) {
	if err := AuthorizeRecordRead(actor, true); err != nil {
		return nil, err
	}
	query, order, err := prepareReadQuery(query, []string{"occurred_at", "entity_type", "entity_id", "event_type", "actor_id"}, contract.RecordReadFilterNames("audit_list"))
	if err != nil {
		return nil, err
	}
	if err = validateAuditReadFilters(query.Filters); err != nil {
		return nil, err
	}
	f := composeTypes.City311AuditEventFilter{Paging: filter.Paging{Limit: 100}}
	var records []readableRecord
	for {
		page, next, err := store.SearchCity311AuditEvents(ctx, svc.store, f)
		if err != nil {
			return nil, err
		}
		selected, err := svc.auditReadRecords(ctx, actor, query.Filters, page)
		if err != nil {
			return nil, err
		}
		records = append(records, selected...)
		if next.NextPage == nil {
			break
		}
		f.PageCursor = next.NextPage
	}
	return readPage("audit-events", actor, query, order, records)
}

func (svc *Service) auditReadRecords(ctx context.Context, actor contract.Actor, filters map[string]string, page composeTypes.City311AuditEventSet) ([]readableRecord, error) {
	var records []readableRecord
	for _, item := range page {
		allowed, err := svc.canReadAudit(ctx, actor, item)
		if err != nil {
			return nil, err
		}
		if !allowed || !matchesAuditRead(item, filters) {
			continue
		}
		values, err := mapFrom(contract.AuditEvent{EntityType: item.EntityType, EntityID: item.EntityID, EventType: item.EventType, ActorType: item.ActorType, ActorID: strconv.FormatUint(item.ActorID, 10), OccurredAt: item.CreatedAt, SourceChannel: item.SourceChannel, Before: item.Before, After: item.After})
		if err != nil {
			return nil, err
		}
		records = append(records, readableRecord{id: item.ID, values: values, order: map[string]string{"occurred_at": item.CreatedAt.UTC().Format(readOrderTimeLayout), "entity_type": item.EntityType, "entity_id": item.EntityID, "event_type": item.EventType, "actor_id": strconv.FormatUint(item.ActorID, 10)}})
	}
	return records, nil
}

func (svc *Service) canReadAudit(ctx context.Context, actor contract.Actor, item *composeTypes.City311AuditEvent) (bool, error) {
	if hasRole(actor, contract.ApplicationRolePlatformAdministrator) {
		return true, nil
	}
	if item.RequestID != 0 {
		request, err := store.LookupCity311ServiceRequestByID(ctx, svc.store, item.RequestID)
		if errors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return request.OwningDepartment == actor.Department, nil
	}
	if item.EntityType == "constituent" {
		constituent, err := store.LookupCity311ConstituentByConstituentID(ctx, svc.store, item.EntityID)
		if errors.IsNotFound(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return constituent.OwningDepartment != "" && constituent.OwningDepartment == actor.Department, nil
	}
	// Identity/configuration/global records have no department owner. Do not
	// infer permission from event payloads or from the actor who caused them.
	return false, nil
}

func validateAuditReadFilters(values map[string]string) error {
	for _, name := range []string{"actor_id", "request_id"} {
		if value := values[name]; value != "" {
			id, err := strconv.ParseUint(value, 10, 64)
			if err != nil || id == 0 {
				return readQueryError("filters/" + name)
			}
		}
	}
	from, err := readDate(values["from"])
	if err != nil {
		return readQueryError("filters/from")
	}
	to, err := readDate(values["to"])
	if err != nil || (!from.IsZero() && !to.IsZero() && to.Before(from)) {
		return readQueryError("filters/to")
	}
	if value := values["source_channel"]; value != "" && !containsEnums([]contract.SourceChannel{contract.SourceChannel(value)}, contract.SourceChannels) {
		return readQueryError("filters/source_channel")
	}
	return nil
}

func matchesAuditRead(item *composeTypes.City311AuditEvent, values map[string]string) bool {
	for name, actual := range map[string]string{"entity_type": item.EntityType, "entity_id": item.EntityID, "event_type": item.EventType, "actor_id": strconv.FormatUint(item.ActorID, 10), "request_id": strconv.FormatUint(item.RequestID, 10), "source_channel": string(item.SourceChannel)} {
		if values[name] != "" && values[name] != actual {
			return false
		}
	}
	from, _ := readDate(values["from"])
	to, _ := readDate(values["to"])
	return (from.IsZero() || !item.CreatedAt.Before(from)) && (to.IsZero() || !item.CreatedAt.After(to))
}
