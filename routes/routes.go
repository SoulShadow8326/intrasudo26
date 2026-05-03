package routes

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"net/url"

	"intrasudo26/handlers"
)

func Register(app *handlers.App) http.Handler {
	mux := http.NewServeMux()

	fileServer := http.FileServer(http.Dir("."))
	mux.Handle("/assets/", fileServer)
	mux.Handle("/components/css/", fileServer)
	mux.Handle("/components/js/", fileServer)

	mux.HandleFunc("/", app.Landing)
	mux.HandleFunc("/auth", app.AuthPage)
	mux.HandleFunc("/logout", app.Logout)
	mux.HandleFunc("/leaderboard", app.LeaderboardPage)
	mux.HandleFunc("/play", app.PlayPage)
	mux.HandleFunc("/admin", app.AdminPage)

	mux.HandleFunc("/send_otp", app.SendOTP)

	mux.HandleFunc("/api/auth", app.AuthAPI)
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
	mux.HandleFunc("/send_message", app.ExternalSendMessage)

	return checkCSRF(withNotFound(mux, app))
}

func checkCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
