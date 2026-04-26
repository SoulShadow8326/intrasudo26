package handlers

import (
	"net/http"
	"sort"
)

func (a *App) Landing(w http.ResponseWriter, r *http.Request) {
	data := a.baseData(r)
	data.Title = "Intra Sudo v7.0"
	a.render(w, "landing", data)
}


func (a *App) LeaderboardPage(w http.ResponseWriter, r *http.Request) {
	data := a.baseData(r)
	data.Title = "Intra Sudo v7.0 | Leaderboard"
	data.Leaderboard = a.leaderboardEntries()
	a.render(w, "leaderboard", data)
}

func (a *App) RedirectPage(w http.ResponseWriter, r *http.Request) {
	a.redirectWithToast(w, r, "/", "Redirecting you back home.", "success")
}

func (a *App) NotFound(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotFound)
	data := a.baseData(r)
	data.Title = "Intra Sudo v7.0 | Not Found"
	a.render(w, "not-found", data)
}

func (a *App) redirectWithToast(w http.ResponseWriter, r *http.Request, target, reason, tone string) {
	data := a.baseData(r)
	data.Title = "Intra Sudo v7.0 | Redirecting"
	data.RedirectURL = target
	data.RedirectDelay = 1400
	data.RedirectTone = tone
	data.RedirectReason = reason
	a.render(w, "redirect", data)
}

func (a *App) leaderboardEntries() []LeaderboardEntry {
	var rows []LeaderboardEntry
	_ = a.store.List("leaderboard", &rows)

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Level == rows[j].Level {
			return rows[i].Time < rows[j].Time
		}
		return rows[i].Level > rows[j].Level
	})
	return rows
}
