package pyffi_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/i2y/pyffi"
)

func TestCallContext(t *testing.T) {
	rt := newOrSkip(t)

	t.Run("happy_path", func(t *testing.T) {
		builtins, err := pyffi.Bind(rt, "builtins")
		if err != nil {
			t.Fatal(err)
		}
		defer builtins.Close()

		lenFn := builtins.Attr("len")
		defer lenFn.Close()

		result, err := lenFn.CallContext(context.Background(), []any{1, 2, 3})
		if err != nil {
			t.Fatal(err)
		}
		defer result.Close()

		v, err := result.Int64()
		if err != nil {
			t.Fatal(err)
		}
		if v != 3 {
			t.Fatalf("got %d, want 3", v)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		if err := rt.Exec("import time"); err != nil {
			t.Fatal(err)
		}
		timeMod, err := rt.Import("time")
		if err != nil {
			t.Fatal(err)
		}
		defer timeMod.Close()

		sleepFn := timeMod.Attr("sleep")
		if sleepFn == nil {
			t.Fatal("time.sleep not found")
		}
		defer sleepFn.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		start := time.Now()
		_, err = sleepFn.CallContext(ctx, 10.0) // sleep 10 seconds
		elapsed := time.Since(start)

		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected DeadlineExceeded, got %v", err)
		}
		if elapsed > 2*time.Second {
			t.Fatalf("CallContext took %v; expected to return promptly on timeout", elapsed)
		}
	})

	t.Run("pre_cancelled", func(t *testing.T) {
		builtins, err := pyffi.Bind(rt, "builtins")
		if err != nil {
			t.Fatal(err)
		}
		defer builtins.Close()

		lenFn := builtins.Attr("len")
		defer lenFn.Close()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately

		_, err = lenFn.CallContext(ctx, []any{1, 2, 3})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected Canceled, got %v", err)
		}
	})

	t.Run("nil_object", func(t *testing.T) {
		var o *pyffi.Object
		_, err := o.CallContext(context.Background())
		if !errors.Is(err, pyffi.ErrNilObject) {
			t.Fatalf("expected ErrNilObject, got %v", err)
		}
	})
}

func TestCallKwContext(t *testing.T) {
	rt := newOrSkip(t)

	if err := rt.Exec(`
def greet(name, greeting="Hello"):
    return f"{greeting}, {name}!"
`); err != nil {
		t.Fatal(err)
	}

	greet, err := rt.Eval("greet")
	if err != nil {
		t.Fatal(err)
	}
	defer greet.Close()

	result, err := greet.CallKwContext(context.Background(), []any{"World"}, map[string]any{"greeting": "Hi"})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()

	v, _ := result.GoValue()
	if v != "Hi, World!" {
		t.Fatalf("got %q, want %q", v, "Hi, World!")
	}
}

func TestBoundModuleCallContext(t *testing.T) {
	rt := newOrSkip(t)

	builtins, err := pyffi.Bind(rt, "builtins")
	if err != nil {
		t.Fatal(err)
	}
	defer builtins.Close()

	t.Run("Call", func(t *testing.T) {
		result, err := builtins.CallContext(context.Background(), "len", []any{1, 2, 3, 4})
		if err != nil {
			t.Fatal(err)
		}
		defer result.Close()

		v, err := result.Int64()
		if err != nil {
			t.Fatal(err)
		}
		if v != 4 {
			t.Fatalf("got %d, want 4", v)
		}
	})

	t.Run("CallKw", func(t *testing.T) {
		if err := rt.Exec(`
def fmt_num(n, base=10):
    return format(n, 'x' if base == 16 else 'd')
`); err != nil {
			t.Fatal(err)
		}

		fmtNum, err := pyffi.Bind(rt, "__main__")
		if err != nil {
			t.Fatal(err)
		}
		defer fmtNum.Close()

		result, err := fmtNum.CallKwContext(context.Background(), "fmt_num", []any{255}, map[string]any{"base": 16})
		if err != nil {
			t.Fatal(err)
		}
		defer result.Close()

		v, _ := result.GoValue()
		if v != "ff" {
			t.Fatalf("got %q, want %q", v, "ff")
		}
	})
}
