package database

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/haidarrasyad/absensi-api/internal/pkg/apperror"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Queryer interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}
type transactionKey struct{}
type transactionState struct {
	tx      pgx.Tx
	pool    *pgxpool.Pool
	cleanup []func(context.Context) error
}

func Executor(ctx context.Context, pool *pgxpool.Pool) Queryer {
	if state, ok := ctx.Value(transactionKey{}).(*transactionState); ok && state.pool == pool {
		return state.tx
	}
	return pool
}

// OnRollback registers compensation for a newly created file. Filesystem and DB
// cannot form one atomic transaction; ambiguous commits retain files for reconciliation.
func OnRollback(ctx context.Context, fn func(context.Context) error) bool {
	state, ok := ctx.Value(transactionKey{}).(*transactionState)
	if ok {
		state.cleanup = append(state.cleanup, fn)
	}
	return ok
}
func ConcurrencyError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "55P03" || pgErr.Code == "40P01" || pgErr.Code == "40001" || pgErr.Code == "57014") || errors.Is(err, context.DeadlineExceeded) {
		return apperror.New(http.StatusServiceUnavailable, "operation_busy", "Operasi sedang sibuk. Coba lagi sebentar.")
	}
	return err
}

// Rollback uses a fresh bounded context even if the request was cancelled.
func Rollback(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = tx.Rollback(ctx)
}
func ConfigureTransaction(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `SET LOCAL lock_timeout='3s'; SET LOCAL statement_timeout='15s'; SET LOCAL idle_in_transaction_session_timeout='30s'`)
	return err
}

type Coordinator struct{ pool *pgxpool.Pool }

func NewCoordinator(pool *pgxpool.Pool) *Coordinator { return &Coordinator{pool: pool} }
func (c *Coordinator) Within(ctx context.Context, key string, fn func(context.Context) error) (err error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	tx, err := c.pool.Begin(ctx)
	if err != nil {
		return ConcurrencyError(err)
	}
	state := &transactionState{tx: tx, pool: c.pool}
	committed, commitAttempted := false, false
	defer func() {
		if !committed {
			Rollback(tx)
			if !commitAttempted || errors.Is(err, pgx.ErrTxCommitRollback) {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cleanupCancel()
				for i := len(state.cleanup) - 1; i >= 0; i-- {
					if cleanupErr := state.cleanup[i](cleanupCtx); cleanupErr != nil {
						slog.Error("rollback file cleanup failed", "error", cleanupErr)
					}
				}
			}
		}
		err = ConcurrencyError(err)
	}()
	if err = ConfigureTransaction(ctx, tx); err != nil {
		return err
	}
	// Transaction-scoped lock is released on commit, rollback or lost connection.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, key); err != nil {
		return err
	}
	if err = fn(context.WithValue(ctx, transactionKey{}, state)); err != nil {
		return err
	}
	commitAttempted = true
	err = tx.Commit(ctx)
	committed = err == nil
	return err
}
