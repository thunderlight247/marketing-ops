// Package config loads and validates API configuration from environment
// variables. Validation messages never echo configured values, so a malformed
// DATABASE_URL cannot leak credentials into logs or terminals.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"time"

	"marketingops/api/internal/secretcipher"
)

// Environment is the deployment mode of the API process.
type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvTest        Environment = "test"
	EnvProduction  Environment = "production"
)

// Config is the validated runtime configuration.
//
// DatabaseURL may contain credentials. Do not log it or print it directly;
// use DatabaseTarget, String, or LogValue instead.
type Config struct {
	Env                  Environment
	Port                 int
	LogLevel             slog.Level
	DatabaseURL          string
	AllowedOrigins       []string
	ShutdownTimeout      time.Duration
	HealthTimeout        time.Duration
	CredentialKey        []byte
	CredentialKeyVersion int

	// APIKey is the operator's Bearer token used by the auth middleware.
	// Required when APP_ENV=production; optional (auth disabled) in development.
	// Never log it.
	APIKey string

	// AI drafting. AIProvider selects the adapter: "openrouter" when a key is
	// configured, or "stub" for offline development. AIModel is the routed
	// model name (provider-specific, e.g. "anthropic/claude-3.5-haiku").
	AIProvider      string
	AIModel         string
	AIKey           string // secret; never log it
	GenerateTimeout time.Duration

	// SanityOpsBase overrides the Sanity API host for local end-to-end runs
	// against a mock. It is honored only outside production; in production a
	// value is rejected, so real publishing always talks to sanity.io.
	SanityOpsBase string

	// Publishing worker (Milestone 9). PublishWorkerInterval is the scan
	// cadence for due jobs; MaxAttempts bounds retries before a job waits for
	// an operator; the backoff grows exponentially between those attempts.
	PublishWorkerInterval    time.Duration
	PublishWorkerMaxAttempts int
	PublishWorkerBaseDelay   time.Duration
	PublishWorkerMaxDelay    time.Duration
}

