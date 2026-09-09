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

type OperationStatus string

const (
	OperationStatusPending   OperationStatus = "PENDING"
	OperationStatusRunning   OperationStatus = "RUNNING"
	OperationStatusSucceeded OperationStatus = "SUCCEEDED"
	OperationStatusFailed    OperationStatus = "FAILED"
	OperationStatusCancelled OperationStatus = "CANCELLED"
)

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

type AuditFilter struct {
	RequestIDs     []string         `json:"request_id,omitempty"`
	EntityTypes    []string         `json:"entity_type,omitempty"`
	EntityIDs      []string         `json:"entity_id,omitempty"`
	EventTypes     []string         `json:"event_type,omitempty"`
	ActorTypes     []AuditActorType `json:"actor_type,omitempty"`
	ActorIDs       []string         `json:"actor_id,omitempty"`
	SourceChannels []SourceChannel  `json:"source_channel,omitempty"`
	OccurredFrom   *time.Time       `json:"occurred_from,omitempty"`
	OccurredTo     *time.Time       `json:"occurred_to,omitempty"`
}

type AuditQuery struct {
	Filters   AuditFilter
	PageSize  uint
	PageToken string
	Sort      string
}

type AuditExport struct {
	Filters AuditFilter `json:"filters"`
}

type AuditListResponse struct {
	Items          []AuditEvent   `json:"items"`
	NextPageToken  *string        `json:"next_page_token"`
	TotalCount     int            `json:"total_count"`
	AppliedFilters map[string]any `json:"applied_filters"`
	Sort           []string       `json:"sort"`
}

type Operation struct {
	OperationID string          `json:"operation_id"`
	Kind        string          `json:"kind"`
	Status      OperationStatus `json:"status"`
	Progress    int             `json:"progress"`
	Result      map[string]any  `json:"result"`
	Error       *APIError       `json:"error"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	CompletedAt *time.Time      `json:"completed_at"`
}

type CalendarImport struct {
	ICS string `json:"ics"`
}

type CalendarExport struct {
	ContentType string `json:"content_type"`
	Body        string `json:"body"`
}

type CivicWorksWorkOrderCreate struct {
	SourceCaseID         string         `json:"source_case_id"`
	ServiceRequestNumber string         `json:"service_request_number"`
	ServiceType          ServiceType    `json:"service_type"`
	Summary              string         `json:"summary"`
	DepartmentCode       DepartmentCode `json:"department_code"`
	Location             map[string]any `json:"location,omitempty"`
	CallbackURL          string         `json:"callback_url"`
}

type DataExportQuery struct {
	Filters      map[string][]string
	PageSize     uint
	PageToken    string
	UpdatedSince *time.Time
}

type ContactEmailExport struct {
	Filters map[string][]string `json:"filters"`
}

type MailCompose struct {
	TemplateID  *string           `json:"template_id,omitempty"`
	To          []string          `json:"to"`
	Subject     string            `json:"subject"`
	Text        string            `json:"text"`
	HTML        string            `json:"html,omitempty"`
	Attachments []AttachmentInput `json:"attachments,omitempty"`
}

type MailPreview struct {
	Subject string `json:"subject"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
}

type MailDelivery struct {
	DeliveryID string    `json:"delivery_id"`
	Status     string    `json:"status"`
	Attempts   int       `json:"attempts"`
	UpdatedAt  time.Time `json:"updated_at"`
	Error      *APIError `json:"error,omitempty"`
}

type WorkflowDefinition struct {
	WorkflowID string           `json:"workflow_id"`
	Name       string           `json:"name"`
	Trigger    string           `json:"trigger"`
	Active     bool             `json:"active"`
	Conditions []map[string]any `json:"conditions"`
	Actions    []map[string]any `json:"actions"`
	Version    uint64           `json:"version"`
	UpdatedAt  time.Time        `json:"updated_at"`
}

type WorkflowTest struct {
	RequestID string `json:"request_id"`
}

type WorkflowDefinitionList struct {
	Items          []WorkflowDefinition `json:"items"`
	NextPageToken  *string              `json:"next_page_token"`
	TotalCount     int                  `json:"total_count"`
	AppliedFilters map[string]any       `json:"applied_filters"`
	Sort           []string             `json:"sort"`
}

type WorkflowExecutionList struct {
	Items          []WorkflowExecution `json:"items"`
	NextPageToken  *string             `json:"next_page_token"`
	TotalCount     int                 `json:"total_count"`
	AppliedFilters map[string]any      `json:"applied_filters"`
	Sort           []string            `json:"sort"`
}

type WorkflowActionRequest struct {
	Action    string         `json:"action"`
	RequestID string         `json:"request_id"`
	Payload   map[string]any `json:"payload"`
}

