package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prawirdani/golang-restapi/config"
	"github.com/prawirdani/golang-restapi/pkg/log"
)

// PGQuery abstracts the query operations used by repository implementations.
// It is implemented by both *pgxpool.Pool and pgx.Tx, allowing code to operate
// transparently with or without an active transaction.
type PGQuery interface {
	// Exec executes a statement that does not return rows.
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)

	// Query executes a query that returns multiple rows.
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)

	// QueryRow executes a query expected to return at most one row.
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row

	// CopyFrom performs a bulk insert into the target table.
	CopyFrom(
		ctx context.Context,
		tableName pgx.Identifier,
		columnNames []string,
		rowSrc pgx.CopyFromSource,
	) (int64, error)

	// SendBatch executes a batch of queued statements.
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// DB wraps a pgxpool.Pool and provides helpers for transactional and
// non-transactional query execution.
type DB struct {
	pool *pgxpool.Pool
}

type ctxKey struct{}

var txCtxKey ctxKey

// GetConn returns the transactional connection stored in the context if one
// exists; otherwise it returns the default pool connection.
func (db *DB) GetConn(ctx context.Context) PGQuery {
	tx, ok := ctx.Value(txCtxKey).(pgx.Tx)
	if !ok {
		return db.pool
	}
	return tx
}

// MustGetTxConn returns the active transactional connection. It returns an error if
// no transaction has been started in the provided context.
func (db *DB) MustGetTxConn(ctx context.Context) (PGQuery, error) {
	tx, ok := ctx.Value(txCtxKey).(pgx.Tx)
	if !ok {
		return nil, errors.New("required transaction connection")
	}
	return tx, nil
}

// IsTxConn reports whether the given connection is a pgx.Tx.
func (db *DB) IsTxConn(conn any) bool {
	switch conn.(type) {
	case pgx.Tx:
		return true
	default:
		return false
	}
}

// Transact runs fn within a database transaction. If fn returns an error,
// the transaction is rolled back; otherwise it is committed. The transactional
// connection is injected into the context passed to fn.
func (db *DB) Transact(
	ctx context.Context,
	fn func(ctx context.Context) error,
) (err error) { // named return for defer
	conn, err := db.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("tx failed to acquire connection: %w", err)
	}
	defer conn.Release()

	tx, err := conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("tx failed to begin transaction: %w", err)
	}

	log.DebugCtx(ctx, "Transaction Begin")

	// Handle panics explicitly for better observability and resource efficiency.
	// Note: pgx's conn.Release() will detect open transactions (TxStatus != 'I')
	// and destroy the connection, which causes PostgreSQL to auto-rollback.
	// However, explicit panic handling is better because:
	// - Sends graceful ROLLBACK command instead of destroying connection
	// - Keeps connection healthy and returns it to pool (avoids connection churn)
	// - Releases locks immediately rather than waiting for PG to detect disconnection
	// - Provides explicit logging for debugging
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback(ctx)
			log.DebugCtx(ctx, "Transaction Rollback (panic)")
			panic(p) // re-panic after rollback
		} else if err != nil {
			if rbErr := tx.Rollback(ctx); rbErr != nil {
				err = fmt.Errorf("tx failed: %w, rollback failed: %w", err, rbErr)
			} else {
				log.DebugCtx(ctx, "Transaction Rollback")
			}
		} else {
			if commitErr := tx.Commit(ctx); commitErr != nil {
				err = fmt.Errorf("tx failed to commit: %w", commitErr)
			} else {
				log.DebugCtx(ctx, "Transaction Committed")
			}
		}
	}()

	txCtx := context.WithValue(ctx, txCtxKey, tx)
	err = fn(txCtx)
	return err
}

// Close shuts down the underlying PostgreSQL connection pool.
func (db *DB) Close() {
	db.pool.Close()
}

// New initializes a PostgreSQL connection pool using the provided configuration.
// It verifies connectivity by performing an initial Ping.
func New(cfg config.Postgres) (*DB, error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%v/%s",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Name,
	)

	pgConf, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}

	pgConf.MinConns = int32(cfg.MinConns)
	pgConf.MaxConns = int32(cfg.MaxConns)
	pgConf.MaxConnLifetime = cfg.MaxConnLifetime

	pool, err := pgxpool.NewWithConfig(context.Background(), pgConf)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(context.Background()); err != nil {
		return nil, err
	}

	return &DB{pool: pool}, nil
}
