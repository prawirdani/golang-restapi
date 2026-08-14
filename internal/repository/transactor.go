// Package repository defines cross-cutting persistence port interfaces shared
// by services and infrastructure, such as the Transactor for atomic multi-repository writes.
package repository

import "context"

// Transactor enables atomic operations across multiple repositories and service layers
// within a single database transaction. It guarantees that all operations either commit
// together or roll back together, maintaining data consistency even when operations span
// different repositories or are called through multiple service layers.
//
// Key Features:
//   - Atomicity: All operations succeed or all fail as a single unit
//   - Cross-Repository: Works across any repository that supports transactional contexts
//   - Cross-Service: Maintains atomicity even when repository calls are nested through
//     multiple service layers
//   - Automatic Management: Handles begin/commit/rollback automatically
//
// Implementation Requirements:
//   - Must use native database atomicity features:
//   - SQL databases: Use database transactions (BEGIN/COMMIT/ROLLBACK)
//   - Other databases: Use their equivalent transactional mechanisms
//   - Repositories must check for and use transactional connections from the context
//   - All participating methods must propagate the transactional context
//
// Usage Example:
//
//	err := transactor.Transact(ctx, func(txCtx context.Context) error {
//	    // These operations will be atomic, even if called through different services
//	    if err := orderService.CreateOrder(txCtx, order); err != nil {
//	        return err // Automatic rollback
//	    }
//	    if err := productService.DecrementQuantity(txCtx, qty); err != nil {
//	        return err // Automatic rollback
//	    }
//	    return nil // Automatic commit
//	})
//
// Critical: Always pass the provided txCtx to all database operations. Without it,
// operations will execute outside the transaction and break atomicity.
type Transactor interface {
	Transact(ctx context.Context, fn func(ctx context.Context) error) error
}
