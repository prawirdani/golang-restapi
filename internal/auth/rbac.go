package auth

type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
	// RoleSystem identifies background workers (actor_id NULL in audit logs).
	RoleSystem Role = "system"
)
