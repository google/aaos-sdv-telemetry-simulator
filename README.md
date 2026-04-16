<!--
  Copyright 2025 Google LLC

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

      http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
-->

# SDV Telemetry Cloud Simulation

## GCP Simulation Infrastructure and Orchestrator

This project sets up a Google Cloud Platform (GCP) infrastructure and orchestrator for running
simulations. It includes configurations for Cloud Functions, Pub/Sub, Firestore, Compute Engine, and
various other GCP services to orchestrate and manage simulation tasks.

### Infrastructure and Orchestration Prerequisites

- Terraform (version ~> 1.10)
- Google Cloud Platform account
- Google Cloud SDK (gcloud CLI)
- Appropriate GCP project permissions
- Go 1.25.4 (for orchestrator functions)

### Project Structure

#### Infrastructure (/infrastructure)

- `main.tf`: main Terraform configuration file
- `provider.tf`: defines the Google Cloud provider configuration and Configures the Terraform
  backend for state storage
- `service_accounts.tf`: sets up various service accounts needed for the project
- `variables.tf`: defines input variables for the Terraform configuration

#### Orchestrator (/orchestrator)

- `delete-simulation/`: Cloud Function for handling simulation deletion
- `finish-simulation/`: Cloud Function for handling simulation finalization
- `receive-request/`: Cloud Function for receiving simulation requests
- `schedule-simulation/`: Cloud Function for scheduling simulations
- `read-simulation/`: Cloud Function for reading simulation information and status from the Database

#### Pipeline Scripts (../pipeline_scripts)

- `deployment_validation/`: Go tool for End-to-End (E2E) validation of the deployment
- `detect_git_changes.sh`: Script used in CI/CD to detect changed components

### Key Components

1. Cloud Functions for simulation orchestration:
   - Receive simulation requests
   - Schedule simulations
   - Finish simulations
   - Delete simulations
   - Read Simulations
2. Pub/Sub topics and subscriptions for event-driven architecture
3. Firestore database for state management
4. Compute Engine instances for running simulations
5. Cloud Scheduler for cleanup tasks
6. Cloud Storage for simulation files (input/output & logcat)

### Terraform Configuration

The infrastructure is primarily configured through Terraform variables. Key variables are defined in
the `infrastructure/variables.tf` file and can be set in a `terraform.tfvars` file or via
command-line flags.

#### Backend Configuration

The Terraform backend is configured to use Google Cloud Storage (GCS) for storing the state. To
customize the backend, create a file named `environments/<environment-name>/backend.hcl` with the
following content:

```hcl
bucket = "your-terraform-state-bucket-name"
prefix = "your/state/prefix"
```

#### Key Variables

- `project_id` (string): Your GCP project ID
- `default_region` (string): Default region for resource deployment (e.g., "us-central1")
- `default_zone` (string): Default zone for resource deployment (e.g., "us-central1-a")
- `location` (string): Location for Cloud Storage buckets
- `location_id` (string): Location ID for Firestore database
- `subnet_name` (string): Name of the subnet to be used for Compute Engine instances
- `agent_docker_image` (string): Docker image for the simulation agent with the SDV build artifacts.
- `supported_agent_builds` (map(string)): A map of supported agent build tags to their corresponding image fingerprints/digests. When not empty, the receive-request function will validate incoming requests against this list. (Optional)

#### Cloud Function Configuration

- `ingress_settings` (string): Controls incoming traffic settings for Cloud Functions. Options
  include "ALLOW_ALL", "ALLOW_INTERNAL_ONLY", or "ALLOW_INTERNAL_AND_GCLB"
- `vpc_connector` (object): Configuration for VPC connector used by Cloud Functions. Includes:
  - `create` (bool): Whether to create a new VPC connector
  - `name` (string): Name of the VPC connector (if not creating a new one)
  - `egress_settings` (string): Egress settings for the VPC connector

#### Service Accounts

The following service accounts are defined in `variables.tf`:

- `receive_request_function_sa` (string): Service account for the receive request function
- `delete_simulation_function_sa` (string): Service account for the delete simulation function
- `simulation_finisher_sa` (string): Service account for the finish simulation function
- `scheduler_trigger_sa` (string): Service account for the Eventarc trigger
- `scheduler_function_sa` (string): Service account for the schedule simulation function
- `simulation_agent_sa` (string): Service account for the simulation compute instances
- `cleanup_scheduler_sa` (string): Service account for the cleanup Cloud Scheduler job

