package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

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

func (s *Store) Set(kind, key string, value any) error {
	switch kind {
	case "levels":
		lv, ok := value.(map[string]any)
		if !ok {
			raw, err := json.Marshal(value)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &lv); err != nil {
				return err
			}
		}
		_, err := s.conn.Exec(`INSERT INTO levels(id, markup, answer, answer_hash, source_hint, updated_at)
            VALUES(?, ?, ?, ?, ?, ?)
            ON CONFLICT(id) DO UPDATE SET markup=excluded.markup, answer=excluded.answer, answer_hash=excluded.answer_hash, source_hint=excluded.source_hint, updated_at=excluded.updated_at`, key, lv["markup"], lv["answer"], lv["answer_hash"], lv["source_hint"], lv["updated_at"])
		return err
	case "accounts":
		u, ok := value.(map[string]any)
		if !ok {
			raw, err := json.Marshal(value)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &u); err != nil {
				return err
			}
		}
		levelsJSON := "null"
		if v, ok := u["levels"]; ok && v != nil {
			b, _ := json.Marshal(v)
			levelsJSON = string(b)
		}
		_, err := s.conn.Exec(`INSERT INTO accounts(email, name, level, levels_json, created_at)
            VALUES(?, ?, ?, ?, ?)
            ON CONFLICT(email) DO UPDATE SET name=excluded.name, level=excluded.level, levels_json=excluded.levels_json`, key, u["name"], u["level"], levelsJSON, u["created_at"])
		return err
	case "leaderboard":
		ent, ok := value.(map[string]any)
		if !ok {
			raw, err := json.Marshal(value)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &ent); err != nil {
				return err
			}
		}
		_, err := s.conn.Exec(`INSERT INTO leaderboard(email, name, level, time)
            VALUES(?, ?, ?, ?)
            ON CONFLICT(email) DO UPDATE SET name=excluded.name, level=excluded.level, time=excluded.time`, key, ent["name"], ent["level"], ent["time"])
		return err
	case "otp":
		rec, ok := value.(map[string]any)
		if !ok {
			raw, err := json.Marshal(value)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &rec); err != nil {
				return err
			}
		}
		_, err := s.conn.Exec(`INSERT INTO otp(email, code, expires_at) VALUES(?, ?, ?) ON CONFLICT(email) DO UPDATE SET code=excluded.code, expires_at=excluded.expires_at`, key, rec["code"], rec["expires_at"])
		return err
	case "otp_rate":
		var arr []int64
		switch v := value.(type) {
		case []int64:
			arr = v
		default:
			b, err := json.Marshal(value)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(b, &arr); err != nil {
				return err
			}
		}
		b, _ := json.Marshal(arr)
		_, err := s.conn.Exec(`INSERT INTO otp_rate(email, sends_json) VALUES(?, ?) ON CONFLICT(email) DO UPDATE SET sends_json=excluded.sends_json`, key, string(b))
		return err
	case "sessions":
		rec, ok := value.(map[string]any)
		if !ok {
			raw, err := json.Marshal(value)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &rec); err != nil {
				return err
			}
		}
		_, err := s.conn.Exec(`INSERT INTO sessions(sid, email, expires_at) VALUES(?, ?, ?) ON CONFLICT(sid) DO UPDATE SET email=excluded.email, expires_at=excluded.expires_at`, key, rec["email"], rec["expires_at"])
		return err
	case "logs":
		content, ok := value.(string)
		if !ok {
			raw, err := json.Marshal(value)
			if err != nil {
				return err
			}
			content = string(raw)
		}
		_, err := s.conn.Exec(`INSERT INTO logs(email, content) VALUES(?, ?) ON CONFLICT(email) DO UPDATE SET content=excluded.content`, key, content)
		return err
	case "messages":
		m, ok := value.(map[string]any)
		if !ok {
			raw, err := json.Marshal(value)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &m); err != nil {
				return err
			}
		}
		payload, _ := json.Marshal(m)
		created := time.Now().Unix()
		if v, ok := m["created_at"]; ok {
			if t, ok := v.(int64); ok && t != 0 {
				created = t
			}
		}
		owner := ""
		if v, ok := m["owner"]; ok {
			owner = fmt.Sprintf("%v", v)
		}
		_, err := s.conn.Exec(`INSERT INTO messages(id, owner, payload_json, created_at) VALUES(?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET owner=excluded.owner, payload_json=excluded.payload_json, created_at=excluded.created_at`, key, owner, string(payload), created)
		return err
	case "hints":
		h, ok := value.(map[string]any)
		if !ok {
			raw, err := json.Marshal(value)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &h); err != nil {
				return err
			}
		}
		payload, _ := json.Marshal(h)
		created := time.Now().Unix()
		if v, ok := h["created_at"]; ok {
			if t, ok := v.(int64); ok && t != 0 {
				created = t
			}
		}
		levelID := ""
		if v, ok := h["level_id"]; ok {
			levelID = fmt.Sprintf("%v", v)
		}
		_, err := s.conn.Exec(`INSERT INTO hints(id, level_id, payload_json, created_at) VALUES(?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET level_id=excluded.level_id, payload_json=excluded.payload_json, created_at=excluded.created_at`, key, levelID, string(payload), created)
		return err
	case "announcements":
		switch v := value.(type) {
		case string:
			timev := time.Now().Unix()
			_, err := s.conn.Exec(`INSERT INTO announcements(id, content, time) VALUES(?, ?, ?) ON CONFLICT(id) DO UPDATE SET content=excluded.content, time=excluded.time`, key, v, timev)
			return err
		case map[string]any:
			a := v
			timev := time.Now().Unix()
			if tv, ok := a["time"]; ok {
				if t, ok := tv.(int64); ok && t != 0 {
					timev = t
				}
			}
			_, err := s.conn.Exec(`INSERT INTO announcements(id, content, time) VALUES(?, ?, ?) ON CONFLICT(id) DO UPDATE SET content=excluded.content, time=excluded.time`, key, a["content"], timev)
			return err
		default:
			raw, err := json.Marshal(value)
			if err != nil {
				return err
			}
			var a map[string]any
			if err := json.Unmarshal(raw, &a); err != nil {
				_, err := s.conn.Exec(`INSERT INTO announcements(id, content, time) VALUES(?, ?, ?) ON CONFLICT(id) DO UPDATE SET content=excluded.content, time=excluded.time`, key, string(raw), time.Now().Unix())
				return err
			}
			timev := time.Now().Unix()
			if tv, ok := a["time"]; ok {
				if t, ok := tv.(int64); ok && t != 0 {
					timev = t
				}
			}
			_, err = s.conn.Exec(`INSERT INTO announcements(id, content, time) VALUES(?, ?, ?) ON CONFLICT(id) DO UPDATE SET content=excluded.content, time=excluded.time`, key, a["content"], timev)
			return err
		}
	case "meta":
		b, err := json.Marshal(value)
		if err != nil {
			return err
		}
		_, err = s.conn.Exec(`INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, string(b))
		return err
	case "status":
		switch v := value.(type) {
		case bool:
			val := 0
			if v {
				val = 1
			}
			_, err := s.conn.Exec(`INSERT INTO status(level, leads) VALUES(?, ?) ON CONFLICT(level) DO UPDATE SET leads=excluded.leads`, key, val)
			return err
		case string:
			b, err := json.Marshal(v)
			if err != nil {
				return err
			}
			var gs struct {
				Leads bool `json:"leads"`
			}
			if err := json.Unmarshal(b, &gs); err != nil {
				if v == "true" || v == "1" {
					_, err := s.conn.Exec(`INSERT INTO status(level, leads) VALUES(?, ?) ON CONFLICT(level) DO UPDATE SET leads=excluded.leads`, key, 1)
					return err
				}
				if v == "false" || v == "0" {
					_, err := s.conn.Exec(`INSERT INTO status(level, leads) VALUES(?, ?) ON CONFLICT(level) DO UPDATE SET leads=excluded.leads`, key, 0)
					return err
				}
				return err
			}
			val := 0
			if gs.Leads {
				val = 1
			}
			_, err = s.conn.Exec(`INSERT INTO status(level, leads) VALUES(?, ?) ON CONFLICT(level) DO UPDATE SET leads=excluded.leads`, key, val)
			return err
		default:
			b, err := json.Marshal(value)
			if err != nil {
				return err
			}
			var gs struct {
				Leads bool `json:"leads"`
			}
			if err := json.Unmarshal(b, &gs); err != nil {
				return err
			}
			val := 0
			if gs.Leads {
				val = 1
			}
			_, err = s.conn.Exec(`INSERT INTO status(level, leads) VALUES(?, ?) ON CONFLICT(level) DO UPDATE SET leads=excluded.leads`, key, val)
			return err
		}
	case "level_channels":
		switch v := value.(type) {
		case string:
			var m map[string]int64
			if err := json.Unmarshal([]byte(v), &m); err != nil {
				return fmt.Errorf("invalid level_channels payload")
			}
			_, err := s.conn.Exec(`INSERT INTO level_channels(level, level_channel, hint_channel) VALUES(?, ?, ?) ON CONFLICT(level) DO UPDATE SET level_channel=excluded.level_channel, hint_channel=excluded.hint_channel`, key, m["level"], m["hint"])
			return err
		case map[string]any:
			m := v
			lvl := int64(0)
			hint := int64(0)
			if vv, ok := m["level"]; ok {
				if n, ok := vv.(float64); ok {
					lvl = int64(n)
				}
			}
			if vv, ok := m["hint"]; ok {
				if n, ok := vv.(float64); ok {
					hint = int64(n)
				}
			}
			_, err := s.conn.Exec(`INSERT INTO level_channels(level, level_channel, hint_channel) VALUES(?, ?, ?) ON CONFLICT(level) DO UPDATE SET level_channel=excluded.level_channel, hint_channel=excluded.hint_channel`, key, lvl, hint)
			return err
		default:
			b, err := json.Marshal(value)
			if err != nil {
				return err
			}
			var m map[string]int64
			if err := json.Unmarshal(b, &m); err != nil {
				return err
			}
			_, err = s.conn.Exec(`INSERT INTO level_channels(level, level_channel, hint_channel) VALUES(?, ?, ?) ON CONFLICT(level) DO UPDATE SET level_channel=excluded.level_channel, hint_channel=excluded.hint_channel`, key, m["level"], m["hint"])
			return err
		}
	case "backlinks":
		str, ok := value.(string)
		if !ok {
			b, err := json.Marshal(value)
			if err != nil {
				return err
			}
			str = string(b)
		}
		_, err := s.conn.Exec(`INSERT INTO backlinks(backlink, url) VALUES(?, ?) ON CONFLICT(backlink) DO UPDATE SET url=excluded.url`, key, str)
		return err
	case "discord_messages":
		str, ok := value.(string)
		if !ok {
			b, err := json.Marshal(value)
			if err != nil {
				return err
			}
			str = string(b)
		}
		_, err := s.conn.Exec(`INSERT INTO discord_messages(id, email) VALUES(?, ?) ON CONFLICT(id) DO UPDATE SET email=excluded.email`, key, str)
		return err
	case "disqualified":
		switch v := value.(type) {
		case bool:
			val := 0
			if v {
				val = 1
			}
			_, err := s.conn.Exec(`INSERT INTO disqualified(email, disqualified) VALUES(?, ?) ON CONFLICT(email) DO UPDATE SET disqualified=excluded.disqualified`, key, val)
			return err
		case string:
			lower := strings.ToLower(v)
			val := 0
			if lower == "true" || lower == "1" {
				val = 1
			}
			_, err := s.conn.Exec(`INSERT INTO disqualified(email, disqualified) VALUES(?, ?) ON CONFLICT(email) DO UPDATE SET disqualified=excluded.disqualified`, key, val)
			return err
		default:
			b, err := json.Marshal(value)
			if err != nil {
				return err
			}
			var val bool
			if err := json.Unmarshal(b, &val); err != nil {
				return err
			}
			iv := 0
			if val {
				iv = 1
			}
			_, err = s.conn.Exec(`INSERT INTO disqualified(email, disqualified) VALUES(?, ?) ON CONFLICT(email) DO UPDATE SET disqualified=excluded.disqualified`, key, iv)
			return err
		}
	}
	return fmt.Errorf("unsupported kind: %s", kind)
}

func (s *Store) Get(kind, key string, dest any) (bool, error) {
	switch kind {
	case "levels":
		var markup, answer, answerHash, source sql.NullString
		var updated sql.NullInt64
		err := s.conn.QueryRow(`SELECT markup, answer, answer_hash, source_hint, updated_at FROM levels WHERE id = ?`, key).Scan(&markup, &answer, &answerHash, &source, &updated)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		out := map[string]any{"id": key, "markup": markup.String, "answer": answer.String, "answer_hash": answerHash.String, "source_hint": source.String, "updated_at": updated.Int64}
		b, _ := json.Marshal(out)
		return true, json.Unmarshal(b, dest)
	case "accounts":
		var name, level, levelsJSON sql.NullString
		var created sql.NullInt64
		err := s.conn.QueryRow(`SELECT name, level, levels_json, created_at FROM accounts WHERE email = ?`, key).Scan(&name, &level, &levelsJSON, &created)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		out := map[string]any{"email": key, "name": name.String, "level": level.String, "levels": nil, "created_at": created.Int64}
		if levelsJSON.Valid && levelsJSON.String != "null" {
			var lv map[string]string
			if err := json.Unmarshal([]byte(levelsJSON.String), &lv); err == nil {
				out["levels"] = lv
			}
		}
		b, _ := json.Marshal(out)
		return true, json.Unmarshal(b, dest)
	case "leaderboard":
		var name sql.NullString
		var level sql.NullInt64
		var t sql.NullInt64
		err := s.conn.QueryRow(`SELECT name, level, time FROM leaderboard WHERE email = ?`, key).Scan(&name, &level, &t)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		out := map[string]any{"email": key, "name": name.String, "level": int(level.Int64), "time": t.Int64}
		b, _ := json.Marshal(out)
		return true, json.Unmarshal(b, dest)
	case "otp":
		var code sql.NullString
		var expires sql.NullInt64
		err := s.conn.QueryRow(`SELECT code, expires_at FROM otp WHERE email = ?`, key).Scan(&code, &expires)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		out := map[string]any{"code": code.String, "expires_at": expires.Int64}
		b, _ := json.Marshal(out)
		return true, json.Unmarshal(b, dest)
	case "otp_rate":
		var sends sql.NullString
		err := s.conn.QueryRow(`SELECT sends_json FROM otp_rate WHERE email = ?`, key).Scan(&sends)
		if errors.Is(err, sql.ErrNoRows) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		if !sends.Valid || sends.String == "" {
			return true, nil
		}
		var arr []int64
		if err := json.Unmarshal([]byte(sends.String), &arr); err != nil {
			return false, err
		}
		b, _ := json.Marshal(arr)
		return true, json.Unmarshal(b, dest)
	case "sessions":
		var email sql.NullString
		var expires sql.NullInt64
		err := s.conn.QueryRow(`SELECT email, expires_at FROM sessions WHERE sid = ?`, key).Scan(&email, &expires)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		out := map[string]any{"email": email.String, "expires_at": expires.Int64}
		b, _ := json.Marshal(out)
		return true, json.Unmarshal(b, dest)
	case "logs":
		var content sql.NullString
		err := s.conn.QueryRow(`SELECT content FROM logs WHERE email = ?`, key).Scan(&content)
		if errors.Is(err, sql.ErrNoRows) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		var out string
		out = content.String
		b, _ := json.Marshal(out)
		return true, json.Unmarshal(b, dest)
	case "messages":
		var payload sql.NullString
		err := s.conn.QueryRow(`SELECT payload_json FROM messages WHERE id = ?`, key).Scan(&payload)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if !payload.Valid {
			return true, nil
		}
		return true, json.Unmarshal([]byte(payload.String), dest)
	case "hints":
		var payload sql.NullString
		err := s.conn.QueryRow(`SELECT payload_json FROM hints WHERE id = ?`, key).Scan(&payload)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if !payload.Valid {
			return true, nil
		}
		return true, json.Unmarshal([]byte(payload.String), dest)
	case "announcements":
		var content sql.NullString
		var t sql.NullInt64
		err := s.conn.QueryRow(`SELECT content, time FROM announcements WHERE id = ?`, key).Scan(&content, &t)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		out := map[string]any{"id": key, "content": content.String, "time": t.Int64}
		b, _ := json.Marshal(out)
		return true, json.Unmarshal(b, dest)
	case "level_channels":
		var lvl sql.NullInt64
		var hint sql.NullInt64
		err := s.conn.QueryRow(`SELECT level_channel, hint_channel FROM level_channels WHERE level = ?`, key).Scan(&lvl, &hint)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		out := map[string]any{"level": lvl.Int64, "hint": hint.Int64}
		b, _ := json.Marshal(out)
		return true, json.Unmarshal(b, dest)
	case "backlinks":
		var url sql.NullString
		err := s.conn.QueryRow(`SELECT url FROM backlinks WHERE backlink = ?`, key).Scan(&url)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		var out = url.String
		b, _ := json.Marshal(out)
		return true, json.Unmarshal(b, dest)
	case "discord_messages":
		var email sql.NullString
		err := s.conn.QueryRow(`SELECT email FROM discord_messages WHERE id = ?`, key).Scan(&email)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		b, _ := json.Marshal(email.String)
		return true, json.Unmarshal(b, dest)
	case "disqualified":
		var dq sql.NullInt64
		err := s.conn.QueryRow(`SELECT disqualified FROM disqualified WHERE email = ?`, key).Scan(&dq)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		b, _ := json.Marshal(dq.Int64 == 1)
		return true, json.Unmarshal(b, dest)
	case "meta":
		var val sql.NullString
		err := s.conn.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&val)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if !val.Valid {
			return false, nil
		}
		return true, json.Unmarshal([]byte(val.String), dest)
	case "status":
		var leads sql.NullInt64
		err := s.conn.QueryRow(`SELECT leads FROM status WHERE level = ?`, key).Scan(&leads)
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		b, _ := json.Marshal(leads.Int64 == 1)
		return true, json.Unmarshal(b, dest)
	}
	mk := kind + ":" + key
	var val sql.NullString
	err := s.conn.QueryRow(`SELECT value FROM meta WHERE key = ?`, mk).Scan(&val)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !val.Valid {
		return false, nil
	}
	return true, json.Unmarshal([]byte(val.String), dest)
}

func (s *Store) GetRaw(kind, key string) (json.RawMessage, bool, error) {
	switch kind {
	case "levels":
		var markup, answer, answerHash, source sql.NullString
		var updated sql.NullInt64
		err := s.conn.QueryRow(`SELECT markup, answer, answer_hash, source_hint, updated_at FROM levels WHERE id = ?`, key).Scan(&markup, &answer, &answerHash, &source, &updated)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		out := map[string]any{"id": key, "markup": markup.String, "answer": answer.String, "answer_hash": answerHash.String, "source_hint": source.String, "updated_at": updated.Int64}
		b, _ := json.Marshal(out)
		return b, true, nil
	case "accounts":
		var name, level, levelsJSON sql.NullString
		var created sql.NullInt64
		err := s.conn.QueryRow(`SELECT name, level, levels_json, created_at FROM accounts WHERE email = ?`, key).Scan(&name, &level, &levelsJSON, &created)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		out := map[string]any{"email": key, "name": name.String, "level": level.String, "levels_json": levelsJSON.String, "created_at": created.Int64}
		b, _ := json.Marshal(out)
		return b, true, nil
	case "messages":
		var payload sql.NullString
		err := s.conn.QueryRow(`SELECT payload_json FROM messages WHERE id = ?`, key).Scan(&payload)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		if !payload.Valid {
			return nil, true, nil
		}
		return json.RawMessage(payload.String), true, nil
	case "level_channels":
		var lvl, hint sql.NullInt64
		err := s.conn.QueryRow(`SELECT level_channel, hint_channel FROM level_channels WHERE level = ?`, key).Scan(&lvl, &hint)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		out := map[string]any{"level": lvl.Int64, "hint": hint.Int64}
		b, _ := json.Marshal(out)
		return json.RawMessage(b), true, nil
	case "discord_messages":
		var email sql.NullString
		err := s.conn.QueryRow(`SELECT email FROM discord_messages WHERE id = ?`, key).Scan(&email)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		b, _ := json.Marshal(email.String)
		return json.RawMessage(b), true, nil
	case "status":
		var leads sql.NullInt64
		err := s.conn.QueryRow(`SELECT leads FROM status WHERE level = ?`, key).Scan(&leads)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}
		b, _ := json.Marshal(map[string]bool{"leads": leads.Int64 == 1})
		return json.RawMessage(b), true, nil
	}
	return nil, false, fmt.Errorf("unsupported kind for GetRaw: %s", kind)
}

func (s *Store) Delete(kind, key string) error {
	switch kind {
	case "levels":
		_, err := s.conn.Exec(`DELETE FROM levels WHERE id = ?`, key)
		return err
	case "accounts":
		_, err := s.conn.Exec(`DELETE FROM accounts WHERE email = ?`, key)
		return err
	case "leaderboard":
		_, err := s.conn.Exec(`DELETE FROM leaderboard WHERE email = ?`, key)
		return err
	case "otp":
		_, err := s.conn.Exec(`DELETE FROM otp WHERE email = ?`, key)
		return err
	case "otp_rate":
		_, err := s.conn.Exec(`DELETE FROM otp_rate WHERE email = ?`, key)
		return err
	case "sessions":
		_, err := s.conn.Exec(`DELETE FROM sessions WHERE sid = ?`, key)
		return err
	case "logs":
		_, err := s.conn.Exec(`DELETE FROM logs WHERE email = ?`, key)
		return err
	case "messages":
		_, err := s.conn.Exec(`DELETE FROM messages WHERE id = ?`, key)
		return err
	case "hints":
		_, err := s.conn.Exec(`DELETE FROM hints WHERE id = ?`, key)
		return err
	case "announcements":
		_, err := s.conn.Exec(`DELETE FROM announcements WHERE id = ?`, key)
		return err
	case "meta":
		_, err := s.conn.Exec(`DELETE FROM meta WHERE key = ?`, key)
		return err
	case "level_channels":
		_, err := s.conn.Exec(`DELETE FROM level_channels WHERE level = ?`, key)
		return err
	case "backlinks":
		_, err := s.conn.Exec(`DELETE FROM backlinks WHERE backlink = ?`, key)
		return err
	case "discord_messages":
		_, err := s.conn.Exec(`DELETE FROM discord_messages WHERE id = ?`, key)
		return err
	case "disqualified":
		_, err := s.conn.Exec(`DELETE FROM disqualified WHERE email = ?`, key)
		return err
	case "status":
		_, err := s.conn.Exec(`DELETE FROM status WHERE level = ?`, key)
		return err
	}
	return fmt.Errorf("unsupported kind: %s", kind)
}

func (s *Store) List(kind string, dest any) error {
	switch kind {
	case "levels":
		rows, err := s.conn.Query(`SELECT id, markup, answer, answer_hash, source_hint, updated_at FROM levels ORDER BY id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		rv := reflect.ValueOf(dest)
		if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Slice {
			return fmt.Errorf("dest must be pointer to slice")
		}
		slice := reflect.MakeSlice(rv.Elem().Type(), 0, 0)
		elemType := rv.Elem().Type().Elem()
		for rows.Next() {
			var id, markup, answer, answerHash, source sql.NullString
			var updated sql.NullInt64
			if err := rows.Scan(&id, &markup, &answer, &answerHash, &source, &updated); err != nil {
				return err
			}
			elemPtr := reflect.New(elemType)
			m := map[string]any{"id": id.String, "markup": markup.String, "answer": answer.String, "answer_hash": answerHash.String, "source_hint": source.String, "updated_at": updated.Int64}
			b, _ := json.Marshal(m)
			if err := json.Unmarshal(b, elemPtr.Interface()); err != nil {
				return err
			}
			slice = reflect.Append(slice, elemPtr.Elem())
		}
		rv.Elem().Set(slice)
		return nil
	case "announcements":
		rows, err := s.conn.Query(`SELECT id, content, time FROM announcements ORDER BY time`)
		if err != nil {
			return err
		}
		defer rows.Close()
		rv := reflect.ValueOf(dest)
		if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Slice {
			return fmt.Errorf("dest must be pointer to slice")
		}
		slice := reflect.MakeSlice(rv.Elem().Type(), 0, 0)
		elemType := rv.Elem().Type().Elem()
		for rows.Next() {
			var id, content sql.NullString
			var t sql.NullInt64
			if err := rows.Scan(&id, &content, &t); err != nil {
				return err
			}
			elemPtr := reflect.New(elemType)
			m := map[string]any{"id": id.String, "content": content.String, "time": t.Int64}
			b, _ := json.Marshal(m)
			if err := json.Unmarshal(b, elemPtr.Interface()); err != nil {
				return err
			}
			slice = reflect.Append(slice, elemPtr.Elem())
		}
		rv.Elem().Set(slice)
		return nil
	case "leaderboard":
		rows, err := s.conn.Query(`SELECT email, name, level, time FROM leaderboard`)
		if err != nil {
			return err
		}
		defer rows.Close()
		rv := reflect.ValueOf(dest)
		if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Slice {
			return fmt.Errorf("dest must be pointer to slice")
		}
		slice := reflect.MakeSlice(rv.Elem().Type(), 0, 0)
		elemType := rv.Elem().Type().Elem()
		for rows.Next() {
			var email, name sql.NullString
			var level sql.NullInt64
			var t sql.NullInt64
			if err := rows.Scan(&email, &name, &level, &t); err != nil {
				return err
			}
			elemPtr := reflect.New(elemType)
			m := map[string]any{"email": email.String, "name": name.String, "level": int(level.Int64), "time": t.Int64}
			b, _ := json.Marshal(m)
			if err := json.Unmarshal(b, elemPtr.Interface()); err != nil {
				return err
			}
			slice = reflect.Append(slice, elemPtr.Elem())
		}
		rv.Elem().Set(slice)
		return nil
	case "level_channels":
		rows, err := s.conn.Query(`SELECT level, level_channel, hint_channel FROM level_channels ORDER BY level`)
		if err != nil {
			return err
		}
		defer rows.Close()
		rv := reflect.ValueOf(dest)
		if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Slice {
			return fmt.Errorf("dest must be pointer to slice")
		}
		slice := reflect.MakeSlice(rv.Elem().Type(), 0, 0)
		elemType := rv.Elem().Type().Elem()
		for rows.Next() {
			var level sql.NullString
			var lvl sql.NullInt64
			var hint sql.NullInt64
			if err := rows.Scan(&level, &lvl, &hint); err != nil {
				return err
			}
			elemPtr := reflect.New(elemType)
			m := map[string]any{"level": level.String, "level_channel": lvl.Int64, "hint_channel": hint.Int64}
			b, _ := json.Marshal(m)
			if err := json.Unmarshal(b, elemPtr.Interface()); err != nil {
				return err
			}
			slice = reflect.Append(slice, elemPtr.Elem())
		}
		rv.Elem().Set(slice)
		return nil
	}
	return fmt.Errorf("unsupported kind for List: %s", kind)
}

