package cache

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config holds Redis client options for cache-redis.
type Config struct {
	Addr      string
	Username  string
	Password  string
	DB        int
	KeyPrefix string
	TLS       *tls.Config
}

// ConfigFromEnv builds Config from standard REDIS_* / CACHE_KEY_PREFIX env vars.
// REDIS_URL (redis:// or rediss://) overrides addr/password/db/TLS when set.
func ConfigFromEnv() (Config, error) {
	cfg := Config{
		Addr:      envOr("REDIS_ADDR", "localhost:6379"),
		Username:  os.Getenv("REDIS_USERNAME"),
		Password:  os.Getenv("REDIS_PASSWORD"),
		DB:        envInt("REDIS_DB", 0),
		KeyPrefix: os.Getenv("CACHE_KEY_PREFIX"),
	}
	if raw := strings.TrimSpace(os.Getenv("REDIS_URL")); raw != "" {
		if err := cfg.applyURL(raw); err != nil {
			return Config{}, err
		}
	}
	tlsCfg, err := tlsConfigFromEnv()
	if err != nil {
		return Config{}, err
	}
	if tlsCfg != nil {
		cfg.TLS = tlsCfg
	}
	return cfg, nil
}

func (cfg *Config) applyURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("REDIS_URL: %w", err)
	}
	switch u.Scheme {
	case "redis", "rediss":
	default:
		return fmt.Errorf("REDIS_URL: unsupported scheme %q", u.Scheme)
	}
	host := u.Host
	if host == "" {
		return fmt.Errorf("REDIS_URL: missing host")
	}
	cfg.Addr = host
	if u.User != nil {
		cfg.Username = u.User.Username()
		if pass, ok := u.User.Password(); ok {
			cfg.Password = pass
		}
	}
	if u.Path != "" && u.Path != "/" {
		n, err := strconv.Atoi(strings.TrimPrefix(u.Path, "/"))
		if err != nil || n < 0 {
			return fmt.Errorf("REDIS_URL: invalid db index %q", u.Path)
		}
		cfg.DB = n
	}
	if u.Scheme == "rediss" {
		cfg.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return nil
}

func tlsConfigFromEnv() (*tls.Config, error) {
	if !envBool("REDIS_TLS", false) && !envBool("REDIS_TLS_ENABLED", false) {
		return nil, nil
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if v := strings.TrimSpace(os.Getenv("REDIS_TLS_SERVER_NAME")); v != "" {
		cfg.ServerName = v
	}
	if envBool("REDIS_TLS_INSECURE", false) {
		cfg.InsecureSkipVerify = true //nolint:gosec // operator opt-in for dev/self-signed
	}
	if caFile := strings.TrimSpace(os.Getenv("REDIS_TLS_CA_FILE")); caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("REDIS_TLS_CA_FILE: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("REDIS_TLS_CA_FILE: no certificates parsed")
		}
		cfg.RootCAs = pool
	}
	certFile := strings.TrimSpace(os.Getenv("REDIS_TLS_CERT_FILE"))
	keyFile := strings.TrimSpace(os.Getenv("REDIS_TLS_KEY_FILE"))
	if certFile != "" || keyFile != "" {
		if certFile == "" || keyFile == "" {
			return nil, fmt.Errorf("REDIS_TLS_CERT_FILE and REDIS_TLS_KEY_FILE must both be set")
		}
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("client TLS cert: %w", err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}
