// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	compute "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"cloud.google.com/go/firestore"
	functions "cloud.google.com/go/functions/apiv2"
	functionspb "cloud.google.com/go/functions/apiv2/functionspb"
	"cloud.google.com/go/storage"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/iterator"
)

const (
	// defaultCurl and defaultJq are not needed as we use native Go packages.
	// defaultGcloud is not needed as we use the Go Cloud SDK.

	defaultEnvironment  = "staging"
	defaultRegion       = "europe-west3"
	defaultTestFilePath = "e2e-test-inputs/"
	defaultBuildID      = "latest"
	defaultLocalTestDir = ""
	defaultInstanceType = "n1-standard-8"
	defaultTimeout      = 600 * time.Second
)

type config struct {
	projectID         string
	region            string
	serviceURL        string
	webClientID       string
	environment       string
	authToken         string
	buildID           string
	instanceType      string
	testFilePath      string
	owner             string
	localTestFilesDir string
	timeout           time.Duration
}

func defineFlags(cfg *config) {
	flag.StringVar(&cfg.projectID, "project-id", "", "GCP Project ID where the simulation infrastructure is deployed.")
	flag.StringVar(&cfg.region, "region", defaultRegion, "Region for the Cloud Function.")
	flag.StringVar(&cfg.serviceURL, "service-url", "", "Override the Cloud Function URL. Useful for local/proxy testing.")
	flag.StringVar(&cfg.webClientID, "web-client-id", "", "Add a web client id for token audience, if required.")
	flag.StringVar(&cfg.environment, "environment", defaultEnvironment, "The environment stack used to execute the simulation.")
	flag.StringVar(&cfg.authToken, "auth-token", "", "Bearer token for authorization. Defaults to gcloud identity-token.")
	flag.StringVar(&cfg.buildID, "build-id", defaultBuildID, "The agent build ID/tag to test.")
	flag.StringVar(&cfg.instanceType, "instance-type", defaultInstanceType, "GCE instance type for the simulation.")
	flag.StringVar(&cfg.testFilePath, "test-file-path", defaultTestFilePath, "GCS path for test input files.")
	flag.StringVar(&cfg.localTestFilesDir, "local-test-files-dir", defaultLocalTestDir, "Local directory with test files to upload. If set, files will be uploaded to GCS before simulation.")
	flag.StringVar(&cfg.owner, "owner", "", "Owner email for the simulation request.")
	flag.DurationVar(&cfg.timeout, "timeout", defaultTimeout, "Timeout to wait for simulation completion.")
}

func main() {
	var cfg config
	defineFlags(&cfg)

	// Custom usage message
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s --project-id <ID> [OPTIONS]\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Triggers and validates a cloud telemetry simulation.")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
	}

	flag.Parse()

	if err := run(cfg); err != nil {
		log.Fatalf("❌ E2E Test Failed: %v", err)
	}

	log.Println("✅🎉 E2E Test Passed: All checks were successful.")
}

