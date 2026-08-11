package repository

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"
	"github.com/scoutpulse/libs/platform/apperr"
)

// Postgres SQLSTATE codes worth distinguishing from a generic failure.
const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
	pgNotNullViolation    = "23502"
	pgCheckViolation      = "23514"
	// pgInvalidTextRepresentation is what Postgres returns for a malformed
	// UUID. Every id in this schema is a UUID, so without this case a caller
	// sending "/players/not-a-uuid" gets a 500 and an error in the log for
	// what is plainly a bad request.
	pgInvalidTextRepresentation = "22P02"
)

// translate converts a database error into the shared error taxonomy.
//
// Every repository routes its errors through here so that callers can
// distinguish "row missing" from "database unreachable" — and so that driver
// text, which can embed SQL and constraint names, is carried as an internal
// cause rather than a client-facing message.
func translate(entity string, err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, sql.ErrNoRows) {
		return apperr.NotFound(entity + " not found")
	}

	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch string(pqErr.Code) {
		case pgUniqueViolation:
			return apperr.Wrap(apperr.KindConflict, entity+" already exists", err)
		case pgForeignKeyViolation:
			return apperr.Wrap(apperr.KindInvalid, "referenced record does not exist", err)
		case pgNotNullViolation:
			return apperr.Wrap(apperr.KindInvalid, "a required field is missing", err)
		case pgCheckViolation:
			// The schema's CHECK constraints restate rules the service layer
			// also enforces. Reaching one means the two disagree, or a write
			// bypassed validation -- either way it is the caller's input that
			// is wrong, not the server.
			return apperr.Wrap(apperr.KindInvalid, "a field failed a validation rule", err)
		case pgInvalidTextRepresentation:
			return apperr.Wrap(apperr.KindInvalid, "malformed identifier", err)
		}
	}

	return apperr.Internal(fmt.Errorf("%s: %w", entity, err))
}

// affected reports NotFound when a statement matched no rows, so that an
// update or delete against a missing id does not silently succeed.
func affected(entity string, res sql.Result, err error) error {
	if err != nil {
		return translate(entity, err)
	}

	n, err := res.RowsAffected()
	if err != nil {
		return apperr.Internal(fmt.Errorf("%s: reading rows affected: %w", entity, err))
	}
	if n == 0 {
		return apperr.NotFound(entity + " not found")
	}
	return nil
}
