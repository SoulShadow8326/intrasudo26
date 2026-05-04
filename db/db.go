package db

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

type Store struct {
	conn *sql.DB
}

func New(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		log.Printf("PRAGMA journal_mode=WAL failed: %v", err)
	}
	if _, err := conn.Exec("PRAGMA busy_timeout=5000;"); err != nil {
		log.Printf("PRAGMA busy_timeout=5000 failed: %v", err)
	}
	if _, err := conn.Exec("PRAGMA synchronous=NORMAL;"); err != nil {
		log.Printf("PRAGMA synchronous=NORMAL failed: %v", err)
	}

	schema := []string{
		`CREATE TABLE IF NOT EXISTS levels (
            id TEXT PRIMARY KEY,
            markup TEXT NOT NULL,
            answer TEXT NOT NULL,
            answer_hash TEXT,
            source_hint TEXT,
            updated_at INTEGER
        );`,
		`CREATE TABLE IF NOT EXISTS accounts (
            email TEXT PRIMARY KEY,
            name TEXT,
            level TEXT,
            levels_json TEXT,
            created_at INTEGER
        );`,
		`CREATE TABLE IF NOT EXISTS leaderboard (
            email TEXT PRIMARY KEY,
            name TEXT,
            level INTEGER,
            time INTEGER
        );`,
		`CREATE TABLE IF NOT EXISTS otp (
            email TEXT PRIMARY KEY,
            code TEXT,
            expires_at INTEGER
        );`,
		`CREATE TABLE IF NOT EXISTS otp_rate (
            email TEXT PRIMARY KEY,
            sends_json TEXT
        );`,
		`CREATE TABLE IF NOT EXISTS sessions (
            sid TEXT PRIMARY KEY,
            email TEXT,
            expires_at INTEGER
        );`,
		`CREATE TABLE IF NOT EXISTS logs (
            email TEXT PRIMARY KEY,
            content TEXT
        );`,
		`CREATE TABLE IF NOT EXISTS messages (
            id TEXT PRIMARY KEY,
            owner TEXT,
            payload_json TEXT,
            created_at INTEGER
        );`,
		`CREATE TABLE IF NOT EXISTS hints (
            id TEXT PRIMARY KEY,
            level_id TEXT,
            payload_json TEXT,
            created_at INTEGER
        );`,
		`CREATE TABLE IF NOT EXISTS announcements (
            id TEXT PRIMARY KEY,
            content TEXT,
            time INTEGER
        );`,
		`CREATE TABLE IF NOT EXISTS meta (
		    key TEXT PRIMARY KEY,
		    value TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS level_channels (
		    level TEXT PRIMARY KEY,
		    level_channel INTEGER,
		    hint_channel INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS backlinks (
		    backlink TEXT PRIMARY KEY,
		    url TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS discord_messages (
		    id TEXT PRIMARY KEY,
		    email TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS disqualified (
		    email TEXT PRIMARY KEY,
		    disqualified INTEGER
		);`,
		`CREATE TABLE IF NOT EXISTS status (
		    level TEXT PRIMARY KEY,
		    leads INTEGER
		);`,
	}

	for _, s := range schema {
		if _, err := conn.Exec(s); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}

	return &Store{conn: conn}, nil
}

func (s *Store) Close() error {
	return s.conn.Close()
}

func (s *Store) WithTx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := s.conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}
