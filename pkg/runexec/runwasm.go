package runexec

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

func detectWasi(imports []api.FunctionDefinition) bool {
	for _, f := range imports {
		moduleName, _, _ := f.Import()
		if moduleName == wasi_snapshot_preview1.ModuleName {
			return true
		}
	}
	return false
}

func NewWasmCmd(path string, args []string, env []string) (RunnableCmd, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	errR, errW := io.Pipe()
	outR, outW := io.Pipe()
	return &WasmRunnableCmd{
		wasm:         data,
		args:         args,
		env:          env,
		stderrReader: errR,
		stderrWriter: errW,
		stdoutReader: outR,
		stdoutWriter: outW,
		running:      false,
		wait:         make(chan bool),
	}, nil
}

type WasmRunnableCmd struct {
	wasm []byte
	args []string
	env  []string

	runtime wazero.Runtime
	module  wazero.CompiledModule
	ctx     context.Context

	stderrReader io.ReadCloser
	stderrWriter io.WriteCloser
	stdoutReader io.ReadCloser
	stdoutWriter io.WriteCloser
	running      bool
	wait         chan bool
}

func (w *WasmRunnableCmd) StderrPipe() (io.ReadCloser, error) {
	return w.stderrReader, nil
}

func (w *WasmRunnableCmd) StdoutPipe() (io.ReadCloser, error) {
	return w.stdoutReader, nil
}

func (w *WasmRunnableCmd) HasProcess() bool {
	return false
}

func (w *WasmRunnableCmd) Pid() int {
	return -1
}

func (w *WasmRunnableCmd) Running() bool {
	return w.running
}

func (w *WasmRunnableCmd) Wait() error {
	if w.running {
		<-w.wait
	}
	return nil
}

func (w *WasmRunnableCmd) Kill() error {
	return w.runtime.Close(w.ctx)
}

func (w *WasmRunnableCmd) Start() error {
	var err error
	config := wazero.NewRuntimeConfig()
	w.ctx = context.TODO()

	// Create the runtime, which when closed releases any resources associated with it.
	w.runtime = wazero.NewRuntimeWithConfig(w.ctx, config)

	// Compile the module, which reduces execution time of Invoke
	w.module, err = w.runtime.CompileModule(w.ctx, w.wasm)
	if err != nil {
		_ = w.runtime.Close(context.Background())
		return fmt.Errorf("wasm: error compiling binary: %w", err)
	}

	if detectWasi(w.module.ImportedFunctions()) {
		_, err = wasi_snapshot_preview1.Instantiate(w.ctx, w.runtime)

		if err != nil {
			_ = w.runtime.Close(context.Background())
			return fmt.Errorf("wasm: error instantiating host functions: %w", err)
		}
	}

	moduleConfig := wazero.NewModuleConfig().
		WithStderr(w.stderrWriter).
		WithStdout(w.stdoutWriter).
		WithArgs(w.args...)

	for _, env := range w.env {
		parts := strings.Split(env, "=")
		switch len(parts) {
		case 1:
			moduleConfig = moduleConfig.WithEnv(parts[0], "")
		case 2:
			moduleConfig = moduleConfig.WithEnv(parts[0], parts[1])
		default:
			return fmt.Errorf("unexpected environment variable: %s", env)
		}
	}

	go func() {
		mod, err := w.runtime.InstantiateModule(w.ctx, w.module, moduleConfig)
		if err != nil {
			fmt.Println(err.Error())
		}
		_ = mod.Close(w.ctx)

		w.wait <- true
	}()

	return nil
}
