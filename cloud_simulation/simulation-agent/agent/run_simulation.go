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
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	LOGCAT_FILE_PATH       = "/opt/android/cuttlefish_runtime/logs/logcat"
	ANDROID_INFO_FILE_PATH = "/opt/android/android-info.txt"
)

func logStream(rc io.ReadCloser, component, stream string, logger *slog.Logger) {
	defer rc.Close()
	scanner := bufio.NewScanner(rc)
	for scanner.Scan() {
		logger.Info(scanner.Text(), "component", component, "stream", stream)
	}
	if err := scanner.Err(); err != nil {
		logger.Warn("Error reading from stream", "component", component, "stream", stream, "error", err)
	}
}

func (a *agent) runSimulation(configs_dir string, max_simulation_time, max_report_count int) error {
	// Check android-info.txt to decide whether to use an instance name.
	file, err := os.Open(ANDROID_INFO_FILE_PATH)
	if err != nil {
		a.logger.Warn("Could not open android-info.txt, attempting fallback strategy.", "file", ANDROID_INFO_FILE_PATH, "error", err)

		a.logger.Info("Starting simulation attempt with instance name (fallback)...")
		err = a.runSimulationAttempt(configs_dir, max_simulation_time, max_report_count, true)
		if err == nil {
			return nil // Success
		}

		a.logger.Warn("Simulation with instance name failed, trying without instance name as a final fallback.", "error", err)
		return a.runSimulationAttempt(configs_dir, max_simulation_time, max_report_count, false)
	}

	defer file.Close()
	withInstanceName := false
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if scanner.Text() == "config=sdv_core_instance1" {
			withInstanceName = true
			break
		}
	}
	if err := scanner.Err(); err != nil {
		a.logger.Warn("Error reading file, proceeding without instance name.", "file", ANDROID_INFO_FILE_PATH, "error", err)
	}
	if withInstanceName {
		a.logger.Info("Starting simulation attempt with instance name (based on android-info.txt)...")
	} else {
		a.logger.Info("Starting simulation attempt without instance name (based on android-info.txt)...")
	}
	return a.runSimulationAttempt(configs_dir, max_simulation_time, max_report_count, withInstanceName)
}

func (a *agent) runSimulationAttempt(configs_dir string, max_simulation_time, max_report_count int, withInstanceName bool) error {
	a.logger.Info("Starting VM...")
	// Before starting a new Cuttlefish VM, it's good practice to ensure no old one is running.
	// Using `stop_cvd` is the proper way to clean up any lingering processes or resources
	// from a previous run. This is more reliable than `pkill` and addresses the resource
	// conflict errors seen on retries. We ignore the error as it might fail if nothing
	// is running, which is a desired state.
	a.logger.Info("Running `stop_cvd` to ensure a clean state before launching VM...")
	a.adbClient.StopCvd() // We don't check the error, as it's a best-effort cleanup.
	if err := a.startVM(withInstanceName); err != nil {
		return fmt.Errorf("Failed to start VM: %w", err)
	}

	a.logger.Info("Validating metrics config dir")
	metricsDir := configs_dir + "metrics_config"
	if err := a.validateDir(metricsDir); err != nil {
		return fmt.Errorf("Metrics config directory validation failed: %w", err)
	}

	a.logger.Info("Validating publisher config dir")
	publishersDir := configs_dir + "publisher_config"
	if err := a.validateDir(publishersDir); err != nil {
		return fmt.Errorf("Publishers config directory validation failed: %w", err)
	}

	a.logger.Info("Waiting for VM to come online")
	if err := a.waitForVM(); err != nil {
		return fmt.Errorf("VM failed to come online: %w", err)
	}

	a.logger.Info("Pushing metrics config")
	metricsConfigs, err := a.pushMetricsConfigs(metricsDir)
	if err != nil {
		a.logger.Error("Failed to push metrics configs", "error", err)
		return fmt.Errorf("Failed to push metrics configs: %w", err)
	}

	a.logger.Info("Pushing publisher config")
	publisherConfigs, err := a.pushPublisherConfigs(publishersDir)
	if err != nil {
		a.logger.Error("Failed to push publisher configs", "error", err)
		return fmt.Errorf("Failed to push publisher configs: %w", err)
	}

	a.logger.Info("Running Telemetry Simulator")
	simulatorErr := a.runTelemetrySimulator(metricsConfigs, publisherConfigs, max_simulation_time, max_report_count) // Safe the error, and return it after collecting the output files in case there are any.

	a.logger.Info("Collecting output files")
	if err := a.collectOutputFiles(); err != nil {
		return fmt.Errorf("Error collecting simulation results: %w", err)
	}

	if simulatorErr != nil {
		return fmt.Errorf("Error running simulator: %w", simulatorErr)
	}

	return nil
}

