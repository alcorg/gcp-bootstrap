package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const defaultConfigFilename = "config.yaml"

func main() {
	// Allow specifying config file path via flag
	configPath := flag.String("config", defaultConfigFilename, "Path to the configuration YAML file")
	flag.Parse()

	// Determine absolute path if relative path is given
	if !filepath.IsAbs(*configPath) {
		cwd, err := os.Getwd()
		if err != nil {
			logError("Failed to get current working directory: %v", err)
		}
		*configPath = filepath.Join(cwd, *configPath)
	}

	// --- Prerequisites ---
	checkGcloud() // Check gcloud exists and is authenticated

	// --- Load Config ---
	cfg, err := loadConfig(*configPath)
	if err != nil {
		logError("Failed to load configuration: %v", err)
	}

	// --- Confirm ---
	confirmExecution(cfg) // Show summary and ask user to proceed

	// --- Execute Bootstrap Steps ---
	logInfo("Starting GCP bootstrap...")

	// Set project context for subsequent gcloud commands
	err = runCommand("gcloud", "config", "set", "project", cfg.ProjectID)
	if err != nil {
		logError("Failed to set gcloud project context: %v", err)
	}

	// Execute steps sequentially
	if err := createProject(cfg); err != nil {
		logError("Bootstrap failed during project creation: %v", err)
	}
	if err := linkBilling(cfg); err != nil {
		logError("Bootstrap failed during billing linking: %v", err)
	}
	if err := enableAPIs(cfg); err != nil {
		logError("Bootstrap failed during API enablement: %v", err)
	}
	if err := createServiceAccount(cfg); err != nil {
		logError("Bootstrap failed during service account creation: %v", err)
	}
	if err := grantIAMRoles(cfg); err != nil {
		// Log error but don't necessarily exit, roles might exist
		logWarning("Potential issue during IAM role granting: %v", err)
	}
	if err := createBucket(cfg); err != nil {
		logError("Bootstrap failed during GCS bucket creation: %v", err)
	}
	if err := enableBucketVersioning(cfg); err != nil {
		logError("Bootstrap failed during bucket versioning enablement: %v", err)
	}
	if err := generateSAKey(cfg); err != nil {
		logError("Bootstrap failed during service account key generation: %v", err)
	}

	// --- Completion Message ---
	logInfo("GCP bootstrap process completed successfully!")
	fmt.Println("-----------------------------------------------------")
	fmt.Println(" Next Steps:")
	fmt.Printf(" 1. Configure your Terraform backend ('backend \"gcs\" {}') using bucket: %s\n", cfg.TFStateBucketName)
	fmt.Println(" 2. Configure Terraform GCP provider authentication:")
	if cfg.GenerateTFSAKey {
		fmt.Printf("    - Using generated key: export GOOGLE_APPLICATION_CREDENTIALS=\"%s\"\n", cfg.TFSAKeyPath)
	}
	fmt.Println("    - Using your user credentials (for local dev): 'gcloud auth application-default login'")
	fmt.Printf("    - Using impersonation (local dev): 'gcloud auth application-default login --impersonate-service-account=%s'\n", cfg.TFServiceAccountEmail)
	fmt.Println("    - Using Workload Identity Federation (Recommended for CI/CD): Configure WIF pool/provider and use 'google-github-actions/auth'.")
	fmt.Println(" 3. Run 'terraform init' and then 'terraform apply' to deploy your infrastructure.")
	fmt.Println("-----------------------------------------------------")
}
