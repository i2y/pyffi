package pyffi

import (
	"errors"
	"fmt"
	"strings"
)

// ErrPythonNotFound is returned when no suitable Python shared library
// can be located on the system.
var ErrPythonNotFound = errors.New("pyffi: shared library not found")

// ErrAlreadyClosed is returned when operations are attempted on a
// closed Runtime.
var ErrAlreadyClosed = errors.New("pyffi: runtime already closed")

// ErrNilObject is returned when an operation is attempted on a nil Object.
var ErrNilObject = errors.New("pyffi: operation on nil Object")

// PythonError represents an error that originated in the Python interpreter.
type PythonError struct {
	Context   string // Go-side context, e.g. "Exec", "Import(\"foo\")"
	Type      string // Python exception type, e.g. "ValueError"
	Message   string // Exception message
	Traceback string // Full traceback string (Python 3.12+)
}

func (e *PythonError) Error() string {
	var msg string
	if e.Type != "" && e.Message != "" {
		msg = fmt.Sprintf("pyffi: %s: %s: %s", e.Context, e.Type, e.Message)
	} else if e.Type != "" {
		msg = fmt.Sprintf("pyffi: %s: %s", e.Context, e.Type)
	} else {
		msg = fmt.Sprintf("pyffi: exception during %s", e.Context)
	}
	if e.Traceback != "" {
		return msg + "\n" + e.Traceback
	}
	return msg
}

// LibraryNotFoundError provides detailed information about which paths
// were searched when Python could not be found.
type LibraryNotFoundError struct {
	Searched []string
}

func (e *LibraryNotFoundError) Error() string {
	var b strings.Builder
	b.WriteString("pyffi: shared library not found\nsearched:\n")
	for _, p := range e.Searched {
		fmt.Fprintf(&b, "  - %s\n", p)
	}
	b.WriteString("hint: install Python with uv:  curl -LsSf https://astral.sh/uv/install.sh | sh && uv python install 3.13\n")
	return b.String()
}

func (e *LibraryNotFoundError) Unwrap() error {
	return ErrPythonNotFound
}

// currentError extracts the current Python exception info and returns a PythonError.
// It clears the Python error indicator.
func (r *Runtime) currentError(format string, args ...any) *PythonError {
	context := fmt.Sprintf(format, args...)
	pe := &PythonError{Context: context}

	if r.pyErrOccurred() == 0 {
		return pe
	}

	// Try PyErr_GetRaisedException (Python 3.12+) for structured error extraction.
	if r.pyErrGetRaisedException != nil {
		exc := r.pyErrGetRaisedException() // new ref, clears error
		if exc != 0 {
			r.extractFromException(pe, exc)
			r.pyDecRef(exc)
			return pe
		}
	}

	// Fallback for Python < 3.12:
	// 1. Extract type name from PyErr_Occurred before clearing.
	// 2. Call PyErr_Print which clears the error and sets sys.last_value/last_traceback.
	// 3. Extract message and traceback from sys.last_value/last_traceback.
	excType := r.pyErrOccurred() // borrowed ref
	if excType != 0 {
		nameObj := r.pyObjectGetAttrString(excType, "__name__")
		if nameObj != 0 {
			pe.Type = r.pyUnicodeAsUTF8(nameObj)
			r.pyDecRef(nameObj)
		}
	}
	r.pyErrPrint() // clears error, prints to stderr, sets sys.last_*

	// After PyErr_Print, sys.last_value and sys.last_traceback are set.
	r.extractFromSysLast(pe)
	return pe
}

// extractFromException extracts Type, Message, and Traceback from an exception object.
func (r *Runtime) extractFromException(pe *PythonError, exc uintptr) {
	// Get __class__.__name__ for the type.
	cls := r.pyObjectGetAttrString(exc, "__class__")
	if cls != 0 {
		nameObj := r.pyObjectGetAttrString(cls, "__name__")
		if nameObj != 0 {
			pe.Type = r.pyUnicodeAsUTF8(nameObj)
			r.pyDecRef(nameObj)
		}
		r.pyDecRef(cls)
	}

	// Get str(exc) for the message.
	strObj := r.pyObjectStr(exc)
	if strObj != 0 {
		pe.Message = r.pyUnicodeAsUTF8(strObj)
		r.pyDecRef(strObj)
	}

	// Extract traceback via traceback.format_exception(exc).
	// Uses only low-level C-API calls to avoid recursion through currentError().
	r.extractTraceback(pe, exc)
}

