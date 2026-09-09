package rbac

// Permission type, PascalCase per entity convention. Values are
// "<entity>.<verb>" strings so they read naturally in audit/log output.
type Permission string

type PermissionTable = map[Role]map[Permission]struct{}

// Example:
// const (
// 	PermReadTodo	Permission = "todo.read"
// 	PermUpdateTodo	Permission = "todo.update"
// )
//
// var permsTable PermissionTable = PermissionTable{
// 	RoleAdmin: {
// 		PermReadTodo: {},
// 		PermUpdateTodo: {},
// 	},
// 	RoleUser: {
// 		PermReadTodo: {},
// 	},
// }
