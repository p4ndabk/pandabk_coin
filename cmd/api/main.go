package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	_ "zhu/docs"
	"zhu/internal/config"
	"zhu/internal/database"
	"zhu/internal/docs"
	"zhu/internal/health"
	"zhu/internal/user"
)

// @title           base-project-go API
// @version         1.0
// @description     Base Gin + GORM API.
// @BasePath        /api
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	cfg := config.Load()

	db, err := database.Connect(cfg.DBDriver, cfg.DBPath, cfg.DBDSN)
	if err != nil {
		slog.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	router := gin.Default()
	router.Use(cors.New(corsConfig(cfg.CORSAllowedOrigins)))

	api := router.Group("/api")

	healthHandler := &health.Handler{DB: db}
	health.RegisterRoutes(api, healthHandler)

	docs.RegisterRoutes(api)

	userService := &user.Service{DB: db}
	userHandler := &user.Handler{Service: userService, JWTSecret: cfg.JWTSecret}
	user.RegisterRoutes(api, userHandler)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("failed to start server", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	stop()
	slog.Info("shutting down server...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("server exited")
}

// corsConfig builds the CORS config from CORS_ALLOWED_ORIGINS. "*" (the
// default) allows any origin; otherwise it's a comma-separated allowlist.
// Authorization is added explicitly since the API is JWT bearer-token
// based and browsers don't send custom headers cross-origin by default.
func corsConfig(allowedOrigins string) cors.Config {
	cfg := cors.Config{
		AllowMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Authorization"},
	}

	if allowedOrigins == "*" {
		cfg.AllowAllOrigins = true
	} else {
		cfg.AllowOrigins = strings.Split(allowedOrigins, ",")
	}

	return cfg
}
