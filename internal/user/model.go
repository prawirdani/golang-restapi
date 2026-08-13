package user

import "github.com/prawirdani/golang-restapi/pkg/strings"

type CreateUserInput struct {
	Name     string `json:"name"     validate:"required"`
	Email    string `json:"email"    validate:"required,email"`
	Phone    string `json:"phone"`
	Password string `json:"password" validate:"required,min=8,max=72"`
	Gender   string `json:"gender"   validate:"omitempty,oneof=m M f F o O"`
}

// Sanitize implements [validator.Sanitizer]
func (i *CreateUserInput) Sanitize() error {
	i.Name = strings.TrimSpacesConcat(i.Name)
	i.Email = strings.TrimSpaces(i.Email)
	i.Phone = strings.TrimSpaces(i.Phone)
	i.Gender = strings.TrimSpaces(i.Gender)
	return nil
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
