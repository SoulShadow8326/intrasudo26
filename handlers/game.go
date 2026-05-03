package handlers

import (
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
	ok, _ := a.store.Get("levels", userLevelKey, &level)
	if !ok {
		level = Level{
			ID:         userLevelKey,
			Markup:     "You have completed the published set for now.",
			SourceHint: "Stay tuned for more levels.",
		}
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
    levelOK, err := a.store.Get("levels", curKey, &level)
	if err != nil || !levelOK {
		a.writeJSON(w, http.StatusNotFound, map[string]any{"error": "level not found"})
		return
	}

	logKey := strings.ToLower(user.Email)
	if err := a.store.Update("logs", logKey, func(currentRaw json.RawMessage) (any, error) {
		var current string
		if len(currentRaw) > 0 {
			if err := json.Unmarshal(currentRaw, &current); err != nil {
				log.Printf("could not unmarshal current logs: %v", err)
			}
		}
		current += time.Now().Format("2006-01-02 15:04:05") + " : " + answer + "\n"
		if len(current) > 10_240 {
			current = current[len(current)-10_240:]
		}
		return current, nil
	}); err != nil {
		log.Printf("could not update logs: %v", err)
	}

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
	if err := a.store.Set("accounts", strings.ToLower(user.Email), user); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not update account"})
		return
	}
	pos, ok := a.LevelPosition(nextKey)
	if err := a.store.Set("leaderboard", strings.ToLower(user.Email), LeaderboardEntry{
		Email: user.Email,
		Name:  user.Name,
		Level: func() int {
			if ok {
				return pos
			}
			return 0
		}(),
		Time: time.Now().Unix(),
	}); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not update leaderboard"})
		return
	}

	for _, msg := range a.userMessages(user.Email) {
		if err := a.store.Delete("messages", user.Email+"::"+msg.ID); err != nil {
			log.Printf("could not delete message: %v", err)
		}
	}
	if err := a.store.Set("meta", "messages_updated", time.Now().UnixMilli()); err != nil {
		log.Printf("could not update messages meta: %v", err)
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

	logKey := strings.ToLower(user.Email)
	if err := a.store.Update("logs", logKey, func(currentRaw json.RawMessage) (any, error) {
		var current string
		if len(currentRaw) > 0 {
			if err := json.Unmarshal(currentRaw, &current); err != nil {
				log.Printf("could not unmarshal current logs: %v", err)
			}
		}
		current += time.Now().Format("2006-01-02 15:04:05") + " : " + content + "\n"
		if len(current) > 10_240 {
			current = current[len(current)-10_240:]
		}
		return current, nil
	}); err != nil {
		log.Printf("could not update logs: %v", err)
	}

	if err := a.store.Set("meta", "messages_updated", time.Now().UnixMilli()); err != nil {
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
            if _, err := http.Get(u); err != nil {
                log.Printf("could not notify bot: %v", err)
            }
        }(user.Level, user.Name, user.Email, content, "cryptic")
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
        if _, err := a.store.Get("status", levelID, &status); err != nil {
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
	if err := a.store.ListByPrefix("messages", strings.ToLower(email)+"::", &rows); err != nil {
		log.Printf("could not list messages: %v", err)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Time < rows[j].Time })
	return rows
}

func (a *App) levelHints(levelID string) []ChatMessage {
	var rows []ChatMessage
	if err := a.store.ListByPrefix("hints", levelID+"::", &rows); err != nil {
		log.Printf("could not list hints: %v", err)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Time < rows[j].Time })
	return rows
}

func (a *App) metaInt(key string) int64 {
	var v int64
	ok, err := a.store.Get("meta", key, &v)
	if err != nil || !ok {
		return 0
	}
	return v
}
