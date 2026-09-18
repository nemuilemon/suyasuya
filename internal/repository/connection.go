package repository

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"time"

	_ "github.com/lib/pq"

	"suyasuya/internal/platform"
)

func Open(ctx context.Context, role string) (*sql.DB, error) {
	u := url.URL{Scheme: "postgres", User: url.UserPassword(os.Getenv("DB_"+role+"_USER"), os.Getenv("DB_"+role+"_PASSWORD")), Host: platform.Env("DB_HOST", "postgres") + ":" + platform.Env("DB_PORT", "5432"), Path: "/" + platform.Env("DB_NAME", "suyasuya")}
	q := u.Query()
	q.Set("sslmode", platform.Env("DB_SSLMODE", "disable"))
	u.RawQuery = q.Encode()
	db, e := sql.Open("postgres", u.String())
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	ping, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if e = db.PingContext(ping); e != nil {
		db.Close()
		return nil, fmt.Errorf("database not ready: %w", e)
	}
	return db, nil
}
