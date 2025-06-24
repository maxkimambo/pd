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

// GenerateInstanceReports creates detailed reports for each instance and an overall summary
func GenerateInstanceReports(reports *MigrationReports) {
	logger.User.Info("=== REPORTING ===")

	if len(reports.Reports) == 0 {
		logger.User.Info("No instance migration results to report.")
		logger.User.Success("Reporting complete")
		return
	}

	// Update summary before reporting
	reports.UpdateSummary()

	// Generate individual instance reports
	generateIndividualInstanceReports(reports)

	// Generate summary report
	generateSummaryReport(reports)

	logger.User.Success("Reporting complete")
}

// generateIndividualInstanceReports creates detailed reports for each instance
func generateIndividualInstanceReports(reports *MigrationReports) {
	fmt.Println("\n=== INDIVIDUAL INSTANCE REPORTS ===")

	// Sort instances by name for consistent output
	instanceNames := make([]string, 0, len(reports.Reports))
	for name := range reports.Reports {
		instanceNames = append(instanceNames, name)
	}
	sort.Strings(instanceNames)

	for _, instanceName := range instanceNames {
		report := reports.Reports[instanceName]
		generateSingleInstanceReport(report)
	}
}

// generateSingleInstanceReport creates a detailed report for a single instance
func generateSingleInstanceReport(report *InstanceMigrationReport) {
	fmt.Printf("\n--- Instance: %s ---\n", report.InstanceName)
	
	// Basic information
	fmt.Printf("Instance ID: %s\n", report.InstanceID)
	fmt.Printf("Zone: %s\n", report.Zone)
	fmt.Printf("Status: %s\n", report.Status)
	
	// Timing information
	if report.StartTime != nil {
		fmt.Printf("Start Time: %s\n", report.StartTime.Format("2006-01-02 15:04:05"))
	}
	if report.EndTime != nil {
		fmt.Printf("End Time: %s\n", report.EndTime.Format("2006-01-02 15:04:05"))
	}
	
	if report.TotalDuration > 0 {
		fmt.Printf("Total Migration Time: %s\n", formatDuration(report.TotalDuration))
	}
	
	if report.DowntimeDuration > 0 {
		fmt.Printf("Instance Downtime: %s\n", formatDuration(report.DowntimeDuration))
	}

	// Phase-by-phase breakdown
	if len(report.Phases) > 0 {
		fmt.Printf("\nPhase Breakdown:\n")
		
		// Define phase order for consistent reporting
		phaseOrder := []MigrationPhase{
			PhaseDiscovery,
			PhasePreparation,
			PhaseMigration,
			PhaseRestore,
			PhaseCleanup,
		}
		
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "  Phase\tStatus\tDuration\tDetails")
		_, _ = fmt.Fprintln(w, "  -----\t------\t--------\t-------")
		
		for _, phase := range phaseOrder {
			if phaseReport, exists := report.Phases[phase]; exists {
				phaseName := getPhaseDisplayName(phase)
				duration := "N/A"
				if phaseReport.Duration > 0 {
					duration = formatDuration(phaseReport.Duration)
				}
				
				details := phaseReport.Details
				if phaseReport.Error != nil {
					details = fmt.Sprintf("%s (Error: %v)", details, phaseReport.Error)
				}
				
				_, _ = fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n", 
					phaseName, phaseReport.Status, duration, details)
			}
		}
		_ = w.Flush()
	}

	// Disk migration results
	if len(report.DiskResults) > 0 {
		fmt.Printf("\nDisk Migration Results:\n")
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "  Disk\tStatus\tNew Disk\tError")
		_, _ = fmt.Fprintln(w, "  ----\t------\t--------\t-----")
		
		for _, diskResult := range report.DiskResults {
			status := "Failed"
			if diskResult.Success {
				status = "Success"
			}
			
			errorMsg := ""
			if diskResult.Error != nil {
				errorMsg = diskResult.Error.Error()
				if len(errorMsg) > 50 {
					errorMsg = errorMsg[:47] + "..."
				}
			}
			
			_, _ = fmt.Fprintf(w, "  %s\t%s\t%s\t%s\n",
				"disk-name", status, diskResult.NewDiskLink, errorMsg)
		}
		_ = w.Flush()
	}

	// Errors
	if len(report.Errors) > 0 {
		fmt.Printf("\nErrors Encountered:\n")
		for i, err := range report.Errors {
			fmt.Printf("  %d. %v\n", i+1, err)
		}
	}

	fmt.Println(strings.Repeat("-", 50))
}

