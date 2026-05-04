package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Level struct {
	ID         string `json:"id"`
	Markup     string `json:"markup"`
	Answer     string `json:"answer"`
	AnswerHash string `json:"answer_hash"`
	SourceHint string `json:"source_hint"`
	UpdatedAt  int64  `json:"updated_at"`
}

type Announcement struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Time    int64  `json:"time"`
}

type LeaderboardEntry struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Level int    `json:"level"`
	Time  int64  `json:"time"`
}

type GameStatus struct {
	Leads bool `json:"leads"`
}

type Account struct {
	Email     string            `json:"email"`
	Name      string            `json:"name"`
	Level     string            `json:"level"`
	Levels    map[string]string `json:"levels,omitempty"`
	CreatedAt int64             `json:"created_at"`
}

type OTPRecord struct {
	Code      string `json:"code"`
	ExpiresAt int64  `json:"expires_at"`
}

type SessionRecord struct {
	Email     string `json:"email"`
	ExpiresAt int64  `json:"expires_at"`
}

func (s *Store) ListLevels() ([]Level, error) {
	rows, err := s.conn.Query(`SELECT id, markup, answer, answer_hash, source_hint, updated_at FROM levels ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Level{}
	for rows.Next() {
		var id, markup, answer, answerHash, source sql.NullString
		var updated sql.NullInt64
		if err := rows.Scan(&id, &markup, &answer, &answerHash, &source, &updated); err != nil {
			return nil, err
		}
		out = append(out, Level{ID: id.String, Markup: markup.String, Answer: answer.String, AnswerHash: answerHash.String, SourceHint: source.String, UpdatedAt: updated.Int64})
	}
	return out, nil
}

func (s *Store) GetLevel(id string) (Level, bool, error) {
	var lv Level
	var markup, answer, answerHash, source sql.NullString
	var updated sql.NullInt64
	err := s.conn.QueryRow(`SELECT markup, answer, answer_hash, source_hint, updated_at FROM levels WHERE id = ?`, id).Scan(&markup, &answer, &answerHash, &source, &updated)
	if err != nil {
		if err == sql.ErrNoRows {
			return lv, false, nil
		}
		return lv, false, err
	}
	lv = Level{ID: id, Markup: markup.String, Answer: answer.String, AnswerHash: answerHash.String, SourceHint: source.String, UpdatedAt: updated.Int64}
	return lv, true, nil
}

func (s *Store) SetLevel(lv Level) error {
	_, err := s.conn.Exec(`INSERT INTO levels(id, markup, answer, answer_hash, source_hint, updated_at)
        VALUES(?, ?, ?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET markup=excluded.markup, answer=excluded.answer, answer_hash=excluded.answer_hash, source_hint=excluded.source_hint, updated_at=excluded.updated_at`, lv.ID, lv.Markup, lv.Answer, lv.AnswerHash, lv.SourceHint, lv.UpdatedAt)
	return err
}

func (s *Store) ListAnnouncements() ([]Announcement, error) {
	rows, err := s.conn.Query(`SELECT id, content, time FROM announcements ORDER BY time`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Announcement{}
	for rows.Next() {
		var id, content sql.NullString
		var t sql.NullInt64
		if err := rows.Scan(&id, &content, &t); err != nil {
			return nil, err
		}
		out = append(out, Announcement{ID: id.String, Content: content.String, Time: t.Int64})
	}
	return out, nil
}

func (s *Store) SetAnnouncement(a Announcement) error {
	_, err := s.conn.Exec(`INSERT INTO announcements(id, content, time) VALUES(?, ?, ?) ON CONFLICT(id) DO UPDATE SET content=excluded.content, time=excluded.time`, a.ID, a.Content, a.Time)
	return err
}

func (s *Store) DeleteLevel(id string) error {
	_, err := s.conn.Exec(`DELETE FROM levels WHERE id = ?`, id)
	return err
}

func (s *Store) DeleteAnnouncement(id string) error {
	_, err := s.conn.Exec(`DELETE FROM announcements WHERE id = ?`, id)
	return err
}

func (s *Store) ListMessagesForOwner(ownerPrefix string) ([]json.RawMessage, error) {
	rows, err := s.conn.Query(`SELECT payload_json FROM messages WHERE owner LIKE ? ORDER BY created_at`, ownerPrefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []json.RawMessage
	for rows.Next() {
		var payload sql.NullString
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		if payload.Valid {
			out = append(out, json.RawMessage(payload.String))
		}
	}
	return out, nil
}

func (s *Store) ListHintsForLevel(levelPrefix string) ([]json.RawMessage, error) {
	rows, err := s.conn.Query(`SELECT payload_json FROM hints WHERE level_id LIKE ? ORDER BY created_at`, levelPrefix+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []json.RawMessage
	for rows.Next() {
		var payload sql.NullString
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		if payload.Valid {
			out = append(out, json.RawMessage(payload.String))
		}
	}
	return out, nil
}

func (s *Store) SetMessage(id, owner string, payload json.RawMessage, createdAt int64) error {
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}
	_, err := s.conn.Exec(`INSERT INTO messages(id, owner, payload_json, created_at) VALUES(?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET owner=excluded.owner, payload_json=excluded.payload_json, created_at=excluded.created_at`, id, owner, string(payload), createdAt)
	return err
}

func (s *Store) DeleteMessage(id string) error {
	_, err := s.conn.Exec(`DELETE FROM messages WHERE id = ?`, id)
	return err
}

func (s *Store) ListLeaderboard() ([]LeaderboardEntry, error) {
	rows, err := s.conn.Query(`SELECT email, name, level, time FROM leaderboard`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []LeaderboardEntry
	for rows.Next() {
		var email, name sql.NullString
		var level sql.NullInt64
		var t sql.NullInt64
		if err := rows.Scan(&email, &name, &level, &t); err != nil {
			return nil, err
		}
		out = append(out, LeaderboardEntry{Email: email.String, Name: name.String, Level: int(level.Int64), Time: t.Int64})
	}
	return out, nil
}

func (s *Store) GetAccount(email string) (Account, bool, error) {
	var a Account
	var name, level, levelsJSON sql.NullString
	var created sql.NullInt64
	err := s.conn.QueryRow(`SELECT name, level, levels_json, created_at FROM accounts WHERE email = ?`, email).Scan(&name, &level, &levelsJSON, &created)
	if err != nil {
		if err == sql.ErrNoRows {
			return a, false, nil
		}
		return a, false, err
	}
	a = Account{Email: email, Name: name.String, Level: level.String, Levels: map[string]string{}, CreatedAt: created.Int64}
	if levelsJSON.Valid && levelsJSON.String != "null" && levelsJSON.String != "" {
		var m map[string]string
		if err := json.Unmarshal([]byte(levelsJSON.String), &m); err == nil {
			a.Levels = m
		}
	}
	return a, true, nil
}

func (s *Store) SetAccount(email string, acc Account) error {
	levelsJSON := "null"
	if acc.Levels != nil {
		b, _ := json.Marshal(acc.Levels)
		levelsJSON = string(b)
	}
	_, err := s.conn.Exec(`INSERT INTO accounts(email, name, level, levels_json, created_at) VALUES(?, ?, ?, ?, ?) ON CONFLICT(email) DO UPDATE SET name=excluded.name, level=excluded.level, levels_json=excluded.levels_json`, email, acc.Name, acc.Level, levelsJSON, acc.CreatedAt)
	return err
}

func (s *Store) GetStatus(level string) (GameStatus, bool, error) {
	var raw sql.NullInt64
	err := s.conn.QueryRow(`SELECT leads FROM status WHERE level = ?`, level).Scan(&raw)
	if err != nil {
		if err == sql.ErrNoRows {
			return GameStatus{}, false, nil
		}
		return GameStatus{}, false, err
	}
	return GameStatus{Leads: raw.Int64 == 1}, true, nil
}

func (s *Store) SetStatus(level string, gs GameStatus) error {
	v := 0
	if gs.Leads {
		v = 1
	}
	_, err := s.conn.Exec(`INSERT INTO status(level, leads) VALUES(?, ?) ON CONFLICT(level) DO UPDATE SET leads=excluded.leads`, level, v)
	return err
}

func (s *Store) SetLeaderboard(email string, ent LeaderboardEntry) error {
	_, err := s.conn.Exec(`INSERT INTO leaderboard(email, name, level, time) VALUES(?, ?, ?, ?) ON CONFLICT(email) DO UPDATE SET name=excluded.name, level=excluded.level, time=excluded.time`, email, ent.Name, ent.Level, ent.Time)
	return err
}

func (s *Store) SetMeta(key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.conn.Exec(`INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, string(b))
	return err
}

func (s *Store) GetMetaInt(key string) (int64, bool, error) {
	var val sql.NullString
	err := s.conn.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&val)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, true, nil
		}
		return 0, false, err
	}
	if !val.Valid || val.String == "" {
		return 0, true, nil
	}
	var v int64
	if err := json.Unmarshal([]byte(val.String), &v); err != nil {
		return 0, false, err
	}
	return v, true, nil
}

