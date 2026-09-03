package store

import (
	"context"
	"fmt"
)

// LockCity311LocalAccount serializes account/profile projection updates inside
// an existing transaction. Keep the hand-written store extension out of codegen.
func LockCity311LocalAccount(ctx context.Context, s Storer, userID uint64) error {
	locker, ok := s.(interface {
		LockCity311LocalAccount(context.Context, uint64) error
	})
	if !ok {
		return fmt.Errorf("store does not support City 311 account locking")
	}
	return locker.LockCity311LocalAccount(ctx, userID)
}
