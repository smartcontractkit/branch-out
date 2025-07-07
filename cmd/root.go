// Package cmd provides the CLI for the branch-out application.
package cmd

import (
	"context"
	"os"

	"github.com/charmbracelet/fang"
	"github.com/rs/zerolog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/smartcontractkit/branch-out/config"
	"github.com/smartcontractkit/branch-out/logging"
	"github.com/smartcontractkit/branch-out/server"
)

var (
	appConfig *config.Config
	logger    zerolog.Logger
)

// root is the root command for the CLI.
var root = &cobra.Command{
	Use:   "branch-out",
	Short: "Branch Out accentuates the capabilities of Trunk.io's flaky test tools",
	PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
		var err error

		appConfig, err = config.Load()
		if err != nil {
			return err
		}

		opts := []logging.Option{
			logging.WithLevel(appConfig.LogLevel),
			logging.WithFileName(appConfig.LogPath),
		}

		logger, err = logging.New(opts...)
		return err
	},
	RunE: func(cmd *cobra.Command, _ []string) error {
		srv := server.New(
			server.WithLogger(logger),
			server.WithPort(appConfig.Port),
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
		StringP("github-token", "t", "", "The GitHub token to use for the GitHub API (try using 'gh auth token') (reads from GITHUB_TOKEN environment variable if not provided)")
	root.PersistentFlags().
		StringP("log-level", "l", "", "The log level to use (error, warn, info, debug, trace, disabled)")
	root.PersistentFlags().
		StringP("log-path", "f", "", "Also logs to a file at the given path")

	root.Flags().IntP("port", "p", config.DefaultPort, "The port for the server to listen on")

	// Bind flags to viper
	if err := viper.BindPFlags(root.PersistentFlags()); err != nil {
		panic(err)
	}
	if err := viper.BindPFlags(root.Flags()); err != nil {
		panic(err)
	}
}

// Execute is the entry point for the CLI.
func Execute() {
	if err := fang.Execute(context.Background(), root, fang.WithVersion(config.VersionString())); err != nil {
		os.Exit(1)
	}
}
