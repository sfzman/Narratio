package main

import (
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/sfzman/Narratio/backend/internal/bootstrap"
)

func main() {
	runtime, err := bootstrap.LoadRuntime()
	if err != nil {
		slog.Error("server bootstrap failed", "error", err)
		os.Exit(1)
	}
	defer runtime.Close()

	slog.Info("narratio backend skeleton ready",
		"port", runtime.Config.Port,
		"database_driver", runtime.Config.DatabaseDriver,
	)

	server := &http.Server{
		Addr:    ":" + runtime.Config.Port,
		Handler: runtime.Router,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server failed", "error", err)
		}
	}()

	waitForSignal(server)
}

func waitForSignal(server *http.Server) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	_ = server.Close()
}
