package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
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

	schema := `
	CREATE TABLE IF NOT EXISTS kv (
		namespace TEXT NOT NULL,
		key TEXT NOT NULL,
		value TEXT NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY(namespace, key)
	);
	`
	if _, err := conn.Exec(schema); err != nil {
		_ = conn.Close()
		return nil, err
	}


	typed := `
	CREATE TABLE IF NOT EXISTS levels (
		id TEXT PRIMARY KEY,
		markup TEXT NOT NULL,
		answer TEXT NOT NULL,
		source_hint TEXT,
		updated_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS accounts (
		email TEXT PRIMARY KEY,
		name TEXT,
		password_hash TEXT,
		level TEXT,
		created_at INTEGER
	);

	CREATE TABLE IF NOT EXISTS leaderboard (
		email TEXT PRIMARY KEY,
		level INTEGER,
		time INTEGER
	);

	CREATE INDEX IF NOT EXISTS idx_leaderboard_level_time ON leaderboard(level DESC, time ASC);
	`
	if _, err := conn.Exec(typed); err != nil {
		_ = conn.Close()
		return nil, err
	}

	return &Store{conn: conn}, nil
}

func (s *Store) Close() error {
	return s.conn.Close()
}

func (s *Store) Set(namespace, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}

	_, err = s.conn.Exec(`
		INSERT INTO kv(namespace, key, value, updated_at, created_at)
		VALUES(?, ?, ?, strftime('%s','now'), strftime('%s','now'))
		ON CONFLICT(namespace, key) DO UPDATE SET
			value = excluded.value,
			updated_at = strftime('%s','now')
	`, namespace, key, string(raw))
	return err
}

func (s *Store) SetEntry(namespace, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}

	_, err = s.conn.Exec(`
		INSERT INTO kv(namespace, key, value, updated_at, created_at)
		VALUES(?, ?, ?, strftime('%s','now'), strftime('%s','now'))
		ON CONFLICT(namespace, key) DO UPDATE SET
			value = excluded.value,
			updated_at = strftime('%s','now')
	`, namespace, key, string(raw))
	return err
}

func (s *Store) Update(namespace, key string, fn func(current json.RawMessage) (any, error)) error {
	return s.WithTx(context.Background(), func(tx *sql.Tx) error {
		var raw sql.NullString
		err := tx.QueryRow(`
			SELECT value
			FROM kv
			WHERE namespace = ? AND key = ?
		`, namespace, key).Scan(&raw)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}

		var current json.RawMessage
		if raw.Valid {
			current = json.RawMessage([]byte(raw.String))
		} else {
			current = nil
		}

		next, err := fn(current)
		if err != nil {
			return err
		}

		out, err := json.Marshal(next)
		if err != nil {
			return err
		}

		_, err = tx.Exec(`
			INSERT INTO kv(namespace, key, value, updated_at, created_at)
			VALUES(?, ?, ?, strftime('%s','now'), strftime('%s','now'))
			ON CONFLICT(namespace, key) DO UPDATE SET
				value = excluded.value,
				updated_at = strftime('%s','now')
		`, namespace, key, string(out))
		return err
	})
}

func (s *Store) Get(namespace, key string, dest any) (bool, error) {
	var raw string
	err := s.conn.QueryRow(`
		SELECT value
		FROM kv
		WHERE namespace = ? AND key = ?
	`, namespace, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(raw), dest)
}

func (s *Store) GetEntry(namespace, key string) (string, error) {
	var raw string
	err := s.conn.QueryRow(`
		SELECT value
		FROM kv
		WHERE namespace = ? AND key = ?
	`, namespace, key).Scan(&raw)
	if err != nil {
		return "", err
	}
	return raw, nil
}

func (s *Store) GetRaw(namespace, key string) (json.RawMessage, bool, error) {
	var raw string
	err := s.conn.QueryRow(`
		SELECT value
		FROM kv
		WHERE namespace = ? AND key = ?
	`, namespace, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return json.RawMessage([]byte(raw)), true, nil
}

func (s *Store) Delete(namespace, key string) error {
	_, err := s.conn.Exec(`DELETE FROM kv WHERE namespace = ? AND key = ?`, namespace, key)
	return err
}

func (s *Store) DeleteEntry(namespace, key string) error {
	_, err := s.conn.Exec(`DELETE FROM kv WHERE namespace = ? AND key = ?`, namespace, key)
	return err
}

func (s *Store) List(namespace string, dest any) error {
	rows, err := s.conn.Query(`
		SELECT value
		FROM kv
		WHERE namespace = ?
		ORDER BY key
	`, namespace)
	if err != nil {
		return err
	}
	defer rows.Close()

	var list []json.RawMessage
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		list = append(list, json.RawMessage([]byte(raw)))
	}
	if err := rows.Err(); err != nil {
		return err
	}

	payload, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, dest)
}

func (s *Store) GetAll(namespace string) (map[string]string, error) {
	rows, err := s.conn.Query(`
		SELECT key, value
		FROM kv
		WHERE namespace = ?
		ORDER BY key
	`, namespace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ListByPrefix(namespace, prefix string, dest any) error {
	rows, err := s.conn.Query(`
		SELECT value
		FROM kv
		WHERE namespace = ? AND key LIKE ?
		ORDER BY key
	`, namespace, fmt.Sprintf("%s%%", prefix))
	if err != nil {
		return err
	}
	defer rows.Close()

	var list []json.RawMessage
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return err
		}
		list = append(list, json.RawMessage([]byte(raw)))
	}
	if err := rows.Err(); err != nil {
		return err
	}

	payload, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, dest)
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
