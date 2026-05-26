package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	htmltmpl "html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"intrasudo26/db"
	tpl "intrasudo26/template"
)

type App struct {
	store       *db.Store
	renderer    *tpl.Renderer
	admins      map[string]bool
	salt        string
	startTime   time.Time
	endTime     time.Time
	mailer      Mailer
	levelsCache []Level
	levelIndex  map[string]int
	levelsMu    sync.RWMutex
}

type Mailer interface {
	Send(to, subject, html string) error
}

type LogMailer struct{}

func (LogMailer) Send(to, subject, html string) error {
	log.Printf("mail to=%s subject=%s size=%d", to, subject, len(html))
	return nil
}

type User struct {
	Name      string            `json:"name"`
	Email     string            `json:"email"`
	Level     string            `json:"level"`
	Levels    map[string]string `json:"levels,omitempty"`
	CreatedAt int64             `json:"created_at"`
}

type Level struct {
	ID         string `json:"id"`
	Markup     string `json:"markup"`
	Answer     string `json:"answer"`
	AnswerHash string `json:"answer_hash,omitempty"`
	SourceHint string `json:"source_hint"`
	UpdatedAt  int64  `json:"updated_at"`
}

func (l *Level) UnmarshalJSON(data []byte) error {
	var aux struct {
		ID         json.RawMessage `json:"id"`
		Markup     string          `json:"markup"`
		Answer     string          `json:"answer"`
		AnswerHash string          `json:"answer_hash"`
		SourceHint string          `json:"source_hint"`
		UpdatedAt  int64           `json:"updated_at"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	var sid string
	if err := json.Unmarshal(aux.ID, &sid); err == nil {
		l.ID = sid
	} else {
		var num float64
		if err := json.Unmarshal(aux.ID, &num); err == nil {
			if num == float64(int64(num)) {
				l.ID = fmt.Sprintf("%d", int64(num))
			} else {
				l.ID = fmt.Sprintf("%v", num)
			}
		} else {
			l.ID = string(aux.ID)
		}
	}

	l.Markup = aux.Markup
	l.Answer = aux.Answer
	l.AnswerHash = aux.AnswerHash
	l.SourceHint = aux.SourceHint
	l.UpdatedAt = aux.UpdatedAt
	return nil
}

type ChatMessage struct {
	ID      string `json:"id"`
	Author  string `json:"author"`
	Content string `json:"content"`
	Time    int64  `json:"time"`
	Kind    string `json:"kind"`
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

type OTPRecord struct {
	Code      string `json:"code"`
	ExpiresAt int64  `json:"expires_at"`
}

type GameStatus struct {
	Leads bool `json:"leads"`
}

type ViewData struct {
	Title               string
	RequestPath         string
	NowUnix             int64
	EventStartUnix      int64
	EventEndUnix        int64
	RedirectURL         string
	RedirectDelay       int
	RedirectTone        string
	RedirectReason      string
	RedirectData        any
	StatusLabel         string
	StatusURL           string
	LoggedIn            bool
	IsAdmin             bool
	User                *User
	CountdownLive       bool
	CountdownEndsIn     bool
	CountdownTargetUnix int64
	Levels              []Level
	Level               Level
	Leaderboard         []LeaderboardEntry
	Announcements       []Announcement
	ShowAnnouncements   bool
	Messages            []ChatMessage
	Hints               []ChatMessage
	SrcHint             htmltmpl.HTML
	LevelDisplay        string
}

func NewApp(store *db.Store, renderer *tpl.Renderer) *App {
	admins := map[string]bool{}
	rawAdmins := strings.TrimSpace(os.Getenv("ADMIN_EMAILS"))
	if rawAdmins != "" {
		if strings.HasPrefix(rawAdmins, "[") {
			var arr []string
			if err := json.Unmarshal([]byte(rawAdmins), &arr); err == nil {
				for _, e := range arr {
					e = strings.TrimSpace(strings.ToLower(e))
					if e != "" {
						admins[e] = true
					}
				}
			}
		} else {
			for _, email := range strings.Split(rawAdmins, ",") {
				email = strings.TrimSpace(strings.ToLower(email))
				if email != "" {
					admins[email] = true
				}
			}
		}
	}

	start, err := requiredUnixEnv("EVENT_START_UNIX")
	if err != nil {
		log.Fatalf("event config: %v", err)
	}
	end, err := requiredUnixEnv("EVENT_END_UNIX")
	if err != nil {
		log.Fatalf("event config: %v", err)
	}
	if !end.After(start) {
		log.Fatalf("event config: EVENT_END_UNIX must be after EVENT_START_UNIX")
	}
	salt := os.Getenv("AUTH_SALT")
	if salt == "" {
		salt = "intrasudo26-dev-salt"
	}

	app := &App{
		store:     store,
		renderer:  renderer,
		admins:    admins,
		salt:      salt,
		startTime: start,
		endTime:   end,
		mailer:    LogMailer{},
	}

	if strings.TrimSpace(os.Getenv("SMTP_HOST")) != "" {
		app.mailer = NewSMTPMailerFromEnv()
	}

	app.loadLevelsCache()

	return app
}

func requiredUnixEnv(key string) (time.Time, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return time.Time{}, fmt.Errorf("%s is required", key)
	}
	unix, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be a unix timestamp: %w", key, err)
	}
	return time.Unix(unix, 0), nil
}

func (a *App) Seed() error {
	if gs, ok, err := a.store.GetRaw("status", "global"); err != nil {
		return err
	} else if !ok {
		if err := a.store.SetStatus(context.Background(), "global", db.GameStatus{Leads: true}); err != nil {
			return err
		}
	} else {
		_ = gs
	}

	lvls, err := a.store.ListLevels(context.Background())
	if err != nil {
		return err
	}
	if len(lvls) == 0 {
		level := Level{
			ID:         "cryptic-0",
			Markup:     "Welcome to **Intra Sudo v7.0**.",
			Answer:     "intrasudo26",
			SourceHint: "Kickoff level",
			UpdatedAt:  time.Now().Unix(),
		}
		if err := a.store.SetLevel(context.Background(), db.Level{ID: level.ID, Markup: level.Markup, Answer: level.Answer, SourceHint: level.SourceHint, UpdatedAt: level.UpdatedAt}); err != nil {
			return err
		}
		a.loadLevelsCache()
		if err := a.store.SetStatus(context.Background(), level.ID, db.GameStatus{Leads: true}); err != nil {
			log.Printf("could not seed status for %s: %v", level.ID, err)
		}
	}

	anns, err := a.store.ListAnnouncements(context.Background())
	if err != nil {
		return err
	}
	if len(anns) == 0 {
		return a.store.SetAnnouncement(context.Background(), db.Announcement{ID: "welcome", Content: "Welcome to Intra Sudo v7.0.", Time: time.Now().Unix()})
	}
	return nil
}

func (a *App) render(w http.ResponseWriter, name string, data ViewData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := a.renderer.Render(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *App) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("could not write json response: %v", err)
	}
}

func (a *App) currentUser(r *http.Request) (*User, bool) {
	email, ok := GetEmailFromRequest(a.store, r)
	if !ok || email == "" {
		return nil, false
	}

	var user User
	acc, okAcc, err := a.store.GetAccount(context.Background(), strings.ToLower(email))
	if err != nil || !okAcc {
		return nil, false
	}
	user = User{Name: acc.Name, Email: acc.Email, Level: acc.Level, Levels: acc.Levels, CreatedAt: acc.CreatedAt}
	return &user, true
}

func (a *App) setAuthCookies(w http.ResponseWriter, user User) {
	email := strings.ToLower(user.Email)
	maxAge := 60 * 60 * 24 * 30

	http.SetCookie(w, &http.Cookie{
		Name:     "email",
		Value:    email,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   cookiesSecure(),
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *App) clearAuthCookies(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "email",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   cookiesSecure(),
		SameSite: http.SameSiteLaxMode,
	})
}

func (a *App) appendToLogs(email, line string) {
	if err := a.store.AppendLog(email, line); err != nil {
		log.Printf("could not append to logs: %v", err)
	}
}

func (a *App) isAdmin(email string) bool {
	return a.admins[strings.ToLower(email)]
}

func (a *App) duringEvent() bool {
	now := time.Now()
	return (now.Equal(a.startTime) || now.After(a.startTime)) && now.Before(a.endTime)
}

func (a *App) otpCode(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(email) + ":" + a.salt))
	raw := int(sum[0])<<16 | int(sum[1])<<8 | int(sum[2])
	return fmt.Sprintf("%06d", raw%1000000)
}

func (a *App) baseData(r *http.Request) ViewData {
	user, loggedIn := a.currentUser(r)
	now := time.Now()
	data := ViewData{
		RequestPath:    r.URL.Path,
		NowUnix:        now.Unix(),
		EventStartUnix: a.startTime.Unix(),
		EventEndUnix:   a.endTime.Unix(),
		LoggedIn:       loggedIn,
	}

	switch {
	case now.Before(a.startTime):
		data.CountdownLive = true
		data.CountdownEndsIn = false
		data.CountdownTargetUnix = a.startTime.Unix()
	case a.duringEvent():
		data.CountdownLive = true
		data.CountdownEndsIn = true
		data.CountdownTargetUnix = a.endTime.Unix()
	default:
		data.CountdownLive = false
	}

	if loggedIn {
		data.User = user
		data.IsAdmin = a.isAdmin(user.Email)
		data.StatusLabel = "Logout"
		data.StatusURL = "/logout"
	} else {
		data.StatusLabel = "Login"
		data.StatusURL = "/auth"
	}

	if anns, err := a.store.ListAnnouncements(r.Context()); err == nil {
		data.Announcements = make([]Announcement, len(anns))
		for i := range anns {
			data.Announcements[i] = Announcement{ID: anns[i].ID, Content: anns[i].Content, Time: anns[i].Time}
		}
		sort.Slice(data.Announcements, func(i, j int) bool { return data.Announcements[i].Time < data.Announcements[j].Time })
	} else {
		log.Printf("could not list announcements: %v", err)
	}

	return data
}

func normalizeAnswer(v string) string {
	return strings.TrimSpace(strings.ToLower(v))
}

func (a *App) loadLevelsCache() {
	var all []Level
	lvls, err := a.store.ListLevels(context.Background())
	if err != nil {
		log.Printf("could not load levels cache: %v", err)
		a.levelsMu.Lock()
		a.levelsCache = nil
		a.levelIndex = map[string]int{}
		a.levelsMu.Unlock()
		return
	}
	for i := range lvls {
		lvl := lvls[i]
		all = append(all, Level{ID: lvl.ID, Markup: lvl.Markup, Answer: lvl.Answer, AnswerHash: lvl.AnswerHash, SourceHint: lvl.SourceHint, UpdatedAt: lvl.UpdatedAt})
	}
	sort.Slice(all, func(i, j int) bool { return compareLevelIDs(all[i].ID, all[j].ID) })
	idx := make(map[string]int, len(all))
	for i, lv := range all {
		idx[lv.ID] = i
	}
	a.levelsMu.Lock()
	a.levelsCache = all
	a.levelIndex = idx
	a.levelsMu.Unlock()
}

func (a *App) getFirstLevelForType(t string) string {
	a.levelsMu.RLock()
	defer a.levelsMu.RUnlock()
	for _, lv := range a.levelsCache {
		if strings.HasPrefix(lv.ID, t+"-") {
			return lv.ID
		}
	}
	if len(a.levelsCache) > 0 {
		return a.levelsCache[0].ID
	}
	return ""
}

func (a *App) NextLevelForType(curKey string, t string) string {
	a.levelsMu.RLock()
	defer a.levelsMu.RUnlock()
	if a.levelIndex == nil {
		return curKey
	}
	if idx, ok := a.levelIndex[curKey]; ok {
		for i := idx + 1; i < len(a.levelsCache); i++ {
			if strings.HasPrefix(a.levelsCache[i].ID, t+"-") {
				return a.levelsCache[i].ID
			}
		}
		return curKey
	}
	return curKey
}

func (a *App) getFirstLevel() string {
	a.levelsMu.RLock()
	if len(a.levelsCache) > 0 {
		first := a.levelsCache[0].ID
		a.levelsMu.RUnlock()
		return first
	}
	a.levelsMu.RUnlock()
	var all []Level
	lvls, err := a.store.ListLevels(context.Background())
	if err != nil {
		log.Printf("could not list levels: %v", err)
		return ""
	}
	for i := range lvls {
		l := lvls[i]
		all = append(all, Level{ID: l.ID, Markup: l.Markup, Answer: l.Answer, AnswerHash: l.AnswerHash, SourceHint: l.SourceHint, UpdatedAt: l.UpdatedAt})
	}
	if len(all) == 0 {
		return ""
	}
	sort.Slice(all, func(i, j int) bool { return compareLevelIDs(all[i].ID, all[j].ID) })
	return all[0].ID
}

func (a *App) NextLevel(curKey string) string {
	a.levelsMu.RLock()
	defer a.levelsMu.RUnlock()
	if a.levelIndex == nil {
		return curKey
	}
	if idx, ok := a.levelIndex[curKey]; ok {
		if idx+1 < len(a.levelsCache) {
			return a.levelsCache[idx+1].ID
		}
		return curKey
	}
	return curKey
}

func (a *App) LevelPosition(id string) (int, bool) {
	a.levelsMu.RLock()
	defer a.levelsMu.RUnlock()
	if a.levelIndex == nil {
		return 0, false
	}
	if idx, ok := a.levelIndex[id]; ok {
		return idx, true
	}
	return 0, false
}

func (a *App) ListLevels() []Level {
	a.levelsMu.RLock()
	defer a.levelsMu.RUnlock()
	out := make([]Level, len(a.levelsCache))
	copy(out, a.levelsCache)
	return out
}

func (a *App) BotAuthOK(r *http.Request) bool {
	token := r.Header.Get("X-BOT-TOKEN")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		_ = r.ParseForm()
		token = strings.TrimSpace(r.FormValue("token"))
	}
	expected := strings.TrimSpace(os.Getenv("BOT_API_TOKEN"))
	if expected == "" {
		expected = strings.TrimSpace(os.Getenv("BOT_TOKEN"))
	}
	return token != "" && expected != "" && token == expected
}

func (a *App) BotGet(w http.ResponseWriter, r *http.Request) {
	if !a.BotAuthOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ns := strings.TrimSpace(r.URL.Query().Get("ns"))
	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if ns == "" || key == "" {
		http.Error(w, "missing ns or key", http.StatusBadRequest)
		return
	}
	if ns == "backlinks" {
		url, ok, err := a.store.GetBacklink(r.Context(), key)
		if err != nil {
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(url))
		return
	}
	if ns == "otp" {
		rec, ok, err := a.store.GetOTP(key)
		if err != nil {
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		b, err := json.Marshal(rec)
		if err != nil {
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
		return
	}
	if _, ok := strings.CutPrefix(ns, "messages/"); ok {
		raw, ok, err := a.store.GetMessage(key)
		if err != nil {
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(raw)
		return
	}
	if _, ok := strings.CutPrefix(ns, "hints/"); ok {
		raw, ok, err := a.store.GetHint(r.Context(), key)
		if err != nil {
			http.Error(w, "internal", http.StatusInternalServerError)
			return
		}
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(raw)
		return
	}
	k := ns + ":" + key
	raw, ok, err := a.store.GetMeta(k)
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(raw)
}

func (a *App) BotSet(w http.ResponseWriter, r *http.Request) {
	if !a.BotAuthOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	ns := strings.TrimSpace(r.FormValue("ns"))
	key := strings.TrimSpace(r.FormValue("key"))
	val := strings.TrimSpace(r.FormValue("val"))
	if ns == "" || key == "" || val == "" {
		http.Error(w, "missing ns, key, or val", http.StatusBadRequest)
		return
	}
	if ns == "backlinks" {
		if err := a.store.SetBacklink(r.Context(), key, val); err != nil {
			http.Error(w, "could not set", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
		return
	}
	if ns == "otp" {
		var rec db.OTPRecord
		if err := json.Unmarshal([]byte(val), &rec); err != nil {
			http.Error(w, "invalid otp payload", http.StatusBadRequest)
			return
		}
		if err := a.store.SetOTP(key, rec); err != nil {
			http.Error(w, "could not set", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
		return
	}
	if owner, ok := strings.CutPrefix(ns, "messages/"); ok {
		email := strings.ToLower(strings.TrimSpace(owner))
		msg := ChatMessage{
			ID:      key,
			Author:  "Exun Clan",
			Content: val,
			Time:    time.Now().Unix(),
			Kind:    "hint",
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			http.Error(w, "could not serialize", http.StatusInternalServerError)
			return
		}
		if err := a.store.SetMessage(r.Context(), key, email, payload, msg.Time); err != nil {
			http.Error(w, "could not set", http.StatusInternalServerError)
			return
		}
		if err := a.store.SetMeta(r.Context(), "messages_updated", time.Now().UnixMilli()); err != nil {
			log.Printf("could not update messages meta: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
		return
	}
	if level, ok := strings.CutPrefix(ns, "hints/"); ok {
		levelID := strings.TrimSpace(level)
		if levelID == "" {
			http.Error(w, "missing level id", http.StatusBadRequest)
			return
		}
		msg := ChatMessage{
			ID:      key,
			Author:  "Exun Clan",
			Content: val,
			Time:    time.Now().Unix(),
			Kind:    "hint",
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			http.Error(w, "could not serialize", http.StatusInternalServerError)
			return
		}
		if err := a.store.SetHint(r.Context(), key, levelID, payload, msg.Time); err != nil {
			http.Error(w, "could not set", http.StatusInternalServerError)
			return
		}
		if err := a.store.SetMeta(r.Context(), "messages_updated", time.Now().UnixMilli()); err != nil {
			log.Printf("could not update messages meta: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
		return
	}
	k := ns + ":" + key
	if err := a.store.SetMeta(r.Context(), k, val); err != nil {
		http.Error(w, "could not set", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (a *App) BotDelete(w http.ResponseWriter, r *http.Request) {
	if !a.BotAuthOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ns := strings.TrimSpace(r.URL.Query().Get("ns"))
	key := strings.TrimSpace(r.URL.Query().Get("key"))
	if ns == "" || key == "" {
		http.Error(w, "missing ns or key", http.StatusBadRequest)
		return
	}
	if ns == "otp" {
		if err := a.store.DeleteOTP(key); err != nil {
			http.Error(w, "could not delete", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
		return
	}
	if _, ok := strings.CutPrefix(ns, "messages/"); ok {
		if err := a.store.DeleteMessage(r.Context(), key); err != nil {
			http.Error(w, "could not delete", http.StatusInternalServerError)
			return
		}
		if err := a.store.SetMeta(r.Context(), "messages_updated", time.Now().UnixMilli()); err != nil {
			log.Printf("could not update messages meta: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
		return
	}
	if _, ok := strings.CutPrefix(ns, "hints/"); ok {
		if err := a.store.DeleteHint(r.Context(), key); err != nil {
			http.Error(w, "could not delete", http.StatusInternalServerError)
			return
		}
		if err := a.store.SetMeta(r.Context(), "messages_updated", time.Now().UnixMilli()); err != nil {
			log.Printf("could not update messages meta: %v", err)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
		return
	}
	k := ns + ":" + key
	if err := a.store.DeleteMeta(r.Context(), k); err != nil {
		http.Error(w, "could not delete", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func (a *App) BotLevelsCount(w http.ResponseWriter, r *http.Request) {
	if !a.BotAuthOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	levels, err := a.store.ListLevels(r.Context())
	if err != nil {
		http.Error(w, "internal", http.StatusInternalServerError)
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{
		"count": len(levels),
	})
}

func (a *App) ExternalSendMessage(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	level := strings.TrimSpace(r.FormValue("level"))
	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))
	content := strings.TrimSpace(r.FormValue("content"))
	if level == "" || email == "" || content == "" {
		http.Error(w, "missing fields", http.StatusBadRequest)
		return
	}
	botService := strings.TrimSpace(os.Getenv("BOT_SERVICE_URL"))
	if botService == "" {
		botService = "http://localhost:5555"
	}
	u := botService + "/send_message?level=" + url.QueryEscape(level) + "&name=" + url.QueryEscape(name) + "&email=" + url.QueryEscape(email) + "&content=" + url.QueryEscape(content)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(u)
	if err != nil {
		log.Printf("could not call bot service: %v", err)
		http.Error(w, "could not call bot", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		http.Error(w, "bot error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}

func parseTrailingInt(s string) (prefix string, n int, ok bool) {
	for i := len(s) - 1; i >= 0; i-- {
		c := s[i]
		if c < '0' || c > '9' {
			if i == len(s)-1 {
				return s, 0, false
			}
			num, err := strconv.Atoi(s[i+1:])
			if err != nil {
				return s, 0, false
			}
			return s[:i+1], num, true
		}
	}
	num, err := strconv.Atoi(s)
	if err != nil {
		return s, 0, false
	}
	return "", num, true
}

func compareLevelIDs(a, b string) bool {
	pa, na, oka := parseTrailingInt(a)
	pb, nb, okb := parseTrailingInt(b)
	if oka && okb && pa == pb {
		return na < nb
	}
	return a < b
}
