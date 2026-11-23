package internals

import (
	"database/sql"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"
)

const staffSessionCookie = "lixr_staff_session"

type StaffIdentity struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Protected   bool   `json:"protected"`
}

func (a *App) handleAuthSession(w http.ResponseWriter, r *http.Request) {
	staff, ok := a.sessionFromRequest(r)
	if !ok {
		JSON(w, http.StatusOK, Envelope{"ok": true, "authenticated": false})
		return
	}
	JSON(w, http.StatusOK, Envelope{"ok": true, "authenticated": true, "staff": staff})
}

func (a *App) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Login    string `json:"login"`
		Username string `json:"username"`
		Email    string `json:"email"`
		Password string `json:"password"`
		Remember bool   `json:"remember"`
	}
	if err := decodeJSON(r, &input); err != nil {
		Error(w, http.StatusBadRequest, "invalid request")
		return
	}
	login := strings.ToLower(strings.TrimSpace(input.Login))
	if login == "" {
		login = strings.ToLower(strings.TrimSpace(input.Username))
	}
	if login == "" {
		login = strings.ToLower(strings.TrimSpace(input.Email))
	}
	if len(login) < 2 || len(login) > 254 || len(input.Password) < 8 || len(input.Password) > 256 {
		a.auditAccess(nil, login, "login_failed", r)
		Error(w, http.StatusUnauthorized, "username/email or password is incorrect")
		return
	}

	var staff StaffIdentity
	var saltText, hashText string
	var iterations, active, protected int
	err := a.db.QueryRowContext(r.Context(), `
		SELECT id, username, COALESCE(email,''), display_name, role, password_salt, password_hash, password_iterations, active, protected
		FROM staff_accounts
		WHERE lower(username)=? OR lower(COALESCE(email,''))=?
		LIMIT 1
	`, login, login).Scan(&staff.ID, &staff.Username, &staff.Email, &staff.DisplayName, &staff.Role, &saltText, &hashText, &iterations, &active, &protected)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			Error(w, http.StatusInternalServerError, "sign in is temporarily unavailable")
			return
		}
		a.auditAccess(nil, login, "login_failed", r)
		Error(w, http.StatusUnauthorized, "username/email or password is incorrect")
		return
	}
	staff.Protected = protected == 1
	if active != 1 {
		a.auditAccess(&staff.ID, login, "login_inactive", r)
		Error(w, http.StatusForbidden, "this staff account is inactive")
		return
	}
	if !verifyPassword(input.Password, saltText, hashText, iterations) {
		a.auditAccess(&staff.ID, login, "login_failed", r)
		Error(w, http.StatusUnauthorized, "username/email or password is incorrect")
		return
	}

	duration := time.Duration(a.cfg.SessionHours) * time.Hour
	if input.Remember {
		duration = 30 * 24 * time.Hour
	}
	token, tokenHash, err := newSessionToken()
	if err != nil {
		Error(w, http.StatusInternalServerError, "could not create session")
		return
	}
	expiresAt := time.Now().Add(duration)
	tx, err := a.db.BeginTx(r.Context(), nil)
	if err != nil {
		Error(w, http.StatusInternalServerError, "could not create session")
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(r.Context(), `DELETE FROM staff_sessions WHERE expires_at<=?`, time.Now().Unix()); err != nil {
		Error(w, http.StatusInternalServerError, "could not create session")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO staff_sessions(staff_id,token_hash,expires_at) VALUES(?,?,?)`, staff.ID, tokenHash, expiresAt.Unix()); err != nil {
		Error(w, http.StatusInternalServerError, "could not create session")
		return
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE staff_accounts SET last_login_at=CURRENT_TIMESTAMP, updated_at=CURRENT_TIMESTAMP WHERE id=?`, staff.ID); err != nil {
		Error(w, http.StatusInternalServerError, "could not update account")
		return
	}
	if err = tx.Commit(); err != nil {
		Error(w, http.StatusInternalServerError, "could not create session")
		return
	}

	a.setSessionCookie(w, r, token, duration)
	a.auditAccess(&staff.ID, login, "login_success", r)
	JSON(w, http.StatusOK, Envelope{"ok": true, "authenticated": true, "staff": staff, "destination": roleDestination(staff.Role)})
}

