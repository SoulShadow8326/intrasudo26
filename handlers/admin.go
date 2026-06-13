package handlers

import (
	"crypto/sha256"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"intrasudo26/db"
)

func (a *App) AdminPage(w http.ResponseWriter, r *http.Request) {
	data := a.baseData(r)
	if !data.LoggedIn || !data.IsAdmin {
		a.redirectWithToast(w, r, "/", "Admin access is required for this page.", "error")
		return
	}
	data.Title = "Intra Sudo v7.0 | Admin"
	data.Levels = a.ListLevels()
	a.render(w, "admin", data)
}

func (a *App) UpsertLevel(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok || !a.isAdmin(user.Email) {
		a.writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "admin access required"})
		return
	}
	if err := r.ParseForm(); err != nil {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}

	id := strings.TrimSpace(r.FormValue("level"))
	if id == "" {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid level id"})
		return
	}
	prefix, _, ok := parseTrailingInt(id)
	if !ok || prefix == "" {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "level id must be in format <type>-<n>"})
		return
	}

	answer := normalizeAnswer(r.FormValue("answer"))
	level := Level{
		ID:         id,
		Markup:     strings.TrimSpace(r.FormValue("markup")),
		Answer:     answer,
		AnswerHash: fmt.Sprintf("%x", sha256.Sum256([]byte(a.salt+answer))),
		SourceHint: strings.TrimSpace(r.FormValue("source")),
		UpdatedAt:  time.Now().Unix(),
	}
	if level.Markup == "" || level.Answer == "" {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "markup and answer are required"})
		return
	}

	if err := a.store.SetLevel(r.Context(), db.Level{ID: level.ID, Markup: level.Markup, Answer: level.Answer, AnswerHash: level.AnswerHash, SourceHint: level.SourceHint, UpdatedAt: level.UpdatedAt}); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not save level"})
		return
	}
	a.loadLevelsCache()
	if err := a.store.SetStatus(r.Context(), level.ID, db.GameStatus{Leads: true}); err != nil {
		log.Printf("could not seed status for %s: %v", level.ID, err)
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *App) DeleteLevel(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok || !a.isAdmin(user.Email) {
		a.writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "admin access required"})
		return
	}

	id := ""
	if strings.HasPrefix(r.URL.Path, "/api/admin/levels/") {
		id = strings.TrimPrefix(r.URL.Path, "/api/admin/levels/")
	}
	if id == "" {
		id = strings.TrimSpace(r.URL.Query().Get("level"))
	}
	if id == "" {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid level id"})
		return
	}

	if err := a.store.DeleteLevel(r.Context(), id); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not delete level"})
		return
	}
	a.loadLevelsCache()
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *App) UpsertAnnouncement(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok || !a.isAdmin(user.Email) {
		a.writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "admin access required"})
		return
	}
	if err := r.ParseForm(); err != nil {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}

	id := strings.TrimSpace(r.FormValue("id"))
	content := strings.TrimSpace(r.FormValue("content"))
	if content == "" {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "content is required"})
		return
	}
	if id == "" {
		id = strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	ann := Announcement{
		ID:      id,
		Content: content,
		Time:    time.Now().Unix(),
	}
	if err := a.store.SetAnnouncement(r.Context(), db.Announcement{ID: ann.ID, Content: ann.Content, Time: ann.Time}); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not save announcement"})
		return
	}
	if err := a.store.SetMeta(r.Context(), "meta:announcements_updated", time.Now().UnixMilli()); err != nil {
		log.Printf("could not update announcements meta: %v", err)
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *App) DeleteAnnouncement(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok || !a.isAdmin(user.Email) {
		a.writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "admin access required"})
		return
	}

	id := ""
	if strings.HasPrefix(r.URL.Path, "/api/admin/announcements/") {
		id = strings.TrimPrefix(r.URL.Path, "/api/admin/announcements/")
	}
	if id == "" {
		id = strings.TrimSpace(r.FormValue("id"))
	}
	if id == "" {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid announcement id"})
		return
	}

	if err := a.store.DeleteAnnouncement(r.Context(), id); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not delete announcement"})
		return
	}
	if err := a.store.SetMeta(r.Context(), "meta:announcements_updated", time.Now().UnixMilli()); err != nil {
		log.Printf("could not update announcements meta: %v", err)
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
