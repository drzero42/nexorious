package main

import (
	"context"
	"fmt"
	"log"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/drzero42/nexorious-go/internal/api"
	"github.com/drzero42/nexorious-go/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	e := api.New(cfg)

	addr := fmt.Sprintf(":%d", cfg.Port)
	sc := echo.StartConfig{
		Address:         addr,
		GracefulTimeout: 10 * time.Second,
		HideBanner:      true,
		HidePort:        true,
	}

	// Wait for interrupt/term signal then shut down gracefully.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Printf("nexorious starting on %s", addr)
	if err := sc.Start(ctx, e); err != nil {
		log.Printf("server stopped: %v", err)
	}
	log.Println("shutdown complete")
}