// generateSummaryReport creates an overall summary across all instances
func generateSummaryReport(reports *MigrationReports) {
	fmt.Println("\n=== MIGRATION SUMMARY ===")
	
	summary := reports.Summary
	
	// Overall statistics
	fmt.Printf("Migration Session: %s\n", reports.SessionID)
	if reports.EndTime != nil {
		fmt.Printf("Session Duration: %s\n", formatDuration(summary.TotalSessionTime))
	} else {
		fmt.Printf("Session Duration: %s (ongoing)\n", formatDuration(summary.TotalSessionTime))
	}
	
	fmt.Printf("\nInstance Statistics:\n")
	fmt.Printf("  Total Instances: %d\n", summary.TotalInstances)
	fmt.Printf("  Successful: %d\n", summary.SuccessfulInstances)
	fmt.Printf("  Failed: %d\n", summary.FailedInstances)
	if summary.RunningInstances > 0 {
		fmt.Printf("  Running: %d\n", summary.RunningInstances)
	}
	
	fmt.Printf("\nDisk Statistics:\n")
	fmt.Printf("  Total Disks: %d\n", summary.TotalDisks)
	fmt.Printf("  Successful: %d\n", summary.SuccessfulDisks)
	fmt.Printf("  Failed: %d\n", summary.FailedDisks)
	
	// Timing statistics
	if summary.SuccessfulInstances > 0 {
		fmt.Printf("\nTiming Statistics:\n")
		fmt.Printf("  Average Migration Time: %s\n", formatDuration(summary.AverageMigrationTime))
		fmt.Printf("  Average Instance Downtime: %s\n", formatDuration(summary.AverageDowntime))
		
		if summary.FastestMigration > 0 {
			fmt.Printf("  Fastest Migration: %s\n", formatDuration(summary.FastestMigration))
		}
		if summary.SlowestMigration > 0 {
			fmt.Printf("  Slowest Migration: %s\n", formatDuration(summary.SlowestMigration))
		}
	}
	
	// Success rate
	if summary.TotalInstances > 0 {
		successRate := float64(summary.SuccessfulInstances) / float64(summary.TotalInstances) * 100
		fmt.Printf("\nSuccess Rate: %.1f%%\n", successRate)
	}
	
	fmt.Println(strings.Repeat("=", 50))
}

// getPhaseDisplayName returns a human-readable name for a migration phase
func getPhaseDisplayName(phase MigrationPhase) string {
	switch phase {
	case PhaseDiscovery:
		return "Discovery"
	case PhasePreparation:
		return "Preparation"
	case PhaseMigration:
		return "Migration"
	case PhaseRestore:
		return "Restore"
	case PhaseCleanup:
		return "Cleanup"
	default:
		return fmt.Sprintf("Phase-%d", int(phase))
	}
}

// formatDuration formats a duration in a user-friendly way
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	} else if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	} else if d < time.Hour {
		minutes := int(d.Minutes())
		seconds := int(d.Seconds()) % 60
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	} else {
		hours := int(d.Hours())
		minutes := int(d.Minutes()) % 60
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
}

// GenerateInstanceReport generates a report for a specific instance
func GenerateInstanceReport(instanceName string, reports *MigrationReports) {
	if report, exists := reports.Reports[instanceName]; exists {
		generateSingleInstanceReport(report)
	} else {
		logger.User.Warnf("No report found for instance: %s", instanceName)
	}
}