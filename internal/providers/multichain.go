package providers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/fbsobreira/gotron-sdk/pkg/client"

	"notificator/internal/domain"
)

const (
	satoshiPerBTC = 100_000_000
	sunPerTRX     = 1_000_000
)

type MultiChainBalanceProvider struct {
	Client      *http.Client
	BTCAPIHost  string
	TRXGRPCHost string

	mu        sync.Mutex
	trxClient *client.GrpcClient
	trxHost   string
}

func (p *MultiChainBalanceProvider) GetBalance(ctx context.Context, address string, currency domain.Currency) (float64, error) {
	switch currency {
	case domain.BTC:
		return p.getBTCBalance(ctx, address)
	case domain.TRX:
		return p.getTRXBalance(ctx, address)
	default:
		return 0, fmt.Errorf("unsupported currency: %s", currency)
	}
}

func (p *MultiChainBalanceProvider) getBTCBalance(ctx context.Context, address string) (float64, error) {
	client := p.buildInsecureBTCClient()

	host := strings.TrimSpace(p.BTCAPIHost)
	if host == "" {
		return 0, fmt.Errorf("BTC API host is empty")
	}

	return p.fetchBTCBalanceFromAPI(ctx, client, host, address)
}

func (p *MultiChainBalanceProvider) fetchBTCBalanceFromAPI(ctx context.Context, client *http.Client, host, address string) (float64, error) {
	endpoint := fmt.Sprintf("%s/api/address/%s", strings.TrimRight(host, "/"), url.PathEscape(address))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, fmt.Errorf("btc api request failed: %s: %s", resp.Status, string(body))
	}

	var payload struct {
		Balance    string `json:"balance"`
		ChainStats struct {
			FundedTXOSum int64 `json:"funded_txo_sum"`
			SpentTXOSum  int64 `json:"spent_txo_sum"`
		} `json:"chain_stats"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, fmt.Errorf("decode response: %w", err)
	}

	if payload.Balance != "" {
		balanceBTC, err := strconv.ParseFloat(payload.Balance, 64)
		if err != nil {
			return 0, fmt.Errorf("parse balance from response: %w", err)
		}
		return balanceBTC, nil
	}

	confirmedBalanceSatoshi := payload.ChainStats.FundedTXOSum - payload.ChainStats.SpentTXOSum
	return float64(confirmedBalanceSatoshi) / satoshiPerBTC, nil
}

func (p *MultiChainBalanceProvider) getTRXBalance(_ context.Context, address string) (float64, error) {
	host := strings.TrimSpace(p.TRXGRPCHost)
	if host == "" {
		return 0, fmt.Errorf("TRX gRPC host is empty")
	}

	grpcClient, err := p.getTRXClient(host)
	if err != nil {
		return 0, err
	}

	account, err := grpcClient.GetAccount(address)
	if err != nil {
		// Force client reconnect on the next request.
		p.resetTRXClient()
		return 0, fmt.Errorf("get trx account: %w", err)
	}

	return float64(account.Balance) / sunPerTRX, nil
}

func (p *MultiChainBalanceProvider) buildInsecureBTCClient() *http.Client {
	base := p.Client
	if base == nil {
		return &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		}
	}

	clone := *base
	transport, ok := base.Transport.(*http.Transport)
	if ok && transport != nil {
		t := transport.Clone()
		if t.TLSClientConfig != nil {
			cfg := t.TLSClientConfig.Clone()
			cfg.InsecureSkipVerify = true
			t.TLSClientConfig = cfg
		} else {
			t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		}
		clone.Transport = t
		return &clone
	}

	clone.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	return &clone
}

func (p *MultiChainBalanceProvider) getTRXClient(host string) (*client.GrpcClient, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.trxClient != nil && p.trxHost == host {
		return p.trxClient, nil
	}

	if p.trxClient != nil {
		p.trxClient.Stop()
		p.trxClient = nil
	}

	grpcClient := client.NewGrpcClient(host)
	if err := grpcClient.Start(client.GRPCInsecure()); err != nil {
		return nil, fmt.Errorf("connect trx grpc node: %w", err)
	}

	p.trxClient = grpcClient
	p.trxHost = host
	return p.trxClient, nil
}

func (p *MultiChainBalanceProvider) resetTRXClient() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.trxClient != nil {
		p.trxClient.Stop()
		p.trxClient = nil
		p.trxHost = ""
	}
}
