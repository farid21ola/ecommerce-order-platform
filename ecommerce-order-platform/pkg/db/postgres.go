package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresConfig struct {
	URL             string
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	SSLMode         string
	MaxConns        int32
	MinConns        int32
	ConnectTimeout  time.Duration
	HealthcheckTime time.Duration
}

func (c PostgresConfig) DSN() string {
	if c.URL != "" {
		return c.URL
	}

	sslMode := c.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}

	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=%s",
		c.User,
		c.Password,
		c.Host,
		c.Port,
		c.Database,
		sslMode,
	)
}

func NewPostgresPool(ctx context.Context, config PostgresConfig) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(config.DSN())
	if err != nil {
		return nil, err
	}

	if config.MaxConns > 0 {
		poolConfig.MaxConns = config.MaxConns
	}
	if config.MinConns > 0 {
		poolConfig.MinConns = config.MinConns
	}
	if config.ConnectTimeout > 0 {
		poolConfig.ConnConfig.ConnectTimeout = config.ConnectTimeout
	}
	if config.HealthcheckTime > 0 {
		poolConfig.HealthCheckPeriod = config.HealthcheckTime
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