// Load reads configuration using getenv (usually os.Getenv). All problems are
// collected and returned together so the operator can fix them in one pass.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Env:             EnvDevelopment,
		Port:            8080,
		LogLevel:        slog.LevelInfo,
		AllowedOrigins:  []string{"http://localhost:3000"},
		ShutdownTimeout: 10 * time.Second,
		HealthTimeout:   2 * time.Second,

		// AI drafting defaults: no provider until a key exists; the stub
		// keeps local development moving without any network access.
		AIProvider:      "stub",
		AIModel:         "stub-v1",
		GenerateTimeout: 90 * time.Second,

		// Publishing worker defaults (Milestone 9).
		PublishWorkerInterval:    15 * time.Second,
		PublishWorkerMaxAttempts: 5,
		PublishWorkerBaseDelay:   30 * time.Second,
		PublishWorkerMaxDelay:    30 * time.Minute,
	}

	var problems []error
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Errorf(format, args...))
	}

	if v := strings.TrimSpace(getenv("APP_ENV")); v != "" {
		env := Environment(strings.ToLower(v))
		switch env {
		case EnvDevelopment, EnvTest, EnvProduction:
			cfg.Env = env
		default:
			add("APP_ENV must be one of: development, test, production")
		}
	}

	if v := strings.TrimSpace(getenv("API_PORT")); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil || port < 1 || port > 65535 {
			add("API_PORT must be an integer between 1 and 65535")
		} else {
			cfg.Port = port
		}
	}

	if v := strings.TrimSpace(getenv("LOG_LEVEL")); v != "" {
		var level slog.Level
		if err := level.UnmarshalText([]byte(v)); err != nil {
			add("LOG_LEVEL must be one of: debug, info, warn, error")
		} else {
			cfg.LogLevel = level
		}
	}

	dbRaw := strings.TrimSpace(getenv("DATABASE_URL"))
	if dbRaw != "" {
		if err := validateDatabaseURL(dbRaw); err != nil {
			add("DATABASE_URL is invalid: %v", err)
		} else {
			cfg.DatabaseURL = dbRaw
		}
	}

	originsRaw := strings.TrimSpace(getenv("CORS_ALLOWED_ORIGINS"))
	if originsRaw != "" {
		origins, err := parseOrigins(originsRaw)
		if err != nil {
			add("CORS_ALLOWED_ORIGINS is invalid: %v", err)
		} else {
			cfg.AllowedOrigins = origins
		}
	}

	// API key authentication (Milestone 13).
	cfg.APIKey = strings.TrimSpace(getenv("API_KEY"))

	cfg.CredentialKeyVersion = 1
	credKeyRaw := strings.TrimSpace(getenv("CREDENTIAL_ENCRYPTION_KEY"))
	if credKeyRaw != "" {
		key, err := secretcipher.LoadKey(credKeyRaw)
		if err != nil {
			add("CREDENTIAL_ENCRYPTION_KEY is invalid: must be 64 hex characters (32 bytes)")
		} else {
			cfg.CredentialKey = key
		}
	} else if cfg.Env != EnvProduction {
		// Deterministic development fallback so local development and tests work out of the box.
		cfg.CredentialKey = bytes.Repeat([]byte{0x42}, secretcipher.KeyLength)
	}

	// AI provider selection and model routing.
	aiProvider := strings.TrimSpace(strings.ToLower(getenv("AI_PROVIDER")))
	aiKey := strings.TrimSpace(getenv("OPENROUTER_API_KEY"))
	aiModel := strings.TrimSpace(getenv("AI_MODEL"))
	switch {
	case aiKey != "":
		cfg.AIProvider = "openrouter"
		cfg.AIKey = aiKey
		if aiModel != "" {
			cfg.AIModel = aiModel
		}
	case aiProvider == "stub" || aiProvider == "":
		cfg.AIProvider = "stub"
		cfg.AIModel = "stub-v1"
	case aiProvider == "openrouter":
		add("OPENROUTER_API_KEY is required when AI_PROVIDER=openrouter")
		cfg.AIProvider = "openrouter"
		if aiModel != "" {
			cfg.AIModel = aiModel
		}
	default:
		add("AI_PROVIDER must be stub or openrouter (openrouter also needs OPENROUTER_API_KEY)")
	}

	if cfg.Env == EnvProduction && cfg.AIProvider == "stub" {
		add("a real AI provider is required when APP_ENV=production (set OPENROUTER_API_KEY)")
	}

	generateTimeoutRaw := strings.TrimSpace(getenv("AI_GENERATE_TIMEOUT"))
	if generateTimeoutRaw != "" {
		timeoutSeconds, err := strconv.Atoi(generateTimeoutRaw)
		if err != nil || timeoutSeconds < 5 || timeoutSeconds > 600 {
			add("AI_GENERATE_TIMEOUT must be an integer between 5 and 600 seconds")
		} else {
			cfg.GenerateTimeout = time.Duration(timeoutSeconds) * time.Second
		}
	}

	workerIntervalRaw := strings.TrimSpace(getenv("PUBLISH_WORKER_INTERVAL"))
	if workerIntervalRaw != "" {
		seconds, err := strconv.Atoi(workerIntervalRaw)
		if err != nil || seconds < 1 || seconds > 3600 {
			add("PUBLISH_WORKER_INTERVAL must be an integer between 1 and 3600 seconds")
		} else {
			cfg.PublishWorkerInterval = time.Duration(seconds) * time.Second
		}
	}
	maxAttemptsRaw := strings.TrimSpace(getenv("PUBLISH_WORKER_MAX_ATTEMPTS"))
	if maxAttemptsRaw != "" {
		attempts, err := strconv.Atoi(maxAttemptsRaw)
		if err != nil || attempts < 1 || attempts > 20 {
			add("PUBLISH_WORKER_MAX_ATTEMPTS must be an integer between 1 and 20")
		} else {
			cfg.PublishWorkerMaxAttempts = attempts
		}
	}
	baseDelayRaw := strings.TrimSpace(getenv("PUBLISH_WORKER_BASE_DELAY"))
	if baseDelayRaw != "" {
		seconds, err := strconv.Atoi(baseDelayRaw)
		if err != nil || seconds < 1 || seconds > 3600 {
			add("PUBLISH_WORKER_BASE_DELAY must be an integer between 1 and 3600 seconds")
		} else {
			cfg.PublishWorkerBaseDelay = time.Duration(seconds) * time.Second
		}
	}
	maxDelayRaw := strings.TrimSpace(getenv("PUBLISH_WORKER_MAX_DELAY"))
	if maxDelayRaw != "" {
		seconds, err := strconv.Atoi(maxDelayRaw)
		if err != nil || seconds < 1 || seconds > 86400 {
			add("PUBLISH_WORKER_MAX_DELAY must be an integer between 1 and 86400 seconds")
		} else {
			cfg.PublishWorkerMaxDelay = time.Duration(seconds) * time.Second
		}
	}

	if cfg.Env == EnvProduction {
		if dbRaw == "" {
			add("DATABASE_URL is required when APP_ENV=production")
		}
		if originsRaw == "" {
			add("CORS_ALLOWED_ORIGINS is required when APP_ENV=production")
		}
		if len(cfg.CredentialKey) == 0 {
			add("CREDENTIAL_ENCRYPTION_KEY is required when APP_ENV=production")
		}
		if cfg.APIKey == "" {
			add("API_KEY is required when APP_ENV=production (set it to a long random string)")
		}
		if strings.TrimSpace(getenv("SANITY_OPS_BASE")) != "" {
			add("SANITY_OPS_BASE must not be set when APP_ENV=production")
		}
	} else {
		// Dev/test-only API-host override for the publishing adapter.
		cfg.SanityOpsBase = strings.TrimRight(strings.TrimSpace(getenv("SANITY_OPS_BASE")), "/")
	}

	if err := errors.Join(problems...); err != nil {
		return Config{}, fmt.Errorf("invalid configuration:\n%w", err)
	}
	return cfg, nil
}