Each of these variables is optional and defaults to `null`. If not specified, Terraform will create
a new service account for each function or purpose.

#### Firestore

- The Firestore database name is derived from the `project_id` with "-simulator" appended

#### Cloud Storage

- A Cloud Storage bucket for simulation files is created with the name derived from the `project_id`

#### Environment Variables

Cloud Functions are configured with the following environment variables:

- `GCP_PROJECT`: Set to the value of `var.project_id`
- `FIRESTORE_DATABASE`: Set to the Firestore database name
- `COMPUTE_ZONE`: Set to the value of `var.default_zone`

#### Example terraform.tfvars

```hcl
project_id       = "my-simulation-project"
default_region   = "us-central1"
default_zone     = "us-central1-a"
location         = "US"
location_id      = "us-central"
subnet_name      = "simulation-subnet"
ingress_settings = "ALLOW_INTERNAL_ONLY"

vpc_connector = {
  create          = true
  name            = "my-vpc-connector"
  egress_settings = "ALL_TRAFFIC"
}

agent_docker_image = "europe-west3-docker.pkg.dev/cloud-telemetry-simulation/simulation/simulation-agent"

supported_agent_builds = {
  "latest" = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
  "0.2.0"  = "sha256:f2ca1bb6c7e907d06dafe4687e579fce76b37e4e93b7605022da52e6ccc26fd2"
}

receive_request_function_sa   = "receive-request-function@my-project.iam.gserviceaccount.com"
delete_simulation_function_sa = "delete-simulation-function@my-project.iam.gserviceaccount.com"
simulation_finisher_sa        = "simulation-finisher@my-project.iam.gserviceaccount.com"
scheduler_trigger_sa          = "scheduler-trigger@my-project.iam.gserviceaccount.com"
scheduler_function_sa         = "scheduler-function@my-project.iam.gserviceaccount.com"
simulation_agent_sa           = "simulation-agent@my-project.iam.gserviceaccount.com"
cleanup_scheduler_sa          = "cleanup-scheduler@my-project.iam.gserviceaccount.com"
```

#### Deployment

1. Configure required variables as described above.
2. Navigate to `infrastructure` folder.
3. Initialize Terraform:

   ```bash
   terraform init -backend-config=environments/<environment-name>/backend.hcl
   ```

4. To dry-run the changes:

   ```bash
   terraform plan --var-file=environments/<environment-name>/variables.tfvars
   ```

5. Apply the configuration:

   ```bash
   terraform apply --var-file=environments/<environment-name>/variables.tfvars
   ```

#### Automated Deployment Configuration

For the automated Cloud Build pipeline, configuration files are fetched from a secure GCS bucket instead of being stored in the repository.
The bucket (specified by \_CONFIG_BUCKET in Cloud Build) must contain the following structure for each environment (prod, staging):

- gs://<CONFIG_BUCKET>/<ENVIRONMENT>/backend.hcl: The backend configuration.
- gs://<CONFIG_BUCKET>/<ENVIRONMENT>/variables.tfvars: The tfvars file containing values for the variables described above.

### Input files

The simulation requires input files to exist in Cloud Storage with the following file structure:

```bash
- cloud-telemetry-simulation-simulation_files/
  - [...]/
    - metrics_config.zip
    - publisher_config.zip
```

- `cloud-telemetry-simulation-simulation_files` is the Cloud Storage Bucket created by Terraform

### Output files

The simulation uploads log files and simulation reports. The file structure looks as follows:

```bash
- cloud-telemetry-simulation-simulation_files/
  - simulations/
    - [autogenerated simulation id]
      - outputs
        - logs/
        — telemetry_simulator_out/
      - inputs/
        - metrics_config.zip
        - publisher_config.zip
```

### API Requests

The simulation system can be interacted with via HTTP requests. Here are some examples using curl:

#### Create a Simulation Request

To create a new simulation request:

