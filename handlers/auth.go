package handlers

import (
	"bytes"
	"context"
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

	"intrasudo26/db"
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

	existing, ok, err := a.store.GetOTP(email)
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

	record := db.OTPRecord{
		Code:      pub,
		ExpiresAt: time.Now().Add(10 * time.Minute).Unix(),
	}
	if err := a.store.SetOTP(email, record); err != nil {
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
	var sends []int64
	_, err := func() (bool, error) {
		arr, ok, err := a.store.GetOTPRate(email)
		if err != nil {
			return false, err
		}
		if ok && arr != nil {
			sends = arr
		}
		return true, nil
	}()
	if err != nil {
		return false, err
	}
	var pruned []int64
	dayAgo := now.Add(-24 * time.Hour).Unix()
	tenMinAgo := now.Add(-10 * time.Minute).Unix()
	for _, ts := range sends {
		if ts >= dayAgo {
			pruned = append(pruned, ts)
		}
	}
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
	arr, ok, err := a.store.GetOTPRate(email)
	if err != nil {
		return err
	}
	if ok && arr != nil {
		sends = arr
	}
	sends = append(sends, now)
	dayAgo := time.Now().Add(-24 * time.Hour).Unix()
	var pruned []int64
	for _, ts := range sends {
		if ts >= dayAgo {
			pruned = append(pruned, ts)
		}
	}
	return a.store.SetOTPRate(email, pruned)
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
	acc, exists, err := a.store.GetAccount(context.Background(), email)
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

	record, ok, err := a.store.GetOTP(email)
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
		if err := a.store.DeleteOTP(email); err != nil {
			log.Printf("could not delete otp: %v", err)
		}
		a.setAuthCookies(w, User{Name: acc.Name, Email: acc.Email, Level: acc.Level, Levels: acc.Levels, CreatedAt: acc.CreatedAt})
		sid, err := genSessionID()
		if err == nil {
			expiresAt := time.Now().Add(24 * time.Hour).Unix()
			if err := a.store.CreateSession(sid, email, expiresAt); err == nil {
				SetSessionCookie(w, sid)
			}
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
	acc = db.Account{Email: email, Name: user.Name, Level: user.Level, Levels: user.Levels, CreatedAt: user.CreatedAt}
	if err := a.store.SetAccount(context.Background(), email, acc); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not create account"})
		return
	}
	if err := a.store.SetLeaderboard(context.Background(), email, db.LeaderboardEntry{Email: email, Name: user.Name, Level: 0, Time: time.Now().Unix()}); err != nil {
		a.writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "could not initialize leaderboard"})
		return
	}
	if err := a.store.DeleteOTP(email); err != nil {
		log.Printf("could not delete otp: %v", err)
	}
	sid, err := genSessionID()
	if err == nil {
		expiresAt := time.Now().Add(24 * time.Hour).Unix()
		if err := a.store.CreateSession(sid, email, expiresAt); err == nil {
			SetSessionCookie(w, sid)
		}
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