// validateDatabaseURL checks the shape of a PostgreSQL URL. It deliberately
// returns generic errors: url.Parse errors embed the input string.
func validateDatabaseURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("not a valid URL")
	}
	if u.Scheme != "postgres" && u.Scheme != "postgresql" {
		return errors.New("scheme must be postgres or postgresql")
	}
	if u.Hostname() == "" {
		return errors.New("host is missing")
	}
	if strings.Trim(u.Path, "/") == "" {
		return errors.New("database name is missing")
	}
	return nil
}

// parseOrigins validates a comma-separated list of browser origins and
// normalizes each to scheme://host[:port].
func parseOrigins(raw string) ([]string, error) {
	var origins []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if part == "*" {
			return nil, errors.New("wildcard origins are not supported")
		}
		u, err := url.Parse(part)
		badShape := err != nil ||
			(u.Scheme != "http" && u.Scheme != "https") ||
			u.Host == "" ||
			(u.Path != "" && u.Path != "/") ||
			u.RawQuery != "" || u.Fragment != "" || u.User != nil
		if badShape {
			return nil, errors.New("each origin must look like https://example.com")
		}
		origins = append(origins, u.Scheme+"://"+u.Host)
	}
	if len(origins) == 0 {
		return nil, errors.New("at least one origin is required")
	}
	return origins, nil
}

// Addr returns the listen address for http.Server.
func (c Config) Addr() string { return fmt.Sprintf(":%d", c.Port) }

// DatabaseConfigured reports whether a database URL was provided.
func (c Config) DatabaseConfigured() bool { return c.DatabaseURL != "" }

// DatabaseTarget returns host:port/dbname without user or password.
func (c Config) DatabaseTarget() string {
	if c.DatabaseURL == "" {
		return ""
	}
	u, err := url.Parse(c.DatabaseURL)
	if err != nil {
		return "invalid"
	}
	return u.Host + u.Path
}

// String implements fmt.Stringer so %v and %+v never print credentials.
func (c Config) String() string {
	return fmt.Sprintf("Config{env=%s port=%d database=%q}", c.Env, c.Port, c.DatabaseTarget())
}

// LogValue implements slog.LogValuer so structured logs stay credential-free.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("env", string(c.Env)),
		slog.Int("port", c.Port),
		slog.Bool("database_configured", c.DatabaseConfigured()),
		slog.String("database_target", c.DatabaseTarget()),
		slog.Any("allowed_origins", c.AllowedOrigins),
		slog.Bool("credential_key_configured", len(c.CredentialKey) > 0),
		slog.Bool("api_key_configured", c.APIKey != ""),
		slog.String("ai_provider", c.AIProvider),
		slog.String("ai_model", c.AIModel),
		slog.Bool("ai_key_configured", c.AIKey != ""),
	)
}