func (s *Store) GetAnnouncement(id string) (Announcement, bool, error) {
	var a Announcement
	var content sql.NullString
	var t sql.NullInt64
	err := s.conn.QueryRow(`SELECT content, time FROM announcements WHERE id = ?`, id).Scan(&content, &t)
	if err != nil {
		if err == sql.ErrNoRows {
			return a, false, nil
		}
		return a, false, err
	}
	a = Announcement{ID: id, Content: content.String, Time: t.Int64}
	return a, true, nil
}

func (s *Store) GetMessage(id string) (json.RawMessage, bool, error) {
	var payload sql.NullString
	err := s.conn.QueryRow(`SELECT payload_json FROM messages WHERE id = ?`, id).Scan(&payload)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	if !payload.Valid {
		return nil, true, nil
	}
	return json.RawMessage(payload.String), true, nil
}

func (s *Store) GetMeta(key string) (json.RawMessage, bool, error) {
	var val sql.NullString
	err := s.conn.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&val)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	if !val.Valid {
		return nil, false, nil
	}
	return json.RawMessage(val.String), true, nil
}

func (s *Store) SetRaw(kind, key string, value any) error {
	return fmt.Errorf("SetRaw has been removed; use typed methods instead")
}

func (s *Store) DeleteRaw(kind, key string) error {
	return fmt.Errorf("DeleteRaw has been removed; use typed methods instead")
}

