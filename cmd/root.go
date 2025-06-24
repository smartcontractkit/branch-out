// Package cmd provides the CLI for the branch-out application.
package cmd

import (
	"context"
	"os"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"

	"github.com/smartcontractkit/branch-out/logging"
	"github.com/smartcontractkit/branch-out/server"
)

// config is the configuration for the CLI.
var config struct {
	WebhookURL string
	Port       int
}

// root is the root command for the CLI.
var root = &cobra.Command{
	Use:   "branch-out",
	Short: "Branch Out is a tool to accentuate the capabilities of Trunk.io's flaky test tools",
	RunE: func(cmd *cobra.Command, _ []string) error {
		logger, err := logging.New()
		if err != nil {
			return err
		}

		server := server.New(
			server.WithLogger(logger),
			server.WithWebhookURL(config.WebhookURL),
			server.WithPort(config.Port),
		)

		return server.Start(cmd.Context())
	},
}

func init() {
	root.Flags().StringVarP(&config.WebhookURL, "webhook-url", "w", "", "The URL to receive webhooks from Trunk.io")
	root.Flags().IntVarP(&config.Port, "port", "p", 8080, "The port for the server to listen on")
}

// Execute is the entry point for the CLI.
func Execute() {
	if err := fang.Execute(context.Background(), root); err != nil {
		os.Exit(1)
	}
}
