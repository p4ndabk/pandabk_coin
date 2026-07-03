package config

import (
	"log/slog"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	Port               string
	DBDriver           string
	DBPath             string
	DBDSN              string
	JWTSecret          string
	CORSAllowedOrigins string
}

func Load() *Config {
	if err := godotenv.Load(); err != nil {
		slog.Info("no .env file found, using environment variables")
	}

	return &Config{
		Port:               getEnv("PORT", "8080"),
		DBDriver:           getEnv("DB_DRIVER", "sqlite"),
		DBPath:             getEnv("DB_PATH", "data/base_project.db"),
		DBDSN:              getEnv("DB_DSN", ""),
		JWTSecret:          getEnv("JWT_SECRET", "dev-secret-change-me"),
		CORSAllowedOrigins: getEnv("CORS_ALLOWED_ORIGINS", "*"),
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
