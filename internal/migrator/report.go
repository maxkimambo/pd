package migrator

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/maxkimambo/pd/internal/logger"
	"github.com/maxkimambo/pd/internal/utils"
)

// MigrationSummary holds the summary statistics for a migration
type MigrationSummary struct {
	TotalProcessed int
	SuccessCount   int
	FailureCount   int
	Duration       time.Duration
	ResourceType   string // "disk" or "instance"
}

// PrintMigrationSummary prints a consistent summary for any migration type
func PrintMigrationSummary(summary MigrationSummary) {
	report := utils.NewReportBuilder().
		Header("\n[SUMMARY] Migration Summary").
		AddKeyValue(fmt.Sprintf("Total %ss processed", summary.ResourceType), fmt.Sprintf("%d", summary.TotalProcessed)).
		AddKeyValue("Successful migrations", fmt.Sprintf("%d", summary.SuccessCount)).
		AddKeyValue("Failed migrations", fmt.Sprintf("%d", summary.FailureCount))
	
	if summary.Duration > 0 {
		report.AddKeyValue("Total duration", summary.Duration.Round(time.Second).String())
	}
	
	logger.Info(report.Build())
}

// CalculateMigrationSummary calculates summary statistics from migration results
func CalculateMigrationSummary(results []MigrationResult, resourceType string) MigrationSummary {
	summary := MigrationSummary{
		TotalProcessed: len(results),
		ResourceType:   resourceType,
	}
	
	for _, result := range results {
		if result.Status == "Success" {
			summary.SuccessCount++
		} else {
			summary.FailureCount++
		}
		summary.Duration += result.Duration
	}
	
	return summary
}

// PrintCompletionSummary prints a detailed completion summary with next steps
func PrintCompletionSummary(summary MigrationSummary, config *Config, startTime time.Time) {
	// Use the new formatted message box for success message
	successBox := utils.NewBox(utils.SuccessMessage, "Migration completed!").
		AddLine(fmt.Sprintf("%s %ss processed", formatCount(summary.TotalProcessed), summary.ResourceType)).
		AddLine(fmt.Sprintf("Successful migrations: %d", summary.SuccessCount)).
		AddLine(fmt.Sprintf("Failed migrations: %d", summary.FailureCount)).
		AddLine(fmt.Sprintf("Duration: %v", time.Since(startTime).Round(time.Second)))
	
	fmt.Println(successBox.Render())
	
	if summary.SuccessCount > 0 {
		nextStepsBuilder := utils.NewReportBuilder().
			Section("Next steps:")
		
		if summary.ResourceType == "disk" {
			nextStepsBuilder.
				AddBullet("Verify disk performance meets expectations").
				AddBullet("Update any documentation referencing disk types").
				AddBullet("Consider deleting backup snapshots after verification period").
				AddEmptyLine().
				Section("Useful commands:").
				AddBullet(fmt.Sprintf("Check disk status: gcloud compute disks list --project=%s", config.ProjectID)).
				AddBullet("List snapshots: gcloud compute snapshots list --filter=\"name:pd-migrate-*\"")
		} else if summary.ResourceType == "instance" {
			nextStepsBuilder.
				AddBullet("Verify instance performance and connectivity").
				AddBullet("Update monitoring dashboards if disk metrics changed").
				AddBullet("Test application functionality on migrated instances").
				AddEmptyLine().
				Section("Useful commands:").
				AddBullet(fmt.Sprintf("Check instance status: gcloud compute instances list --project=%s", config.ProjectID)).
				AddBullet("SSH to instance: gcloud compute ssh INSTANCE_NAME --zone=ZONE").
				AddBullet("View disk details: gcloud compute disks describe DISK_NAME --zone=ZONE")
		}
		
		fmt.Println(nextStepsBuilder.Build())
	}
	
	if summary.FailureCount > 0 {
		warningBox := utils.NewBox(utils.WarningMessage, "Some migrations failed").
			AddLine(fmt.Sprintf("%d out of %d %s migrations failed", summary.FailureCount, summary.TotalProcessed, summary.ResourceType)).
			AddLine("Check the error report above for details")
		fmt.Println(warningBox.Render())
	}
}

func formatCount(count int) string {
	if count == 1 {
		return "Total"
	}
	return fmt.Sprintf("Total %d", count)
}

func GenerateReports(results []MigrationResult) {
	logger.Starting("Reporting Phase")

	if len(results) == 0 {
		logger.Info("No migration results to report.")
		logger.Success("Reporting phase completed")
		return
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].OriginalDisk < results[j].OriginalDisk
	})

	printSummaryReport(results)
	printDetailedReport(results)

	logger.Success("Reporting phase completed")
}

func printSummaryReport(results []MigrationResult) {
	report := utils.NewReportBuilder().
		Header("[REPORT] Migration Summary")
	
	fmt.Println(report.Build())

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

	statsReport := utils.NewReportBuilder().
		Section("[STATISTICS] Overall Summary").
		AddKeyValue("Total Disks Processed", fmt.Sprintf("%d", len(results))).
		AddKeyValue("Successful Migrations", fmt.Sprintf("%d", successCount)).
		AddKeyValue("Failed Migrations", fmt.Sprintf("%d", failureCount))
	
	if len(results) > 0 {
		avgDuration := totalDuration / time.Duration(len(results))
		statsReport.AddKeyValue("Average Duration/Disk", avgDuration.Round(time.Millisecond).String())
	}
	
	statsReport.AddSeparator()
	fmt.Println(statsReport.Build())
}

func printDetailedReport(results []MigrationResult) {
	report := utils.NewReportBuilder().
		Header("[ERRORS] Detailed Error Report")
	
	fmt.Println(report.Build())
	
	failuresFound := false
	for _, res := range results {
		if strings.HasPrefix(res.Status, "Failed") || (res.Status == "Success" && !res.SnapshotCleaned) {
			failuresFound = true
			
			errorDetails := utils.NewReportBuilder().
				AddEmptyLine().
				AddKeyValue("Disk", fmt.Sprintf("%s (Zone: %s)", res.OriginalDisk, res.Zone)).
				AddIndented(fmt.Sprintf("Status: %s", res.Status), 1)
			
			if res.ErrorMessage != "" {
				errorDetails.AddIndented(fmt.Sprintf("Error Details: %s", res.ErrorMessage), 1)
			}
			if !res.SnapshotCleaned {
				errorDetails.AddIndented(fmt.Sprintf("Snapshot Cleanup: Failed for %s", res.SnapshotName), 1)
			}
			
			fmt.Print(errorDetails.Build())
		}
	}

	if !failuresFound {
		fmt.Println("No detailed errors to report.")
	}
	fmt.Println(strings.Repeat("-", 30))
}
