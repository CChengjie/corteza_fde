package store

import (
	"context"
	"fmt"
)

// LockCity311ConfigurationResource serializes reads and writes that enforce a
// managed configuration vocabulary inside an existing transaction.
func LockCity311ConfigurationResource(ctx context.Context, s Storer, resourceType, resourceKey string) error {
	locker, ok := s.(interface {
		LockCity311ConfigurationResource(context.Context, string, string) error
	})
	if !ok {
		return fmt.Errorf("store does not support City 311 configuration resource locking")
	}
	return locker.LockCity311ConfigurationResource(ctx, resourceType, resourceKey)
}
