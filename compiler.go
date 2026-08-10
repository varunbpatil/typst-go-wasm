package typst

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

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

// PageRange selects a contiguous range of pages for PDF export (1-indexed).
// A zero value for Start or End means "no bound" (i.e. from the beginning or
// to the end of the document, respectively).
type PageRange struct {
	Start int // 0 = from first page
	End   int // 0 = to last page
}

// PDFOptions controls PDF-specific output settings for a compilation.
// All fields are optional; the zero value produces the same output as the
// typst defaults (auto ident, no timestamp, all pages, no standard, tagged).
type PDFOptions struct {
	// Ident is a stable document identifier used to produce a reproducible PDF
	// document ID. Leave empty to let typst auto-generate one from title/author.
	Ident string
	// Timestamp sets the document creation timestamp embedded in the PDF.
	// A nil pointer means no timestamp is set.
	Timestamp *time.Time
	// PageRanges restricts which pages are written to the PDF (1-indexed).
	// A nil slice exports all pages.
	PageRanges []PageRange
	// Standards lists PDF standards the output must conform to
	// (e.g. "a-2b", "1.7"). An empty slice uses the typst default.
	Standards []string
	// Tagged controls whether a tagged PDF is written for accessibility.
	// A nil pointer uses the typst default (true).
	Tagged *bool
}

// CompileRequest holds all inputs for a single Typst compilation.
type CompileRequest struct {
	// Template is the content of the main .typ file.
	Template string
	// Files is an optional map of auxiliary filenames to their content
	// (e.g. "layout.typ" → "..."), which the template may import by name.
	Files map[string]string
	// BinaryFiles maps filenames to raw bytes for binary assets the template
	// may reference by name — e.g. "logo.png" → PNG bytes for #image("logo.png").
	BinaryFiles map[string][]byte
	// Data is JSON-serialized and injected as sys.inputs in the template.
	Data any
	// Fonts contains raw TTF/OTF font bytes. At least one font is required;
	// templates that reference a font name not present here will fail to compile.
	Fonts [][]byte
	// PDFOpts holds optional PDF-specific output settings.
	// A zero value uses typst defaults.
	PDFOpts PDFOptions
}

// wirePageRange is the JSON form of a page range.
// Start/End are 1-indexed; 0 means "no bound".
type wirePageRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// wirePDFOptions is the JSON form of PDFOptions sent to the WASM module.
type wirePDFOptions struct {
	Ident      string          `json:"ident,omitempty"`
	Timestamp  *time.Time      `json:"timestamp,omitempty"` // marshals as RFC 3339
	PageRanges []wirePageRange `json:"page_ranges,omitempty"`
	Standards  []string        `json:"standards,omitempty"`
	Tagged     *bool           `json:"tagged,omitempty"`
}

// envelope is the JSON structure sent to the WASM module.
// Go's encoding/json marshals [][]byte as an array of base64 strings,
// which the WASM side decodes back to raw bytes.
type envelope struct {
	Main  string            `json:"main"`
	Files map[string]string `json:"files,omitempty"`
	// []byte values marshal as base64 strings, decoded on the WASM side.
	BinaryFiles map[string][]byte `json:"binary_files,omitempty"`
	Data        any               `json:"data"`
	Fonts       [][]byte          `json:"fonts,omitempty"`
	PDFOptions  *wirePDFOptions   `json:"pdf_options,omitempty"`
}

func buildWirePDFOptions(opt PDFOptions) *wirePDFOptions {
	if opt.Ident == "" && opt.Timestamp == nil && len(opt.PageRanges) == 0 && len(opt.Standards) == 0 && opt.Tagged == nil {
		return nil
	}
	w := &wirePDFOptions{
		Ident:     opt.Ident,
		Timestamp: opt.Timestamp,
		Standards: opt.Standards,
		Tagged:    opt.Tagged,
	}
	for _, r := range opt.PageRanges {
		w.PageRanges = append(w.PageRanges, wirePageRange(r))
	}
	return w
}

// Compile renders a Typst template to PDF bytes.
func (c *Compiler) Compile(ctx context.Context, req CompileRequest) ([]byte, error) {
	if req.PDFOpts.PageRanges != nil && len(req.PDFOpts.PageRanges) == 0 {
		return nil, fmt.Errorf("typst: PDF.PageRanges must not be empty; use nil to export all pages")
	}
	for _, r := range req.PDFOpts.PageRanges {
		if r.Start < 0 || r.End < 0 {
			return nil, fmt.Errorf("typst: PDF.PageRanges: Start and End must be >= 0, got {Start: %d, End: %d}", r.Start, r.End)
		}
	}
	if len(req.PDFOpts.PageRanges) > 0 && (req.PDFOpts.Tagged == nil || *req.PDFOpts.Tagged) {
		return nil, fmt.Errorf("typst: PageRanges and tagged PDF are mutually exclusive; set PDF.Tagged = ptr(false) when using PageRanges (https://github.com/typst/typst/issues/7743)")
	}

	env := envelope{
		Main:        req.Template,
		Files:       req.Files,
		BinaryFiles: req.BinaryFiles,
		Data:        req.Data,
		Fonts:       req.Fonts,
		PDFOptions:  buildWirePDFOptions(req.PDFOpts),
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