// extractTraceback uses the traceback module to format the exception's traceback.
// All failures are silently ignored (graceful degradation).
func (r *Runtime) extractTraceback(pe *PythonError, exc uintptr) {
	tbMod := r.pyImportModule("traceback")
	if tbMod == 0 {
		r.clearError()
		return
	}
	defer r.pyDecRef(tbMod)

	formatExc := r.pyObjectGetAttrString(tbMod, "format_exception")
	if formatExc == 0 {
		r.clearError()
		return
	}
	defer r.pyDecRef(formatExc)

	// Build args tuple: (exc,)
	args := r.pyTupleNew(1)
	if args == 0 {
		r.clearError()
		return
	}
	r.pyIncRef(exc) // PyTuple_SetItem steals the ref
	r.pyTupleSetItem(args, 0, exc)

	// Call traceback.format_exception(exc)
	result := r.pyObjectCall(formatExc, args, 0)
	r.pyDecRef(args)
	if result == 0 {
		r.clearError()
		return
	}
	defer r.pyDecRef(result)

	// result is a list of strings; concatenate in Go.
	n := r.pyListSize(result)
	if n <= 0 {
		return
	}
	var buf strings.Builder
	for i := int64(0); i < n; i++ {
		item := r.pyListGetItem(result, i) // borrowed ref
		if item == 0 {
			continue
		}
		s := r.pyUnicodeAsUTF8(item)
		buf.WriteString(s)
	}
	pe.Traceback = strings.TrimRight(buf.String(), "\n")
}

// extractFromSysLast extracts error info from sys.last_value and sys.last_traceback.
// These are set by PyErr_Print. Used as fallback for Python < 3.12.
func (r *Runtime) extractFromSysLast(pe *PythonError) {
	sys := r.pyImportModule("sys")
	if sys == 0 {
		r.clearError()
		return
	}
	defer r.pyDecRef(sys)

	// Extract message from sys.last_value.
	lastValue := r.pyObjectGetAttrString(sys, "last_value")
	if lastValue != 0 {
		strObj := r.pyObjectStr(lastValue)
		if strObj != 0 {
			pe.Message = r.pyUnicodeAsUTF8(strObj)
			r.pyDecRef(strObj)
		}
		r.pyDecRef(lastValue)
	} else {
		r.clearError()
	}

	// Extract traceback from sys.last_traceback via traceback module.
	lastTB := r.pyObjectGetAttrString(sys, "last_traceback")
	if lastTB == 0 || lastTB == r.pyNone {
		if lastTB != 0 {
			r.pyDecRef(lastTB)
		} else {
			r.clearError()
		}
		return
	}
	defer r.pyDecRef(lastTB)

	lastType := r.pyObjectGetAttrString(sys, "last_type")
	if lastType == 0 {
		r.clearError()
		return
	}
	defer r.pyDecRef(lastType)

	tbMod := r.pyImportModule("traceback")
	if tbMod == 0 {
		r.clearError()
		return
	}
	defer r.pyDecRef(tbMod)

	formatExc := r.pyObjectGetAttrString(tbMod, "format_exception")
	if formatExc == 0 {
		r.clearError()
		return
	}
	defer r.pyDecRef(formatExc)

	// Call traceback.format_exception(last_type, last_value, last_traceback)
	args := r.pyTupleNew(3)
	if args == 0 {
		r.clearError()
		return
	}
	r.pyIncRef(lastType)
	r.pyTupleSetItem(args, 0, lastType)
	// Re-fetch last_value for the tuple (need a new ref)
	lv2 := r.pyObjectGetAttrString(sys, "last_value")
	if lv2 == 0 {
		r.pyDecRef(args)
		r.clearError()
		return
	}
	r.pyTupleSetItem(args, 1, lv2) // steals ref
	r.pyIncRef(lastTB)
	r.pyTupleSetItem(args, 2, lastTB)

	result := r.pyObjectCall(formatExc, args, 0)
	r.pyDecRef(args)
	if result == 0 {
		r.clearError()
		return
	}
	defer r.pyDecRef(result)

	n := r.pyListSize(result)
	if n <= 0 {
		return
	}
	var buf strings.Builder
	for i := int64(0); i < n; i++ {
		item := r.pyListGetItem(result, i)
		if item == 0 {
			continue
		}
		buf.WriteString(r.pyUnicodeAsUTF8(item))
	}
	pe.Traceback = strings.TrimRight(buf.String(), "\n")
}

// clearError silently clears the Python error indicator.
func (r *Runtime) clearError() {
	if r.pyErrOccurred() == 0 {
		return
	}
	if r.pyErrGetRaisedException != nil {
		exc := r.pyErrGetRaisedException()
		if exc != 0 {
			r.pyDecRef(exc)
		}
		return
	}
	r.pyErrPrint()
}
