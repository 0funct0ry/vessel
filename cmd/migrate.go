package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/0funct0ry/vessel/internal/store/sqlitestore"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Apply SQLite schema migrations and exit",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		db, err := cmd.Flags().GetString("db")
		if err != nil {
			return err
		}
		if db == "" {
			return noDBError{}
		}
		s, err := sqlitestore.Open(db)
		if err != nil {
			return fmt.Errorf("migrate database: %w", err)
		}
		defer s.Close()
		version, err := s.SchemaVersion(cmd.Context())
		if err != nil {
			return err
		}
		fmt.Printf("schema version %d\n", version)
		return nil
	},
}

func init() { addDBFlags(migrateCmd.Flags()) }
