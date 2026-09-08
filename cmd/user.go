package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"

	"github.com/0funct0ry/vessel/internal/auth"
	"github.com/0funct0ry/vessel/internal/store"
	"github.com/0funct0ry/vessel/internal/store/sqlitestore"
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
	RunE:  runUserAdd,
}

var userListCmd = &cobra.Command{
	Use:   "list",
	Short: "List users",
	Args:  cobra.NoArgs,
	RunE:  runUserList,
}

var userPasswdCmd = &cobra.Command{
	Use:   "passwd <name>",
	Short: "Change a user's password",
	Args:  cobra.ExactArgs(1),
	RunE:  runUserPasswd,
}

var userRmCmd = &cobra.Command{
	Use:   "rm <name>",
	Short: "Remove a user",
	Args:  cobra.ExactArgs(1),
	RunE:  runUserRm,
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

func openUserStore(cmd *cobra.Command) (*sqlitestore.Store, error) {
	cfgFile, err := cmd.Flags().GetString("config")
	if err != nil {
		return nil, err
	}
	v := newViper(cfgFile)
	if err := v.BindPFlags(cmd.Flags()); err != nil {
		return nil, err
	}
	db := v.GetString("db")
	if db == "" {
		return nil, noDBError{}
	}
	return sqlitestore.Open(db)
}
func runUserAdd(cmd *cobra.Command, args []string) error {
	s, err := openUserStore(cmd)
	if err != nil {
		return err
	}
	defer s.Close()
	roleText, err := cmd.Flags().GetString("role")
	if err != nil {
		return err
	}
	role := store.Role(roleText)
	if role != store.RoleAdmin && role != store.RoleOperator && role != store.RoleViewer {
		return fmt.Errorf("invalid role %q: must be admin, operator, or viewer", role)
	}
	password, err := promptPassword()
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	_, err = s.CreateUser(context.Background(), store.User{Username: args[0], PasswordHash: hash, Role: role, CreatedAt: time.Now().UTC()})
	if err == store.ErrConflict {
		return fmt.Errorf("user %q already exists", args[0])
	}
	if err == nil {
		fmt.Printf("created user %s\n", args[0])
	}
	return err
}
func runUserList(cmd *cobra.Command, _ []string) error {
	s, err := openUserStore(cmd)
	if err != nil {
		return err
	}
	defer s.Close()
	users, err := s.ListUsers(context.Background())
	if err != nil {
		return err
	}
	for _, u := range users {
		fmt.Printf("%d\t%s\t%s\t%s\n", u.ID, u.Username, u.Role, u.CreatedAt.UTC().Format(time.RFC3339))
	}
	return nil
}
func runUserPasswd(cmd *cobra.Command, args []string) error {
	s, err := openUserStore(cmd)
	if err != nil {
		return err
	}
	defer s.Close()
	u, err := s.GetUserByUsername(context.Background(), args[0])
	if err != nil {
		return fmt.Errorf("user %q not found", args[0])
	}
	p, err := promptPassword()
	if err != nil {
		return err
	}
	u.PasswordHash, err = auth.HashPassword(p)
	if err != nil {
		return err
	}
	_, err = s.UpdateUser(context.Background(), u)
	if err == nil {
		fmt.Printf("updated password for %s\n", u.Username)
	}
	return err
}
func runUserRm(cmd *cobra.Command, args []string) error {
	s, err := openUserStore(cmd)
	if err != nil {
		return err
	}
	defer s.Close()
	u, err := s.GetUserByUsername(context.Background(), args[0])
	if err != nil {
		return fmt.Errorf("user %q not found", args[0])
	}
	if err = s.DeleteUser(context.Background(), u.ID); err == nil {
		fmt.Printf("removed user %s\n", u.Username)
	}
	return err
}
func promptPassword() (string, error) {
	fmt.Fprint(os.Stderr, "Password: ")
	first, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, "Confirm password: ")
	second, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if string(first) != string(second) {
		return "", fmt.Errorf("passwords do not match")
	}
	if len([]rune(string(first))) < auth.MinimumPassword {
		return "", fmt.Errorf("password must be at least %d characters", auth.MinimumPassword)
	}
	return strings.TrimSpace(string(first)), nil
}

type notImplementedError string

func (e notImplementedError) Error() string { return string(e) }

func errNotImplemented(msg string) error { return notImplementedError(msg) }
