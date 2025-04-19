package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config holds the application configuration structure, matching config.yaml
type Config struct {
	BillingAccountID string `yaml:"billing_account_id"`
	OrganizationID   string `yaml:"organization_id,omitempty"` // Optional

	ProjectID     string `yaml:"project_id"`
	ProjectName   string `yaml:"project_name"`
	ProjectRegion string `yaml:"project_region"`

	TFStateBucketName string `yaml:"tf_state_bucket_name"`

	TFServiceAccountName string `yaml:"tf_service_account_name"`

	GenerateTFSAKey bool   `yaml:"generate_tf_sa_key"`
	TFSAKeyPath     string `yaml:"tf_sa_key_path"`

	EnableAPIs []string `yaml:"enable_apis"`

	TFServiceAccountProjectRoles []string `yaml:"tf_service_account_project_roles"`
	TFServiceAccountBillingRole  string   `yaml:"tf_service_account_billing_role"`

	// Derived field, not directly from YAML
	TFServiceAccountEmail string `yaml:"-"`
}

// loadConfig reads the YAML configuration file and parses it into the Config struct
func loadConfig(configPath string) (*Config, error) {
	logInfo("Reading configuration from %s...", configPath)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("configuration file not found at %s. Please copy config.yaml.example to config.yaml and fill it out", configPath)
	}

	yamlFile, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading config file %s: %w", configPath, err)
	}

	var cfg Config
	err = yaml.Unmarshal(yamlFile, &cfg)
	if err != nil {
		return nil, fmt.Errorf("error parsing config file %s: %w", configPath, err)
	}

	// Validate required fields
	if cfg.BillingAccountID == "" || cfg.BillingAccountID == "0X0X0X-XXXXXX-XXXXXX" {
		return nil, fmt.Errorf("billing_account_id is not set or is placeholder in %s", configPath)
	}
	if cfg.ProjectID == "" || cfg.ProjectID == "your-unique-project-id" {
		return nil, fmt.Errorf("project_id is not set or is placeholder in %s", configPath)
	}
	if cfg.ProjectName == "" {
		return nil, fmt.Errorf("project_name is not set in %s", configPath)
	}
	if cfg.ProjectRegion == "" {
		return nil, fmt.Errorf("project_region is not set in %s", configPath)
	}
	if cfg.TFStateBucketName == "" || cfg.TFStateBucketName == "your-unique-tfstate-bucket-name-xyz" {
		return nil, fmt.Errorf("tf_state_bucket_name is not set or is placeholder in %s", configPath)
	}
	if cfg.TFServiceAccountName == "" {
		return nil, fmt.Errorf("tf_service_account_name is not set in %s", configPath)
	}
	if len(cfg.EnableAPIs) == 0 {
		logWarning("No APIs listed under 'enable_apis' in config. Ensure essential APIs are enabled.")
	}
	if len(cfg.TFServiceAccountProjectRoles) == 0 {
		return nil, fmt.Errorf("tf_service_account_project_roles list is empty in %s", configPath)
	}
	if cfg.TFServiceAccountBillingRole == "" {
		logWarning("tf_service_account_billing_role is not set in config. Terraform SA won't be able to link other projects to billing.")
	}

	// Derive SA email
	cfg.TFServiceAccountEmail = fmt.Sprintf("%s@%s.iam.gserviceaccount.com", cfg.TFServiceAccountName, cfg.ProjectID)

	logInfo("Configuration loaded successfully.")
	return &cfg, nil
}
