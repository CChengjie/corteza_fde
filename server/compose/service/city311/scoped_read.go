package city311

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	contract "github.com/cortezaproject/corteza/server/compose/types/city311"
)

type RecordReadQuery struct {
	PageSize  uint
	PageToken string
	Sort      string
	Filters   map[string]string
}

const readOrderTimeLayout = "2006-01-02T15:04:05.000000000Z"

type RecordReadPage struct {
	Items          []map[string]any  `json:"items"`
	NextPageToken  *string           `json:"next_page_token"`
	TotalCount     int               `json:"total_count"`
	AppliedFilters map[string]string `json:"applied_filters"`
	Sort           []string          `json:"sort"`
}

type readableRecord struct {
	id     uint64
	values map[string]any
	order  map[string]string
}

type readCursor struct {
	Offset  int    `json:"offset"`
	Binding string `json:"binding"`
}

// AuthorizeRecordRead enforces the role boundary before decoding a staff query.
// Individual records must still pass department/district checks.
func AuthorizeRecordRead(actor contract.Actor, audit bool) error {
	if actor.ID == 0 {
		return apiError(401, contract.ErrorUnauthenticated, "Authentication is required.")
	}
	if hasRole(actor, contract.ApplicationRolePlatformAdministrator) {
		return nil
	}
	if actor.Department == "" {
		return apiError(403, contract.ErrorForbidden, "A department scope is required.")
	}
	if hasRole(actor, contract.ApplicationRoleDepartmentManager) {
		return nil
	}
	if !audit && (hasRole(actor, contract.ApplicationRoleServiceAgent) || hasRole(actor, contract.ApplicationRoleSupervisor)) {
		return nil
	}
	return apiError(403, contract.ErrorForbidden, "A CRM record-reading role is required.")
}

func prepareReadQuery(query RecordReadQuery, columns, filterNames []string) (RecordReadQuery, []string, error) {
	if query.PageSize == 0 {
		query.PageSize = 50
	}
	if query.PageSize > 100 {
		return query, nil, readQueryError("page_size")
	}
	filters := map[string]string{}
	for name, value := range query.Filters {
		if !readContains(filterNames, name) || utf8.RuneCountInString(value) > 500 {
			return query, nil, readQueryError("filters/" + readPointerToken(name))
		}
		filters[name] = strings.TrimSpace(value)
	}
	query.Filters = filters
	if query.Sort == "" {
		query.Sort = columns[0]
	}
	order := strings.Split(query.Sort, ",")
	if len(order) > 3 {
		return query, nil, readQueryError("sort")
	}
	seen := map[string]bool{}
	for index, value := range order {
		value = strings.TrimSpace(value)
		name := strings.TrimPrefix(value, "-")
		if !readContains(columns, name) || seen[name] {
			return query, nil, readQueryError("sort")
		}
		seen[name], order[index] = true, value
	}
	return query, order, nil
}

func readContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func readQueryError(field string) error {
	return validationError(contract.FieldError{Field: "/query/" + field, Code: contract.ValidationInvalidValue})
}

func readPointerToken(value string) string {
	return strings.NewReplacer("~", "~0", "/", "~1").Replace(value)
}

func readPage(kind string, actor contract.Actor, query RecordReadQuery, order []string, records []readableRecord) (*RecordReadPage, error) {
	bindingBytes, err := json.Marshal([]any{kind, actor, order, query.Filters})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(bindingBytes)
	binding := hex.EncodeToString(digest[:])
	offset := 0
	if query.PageToken != "" {
		data, err := base64.RawURLEncoding.DecodeString(query.PageToken)
		var cursor readCursor
		if err != nil || json.Unmarshal(data, &cursor) != nil || cursor.Offset < 0 || cursor.Binding != binding {
			return nil, readQueryError("page_token")
		}
		offset = cursor.Offset
	}
	sort.Slice(records, func(i, j int) bool {
		return readLess(records[i], records[j], order)
	})
	page := &RecordReadPage{Items: []map[string]any{}, TotalCount: len(records), AppliedFilters: query.Filters, Sort: order}
	if offset >= len(records) {
		return page, nil
	}
	end := offset + int(query.PageSize)
	if end > len(records) {
		end = len(records)
	}
	for _, record := range records[offset:end] {
		page.Items = append(page.Items, record.values)
	}
	if end < len(records) {
		data, err := json.Marshal(readCursor{Offset: end, Binding: binding})
		if err != nil {
			return nil, err
		}
		token := base64.RawURLEncoding.EncodeToString(data)
		page.NextPageToken = &token
	}
	return page, nil
}

func readLess(left, right readableRecord, order []string) bool {
	for _, field := range order {
		name := strings.TrimPrefix(field, "-")
		if left.order[name] == right.order[name] {
			continue
		}
		if strings.HasPrefix(field, "-") {
			return left.order[name] > right.order[name]
		}
		return left.order[name] < right.order[name]
	}
	return left.id < right.id
}

func readDate(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, value)
}
