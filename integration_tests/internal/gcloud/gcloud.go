package gcloud

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Instance struct {
	Name              string   `json:"name"`
	Zone              string   `json:"zone"`
	MachineType       string   `json:"machineType"`
	Status            string   `json:"status"`
	Disks             []Disk   `json:"disks"`
	NetworkInterfaces []struct {
		NetworkIP string `json:"networkIP"`
	} `json:"networkInterfaces"`
}

type Disk struct {
	DeviceName string `json:"deviceName"`
	Source     string `json:"source"`
	Mode       string `json:"mode"`
	Type       string `json:"type"`
}

type DiskInfo struct {
	Name   string            `json:"name"`
	Zone   string            `json:"zone"`
	Type   string            `json:"type"`
	Status string            `json:"status"`
	SizeGb string            `json:"sizeGb"`
	Labels map[string]string `json:"labels"`
	Users  []string          `json:"users"`
}

type Client struct {
	projectID string
}

func NewClient(projectID string) *Client {
	return &Client{
		projectID: projectID,
	}
}

func (c *Client) GetInstance(ctx context.Context, zone, instanceName string) (*Instance, error) {
	cmd := exec.CommandContext(ctx, "gcloud", "compute", "instances", "describe",
		instanceName,
		"--zone", zone,
		"--project", c.projectID,
		"--format", "json")
	
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get instance: %w", err)
	}

	var instance Instance
	if err := json.Unmarshal(output, &instance); err != nil {
		return nil, fmt.Errorf("failed to parse instance data: %w", err)
	}

	return &instance, nil
}

func (c *Client) GetDisk(ctx context.Context, zone, diskName string) (*DiskInfo, error) {
	cmd := exec.CommandContext(ctx, "gcloud", "compute", "disks", "describe",
		diskName,
		"--zone", zone,
		"--project", c.projectID,
		"--format", "json")
	
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get disk: %w", err)
	}

	var disk DiskInfo
	if err := json.Unmarshal(output, &disk); err != nil {
		return nil, fmt.Errorf("failed to parse disk data: %w", err)
	}

	return &disk, nil
}

func (c *Client) GetRegionalDisk(ctx context.Context, region, diskName string) (*DiskInfo, error) {
	cmd := exec.CommandContext(ctx, "gcloud", "compute", "disks", "describe",
		diskName,
		"--region", region,
		"--project", c.projectID,
		"--format", "json")
	
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get regional disk: %w", err)
	}

	var disk DiskInfo
	if err := json.Unmarshal(output, &disk); err != nil {
		return nil, fmt.Errorf("failed to parse disk data: %w", err)
	}

	return &disk, nil
}

func (c *Client) ListDisks(ctx context.Context, zone string, labelFilter string) ([]*DiskInfo, error) {
	args := []string{"compute", "disks", "list",
		"--project", c.projectID,
		"--format", "json"}
	
	if zone != "" {
		args = append(args, "--filter", fmt.Sprintf("zone:(%s)", zone))
	}
	
	if labelFilter != "" {
		args = append(args, "--filter", fmt.Sprintf("labels.%s", labelFilter))
	}
	
	cmd := exec.CommandContext(ctx, "gcloud", args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list disks: %w", err)
	}

	var disks []*DiskInfo
	if err := json.Unmarshal(output, &disks); err != nil {
		return nil, fmt.Errorf("failed to parse disks data: %w", err)
	}

	return disks, nil
}

func (c *Client) StopInstance(ctx context.Context, zone, instanceName string) error {
	cmd := exec.CommandContext(ctx, "gcloud", "compute", "instances", "stop",
		instanceName,
		"--zone", zone,
		"--project", c.projectID)
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop instance: %w", err)
	}

	return nil
}

func (c *Client) StartInstance(ctx context.Context, zone, instanceName string) error {
	cmd := exec.CommandContext(ctx, "gcloud", "compute", "instances", "start",
		instanceName,
		"--zone", zone,
		"--project", c.projectID)
	
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start instance: %w", err)
	}

	return nil
}

func ExtractDiskNameFromSource(source string) string {
	parts := strings.Split(source, "/")
	return parts[len(parts)-1]
}

func ExtractZoneFromPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "zones" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}