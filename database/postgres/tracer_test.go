package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jun-develop/junkit/database/postgres"
	"github.com/jun-develop/junkit/tracer"
)

// emptyRows is a pgx.Rows with no rows.
type emptyRows struct{ pgx.Rows }

func (emptyRows) Next() bool                                   { return false }
func (emptyRows) Err() error                                   { return nil }
func (emptyRows) Close()                                       {}
func (emptyRows) FieldDescriptions() []pgconn.FieldDescription { return nil }

// noRow is a pgx.Row for a query that matched nothing.
type noRow struct{}

func (noRow) Scan(...any) error { return pgx.ErrNoRows }

// queryTx is a fakeTx that also answers queries, so traced statements can run inside a transaction.
type queryTx struct{ fakeTx }

func (*queryTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (*queryTx) Query(context.Context, string, ...any) (pgx.Rows, error) { return emptyRows{}, nil }

func (*queryTx) QueryRow(context.Context, string, ...any) pgx.Row { return noRow{} }

// queryPool is a DBPool that answers queries and begins a queryTx.
type queryPool struct {
	postgres.DBPool
	tx *queryTx
}

func (*queryPool) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (*queryPool) Query(context.Context, string, ...any) (pgx.Rows, error) { return emptyRows{}, nil }

func (*queryPool) QueryRow(context.Context, string, ...any) pgx.Row { return noRow{} }

func (p *queryPool) Begin(context.Context) (pgx.Tx, error) { return p.tx, nil }

// TestExecutor_TracesEveryQuery is the tracer contract every engine package must pass: every query
// run through Executor, sqlc-generated or raw, inside or outside a transaction, reaches the Debugger.
func TestExecutor_TracesEveryQuery(t *testing.T) {
	tx := &queryTx{}
	db := postgres.NewDBWithDecorateForTest(&queryPool{tx: tx}, func(inner postgres.DBTX) postgres.DBTX {
		return tracer.NewPgxWrapper(inner)
	})

	d := tracer.New()
	ctx := tracer.WithContext(context.Background(), d)

	run := func(ctx context.Context) {
		if _, err := db.Executor(ctx).Exec(ctx, "UPDATE users SET seen = $1", true); err != nil {
			t.Fatal(err)
		}

		rows, err := db.Executor(ctx).Query(ctx, "SELECT id FROM users")
		if err != nil {
			t.Fatal(err)
		}
		rows.Close()

		var id int
		if err := db.Executor(ctx).QueryRow(ctx, "SELECT id FROM users WHERE id = $1", 7).Scan(&id); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("QueryRow().Scan() error = %v, want pgx.ErrNoRows", err)
		}
	}

	run(ctx)

	if err := db.WithTransaction(ctx, func(txCtx context.Context) error {
		run(txCtx)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if !tx.committed {
		t.Fatal("expected the transaction to be committed")
	}

	want := []string{
		"UPDATE users SET seen = true",
		"SELECT id FROM users",
		"SELECT id FROM users WHERE id = 7",
	}
	want = append(want, want...)

	logs := d.Logs()
	if len(logs) != len(want) {
		t.Fatalf("got %d traced queries, want %d (3 outside and 3 inside the transaction)", len(logs), len(want))
	}

	for i, e := range logs {
		if got := e.Data.(tracer.SQLData).SQL; got != want[i] {
			t.Errorf("query %d SQL = %q, want %q", i, got, want[i])
		}
	}
}
