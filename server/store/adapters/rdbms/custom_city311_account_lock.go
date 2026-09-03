package rdbms

import (
	"context"

	"github.com/doug-martin/goqu/v9"
)

// A no-value-change UPDATE takes a database write lock until the surrounding
// transaction ends, including on SQLite where SELECT FOR UPDATE is unavailable.
// Call before reading any account/profile projection, not with stale row values.
func (s Store) LockCity311LocalAccount(ctx context.Context, userID uint64) error {
	return s.Exec(ctx, s.Dialect.GOQU().Update("compose_city311_local_account").
		Set(goqu.Record{"id": goqu.C("id")}).Where(goqu.C("id").Eq(userID)))
}