func (a *App) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(staffSessionCookie); err == nil && cookie.Value != "" {
		hash := hashSessionToken(cookie.Value)
		var staffID sql.NullInt64
		_ = a.db.QueryRowContext(r.Context(), `SELECT staff_id FROM staff_sessions WHERE token_hash=?`, hash).Scan(&staffID)
		_, _ = a.db.ExecContext(r.Context(), `DELETE FROM staff_sessions WHERE token_hash=?`, hash)
		if staffID.Valid {
			id := staffID.Int64
			a.auditAccess(&id, "", "logout", r)
		}
	}
	clearSessionCookie(w, r, a.cfg.CookieSecure)
	JSON(w, http.StatusOK, Envelope{"ok": true, "authenticated": false})
}

func (a *App) sessionFromRequest(r *http.Request) (StaffIdentity, bool) {
	var staff StaffIdentity
	var protected int
	cookie, err := r.Cookie(staffSessionCookie)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return staff, false
	}
	hash := hashSessionToken(cookie.Value)
	err = a.db.QueryRowContext(r.Context(), `
		SELECT a.id, a.username, COALESCE(a.email,''), a.display_name, a.role, a.protected
		FROM staff_sessions s
		JOIN staff_accounts a ON a.id=s.staff_id
		WHERE s.token_hash=? AND s.expires_at>? AND a.active=1
		LIMIT 1
	`, hash, time.Now().Unix()).Scan(&staff.ID, &staff.Username, &staff.Email, &staff.DisplayName, &staff.Role, &protected)
	if err != nil {
		return StaffIdentity{}, false
	}
	staff.Protected = protected == 1
	_, _ = a.db.ExecContext(r.Context(), `UPDATE staff_sessions SET last_seen_at=CURRENT_TIMESTAMP WHERE token_hash=?`, hash)
	return staff, true
}

func (a *App) requireAdmin(w http.ResponseWriter, r *http.Request) (StaffIdentity, bool) {
	staff, ok := a.sessionFromRequest(r)
	if !ok {
		Error(w, http.StatusUnauthorized, "staff session required")
		return StaffIdentity{}, false
	}
	if staff.Role != "admin" && staff.Role != "developer" {
		Error(w, http.StatusForbidden, "admin access required")
		return StaffIdentity{}, false
	}
	return staff, true
}

func roleDestination(role string) string {
	if role == "cashier" {
		return "/pos"
	}
	return "/admin"
}

func (a *App) setSessionCookie(w http.ResponseWriter, r *http.Request, token string, duration time.Duration) {
	secure := a.cfg.CookieSecure || r.TLS != nil
	http.SetCookie(w, &http.Cookie{
		Name: staffSessionCookie, Value: token, Path: "/", HttpOnly: true,
		Secure: secure, SameSite: http.SameSiteLaxMode,
		MaxAge: int(duration.Seconds()), Expires: time.Now().Add(duration),
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request, configuredSecure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: staffSessionCookie, Value: "", Path: "/", HttpOnly: true,
		Secure: configuredSecure || r.TLS != nil, SameSite: http.SameSiteLaxMode,
		MaxAge: -1, Expires: time.Unix(1, 0),
	})
}

func (a *App) auditAccess(staffID *int64, identity, action string, r *http.Request) {
	ip := strings.TrimSpace(r.RemoteAddr)
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	if len(ip) > 96 {
		ip = ip[:96]
	}
	ua := r.UserAgent()
	if len(ua) > 512 {
		ua = ua[:512]
	}
	if len(identity) > 254 {
		identity = identity[:254]
	}
	var staffValue any
	if staffID != nil {
		staffValue = *staffID
	}
	_, _ = a.db.Exec(`INSERT INTO access_audit(staff_id,email,action,ip_address,user_agent) VALUES(?,?,?,?,?)`, staffValue, identity, action, ip, ua)
}
