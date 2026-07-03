package database

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Connect opens a *gorm.DB for the given driver ("sqlite" or "postgres").
// path is the SQLite file path (used only when driver is "sqlite"); dsn is
// the Postgres connection string (used only when driver is "postgres").
func Connect(driver, path, dsn string) (*gorm.DB, error) {
	var dialector gorm.Dialector

	switch driver {
	case "", "sqlite":
		if dir := filepath.Dir(path); dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return nil, err
			}
		}
		dialector = sqlite.Open(path)
	case "postgres":
		dialector = postgres.Open(dsn)
	default:
		return nil, fmt.Errorf("unsupported DB_DRIVER %q", driver)
	}

	return gorm.Open(dialector, &gorm.Config{TranslateError: true})
}
