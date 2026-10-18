package postgres

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jun-develop/junkit/database"
)

func init() {
	database.RegisterClassifier(classify)
}

// classify implements database.Classifier for pgx errors.
func classify(err error) (database.ErrorType, string, bool) {
	if errors.Is(err, pgx.ErrNoRows) {
		return database.ErrorNotFound, "resource not found", true
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		errType, detail := classifyPgError(pgErr)
		return errType, detail, true
	}

	return database.ErrorNone, "", false
}

// classifyPgError maps a Postgres SQLSTATE code to an ErrorType.
// See https://www.postgresql.org/docs/current/errcodes-appendix.html.
func classifyPgError(pgErr *pgconn.PgError) (database.ErrorType, string) {
	switch pgErr.Code {
	case "P0001": // raise_exception
		return database.ErrorBusiness, pgErr.Message
	case "23505": // unique_violation
		return database.ErrorUniqueViolation, fmt.Sprintf("duplicate value violates unique constraint (%s)", pgErr.ConstraintName)
	case "23503": // foreign_key_violation
		return database.ErrorForeignKey, fmt.Sprintf("foreign key constraint violation (%s)", pgErr.ConstraintName)
	case "23514": // check_violation
		return database.ErrorCheckViolation, fmt.Sprintf("check constraint violation (%s)", pgErr.ConstraintName)
	case "28P01", "42501": // invalid_password, insufficient_privilege
		return database.ErrorPermissionDenied, "permission denied"
	case "08000", "08001", "08003", "08004", "08006", "08P01":
		return database.ErrorConnection, "database connection error"
	case "22P02", "22007", "22008": // invalid_text_representation, invalid_datetime
		return database.ErrorInvalidInput, "invalid input format"
	default:
		return database.ErrorUnknown, fmt.Sprintf("database error (%s): %s", pgErr.Code, pgErr.Message)
	}
}