func run(cfg config) error {
	var missing []string
	if cfg.projectID == "" {
		missing = append(missing, "--project-id")
	}
	if cfg.owner == "" {
		missing = append(missing, "--owner")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required flags: %v", missing)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.timeout+2*time.Minute) // Add buffer for cleanup
	defer cancel()

	// --- Get Service URL and Tokens ---
	serviceURL := cfg.serviceURL
	var err error
	if serviceURL == "" {
		log.Println("🔧 No --service-url provided, discovering Cloud Function URL...")
		serviceURL, err = getCloudFunctionURL(ctx, cfg)
		if err != nil {
			return fmt.Errorf("could not retrieve Cloud Function URL. Check --project-id and --region, or provide --service-url: %w", err)
		}
	} else {
		log.Printf("🔧 Using provided --service-url: %s", serviceURL)
	}

	authToken := cfg.authToken
	if authToken == "" {
		log.Println("🔧 No --auth-token provided, generating a new identity token...")
		authToken, err = getIdentityToken(ctx, cfg.webClientID)
		if err != nil {
			return fmt.Errorf("failed to get identity token: %w", err)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	// --- Upload Test Files if local directory is specified ---
	if cfg.localTestFilesDir != "" {
		log.Println("☁️  Uploading test files to Cloud Storage...")
		if err := uploadTestFiles(ctx, cfg); err != nil {
			return fmt.Errorf("failed to upload test files: %w", err)
		}
	}

	// --- Trigger Simulation ---
	log.Println("▶️  Triggering simulation...")
	simulationID, err := triggerSimulation(ctx, serviceURL, authToken, cfg)
	if err != nil {
		return fmt.Errorf("failed to trigger simulation: %w", err)
	}
	log.Printf("✅ Simulation created with ID: %s", simulationID)

	// --- Poll for Completion ---
	log.Printf("⏳ Waiting for simulation to complete... (Timeout: %s, Project: %s)", cfg.timeout, cfg.projectID)
	simulationDetails, finalStatus, err := pollForCompletion(ctx, simulationID, cfg)
	if err != nil {
		return err
	}

	// --- Verification ---
	log.Println("🏁 Simulation finished. Verifying final status...")
	if finalStatus != "completed" {
		log.Printf("Simulation details: %s", simulationDetails)
		return fmt.Errorf("simulation status is '%s'", finalStatus)
	}
	log.Println("✅ Simulation completed. Proceeding with further checks.")

	instanceName := simulationDetails.InstanceID
	if err := verifyInstanceCleanup(ctx, instanceName, cfg); err != nil {
		return err
	}

	if err := verifyResultsExist(ctx, simulationID, cfg); err != nil {
		return err
	}

	return nil
}

func uploadTestFiles(ctx context.Context, cfg config) error {
	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("storage.NewClient: %w", err)
	}
	defer storageClient.Close()

	bucketName := fmt.Sprintf("%s-simulation_files-%s", cfg.projectID, cfg.environment)
	bkt := storageClient.Bucket(bucketName)

	// Check if the local directory exists
	if _, err := os.Stat(cfg.localTestFilesDir); os.IsNotExist(err) {
		return fmt.Errorf("local test files directory '%s' does not exist", cfg.localTestFilesDir)
	}

	err = filepath.Walk(cfg.localTestFilesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil // Skip directories
		}

		// Create a GCS object path that is relative to the directory being walked.
		relPath, err := filepath.Rel(cfg.localTestFilesDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %w", path, err)
		}
		gcsPath := filepath.ToSlash(filepath.Join(cfg.testFilePath, relPath))

		log.Printf("   Uploading %s to gs://%s/%s", path, bucketName, gcsPath)

		// Upload the file
		obj := bkt.Object(gcsPath)
		writer := obj.NewWriter(ctx)
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open local file %s: %w", path, err)
		}
		defer file.Close()

		if _, err := io.Copy(writer, file); err != nil {
			return fmt.Errorf("failed to copy file %s to GCS: %w", path, err)
		}
		return writer.Close() // Important to close the writer to finalize the upload
	})
	return err
}

func getCloudFunctionURL(ctx context.Context, cfg config) (string, error) {
	functionsClient, err := functions.NewFunctionClient(ctx)
	if err != nil {
		return "", fmt.Errorf("functions.NewFunctionClient: %w", err)
	}
	defer functionsClient.Close()

	functionName := fmt.Sprintf("simulation-orchestrator-receive-requests-%s", cfg.environment)
	req := &functionspb.GetFunctionRequest{
		Name: fmt.Sprintf("projects/%s/locations/%s/functions/%s", cfg.projectID, cfg.region, functionName),
	}
	function, err := functionsClient.GetFunction(ctx, req)
	if err != nil {
		return "", fmt.Errorf("functionsClient.GetFunction: %w", err)
	}

	serviceConfig := function.GetServiceConfig()
	if serviceConfig == nil || serviceConfig.GetUri() == "" {
		return "", fmt.Errorf("cloud function '%s' does not have a valid HTTPS trigger URL", functionName)
	}

	return serviceConfig.GetUri(), nil
}

