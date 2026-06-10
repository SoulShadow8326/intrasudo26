package handlers

import (
	"log"
	"net/http"
	"strconv"
	"strings"
)

func (a *App) Landing(w http.ResponseWriter, r *http.Request) {
	data := a.baseData(r)
	data.Title = "Intra Sudo v7.0"
	a.render(w, "landing", data)
}

func (a *App) TimegatePage(w http.ResponseWriter, r *http.Request) {
	data := a.baseData(r)
	data.Title = "Intra Sudo v7.0 | Timegate"
	a.render(w, "timegate", data)
}

func (a *App) LeaderboardPage(w http.ResponseWriter, r *http.Request) {
	data := a.baseData(r)
	data.Title = "Intra Sudo v7.0 | Leaderboard"
	a.render(w, "leaderboard", data)
}

func (a *App) LeaderboardAPI(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 20
	}
	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}
	if limit > 100 {
		limit = 100
	}
	rows, err := a.store.ListLeaderboardPaginated(r.Context(), limit, offset)
	if err != nil {
		log.Printf("could not load leaderboard: %v", err)
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not load leaderboard"})
		return
	}

	response := map[string]any{
		"rows": rows,
	}

	if email, ok := GetEmailFromRequest(a.store, r); ok && email != "" {
		rank, err := a.store.GetLeaderboardRank(r.Context(), email)
		if err == nil && rank > 0 {
			response["my_rank"] = rank
			acc, okAcc, _ := a.store.GetAccount(r.Context(), email)
			if okAcc {
				lbRows, err := a.store.ListLeaderboardPaginated(r.Context(), 1, rank-1)
				if err == nil && len(lbRows) > 0 {
					response["my_entry"] = lbRows[0]
				} else {
					response["my_entry"] = map[string]any{
						"email": email,
						"name":  acc.Name,
						"level": 0,
					}
				}
			}
		}
	}

	a.writeJSON(w, http.StatusOK, response)
}

func (a *App) NotFound(w http.ResponseWriter, r *http.Request) {
	slug := strings.TrimPrefix(r.URL.Path, "/")
	if slug != "" && !strings.Contains(slug, "/") {
		if url, ok, err := a.store.GetBacklink(r.Context(), slug); err == nil && ok {
			a.redirectWithToast(w, r, url, "Redirecting...", "success")
			return
		}
	}
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
