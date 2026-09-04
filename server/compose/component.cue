package compose

import (
	"github.com/cortezaproject/corteza/server/codegen/schema"
)

component: schema.#component & {
	handle: "compose"

	resources: {
		"attachment":                    attachment
		"chart":                         chart
		"city311-actor-profile":         actorProfile
		"city311-audit-event":           auditEvent
		"city311-constituent":           constituent
		"city311-idempotency-record":    idempotencyRecord
		"city311-identity-notification": identityNotification
		"city311-identity-session":      identitySession
		"city311-local-account":         localAccount
		"city311-password-reset-token":  passwordResetToken
		"city311-public-history-item":   publicHistoryItem
		"city311-request-attachment":    requestAttachment
		"city311-request-constituent":   requestConstituentLink
		"city311-request-note":          requestNote
		"city311-request-sequence":      requestSequence
		"city311-service-request":       serviceRequest
		"module":                        module
		"module-field":                  moduleField
		"namespace":                     namespace
		"page":                          page
		"page-layout":                   pageLayout
		"record":                        record
		"record-revision":               record_revision
	}

	rbac: operations: {
		"settings.read": description:                "Read settings"
		"settings.manage": description:              "Manage settings"
		"namespace.create": description:             "Create namespace"
		"namespaces.search": description:            "List, search or filter namespaces"
		"resource-translations.manage": description: "List, search, create, or update resource translations"
	}
}