```bash
curl -X POST https://[YOUR_CLOUD_FUNCTION_URL]/simulation-orchestrator-receive-request \
  -H "Authorization: Bearer [YOUR_ACCESS_TOKEN]" \
  -H "Content-Type: application/json" \
  -d '{
    "build_id": "latest",
    "file_path": "inputs-123/",
    "instance_type": "n1-standard-8",
    "owner": "user@example.com,
    "max_simulation_time": 60,
    "max_report_count": 1
  }'
```

This request creates a new simulation with the specified variables:

- `build_id`: Docker image tag to the docker image provided in the terraform code.
- `file_path`: Path within the Cloud Storage bucket, which was predefined in terraform, which points to the input files. It should contain metrics_config.zip and publisher_config.zip
- `instance_type`: Google Compute Engine instance type which can run with nested virtualization (e.g. n1)
- `owner`: eemail of the user who owns the simulation
- `max_simulation_time`: maximum time in **seconds** the simulation runs,
- `max_report_count`: maximum number of reports the simulation generates.

The request replies with an generated Simulation ID.

#### Delete a Simulation

To cancel a simulation and delete its VM if existing:

```bash
curl -X POST https://[YOUR_CLOUD_FUNCTION_URL]/simulation-orchestrator-delete-simulation \
 -H "Authorization: Bearer [YOUR_ACCESS_TOKEN]" \
 -H "Content-Type: application/json" \
 -d '{
   "id": "simulation-123",
 }'
```

These curl commands demonstrate basic interactions with the simulation system. Adjust the endpoints and payload structure as necessary based on your specific implementation.

#### Read Simulation Data

To read simulation data, you can interact with the `simulation-reader` function.

**Get the count of currently running simulations:**

```bash
curl -H "Authorization: Bearer $(gcloud auth print-identity-token)" \
  "https://[YOUR_CLOUD_FUNCTION_URL]/simulation-reader/simulations/running/count"
```

Return Format:

```json
{ "count": 0 }
```

**Get the configured maximum number of concurrent running VMs:**

```bash
curl -H "Authorization: Bearer $(gcloud auth print-identity-token)" \
  "https://[YOUR_CLOUD_FUNCTION_URL]/simulation-reader/config/max-running-vms"
```

Return Format:

```json
{ "max_running_vms": 5 }
```

**Get a list of all simulations:**

```bash
curl -H "Authorization: Bearer $(gcloud auth print-identity-token)" \
  "https://[YOUR_CLOUD_FUNCTION_URL]/simulation-reader/simulations"
```

**Get a specific simulation by its ID:**

```bash
curl -H "Authorization: Bearer $(gcloud auth print-identity-token)" \
  "https://[YOUR_CLOUD_FUNCTION_URL]/simulation-reader/simulations/[SIMULATION_ID]"
```

**Filtering and Sorting:**

The list endpoint supports filtering and sorting via query parameters:

- `status`: Filter by status (`running`, `Simulation request received`, `cancelled`, or `completed`).
- `owner`: Filter by the owner's email.
- `sort_by`: Field to sort by (`received_at`, `status_updated_at`, `started_at`). Defaults to `received_at`.
- `sort_order`: `asc` or `desc`. Defaults to `desc`.
- `page_size`: Number of results per page. Defaults to 20.
- `page_token`: Token for fetching the next page of results.

**Example with filtering and sorting:**

```bash
curl -H "Authorization: Bearer $(gcloud auth print-identity-token)" \
  "https://[YOUR_CLOUD_FUNCTION_URL]/simulation-reader/simulations?status=completed&owner=user@example.com&sort_by=status_updated_at&sort_order=asc"
```

**Example with pagination:**

To navigate through large lists of simulations, look for the next_page_token in the response body. Pass this value as the page_token query parameter in your subsequent request to retrieve the next set of results.

```bash
curl -H "Authorization: Bearer $(gcloud auth print-identity-token)" \
  "https://[YOUR_CLOUD_FUNCTION_URL]/simulation-reader/simulations?page_size=10&page_token=[NEXT_PAGE_TOKEN]"
```

### Firestore Indexes

The `read-simulation` function allows for filtering and sorting which requires specific composite indexes in Firestore. If a combination of filtering and sorting is requested for which no index exists, the function will fail.

The necessary indexes are defined in `infrastructure/firestore_indexes.tf`. If you need to support additional filter and sort combinations, you will need to add the corresponding indexes to this file.

#### Response Format

The API returns a JSON object containing an array of simulations and an optional next_page_token.

