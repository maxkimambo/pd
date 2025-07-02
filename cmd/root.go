package cmd

import (
	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/validation"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	debug    bool
	verbose  bool
	jsonLogs bool
	quiet    bool
	version  = "v0.1.0"

	rootCmd = &cobra.Command{
		Use:   "pd",
		Short: "A CLI tool for migrating Google Cloud persistent disks",
		Long:  `A CLI tool for bulk migrating Google Cloud persistent disks from one type to another, either detached or attached to GCE instances.`,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
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

			// Skip validation for non-migrate commands
			if cmd.Name() == "pd" || cmd.Parent() == nil || cmd.Parent().Name() != "migrate" {
				return nil
			}

			// Validate shared flags
			projectID, _ := cmd.Flags().GetString("project")
			if err := validation.ValidateProjectID(projectID); err != nil {
				return err
			}

			throughput, _ := cmd.Flags().GetInt64("throughput")
			if err := validation.ValidateThroughput(throughput); err != nil {
				return err
			}

			iops, _ := cmd.Flags().GetInt64("iops")
			if err := validation.ValidateIOPS(iops); err != nil {
				return err
			}

			return nil
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
