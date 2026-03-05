package services

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"notificator/internal/domain"
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
	currency domain.Currency
	balances map[domain.Asset]float64
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

	current := make(map[string]map[domain.Asset]float64, len(m.Addresses))
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

	balances, err := m.Provider.GetBalances(ctx, address, currency)
	if err != nil {
		return checkResult{
			address:  address,
			currency: currency,
			err:      fmt.Errorf("get balance failed: %w", err),
		}
	}

	return checkResult{
		address:  address,
		currency: currency,
		balances: balances,
	}
}

func (m *Monitor) processCheckResult(
	ctx context.Context,
	result checkResult,
	prev map[string]map[domain.Asset]float64,
	current map[string]map[domain.Asset]float64,
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

	current[result.address] = result.balances

	if firstRun {
		m.Logger.Info(fmt.Sprintf("init snapshot %s (%s): %s", result.address, result.currency, formatBalances(result.balances)))
		return
	}

	oldBalances, exists := prev[result.address]
	if !exists {
		m.Logger.Info(fmt.Sprintf("skip notifications for new address %s (%s): %s", result.address, result.currency, formatBalances(result.balances)))
		return
	}

	changes := collectBalanceChanges(oldBalances, result.balances)
	if len(changes) > 0 {
		m.notifyBalanceChanged(ctx, result, changes)
	}
}

func (m *Monitor) notifyBalanceChanged(ctx context.Context, result checkResult, changes []balanceChange) {
	if m.Notifier == nil {
		return
	}

	lines := make([]string, 0, len(changes)+3)
	lines = append(lines,
		"Balance changed",
		fmt.Sprintf("Currency: %s", result.currency),
		fmt.Sprintf("Address: %s", result.address),
	)
	for _, change := range changes {
		lines = append(lines, fmt.Sprintf("%s: %.8f -> %.8f", change.asset, change.oldValue, change.newValue))
	}

	msg := joinLines(lines)
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

type balanceChange struct {
	asset    domain.Asset
	oldValue float64
	newValue float64
}

func collectBalanceChanges(oldBalances, newBalances map[domain.Asset]float64) []balanceChange {
	assets := sortedAssets(newBalances)
	changes := make([]balanceChange, 0, len(assets))
	for _, asset := range assets {
		newValue := newBalances[asset]
		oldValue, exists := oldBalances[asset]
		if !exists {
			// New tracked asset for this address: skip notification this cycle.
			continue
		}
		if oldValue != newValue {
			changes = append(changes, balanceChange{
				asset:    asset,
				oldValue: oldValue,
				newValue: newValue,
			})
		}
	}
	return changes
}

func formatBalances(balances map[domain.Asset]float64) string {
	if len(balances) == 0 {
		return "{}"
	}

	assets := sortedAssets(balances)
	parts := make([]string, 0, len(assets))
	for _, asset := range assets {
		value := balances[asset]
		parts = append(parts, fmt.Sprintf("%s=%.8f", asset, value))
	}
	return joinWithComma(parts)
}

func sortedAssets(balances map[domain.Asset]float64) []domain.Asset {
	assets := make([]domain.Asset, 0, len(balances))
	for asset := range balances {
		assets = append(assets, asset)
	}
	sort.Slice(assets, func(i, j int) bool {
		return assets[i] < assets[j]
	})
	return assets
}

func joinWithComma(parts []string) string {
	if len(parts) == 0 {
		return ""
	}
	out := parts[0]
	for i := 1; i < len(parts); i++ {
		out += ", " + parts[i]
	}
	return out
}

func joinLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	out := lines[0]
	for i := 1; i < len(lines); i++ {
		out += "\n" + lines[i]
	}
	return out
}
