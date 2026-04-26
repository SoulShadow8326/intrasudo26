package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

var EmailPattern = regexp.MustCompile(`^[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)?@dpsrkp\.net$`)

func (a *App) AuthPage(w http.ResponseWriter, r *http.Request) {
	data := a.baseData(r)
	data.Title = "Intra Sudo v7.0 | Auth"
	a.render(w, "auth", data)
}

func (a *App) SendOTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	if !EmailPattern.MatchString(email) && !a.isAdmin(email) {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid @dpsrkp.net email"})
		return
	}

	code := a.otpCode(email)

	hash := sha256.Sum256([]byte(a.salt + code))
	pub := hex.EncodeToString(hash[:])

	record := OTPRecord{
		Code:      pub,
		ExpiresAt: time.Now().Add(10 * time.Minute).Unix(),
	}
	if err := a.store.Set("otp", email, record); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not save otp"})
		return
	}

	tmpl, err := template.ParseFiles("otp.html")
	if err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not render otp template"})
		return
	}

	digits := make([]string, 6)
	for i := 0; i < 6; i++ {
		if i < len(code) {
			digits[i] = string(code[i])
		} else {
			digits[i] = ""
		}
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any{"Digits": digits}); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not execute otp template"})
		return
	}

	if err := a.mailer.Send(email, "Intra Sudo v7.0 OTP Verification", buf.String()); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not send otp"})
		return
	}

	a.writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *App) AuthAPI(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	if !EmailPattern.MatchString(email) && !a.isAdmin(email) {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid email"})
		return
	}

	var user User
	exists, err := a.store.Get("accounts", email, &user)
	if err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not load account"})
		return
	}

	otp := strings.TrimSpace(r.FormValue("otp"))
	name := strings.TrimSpace(r.FormValue("name"))
	if otp == "" {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "otp required"})
		return
	}

	var record OTPRecord
	ok, err := a.store.Get("otp", email, &record)
	if err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not verify otp"})
		return
	}
	if !ok || time.Now().Unix() > record.ExpiresAt {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid or expired otp"})
		return
	}

	providedHashArr := sha256.Sum256([]byte(a.salt + otp))
	providedHash := hex.EncodeToString(providedHashArr[:])
	if providedHash != record.Code {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid or expired otp"})
		return
	}

	if exists {
		if user.PasswordHash == "" {
			tokenArr := sha256.Sum256([]byte(a.salt + strings.ToLower(user.Email)))
			user.PasswordHash = hex.EncodeToString(tokenArr[:])
			_ = a.store.Set("accounts", email, user)
		}
		_ = a.store.Delete("otp", email)
		a.setAuthCookies(w, user)
		sid, err := CreateSession(a.store, email)
		if err == nil {
			SetSessionCookie(w, sid)
		}
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "redirect": "/play"})
		return
	}

	if name == "" {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name required for new account"})
		return
	}

	tokenArr := sha256.Sum256([]byte(a.salt + strings.ToLower(email)))
	token := hex.EncodeToString(tokenArr[:])
	firstLevel := ""
	var all []Level
	_ = a.store.List("levels", &all)
	if len(all) > 0 {
		sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
		firstLevel = all[0].ID
	}
	user = User{
		Name:         strings.Title(strings.ToLower(name)),
		Email:        email,
		PasswordHash: token,
		Level:        firstLevel,
		CreatedAt:    time.Now().Unix(),
	}
	if err := a.store.Set("accounts", email, user); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not create account"})
		return
	}
	if err := a.store.Set("leaderboard", email, LeaderboardEntry{
		Email: email,
		Name:  user.Name,
		Level: 0,
		Time:  time.Now().Unix(),
	}); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not initialize leaderboard"})
		return
	}
	_ = a.store.Delete("otp", email)
	if sid, err := CreateSession(a.store, email); err == nil {
		SetSessionCookie(w, sid)
	}
	a.setAuthCookies(w, user)
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "redirect": "/play"})
}

func (a *App) Logout(w http.ResponseWriter, r *http.Request) {
	_ = DeleteSession(a.store, r)
	ClearSessionCookie(w)
	a.clearAuthCookies(w)
	a.redirectWithToast(w, r, "/", "You have been logged out.", "success")
}

func (a *App) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := a.currentUser(r)
	if !ok {
		a.writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "not logged in"})
		return
	}
	a.writeJSON(w, http.StatusOK, map[string]any{"email": user.Email, "name": user.Name, "is_admin": a.isAdmin(user.Email)})
}
