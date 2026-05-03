package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var EmailPattern = regexp.MustCompile(`^[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)?@dpsrkp\.net$`)

func (a *App) AuthPage(w http.ResponseWriter, r *http.Request) {
	data := a.baseData(r)
	data.Title = "Intra Sudo v7.0 | Auth"
	a.render(w, "auth", data)
}

func (a *App) validateEmail(email string) bool {
	return EmailPattern.MatchString(email) || a.isAdmin(email)
}

func (a *App) SendOTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	if !a.validateEmail(email) {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid @dpsrkp.net email"})
		return
	}
	rateOK, err := a.checkOTPRateLimit(email)
	if err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not check rate limit"})
		return
	}
	if !rateOK {
		a.writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "rate limit exceeded"})
		return
	}

	var existing OTPRecord
	ok, err := a.store.Get("otp", email, &existing)
	if err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not check otp"})
		return
	}
	if ok && time.Now().Unix() <= existing.ExpiresAt {
		a.writeJSON(w, http.StatusOK, map[string]any{"success": true})
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
	if err := a.recordOTPSend(email); err != nil {
		log.Printf("could not record otp send: %v", err)
	}

	a.writeJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (a *App) checkOTPRateLimit(email string) (bool, error) {
	now := time.Now()
	// key stores a JSON object of send timestamps as []int64
	var sends []int64
	_, err := a.store.Get("otp_rate", email, &sends)
	if err != nil {
		return false, err
	}
	// prune old entries beyond 24h
	var pruned []int64
	dayAgo := now.Add(-24 * time.Hour).Unix()
	tenMinAgo := now.Add(-10 * time.Minute).Unix()
	for _, ts := range sends {
		if ts >= dayAgo {
			pruned = append(pruned, ts)
		}
	}
	// count recent
	recent := 0
	for _, ts := range pruned {
		if ts >= tenMinAgo {
			recent++
		}
	}
	if recent >= 3 {
		return false, nil
	}
	if len(pruned) >= 10 {
		return false, nil
	}
	return true, nil
}

func (a *App) recordOTPSend(email string) error {
	now := time.Now().Unix()
	var sends []int64
	_, err := a.store.Get("otp_rate", email, &sends)
	if err != nil {
		return err
	}
	sends = append(sends, now)
	// persist pruned sends
	dayAgo := time.Now().Add(-24 * time.Hour).Unix()
	var pruned []int64
	for _, ts := range sends {
		if ts >= dayAgo {
			pruned = append(pruned, ts)
		}
	}
	return a.store.Set("otp_rate", email, pruned)
}

func (a *App) AuthAPI(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		a.writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request"})
		return
	}

	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	if !a.validateEmail(email) {
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
		if err := a.store.Delete("otp", email); err != nil {
			log.Printf("could not delete otp: %v", err)
		}
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

	firstLevel := a.getFirstLevel()
	user = User{
		Name:      cases.Title(language.Und).String(strings.ToLower(name)),
		Email:     email,
		Level:     firstLevel,
		CreatedAt: time.Now().Unix(),
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
	if err := a.store.Delete("otp", email); err != nil {
		log.Printf("could not delete otp: %v", err)
	}
	if sid, err := CreateSession(a.store, email); err == nil {
		SetSessionCookie(w, sid)
	}
	a.setAuthCookies(w, user)
	a.writeJSON(w, http.StatusOK, map[string]any{"success": true, "redirect": "/play"})
}

func (a *App) Logout(w http.ResponseWriter, r *http.Request) {
	if err := DeleteSession(a.store, r); err != nil {
		log.Printf("could not delete session: %v", err)
	}
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
