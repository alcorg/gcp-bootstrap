package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time" // Added import
)

// --- Functions wrapping gcloud commands ---

// projectExists checks if a project exists using gcloud projects list --filter
func projectExists(projectID string) (bool, error) {
	// Use list --filter which relies on list permission the user likely has
	filterArg := fmt.Sprintf("project_id=%s", projectID)
	// Use --quiet to suppress interactive prompts if any were possible
	output, err := runCommandGetOutput("gcloud", "projects", "list", "--filter", filterArg, "--format=value(project_id)", "--quiet")
	if err != nil {
		// Don't treat command failure as definitive "doesn't exist", could be other issues
		// Log the error but proceed as if it might not exist, create will fail if it does
		logWarning("Could not definitively check project existence via 'list --filter': %v", err)
		return false, nil // Let the create command handle existence check more robustly
	}
	// If output is exactly the project ID, it exists
	return output == projectID, nil
}

func createProject(cfg *Config) error {
	logInfo("Attempting to create project '%s'...", cfg.ProjectID)
	exists, err := projectExists(cfg.ProjectID)
	if err != nil {
		// Error during check is logged in projectExists, proceed cautiously
		logWarning("Proceeding with project creation despite check error...")
		// return err // Optionally stop here if check failure is critical
	}
	if exists {
		logInfo("Project '%s' already exists.", cfg.ProjectID)
		return nil
	}

	logInfo("Project '%s' does not appear to exist or check failed, attempting creation...", cfg.ProjectID)
	args := []string{"projects", "create", cfg.ProjectID, "--name", cfg.ProjectName}
	if cfg.OrganizationID != "" {
		args = append(args, "--organization", cfg.OrganizationID)
	}

	err = runCommand("gcloud", args...)
	if err != nil {
		// Check if error is because it already exists (race condition or failed check)
		if strings.Contains(err.Error(), "already exists") {
			logWarning("Project creation failed because project '%s' already exists (likely race condition or failed check). Continuing...", cfg.ProjectID)
			return nil // Treat as non-fatal if it already exists
		}
		return fmt.Errorf("failed to create project: %w", err)
	}
	logInfo("Project '%s' created.", cfg.ProjectID)
	return nil
}

func isBillingLinked(projectID, billingAccountID string) (bool, error) {
	output, err := runCommandGetOutput("gcloud", "beta", "billing", "projects", "describe", projectID, "--format=value(billingAccountName)")
	if err != nil {
		// If describe fails, it might not be linked or another issue occurred
		if strings.Contains(err.Error(), "must be associated with a billing account") {
			return false, nil
		}
		// Handle case where project might not be fully ready after creation
		if strings.Contains(err.Error(), "does not have permission") || strings.Contains(err.Error(), "not found") {
			logWarning("Could not describe project billing yet (may need time after creation or permissions): %v", err)
			return false, nil // Assume not linked yet
		}
		return false, fmt.Errorf("failed to check billing status: %w", err)
	}
	// Extract the ID part (e.g., billingAccounts/0X0X0X-XXXXXX-XXXXXX)
	parts := strings.Split(output, "/")
	if len(parts) > 1 && parts[1] == billingAccountID {
		return true, nil
	}
	return false, nil
}

func linkBilling(cfg *Config) error {
	logInfo("Linking project '%s' to billing account '%s'...", cfg.ProjectID, cfg.BillingAccountID)
	linked, err := isBillingLinked(cfg.ProjectID, cfg.BillingAccountID)
	if err != nil {
		// Error during check is logged in isBillingLinked, proceed cautiously
		logWarning("Proceeding with billing link despite check error...")
	}
	if linked {
		logInfo("Billing account already linked.")
		return nil
	}

	logInfo("Billing account not linked or check failed, attempting link...")
	err = runCommand("gcloud", "beta", "billing", "projects", "link", cfg.ProjectID, "--billing-account", cfg.BillingAccountID)
	if err != nil {
		// Check if error is because it's already linked (race condition or failed check)
		if strings.Contains(err.Error(), "already associated") {
			logWarning("Billing link failed because project '%s' is already linked (likely race condition or failed check). Continuing...", cfg.ProjectID)
			return nil // Treat as non-fatal
		}
		return fmt.Errorf("failed to link billing account: %w", err)
	}
	logInfo("Billing account linked.")
	return nil
}

func enableAPIs(cfg *Config) error {
	logInfo("Enabling essential APIs...")
	if len(cfg.EnableAPIs) == 0 {
		logWarning("No APIs specified in config to enable.")
		return nil
	}
	args := []string{"services", "enable"}
	args = append(args, cfg.EnableAPIs...)
	args = append(args, "--project", cfg.ProjectID)

	// Add --async flag to speed up enablement, as it can take time
	args = append(args, "--async")

	err := runCommand("gcloud", args...)
	if err != nil {
		// API enablement can sometimes have transient issues, log warning but continue
		logWarning("Failed to submit API enablement request (run 'gcloud services list --enabled' later to verify): %v", err)
		return nil // Continue bootstrap even if API enablement fails async
	}
	logInfo("API enablement submitted asynchronously for: %s", strings.Join(cfg.EnableAPIs, ", "))
	logInfo("Note: APIs may take a few minutes to become fully active.")
	return nil
}

