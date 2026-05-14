package typst_test

import (
	"context"
	_ "embed"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

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
	require.NoError(t, err)
	t.Cleanup(func() { _ = compiler.Close(ctx) })

	var data any
	require.NoError(t, json.Unmarshal(portfolioSampleJSON, &data))

	pdf, err := compiler.Compile(ctx, typstcompiler.CompileRequest{
		Template: portfolioTyp,
		Files:    map[string]string{"layout.typ": layoutTyp},
		Data:     data,
		Fonts:    testFonts(),
	})
	require.NoError(t, err)
	require.NotEmpty(t, pdf)

	err = os.WriteFile("testdata/portfolio-out.pdf", pdf, 0o644)
	require.NoError(t, err)
}

func TestCompile_Error(t *testing.T) {
	ctx := context.Background()

	compiler, err := typstcompiler.NewCompiler(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = compiler.Close(ctx) })

	// Template references a field that doesn't exist in the empty data dict.
	_, err = compiler.Compile(ctx, typstcompiler.CompileRequest{
		Template: `#let data = sys.inputs; #data.no_such_field`,
		Data:     map[string]string{},
		Fonts:    testFonts(),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "typst: compile error")
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
