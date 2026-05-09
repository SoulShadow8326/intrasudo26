package handlers

import (
	"context"
	"fmt"
	"log"
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
	rows, err := a.leaderboardEntries()
	if err != nil {
		log.Printf("could not load leaderboard: %v", err)
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not load leaderboard"})
		return
	}
	data.Leaderboard = rows
	a.render(w, "leaderboard", data)
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
	data.RedirectData = map[string]any{"url": target, "delay": data.RedirectDelay, "tone": tone, "reason": reason}
	a.render(w, "redirect", data)
}

func (a *App) leaderboardEntries() ([]LeaderboardEntry, error) {
	var rows []LeaderboardEntry
	lbRows, err := a.store.ListLeaderboard(context.Background())
	if err != nil {
		return nil, fmt.Errorf("could not list leaderboard: %w", err)
	}
	for _, r := range lbRows {
		rows = append(rows, LeaderboardEntry{Email: r.Email, Name: r.Name, Level: r.Level, Time: r.Time})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Level == rows[j].Level {
			return rows[i].Time < rows[j].Time
		}
		return rows[i].Level > rows[j].Level
	})
	return rows, nil
}
