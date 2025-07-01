package migrator

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/maxkimambo/pd/internal/logger"
)

func GenerateReports(results []MigrationResult) {
	logger.Info("--- Phase 4: Reporting ---")

	if len(results) == 0 {
		logger.Info("No migration results to report.")
		logger.Info("--- Reporting Phase Complete ---")
		return
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].OriginalDisk < results[j].OriginalDisk
	})

	printSummaryReport(results)
	printDetailedReport(results)

	logger.Info("--- Reporting Phase Complete ---")
}

func printSummaryReport(results []MigrationResult) {
	fmt.Println("\n--- Migration Summary Report ---")

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Original Disk\tNew Disk\tZone\tStatus\tDuration\tSnapshot\tCleaned Up\tError")
	fmt.Fprintln(w, "-------------\t--------\t----\t------\t--------\t--------\t----------\t-----")

	successCount := 0
	failureCount := 0
	totalDuration := time.Duration(0)

	for _, res := range results {
		status := res.Status
		errorMsg := ""
		if strings.HasPrefix(status, "Failed") {
			failureCount++
			if len(res.ErrorMessage) > 50 {
				errorMsg = res.ErrorMessage[:47] + "..."
			} else {
				errorMsg = res.ErrorMessage
			}
		} else if status == "Success" {
			successCount++
		}

		cleanedUp := fmt.Sprintf("%t", res.SnapshotCleaned)
		if status == "Success" && !res.SnapshotCleaned {
			cleanedUp += " (!)"
			if errorMsg == "" {
				errorMsg = "Snapshot cleanup failed"
			}
		}

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			res.OriginalDisk,
			res.NewDiskName,
			res.Zone,
			status,
			res.Duration.Round(time.Millisecond).String(),
			res.SnapshotName,
			cleanedUp,
			errorMsg,
		)
		totalDuration += res.Duration
	}

	w.Flush()

	fmt.Printf("\n--- Overall Stats ---\n")
	fmt.Printf("Total Disks Processed: %d\n", len(results))
	fmt.Printf("Successful Migrations: %d\n", successCount)
	fmt.Printf("Failed Migrations:     %d\n", failureCount)
	if len(results) > 0 {
		avgDuration := totalDuration / time.Duration(len(results))
		fmt.Printf("Average Duration/Disk: %s\n", avgDuration.Round(time.Millisecond).String())
	}
	fmt.Println("---------------------")
}

func printDetailedReport(results []MigrationResult) {
	fmt.Println("\n--- Detailed Error Report ---")
	failuresFound := false
	for _, res := range results {
		if strings.HasPrefix(res.Status, "Failed") || (res.Status == "Success" && !res.SnapshotCleaned) {
			failuresFound = true
			fmt.Printf("\nDisk: %s (Zone: %s)\n", res.OriginalDisk, res.Zone)
			fmt.Printf("  Status: %s\n", res.Status)
			if res.ErrorMessage != "" {
				fmt.Printf("  Error Details: %s\n", res.ErrorMessage)
			}
			if !res.SnapshotCleaned {
				fmt.Printf("  Snapshot Cleanup: Failed for %s\n", res.SnapshotName)
			}
		}
	}

	if !failuresFound {
		fmt.Println("No detailed errors to report.")
	}
	fmt.Println("---------------------------")
}
