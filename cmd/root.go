// Package cmd provides the CLI for the branch-out application.
package cmd

import (
	"context"
	"os"

	"github.com/charmbracelet/fang"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"

	"github.com/smartcontractkit/branch-out/logging"
	"github.com/smartcontractkit/branch-out/server"
)

// DefaultPort is the default port for the server to listen on.
const DefaultPort = 8181

var (
	logger zerolog.Logger

	githubToken string
	logLevel    string

	port int
)

// root is the root command for the CLI.
var root = &cobra.Command{
	Use:   "branch-out",
	Short: "Branch Out accentuates the capabilities of Trunk.io's flaky test tools",
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		if githubToken == "" {
			githubToken = os.Getenv("GITHUB_TOKEN")
		}
		var err error
		logger, err = logging.New(logging.WithLevel(logLevel))
		if err != nil {
			return err
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		srv := server.New(
			server.WithLogger(logger),
			server.WithPort(port),
			server.WithVersion(Version()),
		)
		err := srv.Start(cmd.Context())
		if err != nil {
			logger.Error().Err(err).Msg("Server failure")
		}
		return err
	},
}

func init() {
	root.PersistentFlags().
		StringVarP(&githubToken, "github-token", "t", "", "The GitHub token to use for the GitHub API (try using 'gh auth token') (reads from GITHUB_TOKEN environment variable if not provided)")
	root.PersistentFlags().
		StringVarP(&logLevel, "log-level", "l", "info", "The log level to use (error, warn, info, debug, trace, disabled)")

	root.Flags().IntVarP(&port, "port", "p", DefaultPort, "The port for the server to listen on")
}

// Execute is the entry point for the CLI.
func Execute() {
	if err := fang.Execute(context.Background(), root, fang.WithVersion(Version())); err != nil {
		os.Exit(1)
	}
}
