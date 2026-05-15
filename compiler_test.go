package typst_test

import (
	"context"
	_ "embed"
	"encoding/json"
	"os"
	"strings"
	"testing"

	typstcompiler "github.com/varunbpatil/typst-go-wasm"
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
	ctx := context.Background()

	compiler, err := typstcompiler.NewCompiler(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = compiler.Close(ctx) })

	var data any
	if err := json.Unmarshal(portfolioSampleJSON, &data); err != nil {
		t.Fatal(err)
	}

	pdf, err := compiler.Compile(ctx, typstcompiler.CompileRequest{
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
	ctx := context.Background()

	compiler, err := typstcompiler.NewCompiler(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = compiler.Close(ctx) })

	// Template references a field that doesn't exist in the empty data dict.
	_, err = compiler.Compile(ctx, typstcompiler.CompileRequest{
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

func BenchmarkCompile_Portfolio(b *testing.B) {
	ctx := context.Background()

	compiler, err := typstcompiler.NewCompiler(ctx)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = compiler.Close(ctx) })

	var data any
	if err := json.Unmarshal(portfolioSampleJSON, &data); err != nil {
		b.Fatal(err)
	}

	req := typstcompiler.CompileRequest{
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
