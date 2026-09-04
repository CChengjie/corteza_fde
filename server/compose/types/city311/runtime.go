package city311

import (
	"encoding/json"
	"time"
)

// PortalAttachment is a single-use upload receipt, not a download credential.
type PortalAttachment struct {
	AttachmentToken string    `json:"attachment_token"`
	Filename        string    `json:"filename"`
	MediaType       string    `json:"media_type"`
	Size            uint64    `json:"size"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type AttachmentMetadata struct {
	AttachmentID string `json:"attachment_id"`
	Filename     string `json:"filename"`
	MediaType    string `json:"media_type"`
	Size         uint64 `json:"size"`
}

// BinaryAttachment keeps the existing JSON envelope. The encoding discriminator
// requires RFC 4648 base64 decoding before clients construct file bytes.
type BinaryAttachment struct {
	ContentType        string `json:"content_type"`
	ContentDisposition string `json:"content_disposition"`
	Body               string `json:"body"`
	BodyEncoding       string `json:"body_encoding"`
}

// PortalDraftWrite is a partial draft update. Missing fields preserve the
// existing draft value; a draft may be created before any required submission
// fields are complete.
type PortalDraftWrite struct {
	Summary          *string         `json:"summary,omitempty"`
	Description      *string         `json:"description,omitempty"`
	ServiceType      *ServiceType    `json:"service_type,omitempty"`
	Requester        *RequesterInput `json:"requester,omitempty"`
	Location         *LocationInput  `json:"location,omitempty"`
	CustomFields     *map[string]any `json:"custom_fields,omitempty"`
	AttachmentTokens *[]string       `json:"attachment_tokens,omitempty"`
}

// PortalServiceRequestSubmit is the public-portal submission DTO.
type PortalServiceRequestSubmit struct {
	Summary          string         `json:"summary"`
	Description      string         `json:"description"`
	ServiceType      ServiceType    `json:"service_type"`
	Requester        RequesterInput `json:"requester"`
	Location         *LocationInput `json:"location,omitempty"`
	CustomFields     map[string]any `json:"custom_fields,omitempty"`
	AttachmentTokens []string       `json:"attachment_tokens,omitempty"`
}

// StaffServiceRequestCreate is the contract request used by staff intake.
type StaffServiceRequestCreate struct {
	Request     PortalServiceRequestSubmit `json:"request"`
	Constituent StaffConstituentInput      `json:"constituent"`
}

// StaffConstituentInput represents the contract's constituent-reference XOR
// constituent-create union. Pointers preserve field presence for oneOf checks.
type StaffConstituentInput struct {
	ConstituentID *string `json:"constituent_id,omitempty"`
	DisplayName   *string `json:"display_name,omitempty"`
	Email         *string `json:"email,omitempty"`
}

type RequestTransition struct {
	ToStatus ServiceRequestStatus `json:"to_status"`
	Reason   string               `json:"reason,omitempty"`
}

type AnonymousRequestLink struct {
	RequestNumber string `json:"request_number"`
	Email         string `json:"email"`
}

type ConstituentLink struct {
	ConstituentID    string           `json:"constituent_id"`
	RelationshipType RelationshipType `json:"relationship_type"`
	PortalVisible    bool             `json:"portal_visible"`
	NotifyStatus     bool             `json:"notify_status"`
}

type ConstituentUnlink struct {
	Reason *string `json:"reason"`
}

type RequestNoteWrite struct {
	Body          string `json:"body"`
	PortalVisible *bool  `json:"portal_visible"`
}

type RequestNote struct {
	NoteID              string         `json:"note_id"`
	RequestID           string         `json:"request_id"`
	AuthorType          AuditActorType `json:"author_type"`
	AuthorID            string         `json:"author_id"`
	AuthorConstituentID string         `json:"author_constituent_id,omitempty"`
	Body                string         `json:"body"`
	PortalVisible       bool           `json:"portal_visible"`
	CreatedAt           time.Time      `json:"created_at"`
}

type ReopenRequest struct {
	RequestID string `json:"request_id"`
	Status    string `json:"status"`
}

type ReopenApproval struct {
	Reason string `json:"reason"`
}

type OriginOverride struct {
	OriginClass OriginClass `json:"origin_class"`
	Reason      string      `json:"reason"`
}

type ScopeOverride struct {
	DepartmentCode DepartmentCode `json:"department_code"`
	DistrictCodes  []DistrictCode `json:"district_codes"`
	Reason         string         `json:"reason"`
}

type DuplicateGroupChange struct {
	DuplicateGroupID string `json:"duplicate_group_id"`
	Reason           string `json:"reason"`
}

type BulkAction string

const (
	BulkActionUpdate BulkAction = "UPDATE"
	BulkActionClose  BulkAction = "CLOSE"
)

type BulkRequestItem struct {
	RequestID       string `json:"request_id"`
	ExpectedVersion uint64 `json:"expected_version"`
}

// BulkChanges uses raw values so the service can distinguish an omitted
// primary_assignee_id from an explicit null, which clears the assignment.
// The frozen contract intentionally keeps this object extensible at the JSON
// schema layer; the runtime enforces its four-field allow-list.
type BulkChanges map[string]json.RawMessage

type BulkRequest struct {
	RequestItems []BulkRequestItem `json:"request_items"`
	Action       BulkAction        `json:"action"`
	Changes      *BulkChanges      `json:"changes"`
}

type BulkResult struct {
	UpdatedRequestIDs []string `json:"updated_request_ids"`
	UpdatedCount      int      `json:"updated_count"`
}

type PortalRequestSummary struct {
	RequestID        string               `json:"request_id"`
	RequestNumber    string               `json:"request_number"`
	Summary          string               `json:"summary"`
	ServiceType      ServiceType          `json:"service_type"`
	Status           ServiceRequestStatus `json:"status"`
	OwningDepartment DepartmentCode       `json:"owning_department"`
	UpdatedAt        time.Time            `json:"updated_at"`
}

type PortalRequestList struct {
	Items          []PortalRequestSummary `json:"items"`
	NextPageToken  *string                `json:"next_page_token"`
	TotalCount     int                    `json:"total_count"`
	AppliedFilters map[string]any         `json:"applied_filters"`
	Sort           []string               `json:"sort"`
}

type AuditEvent struct {
	EntityType    string         `json:"entity_type"`
	EntityID      string         `json:"entity_id"`
	EventType     string         `json:"event_type"`
	ActorType     AuditActorType `json:"actor_type"`
	ActorID       string         `json:"actor_id"`
	OccurredAt    time.Time      `json:"occurred_at"`
	SourceChannel SourceChannel  `json:"source_channel"`
	Before        map[string]any `json:"before"`
	After         map[string]any `json:"after"`
}

type StaffServiceRequestDetail struct {
	Request           ServiceRequest      `json:"request"`
	ConstituentLinks  []ConstituentLink   `json:"constituent_links,omitempty"`
	Notes             []RequestNote       `json:"notes,omitempty"`
	AvailableActions  []string            `json:"available_actions"`
	PrimaryAssigneeID *string             `json:"primary_assignee_id"`
	CollaboratorIDs   []string            `json:"collaborator_ids"`
	Reminders         []Reminder          `json:"reminders"`
	History           []PublicHistoryItem `json:"history"`
	Audit             []AuditEvent        `json:"audit"`
	ExternalWorkOrder any                 `json:"external_work_order"`
}

type RequestQueueItem struct {
	RequestID         string               `json:"request_id"`
	RequestNumber     string               `json:"request_number"`
	Summary           string               `json:"summary"`
	ServiceType       ServiceType          `json:"service_type"`
	Status            ServiceRequestStatus `json:"status"`
	OwningDepartment  DepartmentCode       `json:"owning_department"`
	CouncilDistrict   DistrictCode         `json:"council_district,omitempty"`
	OriginClass       OriginClass          `json:"origin_class"`
	SourceChannel     SourceChannel        `json:"source_channel"`
	PrimaryAssigneeID *string              `json:"primary_assignee_id"`
	DuplicateGroupID  *string              `json:"duplicate_group_id"`
	Version           uint64               `json:"version"`
	UpdatedAt         time.Time            `json:"updated_at"`
	AvailableActions  []string             `json:"available_actions"`
}

type ListResponse struct {
	Items          []RequestQueueItem `json:"items"`
	NextPageToken  *string            `json:"next_page_token"`
	TotalCount     int                `json:"total_count"`
	AppliedFilters map[string]any     `json:"applied_filters"`
	Sort           []string           `json:"sort"`
}

type AccountRegistration struct {
	DisplayName       string   `json:"display_name"`
	Email             string   `json:"email"`
	LoginIdentifier   string   `json:"login_identifier"`
	Password          string   `json:"password"`
	PreferredLanguage Language `json:"preferred_language"`
}

type AccountRegistrationAcknowledgement struct {
	Accepted bool `json:"accepted"`
}

type LocalSignIn struct {
	LoginIdentifier string `json:"login_identifier"`
	Password        string `json:"password"`
}

type CurrentActor struct {
	ActorID          string            `json:"actor_id"`
	DisplayName      string            `json:"display_name"`
	OIDCActorType    *string           `json:"oidc_actor_type,omitempty"`
	ApplicationRoles []ApplicationRole `json:"application_roles"`
	DepartmentCodes  []DepartmentCode  `json:"department_codes"`
	DistrictCodes    []DistrictCode    `json:"district_codes"`
	Capabilities     []string          `json:"capabilities"`
	Scopes           []string          `json:"scopes"`
	AvailableRoutes  []string          `json:"available_routes"`
}

type Session struct {
	Authenticated     bool          `json:"authenticated"`
	Actor             *CurrentActor `json:"actor"`
	ExpiresAt         *time.Time    `json:"expires_at"`
	PreferredLanguage Language      `json:"preferred_language"`
}

type PasswordChange struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

type LoginIdentifierChange struct {
	CurrentPassword string `json:"current_password"`
	LoginIdentifier string `json:"login_identifier"`
}

// Actor carries server-resolved roles and record scope.
type Actor struct {
	ID         uint64
	Roles      []ApplicationRole
	Department DepartmentCode
	Districts  []DistrictCode
}