type WorkflowActionAccepted struct {
	ExecutionID string    `json:"execution_id"`
	AcceptedAt  time.Time `json:"accepted_at"`
}

type Branding struct {
	OrganisationName   string    `json:"organisation_name"`
	LogoURL            *string   `json:"logo_url"`
	FaviconURL         *string   `json:"favicon_url"`
	PortalWallpaperURL *string   `json:"portal_wallpaper_url"`
	LoginHeader        string    `json:"login_header"`
	PublicHeader       string    `json:"public_header"`
	PublicFooter       string    `json:"public_footer"`
	PrimaryColour      string    `json:"primary_colour"`
	AccentColour       string    `json:"accent_colour"`
	FontFamily         string    `json:"font_family"`
	Published          bool      `json:"published"`
	Version            uint64    `json:"version"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type BrandingWrite struct {
	OrganisationName   *string `json:"organisation_name,omitempty"`
	LogoURL            *string `json:"logo_url,omitempty"`
	FaviconURL         *string `json:"favicon_url,omitempty"`
	PortalWallpaperURL *string `json:"portal_wallpaper_url,omitempty"`
	LoginHeader        *string `json:"login_header,omitempty"`
	PublicHeader       *string `json:"public_header,omitempty"`
	PublicFooter       *string `json:"public_footer,omitempty"`
	PrimaryColour      *string `json:"primary_colour,omitempty"`
	AccentColour       *string `json:"accent_colour,omitempty"`
	FontFamily         *string `json:"font_family,omitempty"`
}

type BrandingList struct {
	Items         []Branding `json:"items"`
	NextPageToken *string    `json:"next_page_token"`
	TotalCount    int        `json:"total_count"`
	Sort          []string   `json:"sort"`
}

type ContentObject struct {
	ContentKey string    `json:"content_key"`
	Body       string    `json:"body"`
	State      string    `json:"state"`
	Published  bool      `json:"published"`
	Version    uint64    `json:"version"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ContentWrite struct {
	Body string `json:"body"`
}

type ContentList struct {
	Items         []ContentObject `json:"items"`
	NextPageToken *string         `json:"next_page_token"`
	TotalCount    int             `json:"total_count"`
	Sort          []string        `json:"sort"`
}

type HelpContent struct {
	HelpKey   string    `json:"help_key"`
	Language  Language  `json:"language"`
	Body      string    `json:"body"`
	Version   uint64    `json:"version"`
	UpdatedAt time.Time `json:"updated_at"`
}

type HelpWrite struct {
	Language Language `json:"language"`
	Body     string   `json:"body"`
}

// Category is an administrator-managed contact-category vocabulary item.
// Code is immutable after creation; updates create a new persisted revision.
type Category struct {
	Code      string            `json:"code"`
	Active    bool              `json:"active"`
	Labels    map[string]string `json:"labels"`
	Version   uint64            `json:"version"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type CategoryWrite struct {
	Code   string            `json:"code"`
	Active bool              `json:"active"`
	Labels map[string]string `json:"labels"`
}

type CategoryList struct {
	Items          []Category     `json:"items"`
	NextPageToken  *string        `json:"next_page_token"`
	TotalCount     int            `json:"total_count"`
	AppliedFilters map[string]any `json:"applied_filters"`
	Sort           []string       `json:"sort"`
}

// CustomFieldDefinition describes a field made available to forms, filters,
// workflows, reports, and exports for the selected entity.
type CustomFieldDefinition struct {
	Key          string            `json:"key"`
	Labels       map[string]string `json:"labels"`
	Entity       string            `json:"entity"`
	FieldType    CustomFieldType   `json:"field_type"`
	Required     bool              `json:"required"`
	Default      any               `json:"default,omitempty"`
	Active       bool              `json:"active"`
	Validation   map[string]any    `json:"validation,omitempty"`
	ChoiceValues []string          `json:"choice_values,omitempty"`
	Version      uint64            `json:"version"`
	UpdatedAt    time.Time         `json:"updated_at"`
}

type CustomFieldDefinitionList struct {
	Items          []CustomFieldDefinition `json:"items"`
	NextPageToken  *string                 `json:"next_page_token"`
	TotalCount     int                     `json:"total_count"`
	AppliedFilters map[string]any          `json:"applied_filters"`
	Sort           []string                `json:"sort"`
}

type ReportCatalogueItem struct {
	ReportKey         string   `json:"report_key"`
	Name              string   `json:"name"`
	SupportedFilters  []string `json:"supported_filters"`
	SupportedGrouping []string `json:"supported_grouping"`
	SupportedSort     []string `json:"supported_sort"`
}

type ReportCatalogueList struct {
	Items          []ReportCatalogueItem `json:"items"`
	NextPageToken  *string               `json:"next_page_token"`
	TotalCount     int                   `json:"total_count"`
	AppliedFilters map[string]any        `json:"applied_filters"`
	Sort           []string              `json:"sort"`
}

type ReportDefinition struct {
	ReportID  string         `json:"report_id"`
	Name      string         `json:"name"`
	Entity    string         `json:"entity"`
	Columns   []string       `json:"columns"`
	Filters   map[string]any `json:"filters"`
	Grouping  *string        `json:"grouping,omitempty"`
	Sort      []string       `json:"sort"`
	Version   uint64         `json:"version"`
	UpdatedAt time.Time      `json:"updated_at"`
}

type ReportDefinitionList struct {
	Items          []ReportDefinition `json:"items"`
	NextPageToken  *string            `json:"next_page_token"`
	TotalCount     int                `json:"total_count"`
	AppliedFilters map[string]any     `json:"applied_filters"`
	Sort           []string           `json:"sort"`
}

type ReportRun struct {
	Definition ReportDefinition `json:"definition"`
}

type ReportShare struct {
	Roles []ApplicationRole `json:"roles"`
}

type ReportExport struct {
	Format string `json:"format"`
}

type Rollback struct {
	TargetVersion uint64 `json:"target_version"`
}

type FollowUpAction struct {
	ActionType       string         `json:"action_type"`
	Actor            string         `json:"actor"`
	OccurredAt       time.Time      `json:"occurred_at"`
	LocalDisplayTime string         `json:"local_display_time"`
	RequestID        string         `json:"request_id"`
	Visibility       string         `json:"visibility"`
	Payload          map[string]any `json:"payload"`
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
	Request           ServiceRequest       `json:"request"`
	ConstituentLinks  []ConstituentLink    `json:"constituent_links,omitempty"`
	Notes             []RequestNote        `json:"notes,omitempty"`
	AvailableActions  []string             `json:"available_actions"`
	PrimaryAssigneeID *string              `json:"primary_assignee_id"`
	CollaboratorIDs   []string             `json:"collaborator_ids"`
	Reminders         []Reminder           `json:"reminders"`
	History           []PublicHistoryItem  `json:"history"`
	Audit             []AuditEvent         `json:"audit"`
	ExternalWorkOrder *CivicWorksWorkOrder `json:"external_work_order"`
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

type ConstituentList struct {
	Items          []Constituent  `json:"items"`
	NextPageToken  *string        `json:"next_page_token"`
	TotalCount     int            `json:"total_count"`
	AppliedFilters map[string]any `json:"applied_filters"`
	Sort           []string       `json:"sort"`
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

// FederatedRedirect describes the provider authorization hand-off. Account
// linking is never inferred from a matching email address; callers must first
// establish a local session and deliberately start another federated flow.
type FederatedRedirect struct {
	AuthorizationURL         string `json:"authorization_url"`
	LinkConfirmationRequired bool   `json:"link_confirmation_required,omitempty"`
}

type ActorRoleMapping struct {
	AssertedRole    ActorRole       `json:"asserted_role"`
	ApplicationRole ApplicationRole `json:"application_role"`
}

// IdentityConfiguration combines administrator-controlled enablement with
// effective, read-only runtime values. The OIDC secret itself is never part of
// this projection.
type IdentityConfiguration struct {
	OIDCEnabled                bool               `json:"oidc_enabled"`
	SAMLEnabled                bool               `json:"saml_enabled"`
	OIDCIssuerURL              string             `json:"oidc_issuer_url"`
	OIDCStaffClientID          string             `json:"oidc_staff_client_id"`
	OIDCPublicClientID         string             `json:"oidc_public_client_id"`
	OIDCClientSecretConfigured bool               `json:"oidc_client_secret_configured"`
	SAMLMetadataURL            string             `json:"saml_metadata_url"`
	SAMLSPServiceEntityID      string             `json:"saml_sp_entity_id"`
	ActorRoleMappings          []ActorRoleMapping `json:"actor_role_mappings"`
	Version                    uint64             `json:"version"`
	UpdatedAt                  time.Time          `json:"updated_at"`
}

// IdentityConfigurationWrite uses pointers because the frozen PATCH schema
// permits either enablement flag to be omitted.
type IdentityConfigurationWrite struct {
	OIDCEnabled *bool `json:"oidc_enabled,omitempty"`
	SAMLEnabled *bool `json:"saml_enabled,omitempty"`
}

// Actor carries server-resolved roles and record scope.
type Actor struct {
	ID         uint64
	Roles      []ApplicationRole
	Department DepartmentCode
	Districts  []DistrictCode
}
