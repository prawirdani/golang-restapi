package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/prawirdani/golang-restapi/internal/domain"
	"github.com/prawirdani/golang-restapi/internal/domain/user"
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
		"id":            u.ID,
		"name":          u.Name,
		"email":         u.Email,
		"password":      u.Password,
		"phone":         u.Phone,
		"profile_image": u.ProfileImage,
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
		"name":          u.Name,
		"email":         u.Email,
		"password":      u.Password,
		"phone":         u.Phone,
		"profile_image": u.ProfileImage,
		"updated_at":    "NOW()",
		"id":            u.ID, // for WHERE clause
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

func (r *userRepository) getUserBy(
	ctx context.Context,
	field string,
	value any,
) (*user.User, error) {
	query := strs.Concatenate(
		`
		SELECT u.id, u.name, u.email, u.phone, u.password, u.profile_image, u.created_at, u.updated_at FROM users AS u WHERE u.`,
		field,
		"=$1",
	)
	conn := r.db.GetConn(ctx)
	if r.db.IsTxConn(conn) {
		query += "\nFOR UPDATE"
	}

	var u user.User
	if err := pgxscan.Get(ctx, conn, &u, query, value); err != nil {
		if noRowsErr(err) {
			key := "user_id"
			if field == "email" {
				key = "user_email"
			}
			return nil, domain.ErrNotFound.WithDetails(map[string]any{
				key: value,
			})
		}
		return nil, fmt.Errorf("query: %w", err)
	}

	return &u, nil
}
