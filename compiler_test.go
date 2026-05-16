package typst_test

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"

	typst "github.com/varunbpatil/typst-go-wasm"
)

//go:embed testdata/layout.typ
var layoutTyp string

//go:embed testdata/portfolio.typ
var portfolioTyp string

//go:embed testdata/portfolio-sample.json
var portfolioSampleJSON []byte

//go:embed fonts/Rubik-Regular.ttf
var rubikRegular []byte

//go:embed fonts/Rubik-Medium.ttf
var rubikMedium []byte

//go:embed fonts/Rubik-SemiBold.ttf
var rubikSemiBold []byte

func testFonts() [][]byte {
	return [][]byte{rubikRegular, rubikMedium, rubikSemiBold}
}

func TestCompile_Portfolio(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	compiler, err := typst.NewCompiler(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = compiler.Close(ctx) })

	var data any
	if err := json.Unmarshal(portfolioSampleJSON, &data); err != nil {
		t.Fatal(err)
	}

	pdf, err := compiler.Compile(ctx, typst.CompileRequest{
		Template: portfolioTyp,
		Files:    map[string]string{"layout.typ": layoutTyp},
		Data:     data,
		Fonts:    testFonts(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pdf) == 0 {
		t.Fatal("expected non-empty pdf")
	}

	if err := os.WriteFile("testdata/portfolio-out.pdf", pdf, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCompile_Error(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	compiler, err := typst.NewCompiler(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = compiler.Close(ctx) })

	// Template references a field that doesn't exist in the empty data dict.
	_, err = compiler.Compile(ctx, typst.CompileRequest{
		Template: `#let data = sys.inputs; #data.no_such_field`,
		Data:     map[string]string{},
		Fonts:    testFonts(),
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "typst: compile error") {
		t.Fatalf("expected error to contain %q, got %q", "typst: compile error", err.Error())
	}
}

const multiPageTemplate = `= Page 1
#pagebreak()
= Page 2
#pagebreak()
= Page 3
#pagebreak()
= Page 4
#pagebreak()
= Page 5`

func TestCompile_PDFOptions_Smoke(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	compiler, err := typst.NewCompiler(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = compiler.Close(ctx) })

	now := time.Now()
	tagged := false

	pdf, err := compiler.Compile(ctx, typst.CompileRequest{
		Template: `= Hello World`,
		Data:     map[string]any{},
		Fonts:    testFonts(),
		PDFOpts: typst.PDFOptions{
			Ident:     "test-stable-id",
			Timestamp: &now,
			Standards: []string{"1.7"},
			Tagged:    &tagged,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pdf) == 0 {
		t.Fatal("expected non-empty pdf")
	}
}

func TestCompile_PDFOptions_PageRanges(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	compiler, err := typst.NewCompiler(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = compiler.Close(ctx) })

	// tagged PDF + page_ranges is unsupported by typst (https://github.com/typst/typst/issues/7743).
	// Disable tagging.
	untagged := false

	req := typst.CompileRequest{
		Template: multiPageTemplate,
		Data:     map[string]any{},
		Fonts:    testFonts(),
		PDFOpts:  typst.PDFOptions{Tagged: &untagged},
	}

	full, err := compiler.Compile(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	req.PDFOpts = typst.PDFOptions{
		PageRanges: []typst.PageRange{{Start: 1, End: 2}},
		Tagged:     &untagged,
	}
	partial, err := compiler.Compile(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	if got := pdfInfo(t, full).PageCount; got != 5 {
		t.Errorf("full: want 5 pages, got %d", got)
	}
	if got := pdfInfo(t, partial).PageCount; got != 2 {
		t.Errorf("partial: want 2 pages, got %d", got)
	}
}

func TestCompile_PDFOptions_EmptyPageRanges(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	compiler, err := typst.NewCompiler(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = compiler.Close(ctx) })

	untagged := false
	_, err = compiler.Compile(ctx, typst.CompileRequest{
		Template: `= Hello`,
		Data:     map[string]any{},
		Fonts:    testFonts(),
		PDFOpts: typst.PDFOptions{
			PageRanges: []typst.PageRange{}, // non-nil but empty
			Tagged:     &untagged,
		},
	})
	if err == nil {
		t.Fatal("expected error for empty PageRanges slice")
	}
	if !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("expected %q in error, got %q", "must not be empty", err.Error())
	}
}

func TestCompile_PDFOptions_NegativePageRange(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	compiler, err := typst.NewCompiler(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = compiler.Close(ctx) })

	untagged := false
	_, err = compiler.Compile(ctx, typst.CompileRequest{
		Template: `= Hello`,
		Data:     map[string]any{},
		Fonts:    testFonts(),
		PDFOpts: typst.PDFOptions{
			PageRanges: []typst.PageRange{{Start: -1, End: 3}},
			Tagged:     &untagged,
		},
	})
	if err == nil {
		t.Fatal("expected error for negative page range")
	}
	if !strings.Contains(err.Error(), ">= 0") {
		t.Fatalf("expected %q in error, got %q", ">= 0", err.Error())
	}
}

func TestCompile_PDFOptions_PageRangesTaggedError(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	compiler, err := typst.NewCompiler(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = compiler.Close(ctx) })

	_, err = compiler.Compile(ctx, typst.CompileRequest{
		Template: `= Hello`,
		Data:     map[string]any{},
		Fonts:    testFonts(),
		PDFOpts: typst.PDFOptions{
			PageRanges: []typst.PageRange{{Start: 1, End: 1}},
			// Tagged is nil (default true) — should be rejected
		},
	})
	if err == nil {
		t.Fatal("expected error when combining PageRanges with tagged PDF")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("expected %q in error, got %q", "mutually exclusive", err.Error())
	}
}

func TestCompile_PDFOptions_InvalidStandard(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	compiler, err := typst.NewCompiler(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = compiler.Close(ctx) })

	_, err = compiler.Compile(ctx, typst.CompileRequest{
		Template: `= Hello`,
		Data:     map[string]any{},
		Fonts:    testFonts(),
		PDFOpts: typst.PDFOptions{
			Standards: []string{"not-a-standard"},
		},
	})
	if err == nil {
		t.Fatal("expected error for invalid pdf standard")
	}
	if !strings.Contains(err.Error(), "typst: compile error") {
		t.Fatalf("expected %q in error, got %q", "typst: compile error", err.Error())
	}
}

