package state

import (
    "context"
    "fmt"

    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/riverqueue/river"
    riverpgxv5 "github.com/riverqueue/river/riverdriver/riverpgxv5"
)

// InitRiver initializes the River client if not already initialized.
func (s *State) InitRiver(ctx context.Context) error {
    if s.RiverClient != nil {
        return nil
    }
    if s.Config == nil || s.Config.DB == nil || s.Config.DB.URL == "" {
        return fmt.Errorf("missing database configuration for River")
    }

    // Initialize pgx pool and River driver using the billing DB URL
    pool, err := pgxpool.New(ctx, s.Config.DB.URL)
    if err != nil {
        return fmt.Errorf("failed creating pgx pool for River: %w", err)
    }

    drv := riverpgxv5.New(pool)

    client, err := river.NewClient[pgx.Tx](drv, &river.Config{})
    if err != nil {
        return fmt.Errorf("failed creating River client: %w", err)
    }

    s.RiverClient = client
    return nil
}
