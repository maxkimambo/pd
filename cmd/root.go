package cmd

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	// Global flags
	projectID string
	debug     bool

	// Root command
	rootCmd = &cobra.Command{
		Use:   "pd",
		Short: "A CLI tool for managing Google Cloud Persistent Disks",
		Long: `pd is a command-line interface to help manage Google Cloud
Persistent Disks, including bulk migrations between disk types.`,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Setup logger based on debug flag
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

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	// Define global flags
	rootCmd.PersistentFlags().StringVarP(&projectID, "project", "p", "", "Google Cloud Project ID (required)")
	rootCmd.PersistentFlags().BoolVar(&debug, "debug", false, "Enable debug logging")

	// Mark project flag as required globally (Cobra doesn't support global required flags directly,
	// validation will happen in individual command's PreRunE or RunE)
	// We can add a PersistentPreRunE to rootCmd later if needed for global validation.
}

// Helper function to exit on error
func exitWithError(err error) {
	logrus.Error(err)
	os.Exit(1)
}

// Helper function for required flags (can be used in command PreRunE/RunE)
func checkRequiredFlags(cmd *cobra.Command, flags []string) error {
	for _, flagName := range flags {
		val, _ := cmd.Flags().GetString(flagName) // Adjust type if not string
		if val == "" {
			return fmt.Errorf("required flag --%s not set", flagName)
		}
	}
	return nil
}
