package internals

// Integration configuration lives in .env, but credentials are never returned to
// the browser. Google OAuth and SMTP can be enabled without changing the project
// layout; health/system endpoints report only whether each integration is configured.
func (c Config) GoogleOAuthReady() bool {
	return c.GoogleOAuthEnabled && c.GoogleClientID != "" && c.GoogleClientSecret != ""
}

func (c Config) SMTPReady() bool {
	return c.SMTPEnabled && c.SMTPHost != "" && c.SMTPPort > 0 && c.SMTPUsername != "" && c.SMTPPassword != ""
}