func (s *Store) ListByPrefix(kind, prefix string, dest any) error {
	switch kind {
	case "messages":
		rows, err := s.conn.Query(`SELECT payload_json FROM messages WHERE owner LIKE ? ORDER BY created_at`, prefix+"%")
		if err != nil {
			return err
		}
		defer rows.Close()
		rv := reflect.ValueOf(dest)
		if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Slice {
			return fmt.Errorf("dest must be pointer to slice")
		}
		slice := reflect.MakeSlice(rv.Elem().Type(), 0, 0)
		elemType := rv.Elem().Type().Elem()
		for rows.Next() {
			var payload sql.NullString
			if err := rows.Scan(&payload); err != nil {
				return err
			}
			if payload.Valid {
				elemPtr := reflect.New(elemType)
				if err := json.Unmarshal([]byte(payload.String), elemPtr.Interface()); err != nil {
					return err
				}
				slice = reflect.Append(slice, elemPtr.Elem())
			}
		}
		rv.Elem().Set(slice)
		return nil
	case "hints":
		rows, err := s.conn.Query(`SELECT payload_json FROM hints WHERE level_id LIKE ? ORDER BY created_at`, prefix+"%")
		if err != nil {
			return err
		}
		defer rows.Close()
		rv := reflect.ValueOf(dest)
		if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Slice {
			return fmt.Errorf("dest must be pointer to slice")
		}
		slice := reflect.MakeSlice(rv.Elem().Type(), 0, 0)
		elemType := rv.Elem().Type().Elem()
		for rows.Next() {
			var payload sql.NullString
			if err := rows.Scan(&payload); err != nil {
				return err
			}
			if payload.Valid {
				elemPtr := reflect.New(elemType)
				if err := json.Unmarshal([]byte(payload.String), elemPtr.Interface()); err != nil {
					return err
				}
				slice = reflect.Append(slice, elemPtr.Elem())
			}
		}
		rv.Elem().Set(slice)
		return nil
	case "discord_messages":
		rows, err := s.conn.Query(`SELECT email FROM discord_messages WHERE id LIKE ? ORDER BY id`, prefix+"%")
		if err != nil {
			return err
		}
		defer rows.Close()
		rv := reflect.ValueOf(dest)
		if rv.Kind() != reflect.Ptr || rv.Elem().Kind() != reflect.Slice {
			return fmt.Errorf("dest must be pointer to slice")
		}
		slice := reflect.MakeSlice(rv.Elem().Type(), 0, 0)
		elemType := rv.Elem().Type().Elem()
		for rows.Next() {
			var email sql.NullString
			if err := rows.Scan(&email); err != nil {
				return err
			}
			elemPtr := reflect.New(elemType)
			b, _ := json.Marshal(email.String)
			if err := json.Unmarshal(b, elemPtr.Interface()); err != nil {
				return err
			}
			slice = reflect.Append(slice, elemPtr.Elem())
		}
		rv.Elem().Set(slice)
		return nil
	}
	return fmt.Errorf("unsupported kind for ListByPrefix: %s", kind)
}

