package rbac

import (
	"context"
	"sync"

	"github.com/google/uuid"
	"github.com/prawirdani/golang-restapi/internal/apperr"
)

var ErrUnauthorizedPermission = apperr.ForbiddenErr("unauthorized action", "RBAC_UNAUTHORIZED_PERM")

type Authorizer interface {
	// Require passes only when the actor's role holds ALL given permissions.
	Require(ctx context.Context, permissions ...Permission) error
	// RequireSelfOr passes when the actor is the target user, otherwise
	// falls back to the permission check.
	RequireSelfOr(ctx context.Context, userID uuid.UUID, permission Permission) error
	// RegisterPermissions merges an entity's role->permission table into the
	// in-memory authorization table. Called once per entity at startup.
	RegisterPermissions(perms PermissionTable)
}

type authorizer struct {
	mu    sync.RWMutex
	table PermissionTable
}

func NewAuthorizer() *authorizer {
	return &authorizer{
		table: make(PermissionTable),
	}
}

// RegisterPermissions implements [Authorizer].
func (a *authorizer) RegisterPermissions(perms PermissionTable) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for role, permissions := range perms {
		if a.table[role] == nil {
			a.table[role] = make(map[Permission]struct{})
		}
		for perm := range permissions {
			a.table[role][perm] = struct{}{}
		}
	}
}

// Require implements [Authorizer].
func (a *authorizer) Require(ctx context.Context, perms ...Permission) error {
	rbacCtx, err := GetContext(ctx)
	if err != nil {
		return err
	}
	return a.can(rbacCtx.Actor.Role, perms...)
}

// RequireSelfOr implements [Authorizer].
func (a *authorizer) RequireSelfOr(ctx context.Context, userID uuid.UUID, perm Permission) error {
	rbacCtx, err := GetContext(ctx)
	if err != nil {
		return err
	}

	if rbacCtx.Actor.UserID != nil && *rbacCtx.Actor.UserID == userID {
		return nil
	}

	return a.can(rbacCtx.Actor.Role, perm)
}

// can returns nil only when the role holds every requested permission.
func (a *authorizer) can(role Role, perms ...Permission) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	rolePerms, ok := a.table[role]
	if !ok {
		return ErrUnauthorizedPermission
	}

	for _, perm := range perms {
		if _, ok := rolePerms[perm]; !ok {
			return ErrUnauthorizedPermission
		}
	}

	return nil
}