```json
{
  "simulations": [
    {
      "id": "1234-abcd",
      "owner": "some@email.com",
      "status": "completed",
      "status_updated_at": "2025-12-05T14:50:00.952Z",
      "received_at": "2025-12-05T14:46:43.106Z",
      "started_at": "2025-12-05T14:47:05.848Z",
      "ended_at": "0001-01-01T00:00:00Z",
      "instance_id": "sim-1234-abcd",
      "ip": "10.156.15.230",
      "build_id": "europe-west3-docker.pkg.dev/your-project/simulation/simulation-agent:latest",
      "instance_type": "n1-standard-8",
      "file_path": "gs://your-project-simulation_files/path/",
      "max_simulation_time": 60,
      "max_report_count": 1
    },
    {
      "id": "5678-efgh",
      "owner": "some@email.com",
      "status": "completed",
      "status_updated_at": "2025-11-07T14:49:54.25Z",
      "received_at": "2025-11-07T14:46:54.959Z",
      "started_at": "2025-11-07T14:47:13.714Z",
      "ended_at": "0001-01-01T00:00:00Z",
      "instance_id": "sim-5678-efgh",
      "ip": "10.156.15.221",
      "build_id": "europe-west3-docker.pkg.dev/your-project/simulation/simulation-agent:latest",
      "instance_type": "n1-standard-8",
      "file_path": "gs://your-project-simulation_files/path/",
      "max_simulation_time": 60,
      "max_report_count": 1
    }
  ],
  "next_page_token": "M7bydGsAptLncj8SOCb1"
}
```

## Simulation Agent

The Simulation Agent is a Go application packaged in a Docker container, designed to run on a Compute Engine VM. It orchestrates the entire lifecycle of a single simulation run on the VM.

### Key Features

- **Initialization**: Reads simulation parameters (e.g., `simulation_id`, `document_id`, `finish_function_url`) from the Compute Engine instance metadata.
- **Setup**: Downloads simulation input files from Cloud Storage.
- **Execution**: Launches a Cuttlefish Virtual Device (CVD) and runs the telemetry simulation.
- **Output Handling**: Uploads simulation results and `logcat` output back to Cloud Storage.
- **Finalization**: Calls the `finish-simulation` Cloud Function to report the final status and trigger VM cleanup.

### Agent Prerequisites

- Docker
- Google Cloud Platform account and required VM metadata with Object Storage input files set up
- Service Account with necessary permissions
- SDV build artifacts

### Installation

1. Place `cvd-host_package.tar.gz` and `sdv_core_cf-img-*.zip` in the `simulation-agent/sdv-image-resources` directory
2. In the `simulation-agent` directory:

   ```bash
   docker build -t <cloud artifact registry image path>:latest .
   docker push <cloud artifact registry image path>
   ```

### Usage

The Simulation Agent is not run directly by a user. It is packaged into a Docker container and runs automatically on a Compute Engine VM created by the `schedule-simulation` orchestrator function. The agent's behavior is configured entirely through metadata passed to the VM upon creation.

### Troubleshooting

All generated logs can be found in the Logs Explorer section of GCP. ADB logcat is uploaded to
the Bucket Storage as logcat file.

- Check Firestore logs for simulation status updates
- Verify Object Storage for simulation results
- Review Compute Engine logs for VM-related issues

### Testing

#### Unit Tests

The project currently uses unit tests to ensure the reliability and correctness of individual components. These tests are located alongside the source code files.

#### Mock Generation

We use `mockgen` to generate mock objects for our interfaces, which facilitates easier testing. Mock generation is automated using special comments in the Go files.

For more information about mockgen, visit the official repository: [https://github.com/uber-go/mock](https://github.com/uber-go/mock)

For example, in a file named `deletion.go`, you might find a comment like this:

```go
//go:generate mockgen -source=deletion.go -destination=mock/deletion.go
```

This comment instructs go generate to create a mock for the interfaces defined in deletion.go and place the resulting mock in mock/deletion.go.

#### Running Tests

To run the unit tests, use the following command in the project root:

```bash
go test ./...
```

Generating Mocks
To generate or update mocks, run:

```bash
go generate ./...
```

This command will process all `//go:generate` comments in the project and create or update the mock files accordingly.
