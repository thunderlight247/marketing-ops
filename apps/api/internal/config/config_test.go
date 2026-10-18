package config

import (
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatalf("Load with empty env returned error: %v", err)
	}
	if cfg.Env != EnvDevelopment || cfg.Port != 8080 || cfg.LogLevel != slog.LevelInfo {
		t.Errorf("unexpected defaults: %+v", cfg)
	}
	if cfg.DatabaseConfigured() {
		t.Error("database should be unconfigured by default")
	}
	if want := []string{"http://localhost:3000"}; !reflect.DeepEqual(cfg.AllowedOrigins, want) {
		t.Errorf("AllowedOrigins = %v, want %v", cfg.AllowedOrigins, want)
	}
}

func TestLoadValidValues(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"APP_ENV":                   "Production",
		"API_PORT":                  "9000",
		"LOG_LEVEL":                 "debug",
		"DATABASE_URL":              "postgres://app:pw@db.internal:5432/marketing?sslmode=disable",
		"CORS_ALLOWED_ORIGINS":      "https://ops.example.com/, https://admin.example.com",
		"CREDENTIAL_ENCRYPTION_KEY": strings.Repeat("ab", 32),
	}))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Env != EnvProduction || cfg.Port != 9000 || cfg.LogLevel != slog.LevelDebug {
		t.Errorf("unexpected config: %+v", cfg)
	}
	want := []string{"https://ops.example.com", "https://admin.example.com"}
	if !reflect.DeepEqual(cfg.AllowedOrigins, want) {
		t.Errorf("AllowedOrigins = %v, want %v", cfg.AllowedOrigins, want)
	}
	if cfg.Addr() != ":9000" {
		t.Errorf("Addr() = %q", cfg.Addr())
	}
}

func TestLoadRejectsInvalid(t *testing.T) {
	validDB := "postgres://u:p@localhost:5432/db"
	tests := []struct {
		name string
		vars map[string]string
		want string
	}{
		{"unknown env", map[string]string{"APP_ENV": "staging"}, "APP_ENV"},
		{"port zero", map[string]string{"API_PORT": "0"}, "API_PORT"},
		{"port text", map[string]string{"API_PORT": "abc"}, "API_PORT"},
		{"port too high", map[string]string{"API_PORT": "70000"}, "API_PORT"},
		{"bad log level", map[string]string{"LOG_LEVEL": "loud"}, "LOG_LEVEL"},
		{"wrong db scheme", map[string]string{"DATABASE_URL": "mysql://u:p@h/db"}, "DATABASE_URL is invalid"},
		{"db missing host", map[string]string{"DATABASE_URL": "postgres:///db"}, "DATABASE_URL is invalid"},
		{"db missing name", map[string]string{"DATABASE_URL": "postgres://localhost:5432"}, "DATABASE_URL is invalid"},
		{"wildcard origin", map[string]string{"CORS_ALLOWED_ORIGINS": "*"}, "CORS_ALLOWED_ORIGINS is invalid"},
		{"origin with path", map[string]string{"CORS_ALLOWED_ORIGINS": "https://a.example.com/app"}, "CORS_ALLOWED_ORIGINS is invalid"},
		{"prod without db", map[string]string{"APP_ENV": "production", "CORS_ALLOWED_ORIGINS": "https://a.example.com", "CREDENTIAL_ENCRYPTION_KEY": strings.Repeat("ab", 32)}, "DATABASE_URL is required"},
		{"prod without origins", map[string]string{"APP_ENV": "production", "DATABASE_URL": validDB, "CREDENTIAL_ENCRYPTION_KEY": strings.Repeat("ab", 32)}, "CORS_ALLOWED_ORIGINS is required"},
		{"prod without credential key", map[string]string{"APP_ENV": "production", "DATABASE_URL": validDB, "CORS_ALLOWED_ORIGINS": "https://a.example.com"}, "CREDENTIAL_ENCRYPTION_KEY is required"},
		{"invalid credential key", map[string]string{"CREDENTIAL_ENCRYPTION_KEY": "not-hex"}, "CREDENTIAL_ENCRYPTION_KEY is invalid"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Load(env(tc.vars))
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.want)
			}
		})
	}
}

func TestLoadReportsAllProblemsTogether(t *testing.T) {
	_, err := Load(env(map[string]string{"APP_ENV": "bad", "API_PORT": "0"}))
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"APP_ENV", "API_PORT"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q is missing %q", err.Error(), want)
		}
	}
}

func TestErrorsDoNotLeakSecrets(t *testing.T) {
	const secret = "topsecret123"
	for _, raw := range []string{
		"mysql://admin:" + secret + "@db.internal/app",
		"postgres://admin:" + secret + "@db.internal:notaport/app",
		"postgres://admin:" + secret + "@/app",
	} {
		_, err := Load(env(map[string]string{"DATABASE_URL": raw}))
		if err == nil {
			t.Fatalf("expected an error for %q", raw)
		}
		if strings.Contains(err.Error(), secret) {
			t.Errorf("error leaked the password: %q", err.Error())
		}
	}
}

func TestSafeRepresentationsOmitCredentials(t *testing.T) {
	const secret = "topsecret123"
	cfg, err := Load(env(map[string]string{
		"DATABASE_URL": "postgres://admin:" + secret + "@db.internal:5432/marketing",
	}))
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if got, want := cfg.DatabaseTarget(), "db.internal:5432/marketing"; got != want {
		t.Errorf("DatabaseTarget() = %q, want %q", got, want)
	}
	outputs := map[string]string{
		"DatabaseTarget": cfg.DatabaseTarget(),
		"String":         cfg.String(),
		"%+v":            fmt.Sprintf("%+v", cfg),
		"%v":             fmt.Sprintf("%v", cfg),
		"LogValue":       fmt.Sprint(cfg.LogValue()),
	}
	for name, out := range outputs {
		if strings.Contains(out, secret) || strings.Contains(out, "admin") {
			t.Errorf("%s leaked credentials: %q", name, out)
		}
	}
}
