# GCP Bootstrap

This repository provides a template for bootstrapping a new Google Cloud Platform (GCP) project and preparing it for infrastructure management using Terraform. It automates the creation of essential resources like the project itself, billing linkage, service accounts, and the Terraform state bucket using a **Go program** that orchestrates `gcloud` CLI commands.

**Goal:** Minimize manual steps in the GCP Console and provide a repeatable, scriptable foundation before handing over infrastructure management to Terraform, using Go for control flow and configuration management.

## Prerequisites

Before running the bootstrap program, you **MUST** manually complete these steps in the GCP Console:

1.  **GCP Account:** Ensure you have a Google Account.
2.  **GCP Organization (Recommended):** If using this for a company, ensure your domain is set up with Google Workspace/Cloud Identity and a GCP Organization resource exists. Note your **Organization ID**.
3.  **GCP Billing Account:** Create a GCP Billing Account, configure payment methods, and note the **Billing Account ID**.
4.  **Install Tools:**
    *   **Google Cloud SDK (`gcloud`):** The Go program relies on executing `gcloud` commands in the background. [Installation Guide](https://cloud.google.com/sdk/docs/install)
    *   **Go:** Version 1.18 or later. [Installation Guide](https://go.dev/doc/install)
5.  **Authenticate `gcloud`:** Log in with a user account that has sufficient permissions to create projects, link billing, and manage IAM (e.g., Organization Admin, Project Creator, Billing Admin roles). The Go program uses the credentials `gcloud` is configured with.
    ```bash
    gcloud auth login
    gcloud auth application-default login
    ```

## Usage

1.  **Clone the Repository:**
    ```bash
    # Replace with your actual repository URL
    git clone https://github.com/your-username/gcp-bootstrap-template.git
    cd gcp-bootstrap-template/bootstrap
    ```
2.  **Configure:**
    *   Copy the example configuration: `cp config.yaml.example config.yaml`
    *   Edit `config.yaml` and replace the placeholder values with your actual GCP information (Billing Account ID, desired Project ID, etc.). See comments in the file for details.
3.  **Prepare Go Module:**
    ```bash
    # Run from within the 'bootstrap' directory
    go mod tidy
    ```
4.  **Build the Bootstrap Program (Optional but Recommended):**
    ```bash
    # Run from within the 'bootstrap' directory
    go build -o gcp-bootstrap .
    # Now you can run ./gcp-bootstrap
    ```
    Alternatively, you can run directly using `go run .`
5.  **Run the Bootstrap Program:**
    *   Using the built binary: `./gcp-bootstrap`
    *   Or using go run: `go run .`
    *   To specify a different config file: `./gcp-bootstrap -config /path/to/your/config.yaml`
6.  **Review and Confirm:** The program will display a summary of the configuration and ask for confirmation before making any changes to your GCP environment. Type `yes` to proceed.
7.  **Follow Next Steps:** After successful execution, the program will output the next steps required to configure Terraform (backend, authentication).

## What the Program Does

The Go program (`main.go` and supporting files) performs the following actions by orchestrating `gcloud` commands:

1.  Checks for `gcloud` installation and authentication.
2.  Reads configuration from `config.yaml` (or the path specified by the `-config` flag).
3.  Prompts for user confirmation.
4.  Sets the active `gcloud` project context.
5.  Creates the GCP Project (if it doesn't exist).
6.  Links the Project to the specified Billing Account.
7.  Enables essential GCP APIs specified in the config file (e.g., IAM, Storage, Resource Manager, Service Usage).
8.  Creates a dedicated Service Account for Terraform based on the name in the config.
9.  Grants necessary IAM roles (specified in config) to the Terraform Service Account on the project and billing account.
10. Creates a Google Cloud Storage (GCS) bucket for storing Terraform state.
11. Enables versioning on the GCS bucket.
12. (Optional) Generates and downloads a JSON key for the Terraform Service Account if `generate_tf_sa_key` is set to `true` in the config.

## Security Considerations

*   **Service Account Key (`generate_tf_sa_key: true`):** If you choose to generate a Service Account key, **treat this `.json` file like a password**. Do not commit it to Git. Ensure it's listed in your `.gitignore`. For CI/CD pipelines (like GitHub Actions), using **Workload Identity Federation** is strongly recommended over storing long-lived keys.
*   **IAM Permissions:** Review the roles specified in `tf_service_account_project_roles` and `tf_service_account_billing_role` in `config.yaml`. The example uses `roles/owner` for simplicity during bootstrap. For production environments, follow the **principle of least privilege** and grant only the specific roles needed by Terraform to manage the intended resources (e.g., `roles/storage.admin`, `roles/run.admin`, `roles/cloudsql.admin`, etc.).

## Next Steps After Bootstrap

Once the program completes successfully:

1.  **Configure Terraform Backend:** In your separate Terraform project's code, configure the GCS backend using the bucket name specified in `config.yaml`.
    ```terraform
    terraform {
      backend "gcs" {
        bucket = "your-unique-tfstate-bucket-name-xyz" # Use the name from config.yaml
        prefix = "terraform/state"                   # Optional: path within the bucket
      }
    }
    ```
2.  **Configure Terraform Provider Authentication:** Choose a method:
    *   **Workload Identity Federation (Recommended for CI/CD):** Set up WIF between GCP and your CI/CD provider (e.g., GitHub Actions). Use actions like `google-github-actions/auth` to impersonate the Terraform Service Account created by the bootstrap script.
    *   **Application Default Credentials (Local Dev):** If you ran `gcloud auth application-default login`, Terraform can often pick this up automatically.
    *   **Impersonate Service Account (Local Dev):** Use `gcloud auth application-default login --impersonate-service-account=<TF_SERVICE_ACCOUNT_EMAIL>` (replace with the email address output by the script or found in the console). Terraform will then use the impersonated credentials.
    *   **Service Account Key (Use with caution):** If you generated a key (`generate_tf_sa_key: true`), set the `GOOGLE_APPLICATION_CREDENTIALS` environment variable to the path of the downloaded key file.
3.  **Initialize Terraform:** Run `terraform init` in your Terraform project directory.
4.  **Start Defining Infrastructure:** Write your Terraform code (`.tf` files) to define Cloud Run services, Cloud SQL instances, etc. Remember to use Terraform to enable application-specific APIs (`google_project_service`).
5.  **Deploy:** Run `terraform plan` and `terraform apply`.
