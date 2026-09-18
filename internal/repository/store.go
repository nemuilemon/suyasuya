package repository

import (
	"context"
	"database/sql"
	"encoding/json"
)

type executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}
type Queries struct{ db executor }
type Store struct {
	*Queries
	db *sql.DB
}
type Transaction struct {
	*Queries
	tx *sql.Tx
}

func New(db *sql.DB) *Store                     { return &Store{Queries: &Queries{db: db}, db: db} }
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }
func (s *Store) Begin(ctx context.Context) (*Transaction, error) {
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	return &Transaction{Queries: &Queries{db: tx}, tx: tx}, nil
}
func (t *Transaction) Commit() error   { return t.tx.Commit() }
func (t *Transaction) Rollback() error { return t.tx.Rollback() }
func jsonRows(ctx context.Context, db executor, query string, args ...any) ([]json.RawMessage, error) {
	rows, e := db.QueryContext(ctx, query, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []json.RawMessage{}
	for rows.Next() {
		var b []byte
		if e = rows.Scan(&b); e != nil {
			return nil, e
		}
		out = append(out, json.RawMessage(b))
	}
	return out, rows.Err()
}
