package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"time"

	"intrasudo26/db"
)

func genSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func CreateSession(store *db.Store, email string) (string, error) {
	sid, err := genSessionID()
	if err != nil {
		return "", err
	}
	expiresAt := time.Now().Add(24 * time.Hour).Unix()
	rec := struct {
		Email     string `json:"email"`
		ExpiresAt int64  `json:"expires_at"`
	}{
		Email:     email,
		ExpiresAt: expiresAt,
	}
	if err := store.Set("sessions", sid, rec); err != nil {
		return "", err
	}
	return sid, nil
}

func SetSessionCookie(w http.ResponseWriter, sid string) {
	cookie := &http.Cookie{
		Name:     "session_id",
		Value:    sid,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	}
	http.SetCookie(w, cookie)
}

func GetEmailFromRequest(store *db.Store, r *http.Request) (string, bool) {
	c, err := r.Cookie("session_id")
	if err != nil {
		return "", false
	}
	var rec struct {
		Email     string `json:"email"`
		ExpiresAt int64  `json:"expires_at"`
	}
	ok, err := store.Get("sessions", c.Value, &rec)
	if err != nil || !ok {
		return "", false
	}
	if time.Now().Unix() > rec.ExpiresAt {
		_ = store.Delete("sessions", c.Value)
		return "", false
	}
	return rec.Email, true
}

func DeleteSession(store *db.Store, r *http.Request) error {
	c, err := r.Cookie("session_id")
	if err != nil {
		return nil
	}
	return store.Delete("sessions", c.Value)
}

func ClearSessionCookie(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
	http.SetCookie(w, cookie)
}
