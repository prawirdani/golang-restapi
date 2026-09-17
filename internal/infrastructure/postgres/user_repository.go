package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/prawirdani/golang-restapi/internal/apperr"
	"github.com/prawirdani/golang-restapi/internal/ports/repository"
	"github.com/prawirdani/golang-restapi/internal/user"
	strs "github.com/prawirdani/golang-restapi/pkg/strings"
)

type userRepository struct {
	db *DB
}

func NewUserRepository(db *DB) *userRepository {
	return &userRepository{
		db: db,
	}
}

// Store implements [user.Repository].
func (r *userRepository) Store(ctx context.Context, u *user.User) error {
	if u == nil {
		return errors.New("user is nil")
	}

	args := pgx.NamedArgs{
		"id":                u.ID,
		"name":              u.Name,
		"email":             u.Email,
		"email_verified_at": u.EmailVerifiedAt,
		"password":          u.Password,
		"phone":             u.Phone,
		"gender":            u.Gender,
		"role":              u.Role,
		"profile_picture":   u.ProfilePicture,
	}

	query := generateInsertQuery("users", args) + "\nRETURNING created_at, updated_at"
	conn := r.db.GetConn(ctx)
	if err := conn.QueryRow(ctx, query, args).Scan(&u.CreatedAt, &u.UpdatedAt); err != nil {
		if uniqueViolationErr(err, "users_email_key") {
			return user.ErrEmailConflict.WithDetails(map[string]any{
				"email": u.Email,
			})
		}

		return fmt.Errorf("store user: %w", err)
	}
	return nil
}

// List implements [user.Repository].
func (r *userRepository) List(ctx context.Context, filter *user.Filter) ([]user.User, error) {
	if filter == nil {
		return nil, errors.New("filter is nil")
	}

	qb := Select(
		"users",
		"id",
		"name",
		"email",
		"email_verified_at",
		"phone",
		"password",
		"gender",
		"role",
		"profile_picture",
		"created_at",
		"updated_at",
	)
	qb.WhereNull("deleted_at")
	repository.ApplyQuery(qb, filter)

	conn := r.db.GetConn(ctx)
	query, args := qb.SQL()

	users := make([]user.User, 0)
	if err := pgxscan.Select(ctx, conn, &users, query, args...); err != nil {
		return nil, fmt.Errorf("list user: %w", err)
	}

	cQuery, cArgs := qb.CountSQL()
	var totalData int
	if err := pgxscan.Get(ctx, conn, &totalData, cQuery, cArgs...); err != nil {
		return nil, fmt.Errorf("count list user: %w", err)
	}

	filter.SetMeta(totalData)

	return users, nil
}

// GetByEmail implements [user.Repository] [auth.UserRepository].
func (r *userRepository) GetByEmail(ctx context.Context, email string) (*user.User, error) {
	return r.getUserBy(ctx, "email", email)
}

// GetByID implements [user.Repository] [auth.UserRepository].
func (r *userRepository) GetByID(ctx context.Context, userID uuid.UUID) (*user.User, error) {
	return r.getUserBy(ctx, "id", userID)
}

// Update implements [user.Repository] [auth.UserRepository].
func (r *userRepository) Update(ctx context.Context, u *user.User) error {
	if u == nil {
		return errors.New("user is nil")
	}

	args := pgx.NamedArgs{
		"name":              u.Name,
		"email":             u.Email,
		"email_verified_at": u.EmailVerifiedAt,
		"password":          u.Password,
		"phone":             u.Phone,
		"gender":            u.Gender,
		"profile_picture":   u.ProfilePicture,
		"updated_at":        "NOW()",
		"id":                u.ID, // for WHERE clause
	}

	query := generateUpdateQuery("users", args, "id") + "\nRETURNING updated_at"

	conn := r.db.GetConn(ctx)
	err := conn.QueryRow(ctx, query, args).Scan(&u.UpdatedAt)
	if err != nil {
		if uniqueViolationErr(err, "users_email_key") {
			return user.ErrEmailConflict.WithDetails(map[string]any{
				"email": u.Email,
			})
		}

		return fmt.Errorf("update user: %w", err)
	}

	return nil
}

// Delete implements [user.Repository].
func (r *userRepository) Delete(ctx context.Context, u *user.User) error {
	if u == nil {
		return errors.New("user is nil")
	}

	conn := r.db.GetConn(ctx)
	_, err := conn.Exec(ctx, "UPDATE users SET deleted_at=NOW() WHERE id=$1", u.ID)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	return nil
}

const userSelectQuery = `
SELECT 
	u.id,
	u.name,
	u.email,
	u.email_verified_at,
	u.phone,
	u.password,
	u.gender,
	u.role,
	u.profile_picture,
	u.created_at,
	u.updated_at 
FROM users AS u WHERE u.`

func (r *userRepository) getUserBy(
	ctx context.Context,
	field string,
	value any,
) (*user.User, error) {
	query := strs.Concatenate(
		userSelectQuery,
		field,
		"=$1 AND u.deleted_at IS NULL",
	)
	conn := r.db.GetConn(ctx)
	if r.db.IsTxConn(conn) {
		query += "\nFOR UPDATE"
	}

	var u user.User
	if err := pgxscan.Get(ctx, conn, &u, query, value); err != nil {
		if noRowsErr(err) {
			return nil, apperr.ErrNotFound.WithDetails(map[string]any{
				"user_" + field: value,
			})
		}
		return nil, fmt.Errorf("query: %w", err)
	}

	return &u, nil
}