func (a *agent) startVM(withInstanceName bool) error {
	if err := a.adbClient.StartServer(); err != nil {
		return fmt.Errorf("Failed to start server: %w", err)
	}
	stdout, stderr, err := a.adbClient.LaunchCvd(withInstanceName)
	if err != nil {
		return fmt.Errorf("Failed to launch_cvd: %w", err)
	}
	go logStream(stdout, "launch_cvd", "stdout", a.logger)
	go logStream(stderr, "launch_cvd", "stderr", a.logger)
	return nil
}

// poll calls condition repeatedly at the given interval until it returns true or timeout expires.
// It returns true if condition succeeded, or false if the timeout was reached.
func poll(timeout, interval time.Duration, condition func(attempt int) bool) bool {
	deadline := time.Now().Add(timeout)
	for attempt := 0; ; attempt++ {
		if condition(attempt) {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(interval)
	}
}

func (a *agent) waitForVM() error {
	booted := poll(5*time.Minute, 1*time.Second, func(attempt int) bool {
		a.logger.Debug("Attempting to connect to VM...", "attempt", attempt)
		if err := a.adbClient.Connect(); err != nil {
			return false
		}

		out, err := a.adbClient.Shell("getprop", "sys.boot_completed")
		return err == nil && out == "1"
	})
	if !booted {
		return fmt.Errorf("timed out waiting for VM to boot")
	}

	a.logger.Info("VM booted successfully, switching adb to root")

	if err := a.adbClient.Root(); err != nil {
		return fmt.Errorf("Failed to restart adb daemon with root permissions: %w", err)
	}

	// Wait for adb to reconnect as root after restarting adbd
	rootReady := poll(1*time.Minute, 1*time.Second, func(attempt int) bool {
		if err := a.adbClient.Connect(); err != nil {
			return false
		}

		out, err := a.adbClient.Shell("whoami")
		return err == nil && out == "root"
	})
	if !rootReady {
		return fmt.Errorf("timed out waiting for adb root reconnection")
	}

	a.logger.Info("Connected to VM as root")
	return nil
}

func (a *agent) validateDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("%s does not seem to exist", dir)
	}

	// Check if the directory is empty
	launchCmd := exec.Command("ls", "-A", dir)
	output, err := launchCmd.Output()
	if err != nil {
		return fmt.Errorf("Error checking directory contents: %w", err)
	}

	if len(output) == 0 {
		return fmt.Errorf("%s does not contain any files", dir)
	}

	return nil
}

func (a *agent) pushMetricsConfigs(metricsDir string) (string, error) {
	var metricsConfigs strings.Builder

	files, err := filepath.Glob(filepath.Join(metricsDir, "*"))
	if err != nil {
		return "", fmt.Errorf("Error globbing files: %w", err)
	}
	sort.Strings(files)
	for _, metricsConfig := range files {
		fileName := filepath.Base(metricsConfig)
		destPath := filepath.Join("/data/local/tmp/", fileName)

		if err := a.adbClient.Push(metricsConfig, destPath); err != nil {
			return "", fmt.Errorf("Error pushing %s: %w", metricsConfig, err)
		}

		if strings.HasSuffix(fileName, ".textproto") || strings.HasSuffix(fileName, ".txtpb") ||
			strings.HasSuffix(fileName, ".bin") || strings.HasSuffix(fileName, ".pb") ||
			strings.HasSuffix(fileName, ".binpb") {
			metricsConfigs.WriteString(destPath + " ")
		}
	}

	return metricsConfigs.String(), nil
}

