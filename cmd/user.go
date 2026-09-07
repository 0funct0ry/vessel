package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// noDBError is returned by user/webhook subcommands when --db is missing.
// See SPEC §3.2. Exit code 2.
type noDBError struct{}

func (noDBError) Error() string {
	return "no --db given: users are ephemeral, add them at runtime instead"
}

func (noDBError) ExitCode() int { return 2 }

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "Manage Vessel users (requires --db)",
}

var userAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a user",
	Args:  cobra.ExactArgs(1),
	RunE:  requireDB,
}

var userListCmd = &cobra.Command{
	Use:   "list",
	Short: "List users",
	Args:  cobra.NoArgs,
	RunE:  requireDB,
}

var userPasswdCmd = &cobra.Command{
	Use:   "passwd <name>",
	Short: "Change a user's password",
	Args:  cobra.ExactArgs(1),
	RunE:  requireDB,
}

var userRmCmd = &cobra.Command{
	Use:   "rm <name>",
	Short: "Remove a user",
	Args:  cobra.ExactArgs(1),
	RunE:  requireDB,
}

func init() {
	addDBFlags(userAddCmd.Flags())
	userAddCmd.Flags().String("role", "viewer", "admin|operator|viewer")
	addDBFlags(userListCmd.Flags())
	addDBFlags(userPasswdCmd.Flags())
	addDBFlags(userRmCmd.Flags())

	userCmd.AddCommand(userAddCmd, userListCmd, userPasswdCmd, userRmCmd)
}

// addDBFlags registers the --db and --config flags shared by every
// user/webhook leaf command. Each command owns its own local copy — there
// are no persistent/inherited flags in this CLI.
func addDBFlags(flags *pflag.FlagSet) {
	addConfigFlag(flags)
	flags.String("db", "", "SQLite file (required)")
}

// requireDB is a stub RunE shared by every user/webhook subcommand until
// internal/store (M7) and internal/auth (M8) exist. It only enforces the
// --db precondition from SPEC §3.2.
func requireDB(cmd *cobra.Command, args []string) error {
	cfgFile, err := cmd.Flags().GetString("config")
	if err != nil {
		return err
	}

	v := newViper(cfgFile)
	if err := v.BindPFlags(cmd.Flags()); err != nil {
		return err
	}
	if v.GetString("db") == "" {
		return noDBError{}
	}
	return errNotImplemented("user/webhook commands are not implemented until the store (M7) lands")
}

type notImplementedError string

func (e notImplementedError) Error() string { return string(e) }

func errNotImplemented(msg string) error { return notImplementedError(msg) }
