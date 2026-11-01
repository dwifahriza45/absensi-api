package shared

import "context"

// MutationCoordinator serializes a business operation across API instances.
// Callbacks execute synchronously; a transaction must not be shared by goroutines.
type MutationCoordinator interface {
	Within(context.Context, string, func(context.Context) error) error
}
