package routes

import (
	"net/http"

	"intrasudo26/handlers"
)

func Register(app *handlers.App) http.Handler {
	mux := http.NewServeMux()

	fileServer := http.FileServer(http.Dir("."))
	mux.Handle("/assets/", fileServer)
	mux.Handle("/components/css/", fileServer)
	mux.Handle("/components/js/", fileServer)

	type methodMap map[string]http.HandlerFunc
	routes := map[string]methodMap{}

	add := func(method, path string, h http.HandlerFunc) {
		if _, ok := routes[path]; !ok {
			routes[path] = methodMap{}
		}
		routes[path][method] = h
	}

	add("GET", "/", app.Landing)
	add("GET", "/auth", app.AuthPage)
	add("GET", "/logout", app.Logout)
	add("GET", "/leaderboard", app.LeaderboardPage)
	add("GET", "/play", app.PlayPage)
	add("GET", "/admin", app.AdminPage)
	add("GET", "/redirect", app.RedirectPage)

	add("POST", "/send_otp", app.SendOTP)
	add("GET", "/send_otp", app.SendOTP)
	add("POST", "/api/auth", app.AuthAPI)
	add("GET", "/api/auth", app.AuthAPI)
	add("GET", "/api/announcements", app.AnnouncementsAPI)
	add("GET", "/api/chats", app.Chats)
	add("GET", "/api/chats/checksum", app.ChatChecksum)
	add("GET", "/api/me", app.Me)
	add("POST", "/api/messages", app.SubmitMessage)
	add("POST", "/api/submit", app.SubmitAnswer)
	add("POST", "/api/admin/levels", app.UpsertLevel)
	add("DELETE", "/api/admin/levels/", app.DeleteLevel)
	add("POST", "/api/admin/announcements", app.UpsertAnnouncement)
	add("DELETE", "/api/admin/announcements/", app.DeleteAnnouncement)

	add("GET", "/announcements", app.AnnouncementsAPI)
	add("GET", "/chats", app.Chats)
	add("GET", "/chats_checksum", app.ChatChecksum)
	add("POST", "/submit_message", app.SubmitMessage)
	add("GET", "/submit_message", app.SubmitMessage)
	add("POST", "/submit", app.SubmitAnswer)
	add("GET", "/submit", app.SubmitAnswer)
	add("POST", "/set_level", app.UpsertLevel)
	add("GET", "/set_level", app.UpsertLevel)
	add("DELETE", "/delete_level", app.DeleteLevel)
	add("GET", "/delete_level", app.DeleteLevel)

	for path, methods := range routes {
		m := methods
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if h, ok := m[r.Method]; ok {
				h(w, r)
				return
			}
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		})
	}
	return withNotFound(mux, app)
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
