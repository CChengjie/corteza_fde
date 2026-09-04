package city311

import (
	"bytes"
	"context"
	"encoding/csv"
	"net/http"
	"sort"
	"strings"

	composeTypes "github.com/cortezaproject/corteza/server/compose/types"
	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
	"github.com/cortezaproject/corteza/server/store"
)

const (
	contactEmailExportKind       = "CONTACT_EMAIL_EXPORT"
	contactEmailExportAuditEvent = "CONTACT_EMAIL_EXPORTED"
)

var contactEmailExportFilters = map[string]bool{
	"email": true, "department": true, "district": true,
	"primary_category": true, "preferred_language": true,
}

type contactEmailRow struct {
	constituentID     string
	email             string
	displayName       string
	primaryCategory   string
	preferredLanguage string
}

func (svc *Service) StartContactEmailExport(ctx context.Context, actor contract.Actor, input contract.ContactEmailExport) (*contract.Operation, error) {
	if !canAccessAudit(actor) {
		return nil, apiError(http.StatusForbidden, contract.ErrorForbidden, "A department manager or platform administrator role is required.")
	}
	filters, err := normalizeDataExportFilters(input.Filters, contactEmailExportFilters)
	if err != nil {
		return nil, contactEmailValidationError()
	}
	if err = validateContactEmailFilters(filters); err != nil {
		return nil, err
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	pending := &contract.Operation{}
	err = store.Tx(ctx, svc.store, func(ctx context.Context, tx store.Storer) error {
		now := svc.now()
		operation := &composeTypes.City311Operation{
			ID: svc.nextID(), Kind: contactEmailExportKind, Status: string(contract.OperationStatusPending), ActorID: actor.ID,
			Result: composeTypes.City311JSON{}, Error: composeTypes.City311JSON{}, CreatedAt: now, UpdatedAt: now,
		}
		if createErr := store.CreateCity311Operation(ctx, tx, operation); createErr != nil {
			return createErr
		}
		*pending = *toOperation(operation)

		rows, selectErr := svc.selectContactEmails(ctx, tx, actor, filters)
		if selectErr != nil {
			return selectErr
		}
		content, encodeErr := encodeContactEmailCSV(rows)
		if encodeErr != nil {
			return encodeErr
		}
		completedAt := svc.now()
		operation.Status = string(contract.OperationStatusSucceeded)
		operation.Progress = 100
		operation.Content = content
		operation.ContentType = "text/csv; charset=utf-8"
		operation.Filename = "contact-emails-" + completedAt.UTC().Format("20060102T150405Z") + ".csv"
		operation.Result = composeTypes.City311JSON{"download_url": "/api/v1/operations/" + publicOperationID(operation.ID) + "/result"}
		operation.UpdatedAt = completedAt
		operation.CompletedAt = &completedAt
		if updateErr := store.UpdateCity311Operation(ctx, tx, operation); updateErr != nil {
			return updateErr
		}
		return store.CreateCity311AuditEvent(ctx, tx, &composeTypes.City311AuditEvent{
			ID: svc.nextID(), EntityType: "contact_email_export", EntityID: publicOperationID(operation.ID), EventType: contactEmailExportAuditEvent,
			ActorType: contract.AuditActorStaff, ActorID: actor.ID, SourceChannel: contract.SourceChannelStaffInPerson,
			Before: composeTypes.City311JSON{}, After: composeTypes.City311JSON{
				"department_code": optionalActorDepartment(actor), "filters": filters, "row_count": len(rows),
			}, CreatedAt: completedAt,
		})
	})
	if err != nil {
		return nil, err
	}
	return pending, nil
}

func validateContactEmailFilters(filters map[string][]string) error {
	for key, values := range filters {
		switch key {
		case "department":
			if !validExportEnum(values, contract.DepartmentCodes) {
				return contactEmailValidationError()
			}
		case "district":
			if !validExportEnum(values, contract.DistrictCodes) {
				return contactEmailValidationError()
			}
		case "primary_category":
			if !validExportEnum(values, contract.ContactCategories) {
				return contactEmailValidationError()
			}
		case "preferred_language":
			if !validExportEnum(values, contract.Languages) {
				return contactEmailValidationError()
			}
		case "email":
			for _, value := range values {
				if !validEmail(strings.ToLower(value)) {
					return contactEmailValidationError()
				}
			}
		}
	}
	return nil
}

func (svc *Service) selectContactEmails(ctx context.Context, st store.Storer, actor contract.Actor, filters map[string][]string) ([]contactEmailRow, error) {
	accounts, _, err := store.SearchCity311LocalAccounts(ctx, st, composeTypes.City311LocalAccountFilter{})
	if err != nil {
		return nil, err
	}
	verified := make(map[string]bool, len(accounts))
	for _, account := range accounts {
		if email := normalizeEmail(account.VerifiedEmail); email != "" {
			verified[email] = true
		}
	}
	constituents, _, err := store.SearchCity311Constituents(ctx, st, composeTypes.City311ConstituentFilter{})
	if err != nil {
		return nil, err
	}
	sort.Slice(constituents, func(i, j int) bool { return constituents[i].ID < constituents[j].ID })
	rows := make([]contactEmailRow, 0, len(constituents))
	seen := make(map[string]bool, len(constituents))
	for _, item := range constituents {
		if !canReadConstituent(actor, item) {
			continue
		}
		projected := projectExportConstituent(item)
		if projected.EmailOptOut || !matchesString(string(item.OwningDepartment), filters["department"]) ||
			!matchesString(string(item.CouncilDistrict), filters["district"]) ||
			!matchesString(string(projected.PrimaryCategory), filters["primary_category"]) ||
			!matchesString(string(projected.PreferredLanguage), filters["preferred_language"]) {
			continue
		}
		for _, rawEmail := range projected.Emails {
			email := normalizeEmail(rawEmail)
			if !verified[email] || seen[email] || !matchesString(email, lowerValues(filters["email"])) {
				continue
			}
			seen[email] = true
			rows = append(rows, contactEmailRow{
				constituentID: item.ConstituentID, email: email, displayName: projected.DisplayName,
				primaryCategory: string(projected.PrimaryCategory), preferredLanguage: string(projected.PreferredLanguage),
			})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].email == rows[j].email {
			return rows[i].constituentID < rows[j].constituentID
		}
		return rows[i].email < rows[j].email
	})
	return rows, nil
}

func lowerValues(values []string) []string {
	out := make([]string, len(values))
	for index, value := range values {
		out[index] = strings.ToLower(value)
	}
	return out
}

func encodeContactEmailCSV(rows []contactEmailRow) ([]byte, error) {
	buffer := &bytes.Buffer{}
	writer := csv.NewWriter(buffer)
	writer.UseCRLF = true
	if err := writer.Write([]string{"email", "display_name", "primary_category", "preferred_language", "opt_out"}); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if err := writer.Write([]string{row.email, row.displayName, row.primaryCategory, row.preferredLanguage, "false"}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func contactEmailValidationError() *ServiceError {
	return validationError(contract.FieldError{Field: "/filters", Code: contract.ValidationInvalidValue})
}
