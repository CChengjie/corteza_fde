package rdbms

import (
	"context"

	"github.com/doug-martin/goqu/v9"
)

// LockCity311ConfigurationResource takes a database write lock on every
// immutable revision for one stable resource key until the surrounding
// transaction ends. This lets assignment and deactivation share one lock even
// though the logical resource is stored as an append-only revision stream.
func (s Store) LockCity311ConfigurationResource(ctx context.Context, resourceType, resourceKey string) error {
	return s.Exec(ctx, s.Dialect.GOQU().Update("compose_city311_configuration_revision").
		Set(goqu.Record{"id": goqu.C("id")}).
		Where(goqu.Ex{"resource_type": resourceType, "resource_key": resourceKey}))
}