func getIdentityToken(ctx context.Context, audience string) (string, error) {
	// This will use Application Default Credentials.
	// If running locally, you may need to run:
	// gcloud auth application-default login
	tokenSource, err := idtoken.NewTokenSource(ctx, audience)
	if err != nil {
		return "", fmt.Errorf("failed to create identity token source. If running locally, try running 'gcloud auth application-default login'. Original error: %w", err)
	}
	token, err := tokenSource.Token()
	if err != nil {
		return "", fmt.Errorf("failed to get identity token. If running locally, try running 'gcloud auth application-default login'. Original error: %w", err)
	}

	log.Println("🔧 Generated identity token using Application Default Credentials.")
	return token.AccessToken, nil
}

func triggerSimulation(ctx context.Context, serviceURL, authToken string, cfg config) (string, error) {
	reqBody := map[string]interface{}{
		"build_id":            cfg.buildID,
		"file_path":           cfg.testFilePath,
		"instance_type":       cfg.instanceType,
		"owner":               cfg.owner,
		"max_simulation_time": 300,
		"max_report_count":    1,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", serviceURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create http request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+authToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("received non-200 status: %s\nResponse: %s", resp.Status, string(respBody))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to unmarshal response body: %w\nResponse: %s", err, string(respBody))
	}

	if result.ID == "" {
		return "", fmt.Errorf("failed to create simulation, response did not contain an ID. Response: %s", string(respBody))
	}

	return result.ID, nil
}

type SimulationDetails struct {
	ID         string `firestore:"id"`
	Status     string `firestore:"status"`
	InstanceID string `firestore:"instance_id"`
}

func pollForCompletion(ctx context.Context, simulationID string, cfg config) (SimulationDetails, string, error) {
	pollCtx, cancel := context.WithTimeout(ctx, cfg.timeout)
	defer cancel()

	dbID := fmt.Sprintf("cloud-telemetry-simulation-simulator-%s", cfg.environment)
	firestoreClient, err := firestore.NewClientWithDatabase(ctx, cfg.projectID, dbID)
	if err != nil {
		return SimulationDetails{}, "", fmt.Errorf("firestore.NewClientWithDatabase: %w", err)
	}
	defer firestoreClient.Close()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-pollCtx.Done():
			return SimulationDetails{}, "", fmt.Errorf("timed out waiting for simulation to complete")
		case <-ticker.C:
			details, status, err := checkSimulationStatus(pollCtx, firestoreClient, simulationID, cfg)
			if err != nil {
				log.Printf("   Warning: failed to check status: %v. Retrying...", err)
				continue
			}

			if status == "completed" || status == "failed" {
				log.Printf("🏁 Simulation finished with status: %q", status)
				return details, status, nil
			}
			log.Printf("   Current status: %s. Waiting...", status)
		}
	}
}

func checkSimulationStatus(ctx context.Context, client *firestore.Client, simulationID string, cfg config) (SimulationDetails, string, error) {
	query := client.Collection("records").Where("id", "==", simulationID).Limit(1)
	iter := query.Documents(ctx)
	defer iter.Stop()

	doc, err := iter.Next()
	if err == iterator.Done {
		return SimulationDetails{}, "PENDING", nil
	}
	if err != nil {
		return SimulationDetails{}, "", fmt.Errorf("firestore query failed: %w", err)
	}

	var details SimulationDetails
	if err := doc.DataTo(&details); err != nil {
		return SimulationDetails{}, "", fmt.Errorf("failed to decode firestore document: %w", err)
	}

	if details.Status == "" {
		return details, "PENDING", nil
	}

	return details, details.Status, nil
}

