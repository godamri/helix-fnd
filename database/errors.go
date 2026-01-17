package database

import (
	"errors"
	"fmt"

	"github.com/godamri/helix-fnd/http/response"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func MapError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%s: %w", response.ErrNotFound, err)
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505": // unique_violation
			return fmt.Errorf("%s: %s", response.ErrAlreadyExists, pgErr.Detail)
		case "23503": // foreign_key_violation
			return fmt.Errorf("%s: referenced record not found", response.ErrConflict)
		case "23514": // check_violation
			return fmt.Errorf("%s: %s", response.ErrValidation, pgErr.Message)
		case "40001": // serialization_failure
			return fmt.Errorf("%s: retry transaction", response.ErrVersionMismatch)
		case "57014": // query_canceled
			return fmt.Errorf("%s: query timeout", response.ErrGatewayTimeout)
		}
	}

	return fmt.Errorf("%s: %w", response.ErrSystem, err)
}

func IsNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}
