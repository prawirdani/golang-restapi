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
	"name":       "name",
	"created_at": "created_at",
	"updated_at": "updated_at",
}

// ApplySort implements [repository.Sorter]
func (f Filter) ApplySort(q repository.Query) {
	f.Sort.ApplySort(q, userSortFields)
}

// ApplyFilter implements [repository.Filterer].
func (f Filter) ApplyFilter(q repository.Query) {
	q.WhereIn("gender", f.Gender)
	q.WhereIn("role", f.Role)
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
