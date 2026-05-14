package typst_test

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"testing"

	typstcompiler "github.com/varunbpatil/typst-go-wasm"
)

func TestConcurrentMemory(t *testing.T) {
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

	req := typstcompiler.CompileRequest{
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
