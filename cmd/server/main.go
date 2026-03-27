package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"verificador-citas-eros/internal/calendly"
	"verificador-citas-eros/internal/envx"
	"verificador-citas-eros/internal/scheduler"
	"verificador-citas-eros/internal/server"
	"verificador-citas-eros/internal/service"
	"verificador-citas-eros/internal/store"
	"verificador-citas-eros/internal/termlog"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	logger := termlog.New(os.Stdout)

	if err := envx.LoadDotEnv(".env"); err != nil {
		logger.Warn("no se pudo cargar .env", "error", err)
	}

	settings, err := envx.LoadSettings()
	if err != nil {
		logger.Error("configuracion invalida", "error", err)
		os.Exit(1)
	}

	fileStore, err := store.NewFileStore(settings.DataDir)
	if err != nil {
		logger.Error("no se pudo inicializar el almacenamiento", "error", err)
		os.Exit(1)
	}

	calClient := calendly.NewClient(settings.Calendly)
	appService, err := service.New(fileStore, calClient, logger)
	if err != nil {
		logger.Error("no se pudo inicializar el servicio", "error", err)
		os.Exit(1)
	}

	jobScheduler := scheduler.New(appService, logger)
	go jobScheduler.Start(ctx)

	httpServer := &http.Server{
		Addr:    settings.ServerAddr,
		Handler: server.New(appService),
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), server.ShutdownTimeout)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	logger.Section("Servidor listo", "addr", "http://localhost"+settings.ServerAddr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("servidor detenido por error", "error", err)
		os.Exit(1)
	}
	logger.Success("servidor detenido")
}
