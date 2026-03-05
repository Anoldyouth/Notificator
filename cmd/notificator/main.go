package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"

	"notificator/internal/notifiers"
	"notificator/internal/ports"
	"notificator/internal/providers"
	"notificator/internal/services"
	"notificator/internal/store"
)

func main() {
	_ = godotenv.Load()

	addressesFile := getEnv("ADDRESSES_FILE", "addresses.txt")
	snapshotFile := getEnv("SNAPSHOT_FILE", "balances_snapshot.json")
	btcAPIHost := mustGetEnv("BTC_API_HOST")
	trxGRPCHost := mustGetEnv("TRX_GRPC_HOST")
	notifierType := getEnv("NOTIFIER_TYPE", "telegram")
	pollInterval := getDurationEnv("POLL_INTERVAL", time.Hour)
	maxConcurrentChecks := getPositiveIntEnv("MAX_CONCURRENT_CHECKS", 20)
	notifier := buildNotifier(notifierType)

	addresses, err := services.LoadAddressesFromFile(addressesFile)
	if err != nil {
		panic(fmt.Errorf("load addresses: %w", err))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	monitor := &services.Monitor{
		Addresses: addresses,
		Detector:  &services.PrefixAddressDetector{},
		Provider: &providers.MultiChainBalanceProvider{
			BTCAPIHost:  btcAPIHost,
			TRXGRPCHost: trxGRPCHost,
		},
		Notifier: notifier,
		Store: &store.JSONSnapshotStore{
			Path: snapshotFile,
		},
		MaxConcurrentChecks: maxConcurrentChecks,
	}

	if err := monitor.Start(ctx, pollInterval); err != nil && !errors.Is(err, context.Canceled) {
		panic(fmt.Errorf("monitor stopped with error: %w", err))
	}
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func mustGetEnv(key string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		panic(fmt.Errorf("%s is required", key))
	}
	return value
}

func getDurationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	seconds, err := strconv.Atoi(value)
	if err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}

	parsed, err := time.ParseDuration(value)
	if err == nil && parsed > 0 {
		return parsed
	}

	panic(fmt.Errorf("invalid %s value: %q", key, value))
}

func getPositiveIntEnv(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		panic(fmt.Errorf("invalid %s value: %q", key, value))
	}

	return parsed
}

func buildNotifier(notifierType string) ports.Notifier {
	switch strings.ToLower(strings.TrimSpace(notifierType)) {
	case "telegram":
		return &notifiers.TelegramNotifier{
			Token:  mustGetEnv("TG_BOT_TOKEN"),
			ChatID: mustGetEnv("TG_CHAT_ID"),
		}
	case "file":
		return &notifiers.FileNotifier{
			Path: time.Now().Format("2006-01-02") + ".log",
		}
	default:
		panic(fmt.Errorf("unsupported NOTIFIER_TYPE: %s", notifierType))
	}
}
