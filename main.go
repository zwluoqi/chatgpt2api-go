package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"chatgpt2api/config"
	"chatgpt2api/services"
)

func main() {
	baseDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	if err := config.Init(baseDir); err != nil {
		log.Fatal(err)
	}

	cfg := config.Config
	appVersion := services.GetAppVersion(baseDir)

	accountService := services.NewAccountService(cfg.AccountsFile)
	cpaConfigFile := filepath.Join(cfg.DataDir, "cpa_config.json")
	cpaConfig := services.NewCPAConfig(cpaConfigFile)
	cpaImportService := services.NewCPAImportService(cpaConfig, accountService)
	chatGPTService := services.NewChatGPTService(accountService)

	webDistDir := filepath.Join(baseDir, "web_dist")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	services.StartLimitedAccountWatcher(ctx, accountService, cfg.RefreshAccountIntervalMinute)

	router := services.CreateApp(
		cfg.AuthKey,
		appVersion,
		webDistDir,
		accountService,
		cpaConfig,
		cpaImportService,
		chatGPTService,
	)

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	fmt.Printf("chatgpt2api v%s starting on %s\n", appVersion, addr)

	srv := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	fmt.Println("shutting down...")

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatal("server forced shutdown:", err)
	}
	fmt.Println("server stopped")
}
