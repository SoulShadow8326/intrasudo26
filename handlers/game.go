package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	htmltmpl "html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (a *App) PlayPage(w http.ResponseWriter, r *http.Request) {
	data := a.baseData(r)
	if !data.LoggedIn {
		a.redirectWithToast(w, r, "/auth", "Please sign in before entering the hunt.", "error")
		return
	}
	if !data.IsAdmin && !a.duringEvent() {
		a.redirectWithToast(w, r, "/", "The event is not currently active for players.", "error")
		return
	}

	var level Level
	userLevelKey := data.User.Level
	if userLevelKey == "" {
		var all []Level
		_ = a.store.List("levels", &all)
		if len(all) > 0 {
			sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
			userLevelKey = all[0].ID
		}
	}
	ok, _ := a.store.Get("levels", levelKey(userLevelKey), &level)
	if !ok {
		level = Level{
			ID:         userLevelKey,
			Markup:     "You have completed the published set for now.",
			SourceHint: "Stay tuned for more levels.",
		}
	}
	data.Title = "Intra Sudo v7.0 | Play"
	data.Level = level
	data.LevelDisplay = strings.TrimPrefix(level.ID, "cryptic-")
	if level.SourceHint != "" {
		data.SrcHint = htmltmpl.HTML("<!--" + level.SourceHint + "-->")
	} else {
		data.SrcHint = ""
	}
	data.Messages = a.userMessages(data.User.Email)
	data.Hints = a.levelHints(level.ID)
	data.ShowAnnouncements = true
	a.render(w, "play", data)
}

func (a *App) SubmitAnswer(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		a.writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "not logged in"})
		return
	}
	if !a.isAdmin(user.Email) && !a.duringEvent() {
		a.writeJSON(w, http.StatusForbidden, map[string]any{"error": "event is not currently active"})
		return
	}
	if err := r.ParseForm(); err != nil {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}

	answer := normalizeAnswer(r.FormValue("answer"))
	if answer == "" {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "answer is required"})
		return
	}

	var level Level
	curKey := user.Level
	if curKey == "" {
		var all []Level
		_ = a.store.List("levels", &all)
		if len(all) > 0 {
			sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
			curKey = all[0].ID
		}
	}
	levelOK, err := a.store.Get("levels", levelKey(curKey), &level)
	if err != nil || !levelOK {
		a.writeJSON(w, http.StatusNotFound, map[string]any{"error": "level not found"})
		return
	}

	logKey := strings.ToLower(user.Email)
	_ = a.store.Update("logs", logKey, func(currentRaw json.RawMessage) (any, error) {
		var current string
		if len(currentRaw) > 0 {
			_ = jsonUnmarshal(currentRaw, &current)
		}
		current += time.Now().Format("2006-01-02 15:04:05") + " : " + answer + "\n"
		if len(current) > 10_240 {
			current = current[len(current)-10_240:]
		}
		return current, nil
	})

	if normalizeAnswer(level.Answer) != answer {
		a.writeJSON(w, http.StatusOK, map[string]any{"success": false})
		return
	}

	var all []Level
	_ = a.store.List("levels", &all)
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	nextKey := curKey
	for i := range all {
		if all[i].ID == curKey {
			if i+1 < len(all) {
				nextKey = all[i+1].ID
			}
			break
		}
	}

	user.Level = nextKey
	if err := a.store.Set("accounts", strings.ToLower(user.Email), user); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not update account"})
		return
	}
	pos := 0
	for i := range all {
		if all[i].ID == nextKey {
			pos = i
			break
		}
	}
	if err := a.store.Set("leaderboard", strings.ToLower(user.Email), LeaderboardEntry{
		Email: user.Email,
		Name:  user.Name,
		Level: pos,
		Time:  time.Now().Unix(),
	}); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not update leaderboard"})
		return
	}

	for _, msg := range a.userMessages(user.Email) {
		_ = a.store.Delete("messages", user.Email+"::"+msg.ID)
	}

	a.setAuthCookies(w, *user)
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *App) SubmitMessage(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		a.writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "not logged in"})
		return
	}
	if err := r.ParseForm(); err != nil {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}

	content := strings.TrimSpace(r.FormValue("content"))
	if content == "" {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "message is required"})
		return
	}
	if len(content) > 512 {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "message too long"})
		return
	}

	message := ChatMessage{
		ID:      strconv.FormatInt(time.Now().UnixNano(), 10),
		Author:  user.Email,
		Content: content,
		Time:    time.Now().Unix(),
		Kind:    "user",
	}

	if err := a.store.Set("messages", user.Email+"::"+message.ID, message); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not save message"})
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *App) Chats(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		a.writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "not logged in"})
		return
	}
	chats := a.userMessages(user.Email)
	hints := a.levelHints(user.Level)
	announcements := a.baseData(r).Announcements
	hash := sha256.Sum256([]byte(mustJSON(chats) + mustJSON(hints) + mustJSON(announcements)))
	checksum := hex.EncodeToString(hash[:])
	w.Header().Set("X-Chats-Checksum", checksum)
	a.writeJSON(w, http.StatusOK, map[string]any{
		"chats":         chats,
		"hints":         hints,
		"announcements": announcements,
	})
}

func (a *App) ChatChecksum(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		a.writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "not logged in"})
		return
	}
	chats := a.userMessages(user.Email)
	hints := a.levelHints(user.Level)
	announcements := a.baseData(r).Announcements
	status := GameStatus{Leads: true}
	_, _ = a.store.Get("status", "global", &status)

	hash := sha256.Sum256([]byte(mustJSON(chats) + mustJSON(hints) + mustJSON(announcements)))
	checksum := hex.EncodeToString(hash[:])
	w.Header().Set("X-Chats-Checksum", checksum)
	if client := strings.TrimSpace(r.URL.Query().Get("checksum")); client != "" && client == checksum {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{
		"checksum":      checksum,
		"chats":         chats,
		"hints":         hints,
		"announcements": announcements,
		"leads":         status.Leads,
	})
}

func (a *App) AnnouncementsAPI(w http.ResponseWriter, r *http.Request) {
	data := a.baseData(r)
	raw := mustJSON(data.Announcements)
	hash := sha256.Sum256([]byte(raw))
	checksum := hex.EncodeToString(hash[:])
	w.Header().Set("X-Announcements-Checksum", checksum)
	if client := strings.TrimSpace(r.URL.Query().Get("checksum")); client != "" && client == checksum {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"announcements": data.Announcements})
}

func (a *App) userMessages(email string) []ChatMessage {
	var rows []ChatMessage
	_ = a.store.ListByPrefix("messages", strings.ToLower(email)+"::", &rows)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Time < rows[j].Time })
	return rows
}

func (a *App) levelHints(levelID string) []ChatMessage {
	var rows []ChatMessage
	_ = a.store.ListByPrefix("hints", levelKey(levelID)+"::", &rows)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Time < rows[j].Time })
	return rows
}

func mustJSON(v any) string {
	return string(mustMarshal(v))
}

func mustMarshal(v any) []byte {
	raw, _ := jsonMarshal(v)
	return raw
}
