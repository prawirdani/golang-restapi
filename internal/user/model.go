package user

import (
	"github.com/prawirdani/golang-restapi/internal/ports/repository"
	"github.com/prawirdani/golang-restapi/pkg/strings"
)

// Filter is a model for filtering list of users.
type Filter struct {
	Gender []string `query:"gender"`
	Role   []string `query:"role"`
	repository.Sort
	repository.Pagination
}

var userSortFields = map[string]string{
	"id":         "id",
	"created_at": "created_at",
	"updated_at": "updated_at",
}

// Apply implements the filter contract for a user list query: clauses first,
// then sort, then pagination.
//
// The pointer receiver is deliberate. ApplyPagination clamps Page and Limit in
// place, and the pagination metadata is reported from the clamped values.
func (f *Filter) Apply(q repository.Query) {
	q.WhereIn("gender", f.Gender)
	q.WhereIn("role", f.Role)

	f.ApplySort(q, userSortFields)
	f.ApplyPagination(q)
}

type UpdateUserInput struct {
	Name   string `json:"name"   validate:"required"`
	Phone  string `json:"phone"`
	Gender string `json:"gender" validate:"omitempty,oneof=m M f F o O"`
}

// Sanitize implements [validator.Sanitizer]
func (i *UpdateUserInput) Sanitize() error {
	i.Name = strings.TrimSpacesConcat(i.Name)
	i.Phone = strings.TrimSpaces(i.Phone)
	i.Gender = strings.TrimSpaces(i.Gender)
	return nil
}
