package postgres

import (
	"time"

	"github.com/jackc/pgx/v5/tracelog"
	"go.uber.org/zap"
)

// NewQueryTracerForTest exposes newQueryTracer to the external test package,
// whose frames (unlike package postgres's own) count as application code.
func NewQueryTracerForTest(log *zap.Logger, slow time.Duration) tracelog.Logger {
	return newQueryTracer(log, slow)
}

// NewDBWithDecorateForTest exposes newDB so tests can wrap a fake pool the way NewDatabase does.
func NewDBWithDecorateForTest(pool DBPool, decorate func(DBTX) DBTX) *DB {
	return newDB(pool, decorate)
}
