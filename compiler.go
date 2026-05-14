package typst

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed typst_compiler.wasm
var wasmBinary []byte

// Compiler compiles Typst templates to PDF using an embedded WASM module.
// Safe for concurrent use. Create once with NewCompiler; close when done.
type Compiler struct {
	runtime  wazero.Runtime
	compiled wazero.CompiledModule
}

// NewCompiler loads and compiles the embedded WASM module.
// This is expensive and should be called once at startup.
func NewCompiler(ctx context.Context) (*Compiler, error) {
	rt := wazero.NewRuntime(ctx)

	if _, err := wasi_snapshot_preview1.Instantiate(ctx, rt); err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("typst: wasi instantiate: %w", err)
	}

	compiled, err := rt.CompileModule(ctx, wasmBinary)
	if err != nil {
		_ = rt.Close(ctx)
		return nil, fmt.Errorf("typst: compile wasm module: %w", err)
	}

	return &Compiler{runtime: rt, compiled: compiled}, nil
}

// Close releases all resources held by the Compiler.
func (c *Compiler) Close(ctx context.Context) error {
	return c.runtime.Close(ctx)
}

// CompileRequest holds all inputs for a single Typst compilation.
type CompileRequest struct {
	// Template is the content of the main .typ file.
	Template string
	// Files is an optional map of auxiliary filenames to their content
	// (e.g. "layout.typ" → "..."), which the template may import by name.
	Files map[string]string
	// Data is JSON-serialized and injected as sys.inputs in the template.
	Data any
	// Fonts contains raw TTF/OTF font bytes. At least one font is required;
	// templates that reference a font name not present here will fail to compile.
	Fonts [][]byte
}

// envelope is the JSON structure sent to the WASM module.
// Go's encoding/json marshals [][]byte as an array of base64 strings,
// which the WASM side decodes back to raw bytes.
type envelope struct {
	Main  string            `json:"main"`
	Files map[string]string `json:"files,omitempty"`
	Data  any               `json:"data"`
	Fonts [][]byte          `json:"fonts,omitempty"`
}

// Compile renders a Typst template to PDF bytes.
func (c *Compiler) Compile(ctx context.Context, req CompileRequest) ([]byte, error) {
	env := envelope{
		Main:  req.Template,
		Files: req.Files,
		Data:  req.Data,
		Fonts: req.Fonts,
	}

	jsonBytes, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("typst: marshal envelope: %w", err)
	}

	return c.callCompile(ctx, jsonBytes)
}

// callCompile handles the low-level wazero invocation.
//
//nolint:mnd
func (c *Compiler) callCompile(ctx context.Context, jsonBytes []byte) ([]byte, error) {
	var stderr bytes.Buffer

	// Fresh module per call — stateless, concurrency-safe.
	cfg := wazero.NewModuleConfig().WithStderr(&stderr).WithName("")
	mod, err := c.runtime.InstantiateModule(ctx, c.compiled, cfg)
	if err != nil {
		return nil, fmt.Errorf("typst: instantiate module: %w", err)
	}
	defer func() { _ = mod.Close(ctx) }()

	allocFn := mod.ExportedFunction("alloc")
	if allocFn == nil {
		return nil, fmt.Errorf("typst: alloc function not exported")
	}
	deallocFn := mod.ExportedFunction("dealloc")
	if deallocFn == nil {
		return nil, fmt.Errorf("typst: dealloc function not exported")
	}
	compileFn := mod.ExportedFunction("compile")
	if compileFn == nil {
		return nil, fmt.Errorf("typst: compile function not exported")
	}

	jsonLen := uint64(len(jsonBytes))
	ptrRes, err := allocFn.Call(ctx, jsonLen)
	if err != nil {
		return nil, fmt.Errorf("typst: alloc: %w", err)
	}
	ptr := uint32(ptrRes[0])
	defer deallocFn.Call(ctx, uint64(ptr), jsonLen) //nolint:errcheck

	if !mod.Memory().Write(ptr, jsonBytes) {
		return nil, fmt.Errorf("typst: could not write json to wasm memory")
	}

	results, err := compileFn.Call(ctx, uint64(ptr), jsonLen)
	if err != nil {
		return nil, fmt.Errorf("typst: wasm call: %w", err)
	}

	result := results[0]
	if result == ^uint64(0) { // u64::MAX sentinel
		return nil, fmt.Errorf("typst: compile error: %s", stderr.String())
	}

	outPtr := uint32(result >> 32)
	outLen := uint32(result & 0xFFFFFFFF)

	pdf, ok := mod.Memory().Read(outPtr, outLen)
	if !ok {
		return nil, fmt.Errorf("typst: could not read pdf from wasm memory")
	}

	out := make([]byte, len(pdf))
	copy(out, pdf)
	return out, nil
}
