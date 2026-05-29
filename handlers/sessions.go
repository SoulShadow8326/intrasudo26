package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
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
	if err := store.CreateSession(sid, email, expiresAt); err != nil {
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
		Secure:   cookiesSecure(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400 * 30,
	}
	http.SetCookie(w, cookie)
}

func GetEmailFromRequest(store *db.Store, r *http.Request) (string, bool) {
	c, err := r.Cookie("session_id")
	if err != nil {
		return "", false
	}
	rec, ok, err := store.GetSession(c.Value)
	if err != nil || !ok {
		return "", false
	}
	if time.Now().Unix() > rec.ExpiresAt {
		_ = store.DeleteSession(c.Value)
		return "", false
	}
	return rec.Email, true
}

func DeleteSession(store *db.Store, r *http.Request) error {
	c, err := r.Cookie("session_id")
	if err != nil {
		return nil
	}
	return store.DeleteSession(c.Value)
}

func ClearSessionCookie(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   cookiesSecure(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
	http.SetCookie(w, cookie)
}

func cookiesSecure() bool {
	if os.Getenv("INSECURE_COOKIES") != "" {
		return false
	}
	return true
}
