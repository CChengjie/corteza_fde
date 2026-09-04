package city311

import (
	"context"
	"encoding/json"
	"strings"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/pkg/filter"
	"github.com/cortezaproject/corteza/server/store"
)

func (svc *Service) FindConstituent(ctx context.Context, actor contract.Actor, constituentID string) (map[string]any, error) {
	if err := AuthorizeRecordRead(actor, false); err != nil {
		return nil, err
	}
	item, err := svc.ResolveConstituent(ctx, actor, constituentID)
	if err != nil {
		return nil, err
	}
	values, _, err := constituentReadValues(item)
	return values, err
}

func (svc *Service) ListConstituents(ctx context.Context, actor contract.Actor, query RecordReadQuery) (*RecordReadPage, error) {
	if err := AuthorizeRecordRead(actor, false); err != nil {
		return nil, err
	}
	query, order, err := prepareReadQuery(query, []string{"constituent_id", "display_name", "primary_category", "updated_at"}, contract.RecordReadFilterNames("staff_constituent_search"))
	if err != nil {
		return nil, err
	}
	if err = validateConstituentReadFilters(query.Filters); err != nil {
		return nil, err
	}
	f := composeTypes.City311ConstituentFilter{Paging: filter.Paging{Limit: 100}}
	var records []readableRecord
	for {
		page, next, err := store.SearchCity311Constituents(ctx, svc.store, f)
		if err != nil {
			return nil, err
		}
		selected, err := constituentReadRecords(actor, query.Filters, page)
		if err != nil {
			return nil, err
		}
		records = append(records, selected...)
		if next.NextPage == nil {
			break
		}
		f.PageCursor = next.NextPage
	}
	return readPage("constituents", actor, query, order, records)
}

func constituentReadRecords(actor contract.Actor, filters map[string]string, page composeTypes.City311ConstituentSet) ([]readableRecord, error) {
	var records []readableRecord
	for _, item := range page {
		if !canReadConstituent(actor, item) {
			continue
		}
		values, profile, err := constituentReadValues(item)
		if err != nil {
			return nil, err
		}
		if !matchesConstituentRead(item, profile, filters) {
			continue
		}
		records = append(records, readableRecord{id: item.ID, values: values, order: map[string]string{
			"constituent_id": item.ConstituentID, "display_name": profile.DisplayName, "primary_category": string(profile.PrimaryCategory), "updated_at": item.UpdatedAt.UTC().Format(readOrderTimeLayout),
		}})
	}
	return records, nil
}

func constituentReadValues(item *composeTypes.City311Constituent) (map[string]any, contract.Constituent, error) {
	var profile contract.Constituent
	data, err := json.Marshal(item.Profile)
	if err != nil {
		return nil, profile, err
	}
	if err = json.Unmarshal(data, &profile); err != nil {
		return nil, profile, err
	}
	profile.ConstituentID = item.ConstituentID
	if profile.Emails == nil {
		profile.Emails = []string{}
	}
	if profile.PhoneNumbers == nil {
		profile.PhoneNumbers = []contract.PhoneNumber{}
	}
	if profile.Addresses == nil {
		profile.Addresses = []contract.Address{}
	}
	values, err := mapFrom(profile)
	if fields, present := item.Profile["custom_fields"]; present && err == nil {
		values["custom_fields"] = fields
	}
	return values, profile, err
}

func validateConstituentReadFilters(values map[string]string) error {
	if value := values["department"]; value != "" && !containsEnums([]contract.DepartmentCode{contract.DepartmentCode(value)}, contract.DepartmentCodes) {
		return readQueryError("filters/department")
	}
	if value := values["district"]; value != "" && !containsEnums([]contract.DistrictCode{contract.DistrictCode(value)}, contract.DistrictCodes) {
		return readQueryError("filters/district")
	}
	if value := values["category"]; value != "" && !containsEnums([]contract.ContactCategory{contract.ContactCategory(value)}, contract.ContactCategories) {
		return readQueryError("filters/category")
	}
	if value := values["email"]; value != "" && !validEmail(normalizeEmail(value)) {
		return readQueryError("filters/email")
	}
	return nil
}

func matchesConstituentRead(item *composeTypes.City311Constituent, profile contract.Constituent, values map[string]string) bool {
	for key, actual := range map[string]string{"department": string(item.OwningDepartment), "district": string(item.CouncilDistrict), "category": string(profile.PrimaryCategory)} {
		if values[key] != "" && values[key] != actual {
			return false
		}
	}
	if email := values["email"]; email != "" {
		matched := false
		for _, current := range profile.Emails {
			matched = matched || normalizeEmail(current) == normalizeEmail(email)
		}
		if !matched {
			return false
		}
	}
	searchable := profile.DisplayName + " " + strings.Join(profile.Emails, " ")
	for _, phone := range profile.PhoneNumbers {
		searchable += " " + phone.Value
	}
	return strings.Contains(strings.ToLower(searchable), strings.ToLower(values["q"]))
}