func (a *agent) pushPublisherConfigs(publishersDir string) (string, error) {
	var publisherConfigs strings.Builder
	files, err := filepath.Glob(filepath.Join(publishersDir, "*"))
	if err != nil {
		return "", fmt.Errorf("Error globbing files: %w", err)
	}
	sort.Strings(files)

	for _, publisherConfig := range files {
		fileName := filepath.Base(publisherConfig)
		destPath := filepath.Join("/data/local/tmp/", fileName)

		if err := a.adbClient.Push(publisherConfig, destPath); err != nil {
			return "", fmt.Errorf("Error pushing %s: %w", publisherConfig, err)
		}

		if strings.HasSuffix(fileName, ".textproto") || strings.HasSuffix(fileName, ".txtpb") ||
			strings.HasSuffix(fileName, ".bin") || strings.HasSuffix(fileName, ".pb") ||
			strings.HasSuffix(fileName, ".binpb") {
			publisherConfigs.WriteString(destPath + " ")
		}
	}

	return publisherConfigs.String(), nil
}

func (a *agent) runTelemetrySimulator(metricsConfigs, publisherConfigs string, max_simulation_time, max_report_count int) error {
	// Create /outputs directory for simulation results with /outputs/logs for logcat.
	if err := os.MkdirAll(filepath.Join(a.outputsDir, "logs"), 0755); err != nil {
		return fmt.Errorf("error creating /outputs/logs/ directory: %w", err)
	}

	logcatStdout, logcatStderr, err := a.adbClient.Logcat()
	if err != nil {
		a.logger.Warn("Failed to start 'adb logcat'. Logcat will be uploaded in the Storage Bucket after the simulation completes.", "error", err)
		// Don't quit and still try running the simulation
		// Logcat will be uploaded to Storage Bucket after the simulation completes
	} else {
		go logStream(logcatStdout, "adb_logcat", "stdout", a.logger)
		go logStream(logcatStderr, "adb_logcat", "stderr", a.logger)
	}

	// Run the simulation and store the error to return it later.
	_, simErr := a.adbClient.Shell(
		"sdv_telemetry_simulator",
		"--max-simulation-time", fmt.Sprintf("seconds:%d", max_simulation_time),
		"full-simulation",
		"--metrics-configs", metricsConfigs,
		"--publisher-configs", publisherConfigs,
		// "--no-binary-reports",
		"--max-report-count", strconv.Itoa(max_report_count),
	)

	if simErr != nil {
		var exitErr *exec.ExitError
		if errors.As(simErr, &exitErr) {
			a.logger.Error("Failed to run telemetry simulator.", "exit_code", exitErr.ExitCode(), "stderr", string(exitErr.Stderr))
		} else {
			a.logger.Error("Failed to run telemetry simulator.", "error", simErr)
		}
		return fmt.Errorf("Error running telemetry simulator: %w", simErr)
	}

	return nil
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()
	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()
	_, err = io.Copy(destination, source)
	if err != nil {
		return err
	}

	return nil
}

func (a *agent) collectOutputFiles() error {
	if err := a.adbClient.Bugreport(filepath.Join(a.outputsDir, "bugreport.zip")); err != nil {
		a.logger.Warn("Error generating bugreport", "error", err)
	}
	if err := a.adbClient.Pull("/data/local/tmp/telemetry_simulator_out/", a.outputsDir); err != nil {
		return fmt.Errorf("Error pulling output files: %w", err)
	}

	// Always try to copy the logcat file.
	logcatDst := filepath.Join(a.outputsDir, "logs", "logcat")
	if err := copyFile(LOGCAT_FILE_PATH, logcatDst); err != nil {
		a.logger.Warn("error copying logcat file", "source", LOGCAT_FILE_PATH, "destination", logcatDst, "error", err)
	}
	return nil
}
