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

package adb

import (
	"io"
	"os/exec"
	"strings"
)

var (
	ADB_PATH        = "/opt/android/bin/adb"
	LAUNCH_CVD_PATH = "/opt/android/bin/launch_cvd"
	STOP_CVD_PATH   = "/opt/android/bin/stop_cvd"
)

type Client interface {
	StartServer() error
	LaunchCvd(withInstanceName bool) (io.ReadCloser, io.ReadCloser, error)
	StopCvd() error
	Connect() error
	Shell(args ...string) (string, error)
	Root() error
	Push(localSrc, deviceDst string) error
	Pull(deviceSrc, localDst string) error
	Logcat() (io.ReadCloser, io.ReadCloser, error)
	Bugreport(filePath string) error
}

type client struct{}

func NewClient() (Client, error) {
	return &client{}, nil
}

func (ac client) StartServer() error {
	startCmd := exec.Command(ADB_PATH, "start-server")
	return startCmd.Run()
}

func (ac client) LaunchCvd(withInstanceName bool) (io.ReadCloser, io.ReadCloser, error) {
	args := []string{
		"--report_anonymous_usage_stats=n",
		"--daemon",
	}
	if withInstanceName {
		args = append(args, "--extra_bootconfig_args=androidboot.sdv.instance_name=instance1")
	}
	launchCmd := exec.Command(LAUNCH_CVD_PATH, args...)

	stdout, err := launchCmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	stderr, err := launchCmd.StderrPipe()
	if err != nil {
		return nil, nil, err
	}

	err = launchCmd.Start()
	return stdout, stderr, err
}

func (ac client) StopCvd() error {
	stopCmd := exec.Command(STOP_CVD_PATH)
	return stopCmd.Run()
}

func (ac client) Connect() error {
	connectCmd := exec.Command(ADB_PATH, "connect", "0.0.0.0:6520")
	return connectCmd.Run()
}

func (ac client) Shell(args ...string) (string, error) {
	cmdArgs := []string{"shell"}
	cmdArgs = append(cmdArgs, args...)

	shellCmd := exec.Command(ADB_PATH, cmdArgs...)
	out, err := shellCmd.Output()
	return strings.TrimSpace(string(out)), err
}

func (ac client) Root() error {
	rootCmd := exec.Command(ADB_PATH, "root")
	return rootCmd.Run()
}

func (ac client) Push(localSrc, deviceDst string) error {
	pushCmd := exec.Command(ADB_PATH, "push", localSrc, deviceDst)
	return pushCmd.Run()
}

func (ac client) Pull(deviceSrc, localDst string) error {
	pullCmd := exec.Command(ADB_PATH, "pull", deviceSrc, localDst)
	return pullCmd.Run()
}

func (ac client) Logcat() (io.ReadCloser, io.ReadCloser, error) {
	logcatCmd := exec.Command(ADB_PATH, "logcat")

	stdout, err := logcatCmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	stderr, err := logcatCmd.StderrPipe()
	if err != nil {
		return nil, nil, err
	}

	err = logcatCmd.Start()
	return stdout, stderr, err
}

func (ac client) Bugreport(filePath string) error {
	bugreportCmd := exec.Command(ADB_PATH, "bugreport", filePath)
	return bugreportCmd.Run()
}
