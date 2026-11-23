package internals

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	visitorSessionCookie = "lixr_visitor_session"
	googleStateCookie    = "lixr_google_oauth_state"
	googleNextCookie     = "lixr_google_oauth_next"
)

type VisitorIdentity struct {
	ID          int64  `json:"id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	AvatarURL   string `json:"avatar_url"`
}

type googleProfile struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Picture       string `json:"picture"`
}

func (a *App) handleVisitorSession(w http.ResponseWriter, r *http.Request) {
	visitor, ok := a.visitorSessionFromRequest(r)
	JSON(w, http.StatusOK, Envelope{
		"ok":               true,
		"authenticated":    ok,
		"visitor":          visitorOrNil(visitor, ok),
		"google_available": a.cfg.GoogleOAuthReady(),
	})
}

func visitorOrNil(visitor VisitorIdentity, ok bool) any {
	if !ok {
		return nil
	}
	return visitor
}

func (a *App) handleGoogleStart(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.GoogleOAuthReady() {
		Error(w, http.StatusServiceUnavailable, "Google sign-in is not available right now")
		return
	}
	state, err := randomURLToken(24)
	if err != nil {
		Error(w, http.StatusInternalServerError, "could not start Google sign-in")
		return
	}
	next := safeNext(r.URL.Query().Get("next"))
	if next == "" {
		next = "/#reviews"
	}
	redirectURI := a.googleRedirectURI(r)
	http.SetCookie(w, &http.Cookie{Name: googleStateCookie, Value: state, Path: "/", HttpOnly: true, Secure: a.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	http.SetCookie(w, &http.Cookie{Name: googleNextCookie, Value: base64.RawURLEncoding.EncodeToString([]byte(next)), Path: "/", HttpOnly: true, Secure: a.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: 600})

	values := url.Values{}
	values.Set("client_id", a.cfg.GoogleClientID)
	values.Set("redirect_uri", redirectURI)
	values.Set("response_type", "code")
	values.Set("scope", "openid email profile")
	values.Set("state", state)
	values.Set("prompt", "select_account")
	http.Redirect(w, r, "https://accounts.google.com/o/oauth2/v2/auth?"+values.Encode(), http.StatusFound)
}

func (a *App) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	next := "/#reviews"
	if cookie, err := r.Cookie(googleNextCookie); err == nil {
		if decoded, err := base64.RawURLEncoding.DecodeString(cookie.Value); err == nil {
			if candidate := safeNext(string(decoded)); candidate != "" {
				next = candidate
			}
		}
	}
	clearCookie(w, googleStateCookie, a.cfg.CookieSecure)
	clearCookie(w, googleNextCookie, a.cfg.CookieSecure)

	if oauthErr := strings.TrimSpace(r.URL.Query().Get("error")); oauthErr != "" {
		http.Redirect(w, r, appendQuery(next, "auth", "cancelled"), http.StatusSeeOther)
		return
	}
	stateCookie, err := r.Cookie(googleStateCookie)
	if err != nil || stateCookie.Value == "" || r.URL.Query().Get("state") != stateCookie.Value {
		http.Redirect(w, r, appendQuery(next, "auth", "failed"), http.StatusSeeOther)
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if code == "" {
		http.Redirect(w, r, appendQuery(next, "auth", "failed"), http.StatusSeeOther)
		return
	}

	profile, err := a.exchangeGoogleCode(r.Context(), code, a.googleRedirectURI(r))
	if err != nil || profile.Sub == "" || profile.Email == "" || !profile.EmailVerified {
		http.Redirect(w, r, appendQuery(next, "auth", "failed"), http.StatusSeeOther)
		return
	}
	visitor, err := a.upsertVisitor(r.Context(), profile)
	if err != nil {
		http.Redirect(w, r, appendQuery(next, "auth", "failed"), http.StatusSeeOther)
		return
	}
	if err := a.issueVisitorSession(w, r, visitor.ID); err != nil {
		http.Redirect(w, r, appendQuery(next, "auth", "failed"), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, appendQuery(next, "auth", "success"), http.StatusSeeOther)
}

func (a *App) handleVisitorLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(visitorSessionCookie); err == nil && cookie.Value != "" {
		_, _ = a.db.ExecContext(r.Context(), `DELETE FROM visitor_sessions WHERE token_hash=?`, hashSessionToken(cookie.Value))
	}
	clearCookie(w, visitorSessionCookie, a.cfg.CookieSecure)
	JSON(w, http.StatusOK, Envelope{"ok": true})
}

func (a *App) handleCreateRating(w http.ResponseWriter, r *http.Request) {
	visitor, ok := a.visitorSessionFromRequest(r)
	if !ok {
		Error(w, http.StatusUnauthorized, "Sign in with Google to leave a review")
		return
	}
	var in struct {
		Stars   int    `json:"stars"`
		Comment string `json:"comment"`
	}
	if err := decodeJSON(r, &in); err != nil {
		Error(w, http.StatusBadRequest, "Please check your review and try again")
		return
	}
	in.Comment = strings.TrimSpace(in.Comment)
	if in.Stars < 1 || in.Stars > 5 || len(in.Comment) < 4 || len(in.Comment) > 1500 {
		Error(w, http.StatusBadRequest, "Choose a rating and add a short review")
		return
	}
	_, err := a.db.ExecContext(r.Context(), `INSERT INTO ratings(visitor_id,display_name,stars,comment,visible) VALUES(?,?,?,?,1)`, visitor.ID, visitor.DisplayName, in.Stars, in.Comment)
	if err != nil {
		Error(w, http.StatusInternalServerError, "Your review could not be saved right now")
		return
	}
	JSON(w, http.StatusCreated, Envelope{"ok": true, "message": "Thanks for sharing your review."})
}

func (a *App) visitorSessionFromRequest(r *http.Request) (VisitorIdentity, bool) {
	cookie, err := r.Cookie(visitorSessionCookie)
	if err != nil || cookie.Value == "" {
		return VisitorIdentity{}, false
	}
	var visitor VisitorIdentity
	err = a.db.QueryRowContext(r.Context(), `SELECT v.id,v.display_name,v.email,v.avatar_url
		FROM visitor_sessions s JOIN visitor_accounts v ON v.id=s.visitor_id
		WHERE s.token_hash=? AND s.expires_at>? LIMIT 1`, hashSessionToken(cookie.Value), time.Now().Unix()).
		Scan(&visitor.ID, &visitor.DisplayName, &visitor.Email, &visitor.AvatarURL)
	if err != nil {
		return VisitorIdentity{}, false
	}
	_, _ = a.db.ExecContext(r.Context(), `UPDATE visitor_sessions SET last_seen_at=CURRENT_TIMESTAMP WHERE token_hash=?`, hashSessionToken(cookie.Value))
	return visitor, true
}

func (a *App) issueVisitorSession(w http.ResponseWriter, r *http.Request, visitorID int64) error {
	token, tokenHash, err := newSessionToken()
	if err != nil {
		return err
	}
	expires := time.Now().Add(30 * 24 * time.Hour)
	_, err = a.db.ExecContext(r.Context(), `DELETE FROM visitor_sessions WHERE expires_at<=?`, time.Now().Unix())
	if err != nil {
		return err
	}
	_, err = a.db.ExecContext(r.Context(), `INSERT INTO visitor_sessions(visitor_id,token_hash,expires_at) VALUES(?,?,?)`, visitorID, tokenHash, expires.Unix())
	if err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: visitorSessionCookie, Value: token, Path: "/", HttpOnly: true, Secure: a.cfg.CookieSecure, SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: int((30 * 24 * time.Hour).Seconds())})
	return nil
}

func (a *App) upsertVisitor(ctx context.Context, profile googleProfile) (VisitorIdentity, error) {
	name := strings.TrimSpace(profile.Name)
	if name == "" {
		name = strings.Split(profile.Email, "@")[0]
	}
	_, err := a.db.ExecContext(ctx, `INSERT INTO visitor_accounts(google_subject,email,display_name,avatar_url)
		VALUES(?,?,?,?)
		ON CONFLICT(google_subject) DO UPDATE SET email=excluded.email,display_name=excluded.display_name,avatar_url=excluded.avatar_url,updated_at=CURRENT_TIMESTAMP`,
		profile.Sub, strings.ToLower(strings.TrimSpace(profile.Email)), name, strings.TrimSpace(profile.Picture))
	if err != nil {
		return VisitorIdentity{}, err
	}
	var visitor VisitorIdentity
	err = a.db.QueryRowContext(ctx, `SELECT id,display_name,email,avatar_url FROM visitor_accounts WHERE google_subject=?`, profile.Sub).
		Scan(&visitor.ID, &visitor.DisplayName, &visitor.Email, &visitor.AvatarURL)
	return visitor, err
}

func (a *App) exchangeGoogleCode(ctx context.Context, code, redirectURI string) (googleProfile, error) {
	values := url.Values{}
	values.Set("code", code)
	values.Set("client_id", a.cfg.GoogleClientID)
	values.Set("client_secret", a.cfg.GoogleClientSecret)
	values.Set("redirect_uri", redirectURI)
	values.Set("grant_type", "authorization_code")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(values.Encode()))
	if err != nil {
		return googleProfile{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return googleProfile{}, err
	}
	defer resp.Body.Close()
	var token struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&token); err != nil {
		return googleProfile{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || token.AccessToken == "" {
		return googleProfile{}, errors.New("google token exchange failed")
	}
	profileReq, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://openidconnect.googleapis.com/v1/userinfo", nil)
	if err != nil {
		return googleProfile{}, err
	}
	profileReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
	profileResp, err := http.DefaultClient.Do(profileReq)
	if err != nil {
		return googleProfile{}, err
	}
	defer profileResp.Body.Close()
	var profile googleProfile
	if err := json.NewDecoder(profileResp.Body).Decode(&profile); err != nil {
		return googleProfile{}, err
	}
	if profileResp.StatusCode < 200 || profileResp.StatusCode >= 300 {
		return googleProfile{}, fmt.Errorf("google profile request failed: %d", profileResp.StatusCode)
	}
	return profile, nil
}

func (a *App) googleRedirectURI(r *http.Request) string {
	host := strings.ToLower(r.Host)
	if strings.HasPrefix(host, "127.0.0.1") {
		return a.cfg.GoogleRedirect127
	}
	if strings.HasPrefix(host, "localhost") {
		return a.cfg.GoogleRedirectLocal
	}
	return a.cfg.GoogleRedirectOnline
}

func randomURLToken(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func safeNext(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") {
		return ""
	}
	return value
}

func appendQuery(target, key, value string) string {
	fragment := ""
	if index := strings.Index(target, "#"); index >= 0 {
		fragment = target[index:]
		target = target[:index]
	}
	separator := "?"
	if strings.Contains(target, "?") {
		separator = "&"
	}
	return target + separator + url.QueryEscape(key) + "=" + url.QueryEscape(value) + fragment
}

func clearCookie(w http.ResponseWriter, name string, secure bool) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
}
