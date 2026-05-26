package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	htmltmpl "html/template"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"intrasudo26/db"
)

func (a *App) PlayPage(w http.ResponseWriter, r *http.Request) {
	data := a.baseData(r)
	if !data.LoggedIn {
		a.redirectWithToast(w, r, "/auth", "Please sign in before entering the hunt.", "error")
		return
	}
	if !data.IsAdmin {
		now := time.Now()
		if now.Before(a.startTime) {
			a.redirectWithToast(w, r, "/timegate", "The event has not started yet.", "error")
			return
		}
		if !a.duringEvent() {
			a.redirectWithToast(w, r, "/", "The event is not currently active for players.", "error")
			return
		}
	}

	var level Level
	qs := r.URL.Query()
	levelType := strings.TrimSpace(qs.Get("type"))
	if levelType == "" {
		levelType = "cryptic"
	}
	userLevelKey := data.User.Level
	if lvl, ok := data.User.Levels[levelType]; ok {
		userLevelKey = lvl
	}
	if userLevelKey == "" {
		userLevelKey = a.getFirstLevelForType(levelType)
	}
	lv, ok, _ := a.store.GetLevel(r.Context(), userLevelKey)
	if !ok {
		level = Level{
			ID:         userLevelKey,
			Markup:     "You have completed the published set for now.",
			SourceHint: "Stay tuned for more levels.",
		}
	} else {
		level = Level{ID: lv.ID, Markup: lv.Markup, Answer: lv.Answer, AnswerHash: lv.AnswerHash, SourceHint: lv.SourceHint, UpdatedAt: lv.UpdatedAt}
	}
	data.Title = "Intra Sudo v7.0 | Play"
	data.Level = level
	if _, n, ok := parseTrailingInt(level.ID); ok {
		data.LevelDisplay = strconv.Itoa(n)
	} else {
		data.LevelDisplay = level.ID
	}
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

	qs := r.URL.Query()
	levelType := strings.TrimSpace(qs.Get("type"))
	if levelType == "" {
		levelType = "cryptic"
	}
	var level Level
	curKey := user.Level
	if lvl, ok := user.Levels[levelType]; ok {
		curKey = lvl
	}
	if curKey == "" {
		curKey = a.getFirstLevelForType(levelType)
	}
	lvl, ok, err := a.store.GetLevel(r.Context(), curKey)
	if err != nil || !ok {
		a.writeJSON(w, http.StatusNotFound, map[string]any{"error": "level not found"})
		return
	}
	level.AnswerHash = lvl.AnswerHash

	a.appendToLogs(user.Email, answer)

	userHash := sha256.Sum256([]byte(answer))
	userHashHex := hex.EncodeToString(userHash[:])
	targetHash := level.AnswerHash
	if targetHash == "" {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "level missing answer_hash"})
		return
	}
	if userHashHex != targetHash {
		a.writeJSON(w, http.StatusOK, map[string]any{"success": false})
		return
	}

	nextKey := a.NextLevelForType(curKey, levelType)

	if user.Levels == nil {
		user.Levels = map[string]string{}
	}
	user.Levels[levelType] = nextKey
	acc := db.Account{Email: strings.ToLower(user.Email), Name: user.Name, Level: user.Level, Levels: user.Levels, CreatedAt: user.CreatedAt}
	if err := a.store.SetAccount(r.Context(), strings.ToLower(user.Email), acc); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not update account"})
		return
	}
	pos, ok := a.LevelPosition(nextKey)
	lbEntry := db.LeaderboardEntry{
		Email: user.Email,
		Name:  user.Name,
		Level: 0,
		Time:  time.Now().Unix(),
	}
	if ok {
		lbEntry.Level = pos
	}
	if err := a.store.SetLeaderboard(r.Context(), strings.ToLower(user.Email), lbEntry); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not update leaderboard"})
		return
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
	levelType := strings.TrimSpace(r.FormValue("type"))
	if levelType == "" {
		levelType = "cryptic"
	}
	levelID := user.Level
	if lvl, ok := user.Levels[levelType]; ok {
		levelID = lvl
	}
	if levelID == "" {
		levelID = a.getFirstLevelForType(levelType)
	}

	message := ChatMessage{
		ID:      strconv.FormatInt(time.Now().UnixNano(), 10),
		Author:  user.Email,
		Content: content,
		Time:    time.Now().Unix(),
		Kind:    "user",
	}

	payload, _ := json.Marshal(message)
	if err := a.store.SetMessage(r.Context(), message.ID, strings.ToLower(user.Email), payload, message.Time); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not save message"})
		return
	}

	a.appendToLogs(user.Email, content)

	if err := a.store.SetMeta(r.Context(), "messages_updated", time.Now().UnixMilli()); err != nil {
		log.Printf("could not update messages meta: %v", err)
	}

	go func(level, name, email, content, levelType string) {
		botURL := os.Getenv("BOT_API_URL")
		if botURL == "" {
			botURL = "http://localhost:5555/send_message"
		}
		vals := url.Values{}
		vals.Set("level", level)
		vals.Set("type", levelType)
		vals.Set("name", name)
		vals.Set("email", email)
		vals.Set("content", content)
		u := botURL + "?" + vals.Encode()
		client := &http.Client{Timeout: 5 * time.Second}
		if _, err := client.Get(u); err != nil {
			log.Printf("could not notify bot: %v", err)
		}
	}(levelID, user.Name, user.Email, content, levelType)
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *App) Chats(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		a.writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "not logged in"})
		return
	}
	qs := r.URL.Query()
	levelType := strings.TrimSpace(qs.Get("type"))
	if levelType == "" {
		levelType = "cryptic"
	}
	levelID := user.Level
	if lvl, ok := user.Levels[levelType]; ok {
		levelID = lvl
	}
	if levelID == "" {
		levelID = a.getFirstLevelForType(levelType)
	}
	chats := a.userMessages(user.Email)
	hints := a.levelHints(levelID)
	announcements := a.baseData(r).Announcements
	messagesRev := a.metaInt("messages_updated")
	announcementsRev := a.metaInt("announcements_updated")
	combined := fmt.Sprintf("%d:%d", messagesRev, announcementsRev)
	hash := sha256.Sum256([]byte(combined))
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
	messagesRev := a.metaInt("messages_updated")
	announcementsRev := a.metaInt("announcements_updated")
	combined := fmt.Sprintf("%d:%d", messagesRev, announcementsRev)
	hash := sha256.Sum256([]byte(combined))
	checksum := hex.EncodeToString(hash[:])
	w.Header().Set("X-Chats-Checksum", checksum)
	if client := strings.TrimSpace(r.URL.Query().Get("checksum")); client != "" && client == checksum {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	qs := r.URL.Query()
	levelType := strings.TrimSpace(qs.Get("type"))
	if levelType == "" {
		levelType = "cryptic"
	}
	levelID := user.Level
	if lvl, ok := user.Levels[levelType]; ok {
		levelID = lvl
	}
	if levelID == "" {
		levelID = a.getFirstLevelForType(levelType)
	}
	chats := a.userMessages(user.Email)
	hints := a.levelHints(levelID)
	announcements := a.baseData(r).Announcements
	status := GameStatus{Leads: true}
	if gs, ok, err := a.store.GetStatus(r.Context(), levelID); err == nil && ok {
		status = GameStatus{Leads: gs.Leads}
	} else if err != nil {
		log.Printf("could not get status: %v", err)
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
	announcementsRev := a.metaInt("announcements_updated")
	combined := fmt.Sprintf("%d", announcementsRev)
	hash := sha256.Sum256([]byte(combined))
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
	raw, err := a.store.ListMessagesForOwner(context.Background(), strings.ToLower(email))
	if err != nil {
		log.Printf("could not list messages: %v", err)
		return rows
	}
	for _, r := range raw {
		var m ChatMessage
		if err := json.Unmarshal(r, &m); err != nil {
			log.Printf("could not unmarshal message: %v", err)
			continue
		}
		rows = append(rows, m)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Time < rows[j].Time })
	return rows
}

func (a *App) levelHints(levelID string) []ChatMessage {
	var rows []ChatMessage
	raw, err := a.store.ListHintsForLevel(context.Background(), levelID)
	if err != nil {
		log.Printf("could not list hints: %v", err)
		return rows
	}
	for _, r := range raw {
		var m ChatMessage
		if err := json.Unmarshal(r, &m); err != nil {
			log.Printf("could not unmarshal hint: %v", err)
			continue
		}
		rows = append(rows, m)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Time < rows[j].Time })
	return rows
}

func (a *App) metaInt(key string) int64 {
	val, ok, err := a.store.GetMetaInt(context.Background(), key)
	if err != nil || !ok {
		return 0
	}
	return val
}
