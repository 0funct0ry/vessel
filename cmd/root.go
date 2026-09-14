// Package cmd implements the vessel command-line interface (Cobra commands,
// Viper configuration, and the serve/doctor/user/webhook/version subcommands).
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// exitCoder is implemented by errors that want to control the process exit
// code (e.g. GuardBindError exits 3, "no --db given" exits 2).
type exitCoder interface {
	ExitCode() int
}

// rootCmd with no subcommand given is a shortcut for `vessel serve` — it
// shares runServe and registers the identical flag set (see serve.go).
var rootCmd = &cobra.Command{
	Use:   "vessel",
	Short: "A single-binary web UI for the Docker containers on one host",
	Long: `Vessel is one binary. Drop it on a machine that runs Docker, start it,
open a browser, and see and control every container, image, volume and
network on that host.

Running vessel with no subcommand is a shortcut for "vessel serve".`,
	Args:          cobra.NoArgs,
	RunE:          runServe,
	SilenceUsage:  true,
	SilenceErrors: true,
}

// RootCommand returns the root Cobra command tree. It exists so that
// tooling outside this package (the docs CLI-reference generator, M23) can
// walk the real command/flag definitions instead of hand-duplicating them —
// the docs can't drift from the binary if they're generated from this.
func RootCommand() *cobra.Command { return rootCmd }

// Execute runs the root command. It is called once from main.go.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code := 1
		var ec exitCoder
		if as(err, &ec) {
			code = ec.ExitCode()
		}
		os.Exit(code)
	}
}

// as is a tiny errors.As wrapper kept local to avoid importing errors twice
// in every file that needs it.
func as(err error, target *exitCoder) bool {
	for err != nil {
		if ec, ok := err.(exitCoder); ok {
			*target = ec
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func init() {
	addServeFlags(rootCmd.Flags())

	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(userCmd)
	rootCmd.AddCommand(webhookCmd)
	rootCmd.AddCommand(migrateCmd)
}

// addConfigFlag registers the --config flag on a subcommand's own flag set.
// Every command that talks to newViper defines this locally — there are no
// persistent/inherited flags in this CLI.
func addConfigFlag(flags *pflag.FlagSet) {
	flags.String("config", "", "config file (default $XDG_CONFIG_HOME/vessel/vessel.yaml)")
}

// newViper builds a Viper instance for a subcommand's flag set, wiring up
// flag→env→file→default precedence: VESSEL_* environment variables and the
// vessel.yaml config file.
func newViper(cfgFile string) *viper.Viper {
	v := viper.New()

	v.SetEnvPrefix("vessel")
	v.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	v.AutomaticEnv()

	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
	} else {
		configDir := os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			home, err := os.UserHomeDir()
			if err == nil {
				configDir = filepath.Join(home, ".config")
			}
		}
		if configDir != "" {
			v.AddConfigPath(filepath.Join(configDir, "vessel"))
		}
		v.SetConfigName("vessel")
		v.SetConfigType("yaml")
	}

	// Config file is optional; only surface real parse errors.
	if err := v.ReadInConfig(); err != nil {
		if _, notFound := err.(viper.ConfigFileNotFoundError); !notFound {
			fmt.Fprintf(os.Stderr, "warning: %v\n", err)
		}
	}

	return v
}
