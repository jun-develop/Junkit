package postgres

import "go.uber.org/fx"

// Module provides an Fx module that constructs a *DB instance from Options and *zap.Logger. An app
// with several databases skips it and calls NewDatabase from one constructor per database instead.
//
// Usage:
//
//	fx.New(
//	    fx.Supply(postgres.Options{DSN: dsn, MaxConnection: 20}),
//	    postgres.Module,
//	)
var Module = fx.Module("database",
	fx.Provide(NewDatabase),
)
