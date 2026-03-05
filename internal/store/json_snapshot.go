package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
)

type JSONSnapshotStore struct {
	Path string
}

func (s *JSONSnapshotStore) Load(ctx context.Context) (map[string]float64, error) {
	_ = ctx

	data, err := os.ReadFile(s.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]float64{}, nil
		}
		return nil, err
	}

	var balances map[string]float64
	if err := json.Unmarshal(data, &balances); err != nil {
		return nil, err
	}

	if balances == nil {
		balances = map[string]float64{}
	}

	return balances, nil
}

func (s *JSONSnapshotStore) Save(ctx context.Context, balances map[string]float64) error {
	_ = ctx

	data, err := json.MarshalIndent(balances, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.Path, data, 0o644)
}
