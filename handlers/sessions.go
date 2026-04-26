package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

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
    if err := store.Set("sessions", sid, email); err != nil {
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
    var email string
    ok, err := store.Get("sessions", c.Value, &email)
    if err != nil || !ok {
        return "", false
    }
    return email, true
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
