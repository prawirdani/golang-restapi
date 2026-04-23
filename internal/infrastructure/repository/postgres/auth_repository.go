package postgres

import (
	"context"
	"errors"

	"github.com/georgysavva/scany/v2/pgxscan"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/prawirdani/golang-restapi/internal/domain"
	"github.com/prawirdani/golang-restapi/internal/domain/auth"
	"github.com/prawirdani/golang-restapi/pkg/log"
)

type authRepository struct {
	db *DB
}

func NewAuthRepository(db *DB) *authRepository {
	return &authRepository{
		db: db,
	}
}

// StoreSession implements [auth.Repository]
func (r *authRepository) StoreSession(ctx context.Context, session *auth.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	args := pgx.NamedArgs{
		"id":            session.ID,
		"user_id":       session.UserID,
		"refresh_token": session.RefreshTokenHash,
		"user_agent":    session.UserAgent,
		"expires_at":    session.ExpiresAt,
		"accessed_at":   session.AccessedAt,
	}
	query := generateInsertQuery("sessions", args)
	conn := r.db.GetConn(ctx)

	_, err := conn.Exec(ctx, query, args)
	if err != nil {
		log.ErrorCtx(ctx, "Failed to store session", err)
		return err
	}

	return nil
}

// UpdateSession implements [auth.Repository]
func (r *authRepository) UpdateSession(ctx context.Context, session *auth.Session) error {
	if session == nil {
		return errors.New("session is nil")
	}

	query := "UPDATE sessions SET refresh_token=$1, revoked_at=$2 WHERE id=$3"
	conn := r.db.GetConn(ctx)

	if _, err := conn.Exec(ctx, query, session.RefreshTokenHash, session.RevokedAt, session.ID); err != nil {
		log.ErrorCtx(ctx, "Failed to updated session", err)
		return err
	}

	return nil
}

// GetSessionByID implements [auth.Repository]
func (r *authRepository) GetSessionByID(
	ctx context.Context,
	sessionID uuid.UUID,
) (*auth.Session, error) {
	query := "SELECT * FROM sessions WHERE id=$1"
	conn := r.db.GetConn(ctx)
	if r.db.IsTxConn(conn) {
		query += "\nFOR UPDATE"
	}

	var sess auth.Session
	if err := pgxscan.Get(ctx, conn, &sess, query, sessionID); err != nil {
		if noRowsErr(err) {
			return nil, domain.ErrNotFound
		}
		log.ErrorCtx(ctx, "Failed to get session by id", err)
		return nil, err
	}

	return &sess, nil
}

// GetSessionByRefreshTokenHash implements [auth.Repository]
func (r *authRepository) GetSessionByRefreshTokenHash(
	ctx context.Context,
	tokenHash []byte,
) (*auth.Session, error) {
	query := "SELECT * FROM sessions WHERE refresh_token=$1"
	conn := r.db.GetConn(ctx)
	if r.db.IsTxConn(conn) {
		query += "\nFOR UPDATE"
	}

	var session auth.Session
	if err := pgxscan.Get(ctx, conn, &session, query, tokenHash); err != nil {
		if noRowsErr(err) {
			return nil, domain.ErrNotFound
		}
		log.ErrorCtx(ctx, "Failed to get session by refresh token hash", err)
		return nil, err
	}

	return &session, nil
}

// GetPasswordRecoveryToken implements [auth.Repository]
func (r *authRepository) GetPasswordRecoveryToken(
	ctx context.Context,
	tokenHash []byte,
) (*auth.PasswordRecoveryToken, error) {
	query := "SELECT * FROM password_recovery_tokens WHERE token_hash=$1"

	conn := r.db.GetConn(ctx)
	if r.db.IsTxConn(conn) {
		query += "\nFOR UPDATE"
	}

	var token auth.PasswordRecoveryToken
	if err := pgxscan.Get(ctx, conn, &token, query, tokenHash); err != nil {
		if noRowsErr(err) {
			return nil, domain.ErrNotFound
		}
		log.ErrorCtx(ctx, "Failed to get password recovery token", err)
		return nil, err
	}

	return &token, nil
}

// StorePasswordRecoveryToken implements [auth.Repository]
func (r *authRepository) StorePasswordRecoveryToken(
	ctx context.Context,
	token *auth.PasswordRecoveryToken,
) error {
	if token == nil {
		return errors.New("password recovery token is nil")
	}
	args := pgx.NamedArgs{
		"user_id":    token.UserID,
		"token_hash": token.TokenHash,
		"issued_at":  token.IssuedAt,
		"expires_at": token.ExpiresAt,
	}
	query := generateInsertQuery("password_recovery_tokens", args)
	conn := r.db.GetConn(ctx)

	if _, err := conn.Exec(ctx, query, args); err != nil {
		log.ErrorCtx(ctx, "Failed to store password recovery token", err)
		return err
	}

	return nil
}

// UpdatePasswordRecoveryToken implements [auth.Repository]
func (r *authRepository) UpdatePasswordRecoveryToken(
	ctx context.Context,
	token *auth.PasswordRecoveryToken,
) error {
	if token == nil {
		return errors.New("password recovery token is nil")
	}

	query := "UPDATE password_recovery_tokens SET used_at=$1 WHERE id=$2"
	conn := r.db.GetConn(ctx)

	if _, err := conn.Exec(ctx, query, token.UsedAt, token.ID); err != nil {
		log.ErrorCtx(ctx, "Failed to update password recovery token", err)
		return err
	}

	return nil
}
