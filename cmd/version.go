package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/0funct0ry/vessel/internal/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the Vessel version",
	RunE:  runVersion,
}

func init() {
	versionCmd.Flags().Bool("json", false, "Print version info as JSON")
}

func runVersion(cmd *cobra.Command, args []string) error {
	asJSON, err := cmd.Flags().GetBool("json")
	if err != nil {
		return err
	}

	info := version.Get()

	if asJSON {
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "vessel %s (commit %s, built %s)\n", version.String(), info.Commit, info.Date)
	return nil
}