func pdfInfo(t *testing.T, pdf []byte) *pdfcpu.PDFInfo {
	t.Helper()
	info, err := api.PDFInfo(bytes.NewReader(pdf), "", nil, false, model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("pdfcpu PDFInfo: %v", err)
	}
	return info
}

func TestCompile_PDFOptions_VerifyPageCount(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	c, err := typst.NewCompiler(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(ctx) })

	untagged := false
	full, err := c.Compile(ctx, typst.CompileRequest{
		Template: multiPageTemplate,
		Data:     map[string]any{},
		Fonts:    testFonts(),
		PDFOpts:  typst.PDFOptions{Tagged: &untagged},
	})
	if err != nil {
		t.Fatal(err)
	}

	partial, err := c.Compile(ctx, typst.CompileRequest{
		Template: multiPageTemplate,
		Data:     map[string]any{},
		Fonts:    testFonts(),
		PDFOpts: typst.PDFOptions{
			PageRanges: []typst.PageRange{{Start: 1, End: 2}},
			Tagged:     &untagged,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := pdfInfo(t, full).PageCount; got != 5 {
		t.Errorf("full: want 5 pages, got %d", got)
	}
	if got := pdfInfo(t, partial).PageCount; got != 2 {
		t.Errorf("partial: want 2 pages, got %d", got)
	}
}

func TestCompile_PDFOptions_VerifyTimestamp(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	c, err := typst.NewCompiler(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(ctx) })

	ts := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	pdf, err := c.Compile(ctx, typst.CompileRequest{
		Template: `= Hello`,
		Data:     map[string]any{},
		Fonts:    testFonts(),
		PDFOpts:  typst.PDFOptions{Timestamp: &ts},
	})
	if err != nil {
		t.Fatal(err)
	}

	info := pdfInfo(t, pdf)
	// CreationDate is a PDF date string: D:YYYYMMDDHHmmSSOHH'mm'
	if info.CreationDate == "" {
		t.Fatal("expected CreationDate to be set")
	}
	// Must contain the date we set.
	const wantDate = "20240615"
	if !strings.Contains(info.CreationDate, wantDate) {
		t.Errorf("CreationDate %q does not contain %q", info.CreationDate, wantDate)
	}

	// A PDF compiled without a timestamp must have no CreationDate.
	noTS, err := c.Compile(ctx, typst.CompileRequest{
		Template: `= Hello`,
		Data:     map[string]any{},
		Fonts:    testFonts(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := pdfInfo(t, noTS).CreationDate; got != "" {
		t.Errorf("expected no CreationDate without Timestamp, got %q", got)
	}
}

func TestCompile_PDFOptions_VerifyTagged(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	c, err := typst.NewCompiler(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(ctx) })

	tagged, err := c.Compile(ctx, typst.CompileRequest{
		Template: `= Hello`,
		Data:     map[string]any{},
		Fonts:    testFonts(),
		// Tagged is nil → defaults to true
	})
	if err != nil {
		t.Fatal(err)
	}

	f := false
	untagged, err := c.Compile(ctx, typst.CompileRequest{
		Template: `= Hello`,
		Data:     map[string]any{},
		Fonts:    testFonts(),
		PDFOpts:  typst.PDFOptions{Tagged: &f},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !pdfInfo(t, tagged).Tagged {
		t.Error("expected Tagged=true for default compilation")
	}
	if pdfInfo(t, untagged).Tagged {
		t.Error("expected Tagged=false when PDFOpts.Tagged=false")
	}
}

func TestCompile_PDFOptions_VerifyIdent(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	c, err := typst.NewCompiler(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(ctx) })

	req := typst.CompileRequest{
		Template: `= Hello`,
		Data:     map[string]any{},
		Fonts:    testFonts(),
		PDFOpts:  typst.PDFOptions{Ident: "stable-doc-id"},
	}
	a, err := c.Compile(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := c.Compile(ctx, req)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(a, b) {
		t.Error("same Ident must produce byte-identical PDFs across compilations")
	}
}

func TestCompile_PDFOptions_VerifyPDFA(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	c, err := typst.NewCompiler(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close(ctx) })

	ts := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)
	pdf, err := c.Compile(ctx, typst.CompileRequest{
		Template: `= Hello`,
		Data:     map[string]any{},
		Fonts:    testFonts(),
		PDFOpts: typst.PDFOptions{
			Standards: []string{"a-2b"},
			Timestamp: &ts, // PDF/A-2b requires a document date
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// pdfcpu doesn't expose PDF/A conformance directly; check the XMP metadata
	// embedded in the raw bytes, which typst-pdf always writes for PDF/A output.
	if !bytes.Contains(pdf, []byte("pdfaid:part")) {
		t.Error("PDF/A output must contain pdfaid:part XMP metadata")
	}
	if !bytes.Contains(pdf, []byte("pdfaid:conformance>B")) {
		t.Error("PDF/A-2b output must contain pdfaid:conformance>B XMP metadata")
	}
}

func BenchmarkCompile_Portfolio(b *testing.B) {
	ctx := b.Context()

	compiler, err := typst.NewCompiler(ctx)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = compiler.Close(ctx) })

	var data any
	if err := json.Unmarshal(portfolioSampleJSON, &data); err != nil {
		b.Fatal(err)
	}

	req := typst.CompileRequest{
		Template: portfolioTyp,
		Files:    map[string]string{"layout.typ": layoutTyp},
		Data:     data,
		Fonts:    testFonts(),
	}

	for b.Loop() {
		_, err := compiler.Compile(ctx, req)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// TestConcurrentMemory intentionally omits t.Parallel() — sequential subtests
// give stable MemStats baselines between GC cycles.
func TestConcurrentMemory(t *testing.T) {
	ctx := t.Context()

	compiler, err := typst.NewCompiler(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = compiler.Close(ctx) })

	var data any
	if err := json.Unmarshal(portfolioSampleJSON, &data); err != nil {
		t.Fatal(err)
	}

	req := typst.CompileRequest{
		Template: portfolioTyp,
		Files:    map[string]string{"layout.typ": layoutTyp},
		Data:     data,
		Fonts:    testFonts(),
	}

	for _, concurrency := range []int{1, 4, 8, 16, 32} {
		t.Run(fmt.Sprintf("concurrency-%d", concurrency), func(t *testing.T) {
			// Force GC to get a clean baseline.
			runtime.GC()
			var before runtime.MemStats
			runtime.ReadMemStats(&before)

			var wg sync.WaitGroup
			for range concurrency {
				wg.Go(func() {
					_, err := compiler.Compile(ctx, req)
					if err != nil {
						t.Errorf("compile error: %v", err)
					}
				})
			}
			wg.Wait()

			runtime.GC()
			var after runtime.MemStats
			runtime.ReadMemStats(&after)

			t.Logf("concurrency=%-3d  peak sys=%.1f MB  heap inuse after GC=%.1f MB",
				concurrency,
				float64(after.Sys)/1024/1024,
				float64(after.HeapInuse)/1024/1024,
			)
		})
	}
}
