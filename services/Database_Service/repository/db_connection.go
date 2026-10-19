package repository

import (
	"context"
	"net"
	"net/url"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

type Config struct{ Host, Port, Username, Password, DBName, SSLMode string }

func NewPostgresDB(cfg Config) (*sqlx.DB, error) {
	dsn := url.URL{Scheme: "postgres", User: url.UserPassword(cfg.Username, cfg.Password),
		Host: net.JoinHostPort(cfg.Host, cfg.Port), Path: "/" + cfg.DBName}
	query := url.Values{"sslmode": {cfg.SSLMode}, "connect_timeout": {"5"}}
	dsn.RawQuery = query.Encode()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db, err := sqlx.ConnectContext(ctx, "postgres", dsn.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)
	return db, nil
}
