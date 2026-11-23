package internals

import (
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
)

func (a *App) sendNewsletterWelcome(r *http.Request, email, token string) error {
	if !a.cfg.SMTPEnabled || a.cfg.SMTPHost == "" || a.cfg.SMTPUsername == "" || a.cfg.SMTPPassword == "" {
		return nil
	}
	from := strings.TrimSpace(a.cfg.SMTPFromEmail)
	if from == "" {
		from = a.cfg.SMTPUsername
	}
	fromName := strings.TrimSpace(a.cfg.SMTPFromName)
	if fromName == "" {
		fromName = "Lixr"
	}
	base := requestBaseURL(r)
	unsubscribe := base + "/api/newsletter/unsubscribe?token=" + token
	subject := "Welcome to the Lixr list"
	body := `<!doctype html><html><body style="margin:0;background:#f1ede3;color:#1b1b17;font-family:Arial,sans-serif"><div style="max-width:620px;margin:0 auto;padding:42px 24px"><div style="font-family:Georgia,serif;font-size:38px;letter-spacing:.16em;margin-bottom:30px">LIXR</div><h1 style="font-family:Georgia,serif;font-size:38px;font-weight:400;margin:0 0 16px">You’re on the list.</h1><p style="line-height:1.7;color:#57544d">We’ll send occasional updates about new pieces, restocks and store notes.</p><p style="margin:34px 0"><a href="` + unsubscribe + `" style="display:inline-block;padding:14px 20px;background:#1b1b17;color:#f1ede3;text-decoration:none;text-transform:uppercase;font-size:11px;letter-spacing:.12em">Unsubscribe</a></p><p style="font-size:11px;color:#807b70">If you no longer want Lixr updates, use the button above at any time.</p></div></body></html>`
	message := []byte("From: " + fromName + " <" + from + ">\r\n" +
		"To: " + email + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/html; charset=UTF-8\r\n\r\n" + body)
	auth := smtp.PlainAuth("", a.cfg.SMTPUsername, a.cfg.SMTPPassword, a.cfg.SMTPHost)
	return smtp.SendMail(fmt.Sprintf("%s:%d", a.cfg.SMTPHost, a.cfg.SMTPPort), auth, from, []string{email}, message)
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (a *App) handleNewsletterUnsubscribe(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	message := "This unsubscribe link is not valid."
	if token != "" {
		res, err := a.db.ExecContext(r.Context(), `UPDATE newsletter_subscribers SET active=0,unsubscribed_at=CURRENT_TIMESTAMP WHERE unsubscribe_token=? AND active=1`, token)
		if err == nil {
			if n, _ := res.RowsAffected(); n > 0 {
				message = "You have been unsubscribed from Lixr updates."
			} else {
				message = "This email is already unsubscribed or the link has expired."
			}
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = fmt.Fprintf(w, `<!doctype html><html><head><meta name="viewport" content="width=device-width,initial-scale=1"><title>Lixr — Subscription</title><style>body{margin:0;background:#f1ede3;color:#1b1b17;font-family:Arial,sans-serif;display:grid;min-height:100vh;place-items:center}.card{max-width:560px;padding:42px;border:1px solid rgba(27,27,23,.18);background:#f8f5ee}.brand{font:38px Georgia,serif;letter-spacing:.16em}h1{font:40px Georgia,serif;font-weight:400}p{line-height:1.7;color:#625e55}a{display:inline-block;margin-top:18px;color:inherit}</style></head><body><main class="card"><div class="brand">LIXR</div><h1>Subscription updated.</h1><p>%s</p><a href="/">Return to Lixr</a></main></body></html>`, message)
}
