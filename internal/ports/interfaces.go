package ports

import (
	"context"

	"notificator/internal/domain"
)

type BalanceProvider interface {
	GetBalance(ctx context.Context, address string, currency domain.Currency) (float64, error)
}

type Notifier interface {
	Send(ctx context.Context, message string) error
}

type AddressDetector interface {
	Detect(address string) (domain.Currency, error)
}

type SnapshotStore interface {
	Load(ctx context.Context) (map[string]float64, error)
	Save(ctx context.Context, balances map[string]float64) error
}