func createServiceAccount(cfg *Config) error {
	logInfo("Attempting to create Terraform service account '%s'...", cfg.TFServiceAccountEmail)

	// Add a small delay to allow IAM API propagation after enablement, just in case.
	// APIs were enabled asynchronously. While usually fast, this adds robustness.
	logInfo("Waiting a few seconds for API propagation...")
	time.Sleep(5 * time.Second) // Wait 5 seconds

	// Directly attempt creation. gcloud create will fail if it already exists.
	err := runCommand("gcloud", "iam", "service-accounts", "create", cfg.TFServiceAccountName,
		"--display-name", "Terraform Admin Service Account",
		"--project", cfg.ProjectID)
	if err != nil {
		// Check if the error is because it already exists.
		if strings.Contains(err.Error(), "already exists") {
			logWarning("Service account '%s' already exists. Continuing...", cfg.TFServiceAccountName)
			// If it already exists, we can proceed without error.
			return nil
		}
		// Otherwise, it's a real error during creation.
		return fmt.Errorf("failed to create service account: %w", err)
	}

	// If the command succeeded without error, the SA was created.
	logInfo("Service account '%s' created.", cfg.TFServiceAccountEmail)
	return nil
}

func grantIAMRoles(cfg *Config) error {
	logInfo("Granting IAM roles to '%s'...", cfg.TFServiceAccountEmail)
	member := fmt.Sprintf("serviceAccount:%s", cfg.TFServiceAccountEmail)

	// Grant project roles
	for _, role := range cfg.TFServiceAccountProjectRoles {
		logInfo("Granting project role '%s'...", role)
		err := runCommand("gcloud", "projects", "add-iam-policy-binding", cfg.ProjectID,
			"--member", member,
			"--role", role,
			"--condition=None") // Explicitly set no condition
		// Don't fail immediately, just log warning, maybe role was already granted
		if err != nil {
			logWarning("Failed to grant project role %s (may already exist or permissions issue): %v", role, err)
		}
	}

	// Grant billing role
	if cfg.TFServiceAccountBillingRole != "" {
		logInfo("Granting billing role '%s'...", cfg.TFServiceAccountBillingRole)
		err := runCommand("gcloud", "beta", "billing", "accounts", "add-iam-policy-binding", cfg.BillingAccountID,
			"--member", member,
			"--role", cfg.TFServiceAccountBillingRole)
		if err != nil {
			logWarning("Failed to grant billing role %s (may already exist or permissions issue): %v", cfg.TFServiceAccountBillingRole, err)
		}
	}

	logInfo("IAM role granting process completed (check warnings above).")
	return nil // Return nil even if some bindings failed, as they might already exist
}

func bucketExists(bucketName, projectID string) (bool, error) {
	_, err := runCommandGetOutput("gcloud", "storage", "buckets", "describe", fmt.Sprintf("gs://%s", bucketName), "--project", projectID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check bucket existence: %w", err)
	}
	return true, nil
}

func createBucket(cfg *Config) error {
	bucketURL := fmt.Sprintf("gs://%s", cfg.TFStateBucketName)
	logInfo("Attempting to create GCS bucket '%s'...", bucketURL)
	exists, err := bucketExists(cfg.TFStateBucketName, cfg.ProjectID)
	if err != nil {
		return err
	}
	if exists {
		logInfo("GCS bucket '%s' already exists.", bucketURL)
		return nil
	}

	err = runCommand("gcloud", "storage", "buckets", "create", bucketURL,
		"--project", cfg.ProjectID,
		"--location", cfg.ProjectRegion,
		"--uniform-bucket-level-access")
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			logWarning("Bucket creation failed because bucket '%s' already exists (likely race condition or failed check). Continuing...", bucketURL)
			return nil // Treat as non-fatal
		}
		return fmt.Errorf("failed to create GCS bucket: %w", err)
	}
	logInfo("GCS bucket '%s' created.", bucketURL)
	return nil
}

func isVersioningEnabled(bucketName, projectID string) (bool, error) {
	output, err := runCommandGetOutput("gcloud", "storage", "buckets", "describe", fmt.Sprintf("gs://%s", bucketName), "--format=value(versioning.enabled)", "--project", projectID)
	if err != nil {
		return false, fmt.Errorf("failed to check bucket versioning: %w", err)
	}
	return strings.ToLower(output) == "true", nil
}

func enableBucketVersioning(cfg *Config) error {
	bucketURL := fmt.Sprintf("gs://%s", cfg.TFStateBucketName)
	logInfo("Enabling versioning on GCS bucket '%s'...", bucketURL)
	enabled, err := isVersioningEnabled(cfg.TFStateBucketName, cfg.ProjectID)
	if err != nil {
		return err
	}
	if enabled {
		logInfo("Versioning already enabled.")
		return nil
	}

	err = runCommand("gcloud", "storage", "buckets", "update", bucketURL, "--versioning", "--project", cfg.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to enable versioning: %w", err)
	}
	logInfo("Versioning enabled.")
	return nil
}

func generateSAKey(cfg *Config) error {
	if !cfg.GenerateTFSAKey {
		logInfo("Skipping service account key generation as per config.")
		return nil
	}
	logInfo("Generating service account key...")
	// Ensure the target directory exists if TFSAKeyPath includes directories
	keyDir := filepath.Dir(cfg.TFSAKeyPath)
	if err := os.MkdirAll(keyDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory for SA key '%s': %w", keyDir, err)
	}

	err := runCommand("gcloud", "iam", "service-accounts", "keys", "create", cfg.TFSAKeyPath,
		"--iam-account", cfg.TFServiceAccountEmail,
		"--project", cfg.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to generate service account key: %w", err)
	}
	logWarning("Service account key saved to '%s'. HANDLE THIS FILE SECURELY!", cfg.TFSAKeyPath)
	logWarning("Consider adding it to .gitignore if not already done.")
	logWarning("Using Workload Identity Federation is recommended over keys for CI/CD.")
	return nil
}
