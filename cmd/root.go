package cmd

import (
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	projectID string
	debug     bool
	verbose   bool
	jsonLogs  bool
	quiet     bool
	version   = "v0.1.0"

	rootCmd = &cobra.Command{
		Use:   "pd",
		Short: "A CLI tool for migrating Google Cloud persistent disks",
		Long:  `A CLI tool for bulk migrating Google Cloud persistent disks from one type to another, either detached or attached to GCE instances.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Setup the unified logger
			logger.Setup(verbose || debug, jsonLogs, quiet)

			// Legacy logrus setup (will be removed after full migration)
			if debug {
				logrus.SetLevel(logrus.DebugLevel)
				logrus.Debug("Debug logging enabled")
			} else {
				logrus.SetLevel(logrus.InfoLevel)
			}
			logrus.SetFormatter(&logrus.TextFormatter{
				FullTimestamp: true,
			})
		},
	}
)

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.Version = version
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logging")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	rootCmd.PersistentFlags().BoolVar(&jsonLogs, "json", false, "Output logs in JSON format")
	rootCmd.PersistentFlags().BoolVarP(&quiet, "quiet", "q", false, "Suppress non-error output")

	rootCmd.AddCommand(migrateCmd)
}
