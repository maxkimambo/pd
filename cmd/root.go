package cmd

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	projectID string
	debug     bool

	rootCmd = &cobra.Command{
		Use:   "pd",
		Short: "A CLI tool for managing Google Cloud Persistent Disks",
		Long: `pd is a command-line interface to help manage Google Cloud
Persistent Disks, including bulk migrations between disk types.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
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
	rootCmd.PersistentFlags().StringVarP(&projectID, "project", "p", "", "Google Cloud Project ID (required)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logging")

	// Add other commands like convertCmd here
	// e.g., rootCmd.AddCommand(convertCmd)
	// The gceConvertCmd will be added in its own init() function in gce_convert.go
}

func exitWithError(err error) {
	logrus.Error(err)
	os.Exit(1)
}

func checkRequiredFlags(cmd *cobra.Command, flags []string) error {
	for _, flagName := range flags {
		val, _ := cmd.Flags().GetString(flagName)
		if val == "" {
			return fmt.Errorf("required flag --%s not set", flagName)
		}
	}
	return nil
}
