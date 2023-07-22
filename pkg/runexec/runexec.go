/*
Copyright 2023 The Dapr Authors
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package runexec

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/dapr/cli/pkg/standalone"
)

type CmdProcess struct {
	Command      *exec.Cmd
	CommandErr   error
	OutputWriter io.Writer
	ErrorWriter  io.Writer
}

type RunExec struct {
	DaprCMD        *CmdProcess
	AppCMD         *CmdProcess
	AppID          string
	DaprHTTPPort   int
	DaprGRPCPort   int
	DaprMetricPort int
}

type RunnableCmd interface {
	Start() error
	Wait() error
	StderrPipe() (io.ReadCloser, error)
	StdoutPipe() (io.ReadCloser, error)
	Pid() int
	HasProcess() bool
	Running() bool
	Kill() error
}

type RunnableExecCmd struct {
	*exec.Cmd
}

func (r *RunnableExecCmd) Pid() int {
	return r.Process.Pid
}

func (r *RunnableExecCmd) HasProcess() bool {
	return r.Process != nil
}

func (r *RunnableExecCmd) Running() bool {
	return r.ProcessState == nil || !r.ProcessState.Exited()
}

func (r *RunnableExecCmd) Kill() error {
	return r.Process.Kill()
}

// RunOutput represents the run execution.
type RunOutput struct {
	DaprCMD      *exec.Cmd
	DaprErr      error
	DaprHTTPPort int
	DaprGRPCPort int
	AppID        string
	AppCMD       RunnableCmd
	AppErr       error
}

func New(config *standalone.RunConfig, daprCmdProcess *CmdProcess, appCmdProcess *CmdProcess) *RunExec {
	return &RunExec{
		DaprCMD:        daprCmdProcess,
		AppCMD:         appCmdProcess,
		AppID:          config.AppID,
		DaprHTTPPort:   config.HTTPPort,
		DaprGRPCPort:   config.GRPCPort,
		DaprMetricPort: config.MetricsPort,
	}
}

func GetDaprCmdProcess(config *standalone.RunConfig) (*CmdProcess, error) {
	daprCMD, err := standalone.GetDaprCommand(config)
	if err != nil {
		return nil, err
	}
	return &CmdProcess{
		Command: daprCMD,
	}, nil
}

func GetAppCmdProcess(config *standalone.RunConfig) (*CmdProcess, error) {
	//nolint
	var appCMD *exec.Cmd = standalone.GetAppCommand(config)
	return &CmdProcess{
		Command: appCMD,
	}, nil
}

func (c *CmdProcess) WithOutputWriter(w io.Writer) {
	c.OutputWriter = w
}

// SetStdout should be called after WithOutputWriter.
func (c *CmdProcess) SetStdout() error {
	if c.Command == nil {
		return fmt.Errorf("command is nil")
	}
	c.Command.Stdout = c.OutputWriter
	return nil
}

func (c *CmdProcess) WithErrorWriter(w io.Writer) {
	c.ErrorWriter = w
}

// SetStdErr should be called after WithErrorWriter.
func (c *CmdProcess) SetStderr() error {
	if c.Command == nil {
		return fmt.Errorf("command is nil")
	}
	c.Command.Stderr = c.ErrorWriter
	return nil
}

func NewOutput(config *standalone.RunConfig) (*RunOutput, error) {
	// set default values from RunConfig struct's tag.
	config.SetDefaultFromSchema()
	//nolint
	err := config.Validate()
	if err != nil {
		return nil, err
	}

	daprCMD, err := standalone.GetDaprCommand(config)
	if err != nil {
		return nil, err
	}

	//nolint
	var appCMD *exec.Cmd = standalone.GetAppCommand(config)
	var execCmd RunnableCmd
	if appCMD != nil && strings.HasSuffix(appCMD.Path, ".wasm") {
		execCmd, err = NewWasmCmd(appCMD.Path, appCMD.Args, appCMD.Env)
		if err != nil {
			return nil, err
		}
	} else {
		execCmd = &RunnableExecCmd{appCMD}
	}
	return &RunOutput{
		DaprCMD:      daprCMD,
		DaprErr:      nil,
		AppCMD:       execCmd,
		AppErr:       nil,
		AppID:        config.AppID,
		DaprHTTPPort: config.HTTPPort,
		DaprGRPCPort: config.GRPCPort,
	}, nil
}
