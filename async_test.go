package pyffi_test

import (
	"sync"
	"testing"

	"github.com/i2y/pyffi"
)

// TestAsyncSupport consolidates async tests into a single Runtime.
func TestAsyncSupport(t *testing.T) {
	rt := newOrSkip(t)

	t.Run("RunAsync", func(t *testing.T) {
		// Define an async function and run it via RunAsync.
		if err := rt.Exec(`
async def async_add(a, b):
    return a + b
`); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}

		result, err := rt.RunAsync("async_add(1, 2)")
		if err != nil {
			t.Fatalf("RunAsync failed: %v", err)
		}
		defer result.Close()

		val, err := result.Int64()
		if err != nil {
			t.Fatalf("Int64 failed: %v", err)
		}
		if val != 3 {
			t.Errorf("RunAsync result = %d, want 3", val)
		}
	})

	t.Run("RunAsyncString", func(t *testing.T) {
		if err := rt.Exec(`
async def async_greet(name):
    return f"hello {name}"
`); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}

		result, err := rt.RunAsync(`async_greet("world")`)
		if err != nil {
			t.Fatalf("RunAsync failed: %v", err)
		}
		defer result.Close()

		val, err := result.GoString()
		if err != nil {
			t.Fatalf("GoString failed: %v", err)
		}
		if val != "hello world" {
			t.Errorf("RunAsync result = %q, want %q", val, "hello world")
		}
	})

	t.Run("RunAsyncError", func(t *testing.T) {
		if err := rt.Exec(`
async def async_fail():
    raise ValueError("async error")
`); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}

		_, err := rt.RunAsync("async_fail()")
		if err == nil {
			t.Fatal("expected error from RunAsync")
		}
	})

	t.Run("CallAsync", func(t *testing.T) {
		if err := rt.Exec(`
async def async_multiply(x, y):
    return x * y
`); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}

		mod, err := rt.Import("__main__")
		if err != nil {
			t.Fatalf("Import __main__ failed: %v", err)
		}
		defer mod.Close()

		fn := mod.Attr("async_multiply")
		if fn == nil {
			t.Fatal("expected async_multiply attribute")
		}
		defer fn.Close()

		result, err := fn.CallAsync(6, 7)
		if err != nil {
			t.Fatalf("CallAsync failed: %v", err)
		}
		defer result.Close()

		val, err := result.Int64()
		if err != nil {
			t.Fatalf("Int64 failed: %v", err)
		}
		if val != 42 {
			t.Errorf("CallAsync result = %d, want 42", val)
		}
	})

	t.Run("CallAsyncWithKwargs", func(t *testing.T) {
		if err := rt.Exec(`
async def async_format(name, greeting="hi"):
    return f"{greeting} {name}"
`); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}

		mod, err := rt.Import("__main__")
		if err != nil {
			t.Fatalf("Import __main__ failed: %v", err)
		}
		defer mod.Close()

		fn := mod.Attr("async_format")
		if fn == nil {
			t.Fatal("expected async_format attribute")
		}
		defer fn.Close()

		result, err := fn.CallAsync("Go", pyffi.KW{"greeting": "hello"})
		if err != nil {
			t.Fatalf("CallAsync failed: %v", err)
		}
		defer result.Close()

		val, err := result.GoString()
		if err != nil {
			t.Fatalf("GoString failed: %v", err)
		}
		if val != "hello Go" {
			t.Errorf("CallAsync result = %q, want %q", val, "hello Go")
		}
	})
}

func TestAsyncGo(t *testing.T) {
	rt := newOrSkip(t)

	// Define async functions.
	if err := rt.Exec(`
import asyncio

async def async_square(x):
    await asyncio.sleep(0)
    return x * x

async def async_hello(name):
    await asyncio.sleep(0)
    return f"hello {name}"
`); err != nil {
		t.Fatalf("Exec failed: %v", err)
	}

	t.Run("RunAsyncGo", func(t *testing.T) {
		ch := rt.RunAsyncGo("async_square(7)")
		ar := <-ch
		if ar.Err != nil {
			t.Fatalf("RunAsyncGo failed: %v", ar.Err)
		}
		defer ar.Value.Close()
		val, _ := ar.Value.Int64()
		if val != 49 {
			t.Errorf("result = %d, want 49", val)
		}
	})

	t.Run("CallAsyncGo", func(t *testing.T) {
		mod, _ := rt.Import("__main__")
		defer mod.Close()
		fn := mod.Attr("async_hello")
		defer fn.Close()

		ch := fn.CallAsyncGo("world")
		ar := <-ch
		if ar.Err != nil {
			t.Fatalf("CallAsyncGo failed: %v", ar.Err)
		}
		defer ar.Value.Close()
		val, _ := ar.Value.GoString()
		if val != "hello world" {
			t.Errorf("result = %q, want %q", val, "hello world")
		}
	})

	t.Run("MultipleAsyncGo", func(t *testing.T) {
		ch1 := rt.RunAsyncGo("async_square(3)")
		ch2 := rt.RunAsyncGo("async_square(4)")
		ch3 := rt.RunAsyncGo("async_square(5)")
		r1 := <-ch1
		r2 := <-ch2
		r3 := <-ch3
		if r1.Err != nil || r2.Err != nil || r3.Err != nil {
			t.Fatalf("errors: %v, %v, %v", r1.Err, r2.Err, r3.Err)
		}
		defer r1.Value.Close()
		defer r2.Value.Close()
		defer r3.Value.Close()
		v1, _ := r1.Value.Int64()
		v2, _ := r2.Value.Int64()
		v3, _ := r3.Value.Int64()
		if v1 != 9 || v2 != 16 || v3 != 25 {
			t.Errorf("results = %d, %d, %d, want 9, 16, 25", v1, v2, v3)
		}
	})

	t.Run("AsyncGoFromGoroutines", func(t *testing.T) {
		var wg sync.WaitGroup
		results := make([]int64, 5)

		for i := range 5 {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				v, err := rt.RunAsync("async_square(" + string(rune('0'+idx+1)) + ")")
				if err == nil {
					results[idx], _ = v.Int64()
					v.Close()
				}
			}(i)
		}

		wg.Wait()
		for i, v := range results {
			expected := int64((i + 1) * (i + 1))
			if v != expected {
				t.Errorf("results[%d] = %d, want %d", i, v, expected)
			}
		}
	})
}
