package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

func newTestViper(t *testing.T, configYAML string) (*viper.Viper, *pflag.FlagSet) {
	t.Helper()

	v := viper.New()
	v.SetEnvPrefix("vessel")
	v.AutomaticEnv()

	if configYAML != "" {
		dir := t.TempDir()
		path := filepath.Join(dir, "vessel.yaml")
		if err := os.WriteFile(path, []byte(configYAML), 0o644); err != nil {
			t.Fatal(err)
		}
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			t.Fatal(err)
		}
	}

	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	flags.Int("port", 7373, "")
	if err := v.BindPFlags(flags); err != nil {
		t.Fatal(err)
	}

	return v, flags
}

func TestConfigPrecedence_FlagBeatsEnvAndFile(t *testing.T) {
	v, flags := newTestViper(t, "port: 8000\n")
	t.Setenv("VESSEL_PORT", "9000")
	if err := flags.Set("port", "1234"); err != nil {
		t.Fatal(err)
	}

	if got := v.GetInt("port"); got != 1234 {
		t.Errorf("port = %d, want 1234 (flag should win)", got)
	}
}

func TestConfigPrecedence_EnvBeatsFile(t *testing.T) {
	v, _ := newTestViper(t, "port: 8000\n")
	t.Setenv("VESSEL_PORT", "9000")

	if got := v.GetInt("port"); got != 9000 {
		t.Errorf("port = %d, want 9000 (env should win over file)", got)
	}
}

func TestConfigPrecedence_FileBeatsDefault(t *testing.T) {
	v, _ := newTestViper(t, "port: 8000\n")

	if got := v.GetInt("port"); got != 8000 {
		t.Errorf("port = %d, want 8000 (file should win over default)", got)
	}
}

func TestConfigPrecedence_Default(t *testing.T) {
	v, _ := newTestViper(t, "")

	if got := v.GetInt("port"); got != 7373 {
		t.Errorf("port = %d, want 7373 (default)", got)
	}
}

func TestNewConfig_Validation(t *testing.T) {
	cases := []struct {
		name    string
		set     func(flags *pflag.FlagSet)
		wantErr bool
	}{
		{"valid defaults", func(flags *pflag.FlagSet) {}, false},
		{"bad port", func(flags *pflag.FlagSet) { _ = flags.Set("port", "70000") }, true},
		{"bad log level", func(flags *pflag.FlagSet) { _ = flags.Set("log-level", "verbose") }, true},
		{"bad jwt-ttl", func(flags *pflag.FlagSet) { _ = flags.Set("jwt-ttl", "banana") }, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := viper.New()
			flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
			flags.String("addr", "127.0.0.1", "")
			flags.Int("port", 7373, "")
			flags.String("db", "", "")
			flags.String("docker-host", "unix:///var/run/docker.sock", "")
			flags.Bool("auth", false, "")
			flags.String("jwt-ttl", "24h", "")
			flags.Bool("read-only", false, "")
			flags.String("base-path", "/", "")
			flags.Bool("allow-exec", true, "")
			flags.String("log-level", "info", "")
			flags.Bool("open", false, "")
			flags.Bool("i-know-what-im-doing", false, "")

			tc.set(flags)
			if err := v.BindPFlags(flags); err != nil {
				t.Fatal(err)
			}

			_, err := NewConfig(v)
			if (err != nil) != tc.wantErr {
				t.Errorf("NewConfig() err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