func (s *Store) Update(kind, key string, fn func(current json.RawMessage) (any, error)) error {
	return s.WithTx(context.Background(), func(tx *sql.Tx) error {
		switch kind {
		case "logs":
			var raw sql.NullString
			if err := tx.QueryRow(`SELECT content FROM logs WHERE email = ?`, key).Scan(&raw); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			var cur json.RawMessage
			if raw.Valid {
				cur = json.RawMessage([]byte(fmt.Sprintf("%q", raw.String)))
			} else {
				cur = nil
			}
			next, err := fn(cur)
			if err != nil {
				return err
			}
			var out string
			switch v := next.(type) {
			case string:
				out = v
			default:
				b, _ := json.Marshal(v)
				out = string(b)
			}
			if _, err := tx.Exec(`INSERT INTO logs(email, content) VALUES(?, ?) ON CONFLICT(email) DO UPDATE SET content=excluded.content`, key, out); err != nil {
				return err
			}
			return nil
		case "messages":
			var raw sql.NullString
			if err := tx.QueryRow(`SELECT payload_json FROM messages WHERE id = ?`, key).Scan(&raw); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			var cur json.RawMessage
			if raw.Valid {
				cur = json.RawMessage(raw.String)
			} else {
				cur = nil
			}
			next, err := fn(cur)
			if err != nil {
				return err
			}
			b, err := json.Marshal(next)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO messages(id, owner, payload_json, created_at) VALUES(?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET owner=excluded.owner, payload_json=excluded.payload_json, created_at=excluded.created_at`, key, "", string(b), time.Now().Unix()); err != nil {
				return err
			}
			return nil
		case "status":
			var raw sql.NullString
			if err := tx.QueryRow(`SELECT leads FROM status WHERE level = ?`, key).Scan(&raw); err != nil && !errors.Is(err, sql.ErrNoRows) {
			}
			next, err := fn(nil)
			if err != nil {
				return err
			}
			var gs struct {
				Leads bool `json:"leads"`
			}
			b, _ := json.Marshal(next)
			if err := json.Unmarshal(b, &gs); err != nil {
				return err
			}
			v := 0
			if gs.Leads {
				v = 1
			}
			if _, err := tx.Exec(`INSERT INTO status(level, leads) VALUES(?, ?) ON CONFLICT(level) DO UPDATE SET leads=excluded.leads`, key, v); err != nil {
				return err
			}
			return nil
		case "discord_messages":
			var raw sql.NullString
			if err := tx.QueryRow(`SELECT email FROM discord_messages WHERE id = ?`, key).Scan(&raw); err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			next, err := fn(nil)
			if err != nil {
				return err
			}
			emailB, _ := json.Marshal(next)
			var email string
			if err := json.Unmarshal(emailB, &email); err != nil {
				return err
			}
			if _, err := tx.Exec(`INSERT INTO discord_messages(id, email) VALUES(?, ?) ON CONFLICT(id) DO UPDATE SET email=excluded.email`, key, email); err != nil {
				return err
			}
			return nil
		default:
			return fmt.Errorf("unsupported kind for Update: %s", kind)
		}
	})
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