func verifyResultsExist(ctx context.Context, simulationID string, cfg config) error {
	log.Println("🔎 Verifying simulation results in Cloud Storage...")
	resultsBucket := fmt.Sprintf("%s-simulation_files-%s", cfg.projectID, cfg.environment)
	prefix := fmt.Sprintf("simulations/%s/outputs/telemetry_simulator_out/", simulationID)

	storageClient, err := storage.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("storage.NewClient: %w", err)
	}
	defer storageClient.Close()

	it := storageClient.Bucket(resultsBucket).Objects(ctx, &storage.Query{Prefix: prefix})
	_, err = it.Next()
	if err == iterator.Done {
		return fmt.Errorf("simulation results not found in gs://%s/%s", resultsBucket, prefix)
	}
	if err != nil {
		return fmt.Errorf("failed to list objects in Cloud Storage: %w", err)
	}

	resultsPath := fmt.Sprintf("gs://%s/%s", resultsBucket, prefix)

	log.Printf("✅ Simulation results found at: %s", resultsPath)
	return nil
}

func verifyInstanceCleanup(ctx context.Context, instanceName string, cfg config) error {
	if instanceName == "" {
		return fmt.Errorf("instance ID is missing from completed simulation data; cannot verify VM cleanup")
	}

	log.Println("🔎 Verifying GCE instance cleanup...")
	cleanupCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cleanupCtx.Done():
			log.Printf("   Timed out waiting for instance %q to be deleted automatically.", instanceName)
			if err := deleteInstance(ctx, instanceName, cfg); err != nil {
				return fmt.Errorf("timed out waiting for instance deletion and failed to manually delete it: %w", err)
			}
			return fmt.Errorf("instance %q was not cleaned up automatically and had to be force-deleted", instanceName)
		case <-ticker.C:
			log.Printf("   Checking for instance %q...", instanceName)
			exists, err := instanceExists(ctx, instanceName, cfg)
			if err != nil {
				log.Printf("   Warning: failed to check for instance: %v. Retrying...", err)
				continue
			}
			if !exists {
				log.Printf("✅ GCE instance '%s' was successfully deleted.", instanceName)
				return nil
			}
			log.Printf("   Instance %q still exists. Waiting...", instanceName)
		}
	}
}

func instanceExists(ctx context.Context, instanceName string, cfg config) (bool, error) {
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return false, fmt.Errorf("compute.NewInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	// GCE API filter syntax is different from gcloud's.
	filter := fmt.Sprintf("name = %q", instanceName)
	req := &computepb.AggregatedListInstancesRequest{
		Project: cfg.projectID,
		Filter:  &filter,
	}

	it := instancesClient.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return false, fmt.Errorf("failed to list GCE instances: %w", err)
		}
		if len(pair.Value.Instances) > 0 {
			return true, nil
		}
	}

	return false, nil
}

func deleteInstance(ctx context.Context, instanceName string, cfg config) error {
	log.Printf("   Attempting to delete instance %q...", instanceName)
	instancesClient, err := compute.NewInstancesRESTClient(ctx)
	if err != nil {
		return fmt.Errorf("compute.NewInstancesRESTClient: %w", err)
	}
	defer instancesClient.Close()

	filter := fmt.Sprintf("name = %q", instanceName)
	req := &computepb.AggregatedListInstancesRequest{
		Project: cfg.projectID,
		Filter:  &filter,
	}

	it := instancesClient.AggregatedList(ctx, req)
	for {
		pair, err := it.Next()
		if err == iterator.Done {
			log.Printf("   Instance %q not found, assuming already deleted.", instanceName)
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to list GCE instances to find zone: %w", err)
		}
		if len(pair.Value.Instances) > 0 {
			instance := pair.Value.Instances[0]
			zone := ""
			if instance.Zone != nil {
				// The zone URL is like "https://www.googleapis.com/compute/v1/projects/p/zones/z"
				// We just need the last part.
				zoneParts := bytes.Split([]byte(*instance.Zone), []byte{'/'})
				zone = string(zoneParts[len(zoneParts)-1])
			}
			if zone == "" {
				return fmt.Errorf("could not determine zone for instance %q", instanceName)
			}

			log.Printf("   Found instance %q in zone %q. Deleting...", instanceName, zone)
			_, err := instancesClient.Delete(ctx, &computepb.DeleteInstanceRequest{Project: cfg.projectID, Zone: zone, Instance: instanceName})
			return err
		}
	}
}
