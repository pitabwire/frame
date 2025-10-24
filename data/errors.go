package data

import (
	"database/sql"
	"errors"

	"gorm.io/gorm"
)

// ErrorIsNoRows validate if supplied error is because of record missing in DB.
func ErrorIsNoRows(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, sql.ErrNoRows)
}
