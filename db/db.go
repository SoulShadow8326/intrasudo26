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
	conn           *sql.DB
	prepAppendLog  *sql.Stmt
	prepSetMessage *sql.Stmt
	prepCreateSess *sql.Stmt
	prepGetSess    *sql.Stmt
	prepDeleteSess *sql.Stmt
}

func New(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}

	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	if _, err := conn.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		log.Printf("PRAGMA journal_mode=WAL failed: %v", err)
	}
	if _, err := conn.Exec("PRAGMA busy_timeout=5000;"); err != nil {
		log.Printf("PRAGMA busy_timeout=5000 failed: %v", err)
	}
	if _, err := conn.Exec("PRAGMA synchronous=NORMAL;"); err != nil {
		log.Printf("PRAGMA synchronous=NORMAL failed: %v", err)
	}
	if _, err := conn.Exec("PRAGMA foreign_keys=ON;"); err != nil {
		log.Printf("PRAGMA foreign_keys=ON failed: %v", err)
	}

	if _, err := conn.Exec(`DROP TABLE IF EXISTS otp`); err != nil {
		log.Printf("failed to drop otp table: %v", err)
	}
	if _, err := conn.Exec(`DROP TABLE IF EXISTS otp_rate`); err != nil {
		log.Printf("failed to drop otp_rate table: %v", err)
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
            email TEXT PRIMARY KEY REFERENCES accounts(email) ON DELETE CASCADE,
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
            email TEXT REFERENCES accounts(email) ON DELETE CASCADE,
            expires_at INTEGER
        );`,
		`CREATE TABLE IF NOT EXISTS logs (
            email TEXT PRIMARY KEY REFERENCES accounts(email) ON DELETE CASCADE,
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
            level_id TEXT REFERENCES levels(id) ON DELETE CASCADE,
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
            level TEXT PRIMARY KEY REFERENCES levels(id) ON DELETE CASCADE,
            level_channel INTEGER,
            hint_channel INTEGER
        );`,
		`CREATE TABLE IF NOT EXISTS backlinks (
		    backlink TEXT PRIMARY KEY,
		    url TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS discord_messages (
            id TEXT PRIMARY KEY,
            email TEXT REFERENCES accounts(email) ON DELETE CASCADE
        );`,
		`CREATE TABLE IF NOT EXISTS disqualified (
            email TEXT PRIMARY KEY REFERENCES accounts(email) ON DELETE CASCADE,
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

	idx := []string{
		`CREATE INDEX IF NOT EXISTS idx_messages_owner ON messages(owner);`,
		`CREATE INDEX IF NOT EXISTS idx_hints_level_id ON hints(level_id);`,
		`CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at);`,
		`CREATE INDEX IF NOT EXISTS idx_hints_created_at ON hints(created_at);`,
	}
	for _, q := range idx {
		if _, err := conn.Exec(q); err != nil {
			log.Printf("failed to create index: %v", err)
		}
	}

	prepAppendLog, err := conn.Prepare(`INSERT INTO logs(email, content) VALUES(?, ?) ON CONFLICT(email) DO UPDATE SET content = substr(coalesce(content, '') || excluded.content, -10240)`)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	prepSetMessage, err := conn.Prepare(`INSERT INTO messages(id, owner, payload_json, created_at) VALUES(?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET owner=excluded.owner, payload_json=excluded.payload_json, created_at=excluded.created_at`)
	if err != nil {
		_ = prepAppendLog.Close()
		_ = conn.Close()
		return nil, err
	}
	prepCreateSess, err := conn.Prepare(`INSERT INTO sessions(sid, email, expires_at) VALUES(?, ?, ?) ON CONFLICT(sid) DO UPDATE SET email=excluded.email, expires_at=excluded.expires_at`)
	if err != nil {
		_ = prepSetMessage.Close()
		_ = prepAppendLog.Close()
		_ = conn.Close()
		return nil, err
	}
	prepGetSess, err := conn.Prepare(`SELECT email, expires_at FROM sessions WHERE sid = ?`)
	if err != nil {
		_ = prepCreateSess.Close()
		_ = prepSetMessage.Close()
		_ = prepAppendLog.Close()
		_ = conn.Close()
		return nil, err
	}
	prepDeleteSess, err := conn.Prepare(`DELETE FROM sessions WHERE sid = ?`)
	if err != nil {
		_ = prepGetSess.Close()
		_ = prepCreateSess.Close()
		_ = prepSetMessage.Close()
		_ = prepAppendLog.Close()
		_ = conn.Close()
		return nil, err
	}

	return &Store{conn: conn, prepAppendLog: prepAppendLog, prepSetMessage: prepSetMessage, prepCreateSess: prepCreateSess, prepGetSess: prepGetSess, prepDeleteSess: prepDeleteSess}, nil
}

func (s *Store) Close() error {
	if s.prepDeleteSess != nil {
		_ = s.prepDeleteSess.Close()
	}
	if s.prepGetSess != nil {
		_ = s.prepGetSess.Close()
	}
	if s.prepCreateSess != nil {
		_ = s.prepCreateSess.Close()
	}
	if s.prepSetMessage != nil {
		_ = s.prepSetMessage.Close()
	}
	if s.prepAppendLog != nil {
		_ = s.prepAppendLog.Close()
	}
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
