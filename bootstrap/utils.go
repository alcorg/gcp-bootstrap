package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

// logInfo prints an informational message
func logInfo(format string, v ...interface{}) {
	log.Printf("[INFO] "+format+"\n", v...)
}

// logWarning prints a warning message
func logWarning(format string, v ...interface{}) {
	log.Printf("[WARN] "+format+"\n", v...)
}

// logError prints an error message and exits
func logError(format string, v ...interface{}) {
	log.Fatalf("[ERROR] "+format+"\n", v...)
}

// runCommand executes a command and streams its output
func runCommand(name string, args ...string) error {
	logInfo("Executing: %s %s", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		return fmt.Errorf("command failed: %s %s: %w", name, strings.Join(args, " "), err)
	}
	logInfo("Command finished successfully.")
	return nil
}

// runCommandGetOutput executes a command and returns its stdout, suppressing command logs
func runCommandGetOutput(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	outputBytes, err := cmd.Output() // Runs command and captures stdout
	if err != nil {
		// If there's an error, capture stderr as well for better debugging
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		return "", fmt.Errorf("command failed: %s %s: %w\nStderr: %s", name, strings.Join(args, " "), err, stderr)
	}
	return strings.TrimSpace(string(outputBytes)), nil
}

// checkGcloud checks if gcloud exists and is authenticated
func checkGcloud() {
	logInfo("Checking gcloud installation and authentication...")
	_, err := exec.LookPath("gcloud")
	if err != nil {
		logError("'gcloud' command not found in PATH. Please install the Google Cloud SDK: https://cloud.google.com/sdk/docs/install")
	}

	// Check authentication
	output, err := runCommandGetOutput("gcloud", "auth", "list", "--filter=status:ACTIVE", "--format=value(account)")
	if err != nil {
		logError("Failed to check gcloud authentication status: %v. Please run 'gcloud auth login' and 'gcloud auth application-default login'.", err)
	}
	if output == "" {
		logError("Not authenticated to GCP via gcloud. Please run 'gcloud auth login' and 'gcloud auth application-default login'.")
	}
	logInfo("gcloud authenticated as: %s", output)
}

// confirmExecution displays the plan and asks for user confirmation
func confirmExecution(cfg *Config) {
	fmt.Println("-----------------------------------------------------")
	fmt.Println(" GCP Bootstrap Configuration Summary")
	fmt.Println("-----------------------------------------------------")
	fmt.Printf(" Project ID:              %s\n", cfg.ProjectID)
	fmt.Printf(" Project Name:            %s\n", cfg.ProjectName)
	fmt.Printf(" Project Region:          %s\n", cfg.ProjectRegion)
	fmt.Printf(" Billing Account ID:      %s\n", cfg.BillingAccountID)
	if cfg.OrganizationID != "" {
		fmt.Printf(" Organization ID:         %s\n", cfg.OrganizationID)
	}
	fmt.Printf(" TF State Bucket Name:    gs://%s\n", cfg.TFStateBucketName)
	fmt.Printf(" TF Service Account Name: %s\n", cfg.TFServiceAccountName)
	fmt.Printf(" TF Service Account Email:%s\n", cfg.TFServiceAccountEmail)
	fmt.Printf(" Generate TF SA Key:      %t\n", cfg.GenerateTFSAKey)
	if cfg.GenerateTFSAKey {
		fmt.Printf(" TF SA Key Path:          %s\n", cfg.TFSAKeyPath)
	}
	fmt.Printf(" APIs to Enable:          %s\n", strings.Join(cfg.EnableAPIs, ", "))
	fmt.Printf(" TF SA Project Roles:     %s\n", strings.Join(cfg.TFServiceAccountProjectRoles, ", "))
	if cfg.TFServiceAccountBillingRole != "" {
		fmt.Printf(" TF SA Billing Role:      %s\n", cfg.TFServiceAccountBillingRole)
	}
	fmt.Println("-----------------------------------------------------")

	fmt.Print("Proceed with bootstrapping using these settings? (yes/no): ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(input)) != "yes" {
		logInfo("Aborted by user.")
		os.Exit(0)
	}
	logInfo("User confirmed. Starting bootstrap process...")
}
