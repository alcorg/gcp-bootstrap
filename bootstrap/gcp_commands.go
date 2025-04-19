package main

import (
	"fmt"
	"strings"
)

// --- Functions wrapping gcloud commands ---

func projectExists(projectID string) (bool, error) {
	_, err := runCommandGetOutput("gcloud", "projects", "describe", projectID)
	if err != nil {
		// Crude check: if error message contains "not found", assume it doesn't exist
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "could not be found") {
			return false, nil
		}
		// Otherwise, it's a real error
		return false, fmt.Errorf("failed to check project existence: %w", err)
	}
	return true, nil
}

func createProject(cfg *Config) error {
	logInfo("Attempting to create project '%s'...", cfg.ProjectID)
	exists, err := projectExists(cfg.ProjectID)
	if err != nil {
		return err
	}
	if exists {
		logInfo("Project '%s' already exists.", cfg.ProjectID)
		return nil
	}

	args := []string{"projects", "create", cfg.ProjectID, "--name", cfg.ProjectName}
	if cfg.OrganizationID != "" {
		args = append(args, "--organization", cfg.OrganizationID)
	}

	err = runCommand("gcloud", args...)
	if err != nil {
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
		return err
	}
	if linked {
		logInfo("Billing account already linked.")
		return nil
	}

	err = runCommand("gcloud", "beta", "billing", "projects", "link", cfg.ProjectID, "--billing-account", cfg.BillingAccountID)
	if err != nil {
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

	err := runCommand("gcloud", args...)
	if err != nil {
		return fmt.Errorf("failed to enable APIs: %w", err)
	}
	logInfo("Essential APIs enabled: %s", strings.Join(cfg.EnableAPIs, ", "))
	return nil
}

func serviceAccountExists(saEmail, projectID string) (bool, error) {
	_, err := runCommandGetOutput("gcloud", "iam", "service-accounts", "describe", saEmail, "--project", projectID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check service account existence: %w", err)
	}
	return true, nil
}

func createServiceAccount(cfg *Config) error {
	logInfo("Attempting to create Terraform service account '%s'...", cfg.TFServiceAccountEmail)
	exists, err := serviceAccountExists(cfg.TFServiceAccountEmail, cfg.ProjectID)
	if err != nil {
		return err
	}
	if exists {
		logInfo("Service account '%s' already exists.", cfg.TFServiceAccountEmail)
		return nil
	}

	err = runCommand("gcloud", "iam", "service-accounts", "create", cfg.TFServiceAccountName,
		"--display-name", "Terraform Admin Service Account",
		"--project", cfg.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to create service account: %w", err)
	}
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
			"--role", role)
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
