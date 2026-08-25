package store

import (
	"database/sql"
	"errors"
	"strings"

	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

var ErrNotFound = errors.New("not found")

func IsNotFound(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrNotFound) || errors.Is(err, sql.ErrNoRows)
}

func translateNotFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

var ErrConflict = errors.New("conflict")

func IsConflict(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrConflict) {
		return true
	}
	var sqliteErr *sqlite.Error
	if !errors.As(err, &sqliteErr) {
		return false
	}
	return sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE ||
		sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY
}

func IsAppendOnlyViolation(err error) bool {
	if err == nil {
		return false
	}
	var sqliteErr *sqlite.Error
	return errors.As(err, &sqliteErr) &&
		sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_TRIGGER &&
		strings.Contains(sqliteErr.Error(), "append-only")
}
