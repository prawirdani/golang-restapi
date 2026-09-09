package rbac

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
