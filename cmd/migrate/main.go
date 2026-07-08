package main

import (
	"flag"
	"log/slog"
	"os"

	"github.com/go-gormigrate/gormigrate/v2"
	"zhu/internal/config"
	"zhu/internal/database"
	"zhu/internal/database/migrations"
)

func main() {
	rollback := flag.Bool("rollback", false, "roll back the last applied migration instead of migrating")
	flag.Parse()

	cfg := config.Load()

	db, err := database.Connect(cfg.DBDriver, cfg.DBPath, cfg.DBDSN)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	m := gormigrate.New(db, gormigrate.DefaultOptions, migrations.All)

	if *rollback {
		if err := m.RollbackLast(); err != nil {
			slog.Error("rollback failed", "error", err)
			os.Exit(1)
		}
		slog.Info("rollback completed successfully")
		return
	}

	if err := m.Migrate(); err != nil {
		slog.Error("migration failed", "error", err)
		os.Exit(1)
	}

	slog.Info("migration completed successfully")
}
