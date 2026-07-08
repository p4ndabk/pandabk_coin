package main

import (
	"errors"
	"log/slog"
	"os"

	"gorm.io/gorm"
	"zhu/internal/config"
	"zhu/internal/database"
	"zhu/internal/user"
)

func main() {
	cfg := config.Load()

	db, err := database.Connect(cfg.DBDriver, cfg.DBPath, cfg.DBDSN)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	userService := &user.Service{DB: db}

	seedUsers := []user.CreateInput{
		{Name: "Admin", Email: "admin@example.com", Password: "password123", Active: true},
	}

	for _, input := range seedUsers {
		var existing user.User
		err := db.Where("email = ?", input.Email).First(&existing).Error
		if err == nil {
			slog.Info("user already exists, skipping", "email", input.Email)
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Error("failed to check existing user", "error", err)
			os.Exit(1)
		}

		if _, err := userService.Create(input); err != nil {
			slog.Error("failed to seed user", "email", input.Email, "error", err)
			os.Exit(1)
		}
		slog.Info("seeded user", "email", input.Email)
	}

	slog.Info("seed completed successfully")
}
