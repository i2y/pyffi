package pyffi_test

import (
	"errors"
	"testing"
)

func TestRegisterFunc(t *testing.T) {
	rt := newOrSkip(t)

	called := false
	err := rt.RegisterFunc("hello", func() {
		called = true
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := rt.Exec("import go_bridge; go_bridge.hello()"); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("expected Go function to be called")
	}
}

func TestRegisterFuncWithReturn(t *testing.T) {
	rt := newOrSkip(t)

	err := rt.RegisterFunc("double", func(x int) int {
		return x * 2
	})
	if err != nil {
		t.Fatal(err)
	}

	result, err := rt.Import("go_bridge")
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()

	doubleFn := result.Attr("double")
	defer doubleFn.Close()

	val, err := doubleFn.Call(int64(21))
	if err != nil {
		t.Fatal(err)
	}
	defer val.Close()

	n, err := val.Int64()
	if err != nil {
		t.Fatal(err)
	}
	if n != 42 {
		t.Fatalf("got %d, want 42", n)
	}
}

func TestRegisterFuncWithArgs(t *testing.T) {
	rt := newOrSkip(t)

	err := rt.RegisterFunc("greet", func(name string) string {
		return "Hello, " + name + "!"
	})
	if err != nil {
		t.Fatal(err)
	}

	// Call from Python via Exec and capture result.
	if err := rt.Exec(`
import go_bridge
result = go_bridge.greet("World")
assert result == "Hello, World!", f"got {result}"
`); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterFuncError(t *testing.T) {
	rt := newOrSkip(t)

	err := rt.RegisterFunc("failing", func() error {
		return errors.New("something went wrong")
	})
	if err != nil {
		t.Fatal(err)
	}

	err = rt.Exec(`
import go_bridge
try:
    go_bridge.failing()
    assert False, "expected exception"
except RuntimeError as e:
    assert "something went wrong" in str(e)
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRegisterFuncMultiple(t *testing.T) {
	rt := newOrSkip(t)

	rt.RegisterFunc("add", func(a, b int) int { return a + b })
	rt.RegisterFunc("mul", func(a, b int) int { return a * b })

	if err := rt.Exec(`
import go_bridge
assert go_bridge.add(3, 4) == 7
assert go_bridge.mul(3, 4) == 12
`); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterFuncFromExec(t *testing.T) {
	rt := newOrSkip(t)

	var log []string
	rt.RegisterFunc("log_msg", func(msg string) {
		log = append(log, msg)
	})

	if err := rt.Exec(`
import go_bridge
go_bridge.log_msg("first")
go_bridge.log_msg("second")
`); err != nil {
		t.Fatal(err)
	}

	if len(log) != 2 || log[0] != "first" || log[1] != "second" {
		t.Fatalf("got %v", log)
	}
}

func TestRegisterFuncNotAFunc(t *testing.T) {
	rt := newOrSkip(t)

	err := rt.RegisterFunc("bad", "not a function")
	if err == nil {
		t.Fatal("expected error for non-function argument")
	}
}

func TestRegisterFuncWithKwargs(t *testing.T) {
	rt := newOrSkip(t)

	err := rt.RegisterFunc("greet_kw", func(name string, kwargs map[string]any) string {
		greeting, ok := kwargs["greeting"]
		if !ok {
			greeting = "hello"
		}
		return greeting.(string) + " " + name
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := rt.Exec(`
import go_bridge
result = go_bridge.greet_kw("Go", greeting="hi")
assert result == "hi Go", f"got {result!r}"
`); err != nil {
		t.Fatal(err)
	}
}

func TestRegisterFuncKwargsOnly(t *testing.T) {
	rt := newOrSkip(t)

	err := rt.RegisterFunc("config", func(kwargs map[string]any) string {
		name, _ := kwargs["name"]
		if name == nil {
			name = "default"
		}
		return name.(string)
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := rt.Exec(`
import go_bridge
result = go_bridge.config(name="custom")
assert result == "custom", f"got {result!r}"
result2 = go_bridge.config()
assert result2 == "default", f"got {result2!r}"
`); err != nil {
		t.Fatal(err)
	}
}
