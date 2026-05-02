package handlers

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	htmltmpl "html/template"
	"log"
	"net/http"
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
	Name      string `json:"name"`
	Email     string `json:"email"`
	Level     string `json:"level"`
	CreatedAt int64  `json:"created_at"`
}

type Level struct {
	ID         string `json:"id"`
	Markup     string `json:"markup"`
	Answer     string `json:"answer"`
	SourceHint string `json:"source_hint"`
	UpdatedAt  int64  `json:"updated_at"`
}

func (l *Level) UnmarshalJSON(data []byte) error {
	var aux struct {
		ID         json.RawMessage `json:"id"`
		Markup     string          `json:"markup"`
		Answer     string          `json:"answer"`
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
	Title             string
	RequestPath       string
	NowUnix           int64
	EventStartUnix    int64
	EventEndUnix      int64
	RedirectURL       string
	RedirectDelay     int
	RedirectTone      string
	RedirectReason    string
	StatusLabel       string
	StatusURL         string
	LoggedIn          bool
	IsAdmin           bool
	User              *User
	CountdownLive     bool
	Levels            []Level
	Level             Level
	Leaderboard       []LeaderboardEntry
	Announcements     []Announcement
	ShowAnnouncements bool
	Messages          []ChatMessage
	Hints             []ChatMessage
	SrcHint           htmltmpl.HTML
	LevelDisplay      string
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

	start := parseUnixEnv("EVENT_START_UNIX", time.Now().Add(24*time.Hour))
	end := parseUnixEnv("EVENT_END_UNIX", start.Add(48*time.Hour))
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

func parseUnixEnv(key string, fallback time.Time) time.Time {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	unix, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return time.Unix(unix, 0)
}

func (a *App) Seed() error {
	var status GameStatus
	ok, err := a.store.Get("status", "global", &status)
	if err != nil {
		return err
	}
	if !ok {
		if err := a.store.Set("status", "global", GameStatus{Leads: true}); err != nil {
			return err
		}
	}

	var levels []Level
	if err := a.store.List("levels", &levels); err != nil {
		return err
	}
	if len(levels) == 0 {
		level := Level{
			ID:         "cryptic-0",
			Markup:     "Welcome to **Intra Sudo v7.0**.\n\nUse the admin page to add levels, or replace this seed level with your real hunt content.",
			Answer:     "intrasudo26",
			SourceHint: "Kickoff level",
			UpdatedAt:  time.Now().Unix(),
		}
		if err := a.store.Set("levels", level.ID, level); err != nil {
			return err
		}
		a.loadLevelsCache()
	}

	var announcements []Announcement
	if err := a.store.List("announcements", &announcements); err != nil {
		return err
	}
	if len(announcements) == 0 {
		return a.store.Set("announcements", "welcome", Announcement{
			ID:      "welcome",
			Content: "Intra Sudo v7.0 has been initialized. Update announcements from the database or extend the admin tools.",
			Time:    time.Now().Unix(),
		})
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
	exists, err := a.store.Get("accounts", strings.ToLower(email), &user)
	if err != nil || !exists {
		return nil, false
	}
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
		Secure:   true,
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
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
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
	data := ViewData{
		RequestPath:    r.URL.Path,
		NowUnix:        time.Now().Unix(),
		EventStartUnix: a.startTime.Unix(),
		EventEndUnix:   a.endTime.Unix(),
		LoggedIn:       loggedIn,
		CountdownLive:  !a.duringEvent(),
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

	if err := a.store.List("announcements", &data.Announcements); err != nil {
		log.Printf("could not list announcements: %v", err)
	}
	sort.Slice(data.Announcements, func(i, j int) bool {
		return data.Announcements[i].Time < data.Announcements[j].Time
	})

	return data
}

func normalizeAnswer(v string) string {
	return strings.TrimSpace(strings.ToLower(v))
}

func (a *App) loadLevelsCache() {
	var all []Level
	if err := a.store.List("levels", &all); err != nil {
		log.Printf("could not load levels cache: %v", err)
		a.levelsMu.Lock()
		a.levelsCache = nil
		a.levelIndex = map[string]int{}
		a.levelsMu.Unlock()
		return
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

func (a *App) getFirstLevel() string {
	a.levelsMu.RLock()
	if len(a.levelsCache) > 0 {
		first := a.levelsCache[0].ID
		a.levelsMu.RUnlock()
		return first
	}
	a.levelsMu.RUnlock()
	var all []Level
	if err := a.store.List("levels", &all); err != nil {
		log.Printf("could not list levels: %v", err)
		return ""
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

func (a *App) GetUser(email string) (*User, bool, error) {
	var u User
	ok, err := a.store.Get("accounts", strings.ToLower(email), &u)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, nil
	}
	return &u, true, nil
}

func (a *App) SetUser(user User) error {
	return a.store.Set("accounts", strings.ToLower(user.Email), user)
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
