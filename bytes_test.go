package pyffi_test

import (
	"bytes"
	"testing"
)

// TestBytesConversion consolidates bytes/bytearray tests into a single Runtime.
func TestBytesConversion(t *testing.T) {
	rt := newOrSkip(t)

	t.Run("BytesRoundTrip", func(t *testing.T) {
		// Go []byte → Python bytes → Go []byte
		input := []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0x80}
		obj, err := rt.FromGoValue(input)
		if err != nil {
			t.Fatalf("FromGoValue failed: %v", err)
		}
		defer obj.Close()

		got, err := obj.GoBytes()
		if err != nil {
			t.Fatalf("GoBytes failed: %v", err)
		}
		if !bytes.Equal(got, input) {
			t.Errorf("GoBytes = %v, want %v", got, input)
		}
	})

	t.Run("EmptyBytes", func(t *testing.T) {
		obj, err := rt.FromGoValue([]byte{})
		if err != nil {
			t.Fatalf("FromGoValue failed: %v", err)
		}
		defer obj.Close()

		got, err := obj.GoBytes()
		if err != nil {
			t.Fatalf("GoBytes failed: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("expected empty bytes, got %v", got)
		}
	})

	t.Run("BytesGoValue", func(t *testing.T) {
		// Test via GoValue() automatic type detection
		input := []byte("hello bytes")
		obj, err := rt.FromGoValue(input)
		if err != nil {
			t.Fatalf("FromGoValue failed: %v", err)
		}
		defer obj.Close()

		val, err := obj.GoValue()
		if err != nil {
			t.Fatalf("GoValue failed: %v", err)
		}
		got, ok := val.([]byte)
		if !ok {
			t.Fatalf("GoValue returned %T, want []byte", val)
		}
		if !bytes.Equal(got, input) {
			t.Errorf("GoValue = %v, want %v", got, input)
		}
	})

	t.Run("BytesWithNullBytes", func(t *testing.T) {
		// Ensure embedded null bytes are preserved
		input := []byte{0x41, 0x00, 0x42, 0x00, 0x43}
		obj, err := rt.FromGoValue(input)
		if err != nil {
			t.Fatalf("FromGoValue failed: %v", err)
		}
		defer obj.Close()

		got, err := obj.GoBytes()
		if err != nil {
			t.Fatalf("GoBytes failed: %v", err)
		}
		if !bytes.Equal(got, input) {
			t.Errorf("GoBytes = %v, want %v", got, input)
		}
	})

	t.Run("BytearrayConversion", func(t *testing.T) {
		// Create bytearray in Python and convert to Go
		if err := rt.Exec(`test_ba = bytearray(b"\x01\x02\x03")`); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}
		mod, err := rt.Import("__main__")
		if err != nil {
			t.Fatalf("Import __main__ failed: %v", err)
		}
		defer mod.Close()

		ba := mod.Attr("test_ba")
		if ba == nil {
			t.Fatal("expected test_ba attribute")
		}
		defer ba.Close()

		got, err := ba.GoBytes()
		if err != nil {
			t.Fatalf("GoBytes failed: %v", err)
		}
		want := []byte{0x01, 0x02, 0x03}
		if !bytes.Equal(got, want) {
			t.Errorf("GoBytes = %v, want %v", got, want)
		}
	})

	t.Run("BytearrayGoValue", func(t *testing.T) {
		// Bytearray via GoValue() automatic type detection
		if err := rt.Exec(`test_ba2 = bytearray(b"abc")`); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}
		mod, err := rt.Import("__main__")
		if err != nil {
			t.Fatalf("Import __main__ failed: %v", err)
		}
		defer mod.Close()

		ba := mod.Attr("test_ba2")
		if ba == nil {
			t.Fatal("expected test_ba2 attribute")
		}
		defer ba.Close()

		val, err := ba.GoValue()
		if err != nil {
			t.Fatalf("GoValue failed: %v", err)
		}
		got, ok := val.([]byte)
		if !ok {
			t.Fatalf("GoValue returned %T, want []byte", val)
		}
		if !bytes.Equal(got, []byte("abc")) {
			t.Errorf("GoValue = %v, want %v", got, []byte("abc"))
		}
	})

	t.Run("BytesAsCallArg", func(t *testing.T) {
		// Pass []byte as argument to Python function and get it back
		if err := rt.Exec(`
def echo_bytes(b):
    return b
`); err != nil {
			t.Fatalf("Exec failed: %v", err)
		}

		mod, err := rt.Import("__main__")
		if err != nil {
			t.Fatalf("Import __main__ failed: %v", err)
		}
		defer mod.Close()

		fn := mod.Attr("echo_bytes")
		if fn == nil {
			t.Fatal("expected echo_bytes attribute")
		}
		defer fn.Close()

		input := []byte{0xde, 0xad, 0xbe, 0xef}
		result, err := fn.Call(input)
		if err != nil {
			t.Fatalf("Call failed: %v", err)
		}
		defer result.Close()

		got, err := result.GoBytes()
		if err != nil {
			t.Fatalf("GoBytes failed: %v", err)
		}
		if !bytes.Equal(got, input) {
			t.Errorf("GoBytes = %v, want %v", got, input)
		}
	})
}
