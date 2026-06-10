package routes

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"intrasudo26/handlers"
)

var fingerprintedAssetPattern = regexp.MustCompile(`\.[a-f0-9]{8,}\.`)

func Register(app *handlers.App) http.Handler {
	mux := http.NewServeMux()

	fileServer := http.FileServer(http.Dir("."))
	mux.Handle("/assets/", cacheStaticAssets(fileServer))
	mux.Handle("/components/css/", cacheStaticAssets(fileServer))
	mux.Handle("/components/js/", cacheStaticAssets(fileServer))

	mux.HandleFunc("/", app.Landing)
	mux.HandleFunc("/auth", app.AuthPage)
	mux.HandleFunc("/logout", app.Logout)
	mux.HandleFunc("/leaderboard", app.LeaderboardPage)
	mux.HandleFunc("/play", app.PlayPage)
	mux.HandleFunc("/timegate", app.TimegatePage)
	mux.HandleFunc("/admin", app.AdminPage)

	mux.HandleFunc("/send_otp", app.SendOTP)

	mux.HandleFunc("/api/auth", app.AuthAPI)
	mux.HandleFunc("/api/leaderboard", app.LeaderboardAPI)
	mux.HandleFunc("/api/announcements", app.AnnouncementsAPI)
	mux.HandleFunc("/api/chats", app.Chats)
	mux.HandleFunc("/api/chats/checksum", app.ChatChecksum)
	mux.HandleFunc("/api/me", app.Me)
	mux.HandleFunc("/api/messages", app.SubmitMessage)
	mux.HandleFunc("/api/submit", app.SubmitAnswer)
	mux.HandleFunc("/api/admin/levels", app.UpsertLevel)
	mux.HandleFunc("/api/admin/levels/", app.DeleteLevel)
	mux.HandleFunc("/api/admin/announcements", app.UpsertAnnouncement)
	mux.HandleFunc("/api/admin/announcements/", app.DeleteAnnouncement)

	mux.HandleFunc("/bot/get", app.BotGet)
	mux.HandleFunc("/bot/set", app.BotSet)
	mux.HandleFunc("/bot/delete", app.BotDelete)
	mux.HandleFunc("/bot/levels/count", app.BotLevelsCount)
	mux.HandleFunc("/bot/audit", app.BotAudit)
	mux.HandleFunc("/send_message", app.ExternalSendMessage)

	return metricsMiddleware(app, checkCSRF(withNotFound(mux, app)))
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func metricsMiddleware(app *handlers.App, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(recorder, r)
		duration := time.Since(start)
		app.TrackRequest(r.URL.Path, r.Method, recorder.status, duration)
	})
}

func checkCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/bot/") {
			next.ServeHTTP(w, r)
			return
		}
		if r.Method == "GET" {
			if _, err := r.Cookie("csrf"); err != nil {
				b := make([]byte, 16)
				if _, err := rand.Read(b); err != nil {
					log.Printf("could not generate csrf token: %v", err)
					http.Error(w, "internal server error", http.StatusInternalServerError)
					return
				}
				token := hex.EncodeToString(b)
				http.SetCookie(w, &http.Cookie{
					Name:     "csrf",
					Value:    token,
					Path:     "/",
					HttpOnly: false,
					Secure:   r.TLS != nil,
					SameSite: http.SameSiteLaxMode,
				})
			}
		}

		if r.Method == "POST" || r.Method == "DELETE" {
			origin := r.Header.Get("Origin")
			referer := r.Header.Get("Referer")

			if origin == "" && referer == "" {
				log.Printf("csrf: missing origin or referer for %s %s", r.Method, r.URL.Path)
				http.Error(w, "Missing Origin or Referer", http.StatusForbidden)
				return
			}

			if origin != "" {
				u, err := url.Parse(origin)
				if err != nil {
					http.Error(w, "Invalid origin", http.StatusForbidden)
					return
				}
				if u.Host != r.Host {
					http.Error(w, "Invalid origin", http.StatusForbidden)
					return
				}
			} else {
				u, err := url.Parse(referer)
				if err != nil {
					http.Error(w, "Invalid referer", http.StatusForbidden)
					return
				}
				if u.Host != r.Host {
					http.Error(w, "Invalid referer", http.StatusForbidden)
					return
				}
			}

			c, err := r.Cookie("csrf")
			if err != nil {
				log.Printf("csrf: missing csrf cookie for %s %s", r.Method, r.URL.Path)
				http.Error(w, "Missing CSRF cookie", http.StatusForbidden)
				return
			}
			token := r.Header.Get("X-CSRF-Token")
			if token == "" {
				token = r.Header.Get("x-csrf-token")
			}
			if token == "" {
				log.Printf("csrf: missing csrf token for %s %s headers=%v", r.Method, r.URL.Path, r.Header)
				http.Error(w, "Missing CSRF token", http.StatusForbidden)
				return
			}
			if c.Value != token {
				log.Printf("csrf: invalid csrf token for %s %s cookie=%s header=%s", r.Method, r.URL.Path, c.Value, token)
				http.Error(w, "Invalid CSRF token", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func withNotFound(next *http.ServeMux, app *handlers.App) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, pattern := next.Handler(r)
		if pattern == "" {
			app.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func cacheStaticAssets(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ext := strings.ToLower(filepath.Ext(r.URL.Path))
		cacheControl := staticCacheControl(r.URL.Path, ext)
		if cacheControl != "" {
			w.Header().Set("Cache-Control", cacheControl)
		}
		next.ServeHTTP(w, r)
	})
}

func staticCacheControl(pathValue, ext string) string {
	if fingerprintedAssetPattern.MatchString(strings.ToLower(pathValue)) {
		return "public, max-age=31536000, immutable"
	}

	switch ext {
	case ".mp4", ".webm", ".mov", ".m4v", ".mp3", ".m4a", ".wav", ".ogg", ".aac", ".flac", ".opus", ".ttf", ".otf", ".woff", ".woff2", ".eot", ".jpg", ".jpeg", ".png", ".gif", ".svg", ".webp", ".ico", ".avif":
		return "public, max-age=2592000"
	case ".js", ".mjs", ".css":
		return "public, max-age=3600, must-revalidate"
	default:
		return ""
	}
}