func (s *Store) GetRaw(kind, key string) (json.RawMessage, bool, error) {
	return nil, false, fmt.Errorf("GetRaw has been removed; use typed methods instead")
}

func (s *Store) AppendLog(email, line string) error {
	email = strings.ToLower(email)
	var cur sql.NullString
	if err := s.conn.QueryRow(`SELECT content FROM logs WHERE email = ?`, email).Scan(&cur); err != nil && err != sql.ErrNoRows {
		return err
	}
	currentStr := ""
	if cur.Valid {
		currentStr = cur.String
	}
	currentStr += time.Now().Format("2006-01-02 15:04:05") + " : " + line + "\n"
	if len(currentStr) > 10_240 {
		currentStr = currentStr[len(currentStr)-10_240:]
	}
	_, err := s.conn.Exec(`INSERT INTO logs(email, content) VALUES(?, ?) ON CONFLICT(email) DO UPDATE SET content=excluded.content`, email, currentStr)
	return err
}

func (s *Store) GetOTP(email string) (OTPRecord, bool, error) {
	var rec OTPRecord
	var code sql.NullString
	var expires sql.NullInt64
	err := s.conn.QueryRow(`SELECT code, expires_at FROM otp WHERE email = ?`, email).Scan(&code, &expires)
	if err != nil {
		if err == sql.ErrNoRows {
			return rec, false, nil
		}
		return rec, false, err
	}
	rec.Code = code.String
	rec.ExpiresAt = expires.Int64
	return rec, true, nil
}

func (s *Store) SetOTP(email string, rec OTPRecord) error {
	_, err := s.conn.Exec(`INSERT INTO otp(email, code, expires_at) VALUES(?, ?, ?) ON CONFLICT(email) DO UPDATE SET code=excluded.code, expires_at=excluded.expires_at`, email, rec.Code, rec.ExpiresAt)
	return err
}

func (s *Store) GetOTPRate(email string) ([]int64, bool, error) {
	var sends sql.NullString
	err := s.conn.QueryRow(`SELECT sends_json FROM otp_rate WHERE email = ?`, email).Scan(&sends)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	if !sends.Valid || sends.String == "" {
		return nil, true, nil
	}
	var arr []int64
	if err := json.Unmarshal([]byte(sends.String), &arr); err != nil {
		return nil, false, err
	}
	return arr, true, nil
}

func (s *Store) SetOTPRate(email string, arr []int64) error {
	b, _ := json.Marshal(arr)
	_, err := s.conn.Exec(`INSERT INTO otp_rate(email, sends_json) VALUES(?, ?) ON CONFLICT(email) DO UPDATE SET sends_json=excluded.sends_json`, email, string(b))
	return err
}

func (s *Store) CreateSession(sid, email string, expiresAt int64) error {
	_, err := s.conn.Exec(`INSERT INTO sessions(sid, email, expires_at) VALUES(?, ?, ?) ON CONFLICT(sid) DO UPDATE SET email=excluded.email, expires_at=excluded.expires_at`, sid, email, expiresAt)
	return err
}

func (s *Store) GetSession(sid string) (SessionRecord, bool, error) {
	var rec SessionRecord
	var email sql.NullString
	var expires sql.NullInt64
	err := s.conn.QueryRow(`SELECT email, expires_at FROM sessions WHERE sid = ?`, sid).Scan(&email, &expires)
	if err != nil {
		if err == sql.ErrNoRows {
			return rec, false, nil
		}
		return rec, false, err
	}
	rec.Email = email.String
	rec.ExpiresAt = expires.Int64
	return rec, true, nil
}

func (s *Store) DeleteSession(sid string) error {
	_, err := s.conn.Exec(`DELETE FROM sessions WHERE sid = ?`, sid)
	return err
}
