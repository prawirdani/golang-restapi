package audit

import (
	"context"

	"github.com/prawirdani/golang-restapi/internal/rbac"
)

const (
	PermRead rbac.Permission = "audit.read"
)

var permTables = rbac.PermissionTable{
	rbac.RoleSystem: {PermRead: {}},
	rbac.RoleAdmin:  {PermRead: {}},
}

type Service struct {
	authorizer rbac.Authorizer
	reader     Reader
}

func NewAuditService(authorizer rbac.Authorizer, reader Reader) *Service {
	authorizer.RegisterPermissions(permTables)

	return &Service{
		authorizer: authorizer,
		reader:     reader,
	}
}

func (s *Service) List(ctx context.Context) ([]Entry, error) {
	if err := s.authorizer.Require(ctx, PermRead); err != nil {
		return nil, err
	}
	return s.reader.List(ctx)
}
