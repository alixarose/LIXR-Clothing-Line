package internals

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	AppName       string
	AppEnv        string
	ServerHost    string
	ServerPort    int
	DatabasePath  string
	CookieSecure  bool
	SessionHours  int
	SessionSecret string
	AllowedHosts  map[string]struct{}

	GoogleOAuthEnabled   bool
	GoogleClientID       string
	GoogleClientSecret   string
	GoogleRedirect127    string
	GoogleRedirectLocal  string
	GoogleRedirectOnline string

	SMTPEnabled   bool
	SMTPHost      string
	SMTPPort      int
	SMTPUsername  string
	SMTPPassword  string
	SMTPFromEmail string
	SMTPFromName  string
}

func LoadConfig() Config {
	values := readEnvFile(filepath.Join("..", ".env"))
	port := intEnv(values, "SERVER_PORT", 3017)
	sessionHours := intEnv(values, "SESSION_HOURS", 12)
	if sessionHours < 1 {
		sessionHours = 12
	}
	smtpPort := intEnv(values, "SMTP_PORT", 587)
	return Config{
		AppName:              envOr(values, "APP_NAME", "Lixr"),
		AppEnv:               strings.ToLower(envOr(values, "APP_ENV", "development")),
		ServerHost:           envOr(values, "SERVER_HOST", "127.0.0.1"),
		ServerPort:           port,
		DatabasePath:         envOr(values, "DATABASE_PATH", "../database/lixr.db"),
		CookieSecure:         boolEnv(values, "COOKIE_SECURE", false),
		SessionHours:         sessionHours,
		SessionSecret:        envOr(values, "SESSION_SECRET", ""),
		AllowedHosts:         parseHosts(envOr(values, "ALLOWED_HOSTS", "127.0.0.1,localhost,lixr.jpano.dev")),
		GoogleOAuthEnabled:   boolEnv(values, "GOOGLE_OAUTH_ENABLED", false),
		GoogleClientID:       envOr(values, "GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret:   envOr(values, "GOOGLE_CLIENT_SECRET", ""),
		GoogleRedirect127:    envOr(values, "GOOGLE_REDIRECT_URI_127", "http://127.0.0.1:3017/api/auth/google/callback"),
		GoogleRedirectLocal:  envOr(values, "GOOGLE_REDIRECT_URI_LOCALHOST", "http://localhost:3017/api/auth/google/callback"),
		GoogleRedirectOnline: envOr(values, "GOOGLE_REDIRECT_URI_ONLINE", "https://lixr.jpano.dev/api/auth/google/callback"),
		SMTPEnabled:          boolEnv(values, "SMTP_ENABLED", false),
		SMTPHost:             envOr(values, "SMTP_HOST", "smtp.gmail.com"),
		SMTPPort:             smtpPort,
		SMTPUsername:         envOr(values, "SMTP_USERNAME", envOr(values, "SMTP_EMAIL", "")),
		SMTPPassword:         envOr(values, "SMTP_PASSWORD", ""),
		SMTPFromEmail:        envOr(values, "SMTP_FROM_EMAIL", ""),
		SMTPFromName:         envOr(values, "SMTP_FROM_NAME", "Lixr"),
	}
}

func readEnvFile(path string) map[string]string {
	out := map[string]string{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"'")
	}
	return out
}

func envOr(file map[string]string, key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	if v := strings.TrimSpace(file[key]); v != "" {
		return v
	}
	return fallback
}

func intEnv(file map[string]string, key string, fallback int) int {
	v, err := strconv.Atoi(envOr(file, key, ""))
	if err != nil || v < 1 || v > 65535 {
		return fallback
	}
	return v
}

func boolEnv(file map[string]string, key string, fallback bool) bool {
	v := strings.ToLower(strings.TrimSpace(envOr(file, key, "")))
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func parseHosts(raw string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, host := range strings.Split(raw, ",") {
		host = strings.ToLower(strings.TrimSpace(host))
		if host != "" {
			out[host] = struct{}{}
		}
	}
	return out
}
