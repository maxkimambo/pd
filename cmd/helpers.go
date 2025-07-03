package cmd

import (
	"github.com/maxkimambo/pd/internal/migrator"
	"github.com/spf13/cobra"
)

func createMigrationConfig(cmd *cobra.Command) (*migrator.Config, error) {
	projectID, _ := cmd.Flags().GetString("project")
	targetDiskType, _ := cmd.Flags().GetString("target-disk-type")
	labelFilter, _ := cmd.Flags().GetString("label")
	kmsKey, _ := cmd.Flags().GetString("kms-key")
	kmsKeyRing, _ := cmd.Flags().GetString("kms-keyring")
	kmsLocation, _ := cmd.Flags().GetString("kms-location")
	kmsProject, _ := cmd.Flags().GetString("kms-project")
	if kmsProject == "" && kmsKey != "" {
		kmsProject = projectID
	}
	region, _ := cmd.Flags().GetString("region")
	zone, _ := cmd.Flags().GetString("zone")
	autoApprove, _ := cmd.Flags().GetBool("auto-approve")
	concurrency, _ := cmd.Flags().GetInt("concurrency")
	retainName, _ := cmd.Flags().GetBool("retain-name")
	instances, _ := cmd.Flags().GetStringSlice("instances")
	throughput, _ := cmd.Flags().GetInt64("throughput")
	iops, _ := cmd.Flags().GetInt64("iops")
	storagePoolId, _ := cmd.Flags().GetString("storage-pool-id")
	dryRun, _ := cmd.Flags().GetBool("dry-run")

	return &migrator.Config{
		ProjectID:      projectID,
		TargetDiskType: targetDiskType,
		LabelFilter:    labelFilter,
		KmsKey:         kmsKey,
		KmsKeyRing:     kmsKeyRing,
		KmsLocation:    kmsLocation,
		KmsProject:     kmsProject,
		Region:         region,
		Zone:           zone,
		AutoApproveAll: autoApprove,
		Concurrency:    concurrency,
		RetainName:     retainName,
		Instances:      instances,
		Throughput:     throughput,
		Iops:           iops,
		StoragePoolId:  storagePoolId,
		DryRun:         dryRun,
	}, nil
}
