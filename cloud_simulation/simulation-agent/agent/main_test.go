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
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"simulator.code/simulation-agent/finisher"
	mock_main "simulator.code/simulation-agent/mock"
)

func TestAgent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	mockStorage := mock_main.NewMockstorageClient(ctrl)
	mockFinisher := mock_main.NewMockfinisherClient(ctrl)
	mockAdb := mock_main.NewMockadbClient(ctrl)

	tempOutputsDir, err := os.MkdirTemp("", "test-output")
	assert.NoError(t, err)
	defer os.RemoveAll(tempOutputsDir)

	simulationID := "sim-123"
	documentID := "doc-xyz"

	agent := &agent{
		storageClient:     mockStorage,
		finisherClient:    mockFinisher,
		adbClient:         mockAdb,
		metadataBaseURL:   "metadata.url",
		simulationID:      simulationID,
		documentID:        documentID,
		finishURL:         "http://finish.url",
		simulationBucket:  "simulationBucket",
		instanceName:      "test-1",
		configsPath:       "../../../testdata/",
		outputsDir:        tempOutputsDir,
		maxSimulationTime: 60,
		maxReportCount:    3,
		logger:            logger,
	}

	ctx := context.Background()
	projectID := "test-project"
	zone := "us-central1-a"

	// setupAndroidInfo creates a temporary android-info.txt file to ensure that
	// the logic to run with an instance name is triggered during tests.
	// It returns a cleanup function.
	setupAndroidInfo := func(t *testing.T) {
		t.Helper()
		tempDir, err := os.MkdirTemp("", "test-setup")
		assert.NoError(t, err)

		// Mock android-info.txt
		tmpAndroidInfoPath := filepath.Join(tempDir, "android-info.txt")
		err = os.WriteFile(tmpAndroidInfoPath, []byte("config=sdv_core_instance1"), 0666)
		assert.NoError(t, err)
		originalAndroidInfoPath := ANDROID_INFO_FILE_PATH
		ANDROID_INFO_FILE_PATH = tmpAndroidInfoPath

		// Mock logcat file to prevent "no such file or directory" warnings.
		logcatDir := filepath.Join(tempDir, "cuttlefish_runtime", "logs")
		err = os.MkdirAll(logcatDir, 0755)
		assert.NoError(t, err)
		tmpLogcatPath := filepath.Join(logcatDir, "logcat")
		err = os.WriteFile(tmpLogcatPath, []byte("logcat content"), 0666)
		assert.NoError(t, err)
		originalLogcatPath := LOGCAT_FILE_PATH
		LOGCAT_FILE_PATH = tmpLogcatPath

		t.Cleanup(func() {
			ANDROID_INFO_FILE_PATH = originalAndroidInfoPath
			LOGCAT_FILE_PATH = originalLogcatPath
			os.RemoveAll(tempDir)
		})
	}

	t.Run("executeSimulation success", func(t *testing.T) {
		setupAndroidInfo(t)
		mockStorage.EXPECT().Download(ctx, "gs://simulationBucket/simulations/"+simulationID+"/inputs/").Return(nil)

		mockAdb.EXPECT().StopCvd().Return(nil)
		mockAdb.EXPECT().StartServer().Return(nil)
		mockAdb.EXPECT().LaunchCvd(true).Return(
			io.NopCloser(strings.NewReader("")),
			io.NopCloser(strings.NewReader("")),
			nil,
		)
		mockAdb.EXPECT().Connect().Return(nil)
		mockAdb.EXPECT().Shell("getprop", "sys.boot_completed").Return("1", nil)
		mockAdb.EXPECT().Root().Return(nil)
		mockAdb.EXPECT().Connect().Return(nil)
		mockAdb.EXPECT().Shell("whoami").Return("root", nil)
		mockAdb.EXPECT().Push("../../../testdata/metrics_config/average_speed.textproto", "/data/local/tmp/average_speed.textproto").Return(nil)
		mockAdb.EXPECT().Push("../../../testdata/metrics_config/average_speed_vector.textproto", "/data/local/tmp/average_speed_vector.textproto").Return(nil)
		mockAdb.EXPECT().Push("../../../testdata/metrics_config/journey_summary.textproto", "/data/local/tmp/journey_summary.textproto").Return(nil)
		mockAdb.EXPECT().Push("../../../testdata/publisher_config/error_publisher_config.textproto", "/data/local/tmp/error_publisher_config.textproto").Return(nil)
		mockAdb.EXPECT().Push("../../../testdata/publisher_config/error_publisher_data.csv", "/data/local/tmp/error_publisher_data.csv").Return(nil)
		mockAdb.EXPECT().Push("../../../testdata/publisher_config/speed_publisher_config.textproto", "/data/local/tmp/speed_publisher_config.textproto").Return(nil)
		mockAdb.EXPECT().Push("../../../testdata/publisher_config/speed_publisher_data.csv", "/data/local/tmp/speed_publisher_data.csv").Return(nil)
		mockAdb.EXPECT().Logcat().Return(
			io.NopCloser(strings.NewReader("")),
			io.NopCloser(strings.NewReader("")),
			nil,
		)
		mockAdb.EXPECT().Shell(
			"sdv_telemetry_simulator",
			"--max-simulation-time", "seconds:60",
			"full-simulation",
			"--metrics-configs", "/data/local/tmp/average_speed.textproto /data/local/tmp/average_speed_vector.textproto /data/local/tmp/journey_summary.textproto ",
			"--publisher-configs", "/data/local/tmp/error_publisher_config.textproto /data/local/tmp/speed_publisher_config.textproto ",
			"--max-report-count", "3")
		mockAdb.EXPECT().Bugreport(filepath.Join(agent.outputsDir, "bugreport.zip")).Return(nil)
		mockAdb.EXPECT().Pull("/data/local/tmp/telemetry_simulator_out/", agent.outputsDir)

		mockStorage.EXPECT().Upload(ctx, "gs://simulationBucket/simulations/"+simulationID+"/outputs/", agent.outputsDir).Return(nil)

		expectedPayload := finisher.Payload{
			ID:         simulationID,
			ProjectID:  projectID,
			Zone:       zone,
			InstanceID: agent.instanceName,
			DocumentID: agent.documentID,
			Status:     "completed",
		}
		mockFinisher.EXPECT().Finish(ctx, agent.finishURL, expectedPayload).Return(nil)

		err = agent.executeSimulation(ctx, projectID, zone)

		assert.NoError(t, err)
	})

	t.Run("executeSimulation fails on Download", func(t *testing.T) {
		mockStorage.EXPECT().Download(ctx, "gs://simulationBucket/simulations/"+simulationID+"/inputs/").Return(assert.AnError)

		expectedPayload := finisher.Payload{
			ID:         simulationID,
			ProjectID:  projectID,
			Zone:       zone,
			InstanceID: agent.instanceName,
			DocumentID: agent.documentID,
			Status:     "failed",
		}
		mockFinisher.EXPECT().Finish(ctx, agent.finishURL, expectedPayload).Return(nil)

		err = agent.executeSimulation(ctx, projectID, zone)

		assert.Error(t, err)
	})

	t.Run("executeSimulation fails on starting cvd and reports failure", func(t *testing.T) {
		setupAndroidInfo(t)
		mockStorage.EXPECT().Download(ctx, "gs://simulationBucket/simulations/"+simulationID+"/inputs/").Return(nil)

		mockAdb.EXPECT().StopCvd().Return(nil)
		mockAdb.EXPECT().StartServer().Return(nil)
		mockAdb.EXPECT().LaunchCvd(true).Return(nil, nil, assert.AnError)

		mockStorage.EXPECT().Upload(ctx, "gs://simulationBucket/simulations/"+simulationID+"/outputs/", agent.outputsDir).Return(nil)

		expectedPayload := finisher.Payload{
			ID:         simulationID,
			ProjectID:  projectID,
			Zone:       zone,
			InstanceID: agent.instanceName,
			DocumentID: agent.documentID,
			Status:     "failed",
		}
		mockFinisher.EXPECT().Finish(ctx, agent.finishURL, expectedPayload).Return(nil)

		err = agent.executeSimulation(ctx, projectID, zone)

		assert.Error(t, err)
	})

	t.Run("executeSimulation fails on adb push", func(t *testing.T) {
		setupAndroidInfo(t)
		mockStorage.EXPECT().Download(ctx, "gs://simulationBucket/simulations/"+simulationID+"/inputs/").Return(nil)

		// The simulation attempt fails on push.
		mockAdb.EXPECT().StopCvd().Return(nil)
		mockAdb.EXPECT().StartServer().Return(nil)
		mockAdb.EXPECT().LaunchCvd(true).Return(
			io.NopCloser(strings.NewReader("")),
			io.NopCloser(strings.NewReader("")),
			nil,
		)
		mockAdb.EXPECT().Connect().Return(nil)
		mockAdb.EXPECT().Shell("getprop", "sys.boot_completed").Return("1", nil)
		mockAdb.EXPECT().Root().Return(nil)
		mockAdb.EXPECT().Connect().Return(nil)
		mockAdb.EXPECT().Shell("whoami").Return("root", nil)
		mockAdb.EXPECT().Push("../../../testdata/metrics_config/average_speed.textproto", "/data/local/tmp/average_speed.textproto").Return(assert.AnError)

		mockStorage.EXPECT().Upload(ctx, "gs://simulationBucket/simulations/"+simulationID+"/outputs/", agent.outputsDir).Return(nil)

		expectedPayload := finisher.Payload{
			ID:         simulationID,
			ProjectID:  projectID,
			Zone:       zone,
			InstanceID: agent.instanceName,
			DocumentID: agent.documentID,
			Status:     "failed",
		}
		mockFinisher.EXPECT().Finish(ctx, agent.finishURL, expectedPayload).Return(nil)

		err = agent.executeSimulation(ctx, projectID, zone)

		assert.Error(t, err)
	})

	t.Run("executeSimulation fails on adb root", func(t *testing.T) {
		setupAndroidInfo(t)
		mockStorage.EXPECT().Download(ctx, "gs://simulationBucket/simulations/"+simulationID+"/inputs/").Return(nil)

		mockAdb.EXPECT().StopCvd().Return(nil)
		mockAdb.EXPECT().StartServer().Return(nil)
		mockAdb.EXPECT().LaunchCvd(true).Return(
			io.NopCloser(strings.NewReader("")),
			io.NopCloser(strings.NewReader("")),
			nil,
		)
		mockAdb.EXPECT().Connect().Return(nil)
		mockAdb.EXPECT().Shell("getprop", "sys.boot_completed").Return("1", nil)
		mockAdb.EXPECT().Root().Return(assert.AnError)

		mockStorage.EXPECT().Upload(ctx, "gs://simulationBucket/simulations/"+simulationID+"/outputs/", agent.outputsDir).Return(nil)

		expectedPayload := finisher.Payload{
			ID:         simulationID,
			ProjectID:  projectID,
			Zone:       zone,
			InstanceID: agent.instanceName,
			DocumentID: agent.documentID,
			Status:     "failed",
		}
		mockFinisher.EXPECT().Finish(ctx, agent.finishURL, expectedPayload).Return(nil)

		err = agent.executeSimulation(ctx, projectID, zone)

		assert.Error(t, err)
	})

	t.Run("executeSimulation fails on simulator error", func(t *testing.T) {
		setupAndroidInfo(t)
		mockStorage.EXPECT().Download(ctx, "gs://simulationBucket/simulations/"+simulationID+"/inputs/").Return(nil)

		mockAdb.EXPECT().StopCvd().Return(nil)
		mockAdb.EXPECT().StartServer().Return(nil)
		mockAdb.EXPECT().LaunchCvd(true).Return(
			io.NopCloser(strings.NewReader("")),
			io.NopCloser(strings.NewReader("")),
			nil,
		)
		mockAdb.EXPECT().Connect().Return(nil)
		mockAdb.EXPECT().Shell("getprop", "sys.boot_completed").Return("1", nil)
		mockAdb.EXPECT().Root().Return(nil)
		mockAdb.EXPECT().Connect().Return(nil)
		mockAdb.EXPECT().Shell("whoami").Return("root", nil)
		mockAdb.EXPECT().Push("../../../testdata/metrics_config/average_speed.textproto", "/data/local/tmp/average_speed.textproto").Return(nil)
		mockAdb.EXPECT().Push("../../../testdata/metrics_config/average_speed_vector.textproto", "/data/local/tmp/average_speed_vector.textproto").Return(nil)
		mockAdb.EXPECT().Push("../../../testdata/metrics_config/journey_summary.textproto", "/data/local/tmp/journey_summary.textproto").Return(nil)
		mockAdb.EXPECT().Push("../../../testdata/publisher_config/error_publisher_config.textproto", "/data/local/tmp/error_publisher_config.textproto").Return(nil)
		mockAdb.EXPECT().Push("../../../testdata/publisher_config/error_publisher_data.csv", "/data/local/tmp/error_publisher_data.csv").Return(nil)
		mockAdb.EXPECT().Push("../../../testdata/publisher_config/speed_publisher_config.textproto", "/data/local/tmp/speed_publisher_config.textproto").Return(nil)
		mockAdb.EXPECT().Push("../../../testdata/publisher_config/speed_publisher_data.csv", "/data/local/tmp/speed_publisher_data.csv").Return(nil)
		mockAdb.EXPECT().Logcat().Return(
			io.NopCloser(strings.NewReader("")),
			io.NopCloser(strings.NewReader("")),
			nil,
		)
		mockAdb.EXPECT().Shell(
			"sdv_telemetry_simulator",
			"--max-simulation-time", "seconds:60",
			"full-simulation",
			"--metrics-configs", "/data/local/tmp/average_speed.textproto /data/local/tmp/average_speed_vector.textproto /data/local/tmp/journey_summary.textproto ",
			"--publisher-configs", "/data/local/tmp/error_publisher_config.textproto /data/local/tmp/speed_publisher_config.textproto ",
			"--max-report-count", "3").Return("", assert.AnError)
		mockAdb.EXPECT().Bugreport(filepath.Join(agent.outputsDir, "bugreport.zip")).Return(nil)
		mockAdb.EXPECT().Pull("/data/local/tmp/telemetry_simulator_out/", agent.outputsDir).Return(nil)

		mockStorage.EXPECT().Upload(ctx, "gs://simulationBucket/simulations/"+simulationID+"/outputs/", agent.outputsDir).Return(nil)

		expectedPayload := finisher.Payload{
			ID:         simulationID,
			ProjectID:  projectID,
			Zone:       zone,
			InstanceID: agent.instanceName,
			DocumentID: agent.documentID,
			Status:     "failed",
		}
		mockFinisher.EXPECT().Finish(ctx, agent.finishURL, expectedPayload).Return(nil)

		err = agent.executeSimulation(ctx, projectID, zone)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), assert.AnError.Error())
	})
}

func TestPoll(t *testing.T) {
	t.Run("immediate success", func(t *testing.T) {
		calls := 0
		ok := poll(1*time.Second, 10*time.Millisecond, func(attempt int) bool {
			calls++
			return true
		})
		assert.True(t, ok)
		assert.Equal(t, 1, calls)
	})

	t.Run("success after retries", func(t *testing.T) {
		calls := 0
		ok := poll(1*time.Second, 10*time.Millisecond, func(attempt int) bool {
			calls++
			return attempt >= 2
		})
		assert.True(t, ok)
		assert.Equal(t, 3, calls)
	})

	t.Run("timeout", func(t *testing.T) {
		calls := 0
		ok := poll(50*time.Millisecond, 10*time.Millisecond, func(attempt int) bool {
			calls++
			return false
		})
		assert.False(t, ok)
		assert.GreaterOrEqual(t, calls, 2)
	})
}
