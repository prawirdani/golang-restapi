package rbac

import "strings"

// Role is a user role in the RBAC model.
type Role string

const (
	// RoleAdmin has access to everything configuration and user management.
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
	// RoleSystem identifies background workers (actor_id NULL in audit logs).
	RoleSystem Role = "system"
)

func (r Role) Valid() bool {
	switch r {
	case RoleAdmin, RoleSystem, RoleUser:
		return true
	default:
		return false
	}
}

// ParseRole canonicalises and validates a client-supplied role name, so filter
// values can be accepted in any casing and unknown ones rejected.
func ParseRole(s string) (Role, bool) {
	r := Role(strings.ToLower(strings.TrimSpace(s)))

	return r, r.Valid()
}
