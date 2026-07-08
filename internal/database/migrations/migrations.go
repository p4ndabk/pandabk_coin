// Package migrations holds every schema change as a versioned, ordered
// gormigrate.Migration. gormigrate tracks applied IDs in a "migrations"
// table, so cmd/migrate only ever runs what's new and can roll back the
// last one — unlike a bare AutoMigrate call, which has no history and no
// rollback.
//
// Adding a new migration:
//  1. Append a new entry to All with a new, never-reused ID (convention:
//     YYYYMMDDHHMMSS_description, matching creation order).
//  2. Never edit or remove an existing entry once it has shipped — anyone
//     who already ran it has that ID recorded as applied.
package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
	"zhu/internal/user"
)

var All = []*gormigrate.Migration{
	{
		ID: "20260702000001_create_users_table",
		Migrate: func(tx *gorm.DB) error {
			return tx.AutoMigrate(&user.User{})
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Migrator().DropTable(&user.User{})
		},
	},
}
