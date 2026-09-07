package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/viper"
)

// Config is the fully resolved configuration for `vessel serve`, after
// flag→env→file→default precedence has been applied by Viper.
type Config struct {
	Addr       string
	Port       int
	DB         string
	DockerHost string
	Auth       bool
	JWTTTL     time.Duration
	ReadOnly   bool
	BasePath   string
	AllowExec  bool
	LogLevel   string
	Open       bool
	Override   bool // --i-know-what-im-doing
	ConfigFile string
}

var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

// NewConfig builds and validates a Config from the given Viper instance.
func NewConfig(v *viper.Viper) (*Config, error) {
	port := v.GetInt("port")
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid --port %d: must be between 1 and 65535", port)
	}

	logLevel := v.GetString("log-level")
	if !validLogLevels[logLevel] {
		return nil, fmt.Errorf("invalid --log-level %q: must be one of debug|info|warn|error", logLevel)
	}

	jwtTTLRaw := v.GetString("jwt-ttl")
	jwtTTL, err := time.ParseDuration(jwtTTLRaw)
	if err != nil {
		return nil, fmt.Errorf("invalid --jwt-ttl %q: %w", jwtTTLRaw, err)
	}
	if jwtTTL <= 0 {
		return nil, fmt.Errorf("invalid --jwt-ttl %q: must be positive", jwtTTLRaw)
	}

	addr := v.GetString("addr")
	if addr == "" {
		return nil, fmt.Errorf("invalid --addr: must not be empty")
	}

	basePath := v.GetString("base-path")
	if basePath == "" {
		basePath = "/"
	}

	return &Config{
		Addr:       addr,
		Port:       port,
		DB:         v.GetString("db"),
		DockerHost: v.GetString("docker-host"),
		Auth:       v.GetBool("auth"),
		JWTTTL:     jwtTTL,
		ReadOnly:   v.GetBool("read-only"),
		BasePath:   basePath,
		AllowExec:  v.GetBool("allow-exec"),
		LogLevel:   logLevel,
		Open:       v.GetBool("open"),
		Override:   v.GetBool("i-know-what-im-doing"),
		ConfigFile: v.ConfigFileUsed(),
	}, nil
}
