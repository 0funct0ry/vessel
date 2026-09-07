package cmd

import (
	"github.com/spf13/cobra"
)

var webhookCmd = &cobra.Command{
	Use:   "webhook",
	Short: "Manage Vessel webhooks (requires --db)",
}

var webhookListCmd = &cobra.Command{
	Use:   "list",
	Short: "List webhooks",
	Args:  cobra.NoArgs,
	RunE:  requireDB,
}

var webhookTestCmd = &cobra.Command{
	Use:   "test <id>",
	Short: "Fire a synthetic event at a webhook",
	Args:  cobra.ExactArgs(1),
	RunE:  requireDB,
}

func init() {
	addDBFlags(webhookListCmd.Flags())
	addDBFlags(webhookTestCmd.Flags())

	webhookCmd.AddCommand(webhookListCmd, webhookTestCmd)
}
