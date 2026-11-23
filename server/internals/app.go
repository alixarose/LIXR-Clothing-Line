package internals

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"
)

type App struct {
	cfg Config
	db  *sql.DB
}

func Run() error {
	cfg := LoadConfig()
	db, err := openDatabase(cfg.DatabasePath)
	if err != nil {
		return fmt.Errorf("[LIXR] database: %w", err)
	}
	defer db.Close()

	app := &App{cfg: cfg, db: db}
	if err := app.bootstrap(); err != nil {
		return fmt.Errorf("[LIXR] bootstrap: %w", err)
	}

	mux := http.NewServeMux()
	app.routes(mux)

	addr := fmt.Sprintf("%s:%d", cfg.ServerHost, cfg.ServerPort)
	server := &http.Server{
		Addr:              addr,
		Handler:           securityHeaders(allowedHost(cfg.AllowedHosts, requestLogger(mux))),
		ReadHeaderTimeout: 8 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      35 * time.Second,
		IdleTimeout:       75 * time.Second,
	}

	log.Printf("[LIXR] %s running at http://%s", cfg.AppName, addr)
	log.Printf("[LIXR] SQLite: %s", cfg.DatabasePath)
	log.Printf("[LIXR] Allowed hosts: 127.0.0.1, localhost, lixr.jpano.dev")
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
