package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"notificator/internal/domain"
)

type JSONSnapshotStore struct {
	Path string
}

func (s *JSONSnapshotStore) Load(ctx context.Context) (map[string]map[domain.Asset]float64, error) {
	_ = ctx

	data, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]map[domain.Asset]float64{}, nil
		}
		return nil, err
	}

	var balances map[string]map[domain.Asset]float64
	if err := json.Unmarshal(data, &balances); err != nil {
		// Backward compatibility with old format: { "address": 123.45 }
		var legacy map[string]float64
		if legacyErr := json.Unmarshal(data, &legacy); legacyErr != nil {
			return nil, err
		}
		converted := make(map[string]map[domain.Asset]float64, len(legacy))
		for address, value := range legacy {
			converted[address] = map[domain.Asset]float64{
				domain.AssetTRX: value,
			}
		}
		return converted, nil
	}

	if balances == nil {
		balances = map[string]map[domain.Asset]float64{}
	}

	return balances, nil
}

func (s *JSONSnapshotStore) Save(ctx context.Context, balances map[string]map[domain.Asset]float64) error {
	_ = ctx

	data, err := json.MarshalIndent(balances, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.Path, data, 0o644)
}
