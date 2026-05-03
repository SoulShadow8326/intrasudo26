package handlers

import (
    "crypto/sha256"
    "fmt"
    "log"
    "net/http"
    "strconv"
    "strings"
    "time"
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
        AnswerHash: fmt.Sprintf("%x", sha256.Sum256([]byte(answer))),
        SourceHint: strings.TrimSpace(r.FormValue("source")),
        UpdatedAt:  time.Now().Unix(),
    }
	if level.Markup == "" || level.Answer == "" {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "markup and answer are required"})
		return
	}

    if err := a.store.Set("levels", level.ID, level); err != nil {
        a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not save level"})
        return
    }
    a.loadLevelsCache()
    if err := a.store.Set("status", level.ID, GameStatus{Leads: true}); err != nil {
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

	if err := a.store.Delete("levels", id); err != nil {
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
	if err := a.store.Set("announcements", id, ann); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not save announcement"})
		return
	}
	if err := a.store.Set("meta", "announcements_updated", time.Now().UnixMilli()); err != nil {
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

	if err := a.store.Delete("announcements", id); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not delete announcement"})
		return
	}
	if err := a.store.Set("meta", "announcements_updated", time.Now().UnixMilli()); err != nil {
		log.Printf("could not update announcements meta: %v", err)
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true})
}
