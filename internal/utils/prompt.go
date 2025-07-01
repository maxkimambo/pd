package utils

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// PromptForConfirmation prompts the user for confirmation before destructive operations.
// If autoApprove is true, it automatically returns true without prompting.
// The action parameter describes what action will be taken (e.g., "delete disk", "stop instance").
// The details parameter provides specific information about the resource (e.g., disk name, instance name).
func PromptForConfirmation(autoApprove bool, action, details string) (bool, error) {
	if autoApprove {
		return true, nil
	}
	warningMsg := fmt.Sprintf(`
	*********************************************************
	   	WARNING. !!!	
	  	About to %s
	  	Details: %s
	*********************************************************
			"Are you sure you want to continue? (yes/no): "
	*********************************************************
`, action, details)
	fmt.Print(warningMsg)

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read user confirmation: %w", err)
	}

	input = strings.ToLower(strings.TrimSpace(input))
	return input == "yes" || input == "y", nil
}

// PromptForMultipleItems prompts the user to confirm action on multiple items.
// If autoApprove is true, it automatically returns true without prompting.
func PromptForMultipleItems(autoApprove bool, action string, items []string) (bool, error) {
	if autoApprove {
		return true, nil
	}

	fmt.Printf("\nWARNING: About to %s the following %d item(s):\n", action, len(items))
	for i, item := range items {
		fmt.Printf("  %d. %s\n", i+1, item)
	}
	fmt.Print("\nAre you sure you want to continue? (yes/no): ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read user confirmation: %w", err)
	}

	input = strings.ToLower(strings.TrimSpace(input))
	return input == "yes" || input == "y", nil
}
