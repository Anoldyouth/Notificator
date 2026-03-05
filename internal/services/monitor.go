package services

import (
	"context"
	"fmt"
	"sync"
	"time"

	"notificator/internal/ports"
)

type Monitor struct {
	Addresses           []string
	Detector            ports.AddressDetector
	Provider            ports.BalanceProvider
	Notifier            ports.Notifier
	Logger              ports.Logger
	Store               ports.SnapshotStore
	MaxConcurrentChecks int
}

type checkResult struct {
	address  string
	currency string
	balance  float64
	err      error
}

func (m *Monitor) RunOnce(ctx context.Context) error {
	if m.Logger == nil {
		return fmt.Errorf("logger is required")
	}

	prev, err := m.Store.Load(ctx)
	if err != nil {
		return fmt.Errorf("load snapshot: %w", err)
	}

	current := make(map[string]float64, len(m.Addresses))
	firstRun := len(prev) == 0
	results := m.collectAddressChecks(ctx)

	for _, result := range results {
		m.processCheckResult(ctx, result, prev, current, firstRun)
	}

	if err := m.Store.Save(ctx, current); err != nil {
		return fmt.Errorf("save snapshot: %w", err)
	}

	return nil
}

func (m *Monitor) collectAddressChecks(ctx context.Context) []checkResult {
	resultsCh := make(chan checkResult, len(m.Addresses))
	var wg sync.WaitGroup
	sem := make(chan struct{}, m.maxConcurrentChecks())

	for _, address := range m.Addresses {
		wg.Add(1)
		go func(address string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			resultsCh <- m.checkAddress(ctx, address)
		}(address)
	}

	wg.Wait()
	close(resultsCh)

	results := make([]checkResult, 0, len(m.Addresses))
	for result := range resultsCh {
		results = append(results, result)
	}

	return results
}

func (m *Monitor) checkAddress(ctx context.Context, address string) checkResult {
	currency, err := m.Detector.Detect(address)
	if err != nil {
		return checkResult{
			address: address,
			err:     fmt.Errorf("detect currency failed: %w", err),
		}
	}

	balance, err := m.Provider.GetBalance(ctx, address, currency)
	if err != nil {
		return checkResult{
			address:  address,
			currency: string(currency),
			err:      fmt.Errorf("get balance failed: %w", err),
		}
	}

	return checkResult{
		address:  address,
		currency: string(currency),
		balance:  balance,
	}
}

func (m *Monitor) processCheckResult(
	ctx context.Context,
	result checkResult,
	prev map[string]float64,
	current map[string]float64,
	firstRun bool,
) {
	if result.err != nil {
		if result.currency != "" {
			m.Logger.Warning(fmt.Sprintf("skip address %s (%s): %v", result.address, result.currency, result.err))
		} else {
			m.Logger.Warning(fmt.Sprintf("skip address %s: %v", result.address, result.err))
		}
		return
	}

	current[result.address] = result.balance

	if firstRun {
		m.Logger.Info(fmt.Sprintf("init snapshot %s (%s): %.8f", result.address, result.currency, result.balance))
		return
	}

	oldBalance, exists := prev[result.address]
	if !exists {
		m.Logger.Info(fmt.Sprintf("skip notifications for new address %s (%s): %.8f", result.address, result.currency, result.balance))
		return
	}

	if oldBalance != result.balance {
		m.notifyBalanceChanged(ctx, result, oldBalance)
	}
}

func (m *Monitor) notifyBalanceChanged(ctx context.Context, result checkResult, oldBalance float64) {
	if m.Notifier == nil {
		return
	}

	msg := fmt.Sprintf(
		"Balance changed\nCurrency: %s\nAddress: %s\nOld: %.8f\nNew: %.8f",
		result.currency,
		result.address,
		oldBalance,
		result.balance,
	)
	if err := m.Notifier.Send(ctx, msg); err != nil {
		m.Logger.Error(fmt.Sprintf("failed to send notification for %s (%s): %v", result.address, result.currency, err))
	}
}

func (m *Monitor) maxConcurrentChecks() int {
	if m.MaxConcurrentChecks <= 0 {
		return 20
	}
	return m.MaxConcurrentChecks
}

func (m *Monitor) Start(ctx context.Context, interval time.Duration) error {
	if err := m.RunOnce(ctx); err != nil {
		return err
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if err := m.RunOnce(ctx); err != nil {
				m.Logger.Error(fmt.Sprintf("monitor tick error: %v", err))
			}
		}
	}
}
