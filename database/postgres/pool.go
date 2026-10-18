// Package postgres provides PostgreSQL connection pool management, transactions, and query logging.
package postgres

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/tracelog"
	"go.uber.org/fx"
	"go.uber.org/zap"

	"github.com/jun-develop/junkit/console"
	"github.com/jun-develop/junkit/database"
)

// DBTX is the query-execution interface shared by a pool and a transaction (compatible with
// sqlc).
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// DBPool is a connection pool: a DBTX that can also begin transactions, be pinged, and be closed.
type DBPool interface {
	DBTX
	Begin(ctx context.Context) (pgx.Tx, error)
	Ping(ctx context.Context) error
	Close()
}

// Options configures NewDatabase.
type Options struct {
	// Name identifies this database in logs (e.g. "primary"); it matters once an app has several.
	Name string

	// DSN is the PostgreSQL connection string (e.g. "postgres://user:pass@host:5432/db").
	DSN string

	// MaxConnection is the maximum number of pooled connections.
	MaxConnection int32

	// MinConnection is the minimum number of pooled connections kept open.
	MinConnection int32

	// MaxConnLifetime is the maximum age of a pooled connection before it is recycled.
	MaxConnLifetime time.Duration

	// MaxConnIdleTime is the maximum idle time before a pooled connection is closed.
	MaxConnIdleTime time.Duration

	// HealthCheckPeriod is how often the pool checks idle connections' health.
	HealthCheckPeriod time.Duration

	// QueryLogging enables structured per-query logging through the supplied
	// *zap.Logger (see NewDatabase). When false, no query tracer is attached.
	QueryLogging bool

	// SlowQueryThreshold escalates any query slower than this to a WARN log
	// entry when QueryLogging is true. Zero disables slow-query escalation.
	SlowQueryThreshold time.Duration

	// Decorate optionally wraps every DBTX handed out by Executor (e.g. for tracing or metrics).
	Decorate func(DBTX) DBTX
}

// DB is the primary database handle supporting connection pooling and transactions.
type DB struct {
	pool     DBPool
	decorate func(DBTX) DBTX
	executor DBTX
}

// Params are NewDatabase's Fx-injected dependencies.
type Params struct {
	fx.In

	Lifecycle fx.Lifecycle
	Options   Options
	Logger    *zap.Logger
}

// NewDatabase opens a PostgreSQL connection pool and registers Fx lifecycle hooks.
//
// Usage:
//
//	fx.New(fx.Supply(postgres.Options{DSN: dsn, MaxConnection: 20}), postgres.Module)
func NewDatabase(p Params) (*DB, error) {
	poolConfig, err := pgxpool.ParseConfig(p.Options.DSN)
	if err != nil {
		return nil, console.NewError("parse postgres dsn: %w", err)
	}

	poolConfig.MaxConns = p.Options.MaxConnection
	poolConfig.MinConns = p.Options.MinConnection
	poolConfig.MaxConnLifetime = p.Options.MaxConnLifetime
	poolConfig.MaxConnIdleTime = p.Options.MaxConnIdleTime
	poolConfig.HealthCheckPeriod = p.Options.HealthCheckPeriod

	if p.Options.QueryLogging {
		poolConfig.ConnConfig.Tracer = &tracelog.TraceLog{
			Logger:   newQueryTracer(p.Logger, p.Options.SlowQueryThreshold),
			LogLevel: tracelog.LogLevelInfo,
		}
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolConfig)
	if err != nil {
		return nil, console.NewError("create pgx pool: %w", err)
	}

	db := newDB(pool, p.Options.Decorate)
	registerLifecycleHooks(p.Lifecycle, pool, p.Logger, p.Options.Name, p.Options.DSN)

	return db, nil
}

// NewDB builds a DB from an existing DBPool, without a Decorate wrapper.
func NewDB(pool DBPool) *DB {
	return newDB(pool, nil)
}

// newDB constructs a DB, pre-decorating the pool-level executor once if decorate is non-nil.
func newDB(pool DBPool, decorate func(DBTX) DBTX) *DB {
	var executor DBTX = pool
	if decorate != nil {
		executor = decorate(pool)
	}

	return &DB{pool: pool, decorate: decorate, executor: executor}
}

// registerLifecycleHooks pings the pool on start and closes it on stop.
func registerLifecycleHooks(lc fx.Lifecycle, pool DBPool, log *zap.Logger, name, dsn string) {
	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			if err := pool.Ping(ctx); err != nil {
				return console.NewError("ping database: %w", err)
			}

			log.Info("database: connected to PostgreSQL", zap.String("name", name), zap.String("dsn", database.MaskDSN(dsn)))

			return nil
		},
		OnStop: func(ctx context.Context) error {
			log.Info("database: closing connections", zap.String("name", name))
			pool.Close()

			return nil
		},
	})
}

// Executor returns the active transaction DBTX from ctx if available, otherwise the pool executor.
//
// Usage:
//
//	queries := sqlc.New(db.Executor(ctx))
func (d *DB) Executor(ctx context.Context) DBTX {
	if tx, ok := d.getTx(ctx); ok {
		if d.decorate != nil {
			return d.decorate(tx)
		}

		return tx
	}

	return d.executor
}

// Ping verifies the database is reachable. There is deliberately no accessor for the raw pool:
// queries run only through Executor, so they are always traced and join the active transaction.
func (d *DB) Ping(ctx context.Context) error {
	return d.pool.Ping(ctx)
}
