// Package pyffi provides Go bindings for the CPython C-API using [purego].
// No Cgo is required — the CPython shared library is loaded at runtime.
//
// # Quick Start
//
//	rt, err := pyffi.New()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer rt.Close()
//
//	rt.Exec(`x = 1 + 2`)
//	result, _ := rt.Eval("x * 10")
//	defer result.Close()
//	val, _ := result.Int64() // 30
//
// # Import and Call
//
//	math, _ := rt.Import("math")
//	defer math.Close()
//	r, _ := math.Attr("sqrt").Call(16.0)
//	defer r.Close()
//	f, _ := r.Float64() // 4.0
//
// # Collections
//
//	list, _ := rt.NewList(1, 2, 3)
//	dict, _ := rt.NewDict("name", "Go", "year", 2009)
//	tuple, _ := rt.NewTuple("a", "b", "c")
//	set, _ := rt.NewSet(1, 2, 3)
//
//	item, _ := list.GetItem(0)      // list[0]
//	list.SetItem(0, 99)             // list[0] = 99
//	list.DelItem(0)                 // del list[0]
//	ok, _ := list.Contains(2)       // 2 in list
//	n, _ := list.Len()              // len(list)
//
// # Type Conversions
//
// The following Go ↔ Python type mappings are automatic:
//
//	Go type            Python type
//	bool               bool
//	int, int8–64       int
//	uint, uint8–64     int
//	float32, float64   float
//	string             str
//	[]byte             bytes
//	[]any              list
//	map[string]any     dict
//	nil                None
//
// In the reverse direction:
//
//	Python type                Go type
//	bool                       bool
//	int                        int64
//	float                      float64
//	str                        string
//	bytes, bytearray           []byte
//	list, tuple, set           []any
//	dict                       map[string]any
//	None                       nil
//
// # Goroutine Safety (Auto-GIL)
//
// All public methods automatically acquire and release the Python GIL.
// You can safely call any method from any goroutine without manual
// GIL management:
//
//	go func() {
//	    result, _ := rt.Eval("1 + 1")
//	    defer result.Close()
//	    // Safe — GIL is acquired automatically.
//	}()
//
// For batching multiple operations under a single GIL acquisition
// (reducing overhead), use [Runtime.WithGIL]:
//
//	rt.WithGIL(func() error {
//	    rt.Exec("a = 1")
//	    rt.Exec("b = 2")
//	    rt.Exec("c = a + b")
//	    return nil
//	})
//
// # Async Python
//
//	result, _ := rt.RunAsync("some_async_func()")
//	result, _ := rt.Eval("asyncio.run(coro)")
//
//	// Non-blocking (runs in a background goroutine):
//	ch := rt.RunAsyncGo("fetch_data(url)")
//	ar := <-ch // AsyncResult{Value, Err}
//
// # Callbacks (Go → Python → Go)
//
//	rt.RegisterFunc("add", func(a, b int) int { return a + b })
//	rt.Exec(`
//	    import go_bridge
//	    result = go_bridge.add(1, 2)  // calls Go function
//	`)
//
// # Free-Threaded Python
//
// Python 3.13t+ (free-threaded builds) are automatically detected.
// When running on a free-threaded build, [runtime.LockOSThread] calls
// are skipped since there is no GIL to contend for.
//
// [purego]: https://github.com/ebitengine/purego
package pyffi

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ebitengine/purego"
)

// PyObject represents a CPython PyObject* as an opaque handle.
// The zero value represents NULL.
//
// Deprecated: Use *Object instead for new code.
type PyObject uintptr

// IsNull reports whether the PyObject pointer is NULL.
func (o PyObject) IsNull() bool { return o == 0 }

// config holds resolved configuration for a Runtime.
type config struct {
	libraryPath  string   // explicit path; empty = auto-detect
	major        int      // 0 = any
	minor        int      // 0 = any
	preferUV     bool     // prefer uv-managed Python
	projectDir   string   // WithUVProject: pyproject.toml directory
	dependencies []string // Dependencies: inline package requirements
}

// Option configures a Runtime.
type Option func(*config)

// WithLibraryPath sets an explicit path to the Python shared library,
// bypassing auto-detection.
func WithLibraryPath(path string) Option {
	return func(c *config) { c.libraryPath = path }
}

// WithVersion constrains auto-detection to a specific Python major.minor.
func WithVersion(major, minor int) Option {
	return func(c *config) {
		c.major = major
		c.minor = minor
	}
}

// Runtime holds a loaded CPython shared library and its bound C-API functions.
// Only one Runtime should be active per process because Py_Initialize/Py_Finalize
// operate on global state. For multi-goroutine usage, use WithGIL or GILEnsure.
type Runtime struct {
	handle uintptr

	// --- Phase 1: lifecycle & execution ---
	pyInitialize      func()
	pyFinalize        func()
	pyGetVersion      func() string
	pyRunSimpleString func(string) int32
	pyRunStringFlags  func(string, int32, uintptr, uintptr, uintptr) uintptr // PyRun_StringFlags
	pyImportModule    func(string) uintptr
	pyErrOccurred     func() uintptr
	pyErrPrint        func()

	// --- Reference counting ---
	pyIncRef func(uintptr)
	pyDecRef func(uintptr)

	// --- Integer ---
	pyLongFromLongLong         func(int64) uintptr
	pyLongFromUnsignedLongLong func(uint64) uintptr
	pyLongAsLongLong           func(uintptr) int64
	pyLongAsUnsignedLongLong   func(uintptr) uint64

	// --- Float ---
	pyFloatFromDouble func(float64) uintptr
	pyFloatAsDouble   func(uintptr) float64

	// --- String ---
	pyUnicodeFromString func(string) uintptr
	pyUnicodeAsUTF8     func(uintptr) string

	// --- Bytes ---
	pyBytesFromStringAndSize func(uintptr, int64) uintptr
	pyBytesAsString          func(uintptr) uintptr
	pyBytesSize              func(uintptr) int64
	pyBytearrayAsString      func(uintptr) uintptr
	pyBytearraySize          func(uintptr) int64

	// --- Bool ---
	pyBoolFromLong func(int64) uintptr
	pyObjectIsTrue func(uintptr) int32

	// --- List ---
	pyListNew     func(int64) uintptr
	pyListSetItem func(uintptr, int64, uintptr) int32
	pyListGetItem func(uintptr, int64) uintptr
	pyListSize    func(uintptr) int64

	// --- Dict ---
	pyDictNew           func() uintptr
	pyDictSetItemString func(uintptr, string, uintptr) int32
	pyDictGetItemString func(uintptr, string) uintptr
	pyDictKeys          func(uintptr) uintptr
	pyDictSize          func(uintptr) int64

	// --- Tuple ---
	pyTupleNew     func(int64) uintptr
	pyTupleSetItem func(uintptr, int64, uintptr) int32
	pyTupleSize    func(uintptr) int64
	pyTupleGetItem func(uintptr, int64) uintptr

	// --- Set ---
	pySetNew  func(uintptr) uintptr
	pySetAdd  func(uintptr, uintptr) int32
	pySetSize func(uintptr) int64

	// --- Iterator ---
	pyObjectGetIter func(uintptr) uintptr
	pyIterNext      func(uintptr) uintptr

	// --- Object protocol ---
	pyObjectCall            func(uintptr, uintptr, uintptr) uintptr
	pyObjectGetAttrString   func(uintptr, string) uintptr
	pyObjectSetAttrString   func(uintptr, string, uintptr) int32
	pyObjectSetItem         func(uintptr, uintptr, uintptr) int32
	pyObjectGetItem         func(uintptr, uintptr) uintptr
	pyObjectDelItem         func(uintptr, uintptr) int32
	pySequenceContains      func(uintptr, uintptr) int32
	pyObjectLength          func(uintptr) int64
	pyObjectStr             func(uintptr) uintptr
	pyObjectRepr            func(uintptr) uintptr
	pyObjectType            func(uintptr) uintptr
	pyObjectRichCompareBool func(uintptr, uintptr, int32) int32

	// --- Module ---
	pyImportAddModule func(string) uintptr

	// --- CFunction ---
	pyCFunctionNew func(uintptr, uintptr) uintptr

	// --- Callback state ---
	callbacks   []callbackEntry
	bridgeReady bool
	bridgeKeep  []any // prevent GC of callback-related allocations

	// --- Error handling ---
	// PyErr_GetRaisedException (Python 3.12+): returns new ref, clears error.
	pyErrGetRaisedException func() uintptr
	// PyErr_SetRaisedException (Python 3.12+): restores exception.
	pyErrSetRaisedException func(uintptr)
	// PyErr_SetString: set exception with type and message.
	pyErrSetString func(uintptr, string)

	// --- GIL ---
	pyGILStateEnsure    func() int32
	pyGILStateRelease   func(int32)
	pyEvalSaveThread    func() uintptr
	pyEvalRestoreThread func(uintptr)

	// --- Global singletons ---
	pyNone            uintptr // _Py_NoneStruct
	pyExcRuntimeError uintptr // PyExc_RuntimeError

	// --- Cached type pointers (for fast type detection) ---
	cachedBoolType      uintptr
	cachedIntType       uintptr
	cachedFloatType     uintptr
	cachedStrType       uintptr
	cachedBytesType     uintptr
	cachedBytearrayType uintptr
	cachedListType      uintptr
	cachedTupleType     uintptr
	cachedDictType      uintptr
	cachedSetType       uintptr
	cachedFrozenSetType uintptr

	// --- Cached helpers for Exec ---
	cachedMainDict uintptr // __main__.__dict__ (new ref, held for lifetime)

	freeThreaded    bool    // true if running free-threaded Python (3.13t+)
	initThreadState uintptr // saved by pyEvalSaveThread in New()

	closeOnce sync.Once
	closed    atomic.Bool
}

// New creates a new Python runtime. It loads the CPython shared library
// and calls Py_Initialize.
//
// The caller must call Close when finished. Options can customize the
// Python version or library path.
func New(opts ...Option) (*Runtime, error) {
	var cfg config
	for _, o := range opts {
		o(&cfg)
	}

	// Level 2: set up venv if WithUVProject or Dependencies is used.
	var venvDir string
	if cfg.projectDir != "" {
		dir, err := setupUVProject(cfg.projectDir)
		if err != nil {
			return nil, err
		}
		venvDir = dir
	} else if len(cfg.dependencies) > 0 {
		dir, err := ensureDepsVenv(cfg.dependencies, cfg.major, cfg.minor)
		if err != nil {
			return nil, err
		}
		venvDir = dir
	}

	// If we have a venv, derive the library path from it.
	if venvDir != "" && cfg.libraryPath == "" {
		libPath, err := venvPythonLibrary(venvDir)
		if err != nil {
			return nil, err
		}
		cfg.libraryPath = libPath
	}

	path, err := detectLibrary(cfg)
	if err != nil {
		return nil, err
	}

	handle, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return nil, fmt.Errorf("pyffi: failed to load %s: %w", path, err)
	}

	// Set PYTHONHOME so Python can find its standard library.
	if os.Getenv("PYTHONHOME") == "" {
		if home := inferPythonHome(path); home != "" {
			os.Setenv("PYTHONHOME", home)
		}
	}

	rt := &Runtime{handle: handle}
	if err := rt.bindFunctions(); err != nil {
		return nil, err
	}

	rt.pyInitialize()
	rt.cacheTypePointers()

	// Detect free-threaded Python (3.13t+).
	if strings.Contains(rt.pyGetVersion(), "free-threading") {
		rt.freeThreaded = true
	}

	// Add venv site-packages to sys.path.
	if venvDir != "" {
		if siteDir, err := venvSitePackages(venvDir); err == nil {
			rt.addSitePackages(siteDir)
		}
	}

	// Release the init GIL so all subsequent operations use autoGIL.
	rt.initThreadState = rt.pyEvalSaveThread()

	runtime.SetFinalizer(rt, (*Runtime).finalize)

	return rt, nil
}

// Close calls Py_Finalize and releases the library handle.
// It is safe to call Close multiple times.
//
// WARNING: After Close, creating a new Runtime in the same process
// may fail because CPython does not fully support Py_Finalize followed
// by Py_Initialize. Design your application to use a single Runtime
// for the process lifetime.
func (r *Runtime) Close() error {
	r.closeOnce.Do(func() {
		runtime.SetFinalizer(r, nil)
		// Acquire GIL via autoGIL (works from any thread).
		g := r.autoGIL()
		// Set closed BEFORE Py_Finalize so that concurrent finalizers
		// see the flag and bail out instead of calling into a dead runtime.
		r.closed.Store(true)
		r.pyFinalize()
		// Don't release GIL after Finalize — the interpreter is gone.
		_ = g
	})
	return nil
}

// PythonVersion returns the Python version string (e.g. "3.13.2 (main, ...)").
func (r *Runtime) PythonVersion() string {
	g := r.autoGIL()
	defer g.release()
	return r.pyGetVersion()
}

// PythonVersionInfo returns the major, minor, and micro version numbers.
func (r *Runtime) PythonVersionInfo() (major, minor, micro int) {
	g := r.autoGIL()
	defer g.release()
	v := r.pyGetVersion()
	fmt.Sscanf(v, "%d.%d.%d", &major, &minor, &micro)
	return
}

// With executes fn with the context manager obj, calling __enter__ before
// and __exit__ after (even if fn returns an error).
func (r *Runtime) With(obj *Object, fn func(value *Object) error) error {
	g := r.autoGIL()
	defer g.release()
	value, err := obj.Enter()
	if err != nil {
		return err
	}
	defer value.Close()

	fnErr := fn(value)
	exitErr := obj.Exit(fnErr)

	if fnErr != nil {
		return fnErr
	}
	return exitErr
}

// Exec executes the given Python source code in the __main__ module.
func (r *Runtime) Exec(code string) error {
	g := r.autoGIL()
	defer g.release()
	if r.closed.Load() {
		return ErrAlreadyClosed
	}

	// Use PyRun_StringFlags for structured error extraction.
	// Unlike PyRun_SimpleString, it does NOT print to stderr on error,
	// allowing currentError() to capture Type, Message, and Traceback.
	if r.pyRunStringFlags != nil {
		mainDict := r.ensureMainDict()
		if mainDict != 0 {
			// Py_file_input = 257
			result := r.pyRunStringFlags(code, 257, mainDict, mainDict, 0)
			if result == 0 {
				return r.currentError("Exec")
			}
			r.pyDecRef(result)
			return nil
		}
	}

	// Fallback: PyRun_SimpleString (prints traceback to stderr).
	if r.pyRunSimpleString(code) == -1 {
		return &PythonError{Context: "Exec"}
	}
	return nil
}

// Eval evaluates a Python expression and returns the result.
func (r *Runtime) Eval(expr string) (*Object, error) {
	g := r.autoGIL()
	defer g.release()
	if r.closed.Load() {
		return nil, ErrAlreadyClosed
	}

	if r.pyRunStringFlags != nil {
		mainDict := r.ensureMainDict()
		if mainDict != 0 {
			// Py_eval_input = 258
			result := r.pyRunStringFlags(expr, 258, mainDict, mainDict, 0)
			if result == 0 {
				return nil, r.currentError("Eval")
			}
			return r.newObject(result), nil
		}
	}

	// Fallback: use builtins.eval() via Exec + __main__.
	varName := "__pyffi_eval_result"
	code := fmt.Sprintf("%s = eval(%q)", varName, expr)
	if err := r.Exec(code); err != nil {
		return nil, err
	}
	mainMod, err := r.Import("__main__")
	if err != nil {
		return nil, err
	}
	defer mainMod.Close()
	result, err := mainMod.AttrErr(varName)
	if err != nil {
		return nil, err
	}
	r.Exec("del " + varName)
	return result, nil
}

// RunAsync evaluates a Python expression that returns a coroutine and
// executes it with asyncio.run(), returning the result.
func (r *Runtime) RunAsync(expr string) (*Object, error) {
	g := r.autoGIL()
	defer g.release()
	if r.closed.Load() {
		return nil, ErrAlreadyClosed
	}
	code := fmt.Sprintf("import asyncio as __pyffi_asyncio; __pyffi_async_result = __pyffi_asyncio.run(%s)", expr)
	if err := r.Exec(code); err != nil {
		return nil, err
	}
	mainMod, err := r.Import("__main__")
	if err != nil {
		return nil, err
	}
	defer mainMod.Close()

	result, err := mainMod.AttrErr("__pyffi_async_result")
	if err != nil {
		return nil, err
	}
	// Clean up the temporary variable.
	r.Exec("del __pyffi_async_result, __pyffi_asyncio")
	return result, nil
}

// AsyncResult holds the result of an asynchronous Python operation.
type AsyncResult struct {
	Value *Object
	Err   error
}

// RunAsyncGo runs the async expression in a background goroutine.
// The caller's goroutine is not blocked. The GIL must have been released
// via ReleaseGIL before calling this method.
//
// The returned Object (if any) must be accessed with the GIL held.
func (r *Runtime) RunAsyncGo(expr string) <-chan AsyncResult {
	ch := make(chan AsyncResult, 1)
	go func() {
		var ar AsyncResult
		ar.Value, ar.Err = r.RunAsync(expr)
		ch <- ar
	}()
	return ch
}

// ensureMainDict lazily initializes and returns __main__.__dict__.
func (r *Runtime) ensureMainDict() uintptr {
	if r.cachedMainDict != 0 {
		return r.cachedMainDict
	}
	mainMod := r.pyImportAddModule("__main__") // borrowed ref
	if mainMod == 0 {
		return 0
	}
	r.cachedMainDict = r.pyObjectGetAttrString(mainMod, "__dict__") // new ref, held for lifetime
	return r.cachedMainDict
}

// ImportModule imports a Python module by name and returns it as a PyObject.
// The caller receives a new reference.
//
// Deprecated: Use Import instead for new code.
func (r *Runtime) ImportModule(name string) (PyObject, error) {
	g := r.autoGIL()
	defer g.release()
	if r.closed.Load() {
		return 0, ErrAlreadyClosed
	}
	obj := r.pyImportModule(name)
	if obj == 0 {
		if r.pyErrOccurred() != 0 {
			r.pyErrPrint()
		}
		return 0, &PythonError{Context: fmt.Sprintf("ImportModule(%q)", name)}
	}
	return PyObject(obj), nil
}

// Import imports a Python module by name and returns it as an *Object.
func (r *Runtime) Import(name string) (*Object, error) {
	g := r.autoGIL()
	defer g.release()
	if r.closed.Load() {
		return nil, ErrAlreadyClosed
	}
	ptr := r.pyImportModule(name)
	if ptr == 0 {
		return nil, r.currentError("Import(%q)", name)
	}
	return r.newObject(ptr), nil
}

// None returns a new reference to Python's None object.
func (r *Runtime) None() *Object {
	g := r.autoGIL()
	defer g.release()
	r.pyIncRef(r.pyNone)
	return r.newObject(r.pyNone)
}

// FromInt64 creates a Python int from a Go int64.
func (r *Runtime) FromInt64(v int64) *Object {
	g := r.autoGIL()
	defer g.release()
	return r.newObject(r.pyLongFromLongLong(v))
}

// FromFloat64 creates a Python float from a Go float64.
func (r *Runtime) FromFloat64(v float64) *Object {
	g := r.autoGIL()
	defer g.release()
	return r.newObject(r.pyFloatFromDouble(v))
}

// FromString creates a Python str from a Go string.
func (r *Runtime) FromString(v string) *Object {
	g := r.autoGIL()
	defer g.release()
	return r.newObject(r.pyUnicodeFromString(v))
}

// FromBool creates a Python bool from a Go bool.
func (r *Runtime) FromBool(v bool) *Object {
	g := r.autoGIL()
	defer g.release()
	var n int64
	if v {
		n = 1
	}
	return r.newObject(r.pyBoolFromLong(n))
}

// FromGoValue creates a Python object from an arbitrary Go value.
func (r *Runtime) FromGoValue(v any) (*Object, error) {
	g := r.autoGIL()
	defer g.release()
	ptr, err := r.goToPython(v)
	if err != nil {
		return nil, err
	}
	return r.newObject(ptr), nil
}

// NewList creates a Python list from the given Go values.
func (r *Runtime) NewList(items ...any) (*Object, error) {
	g := r.autoGIL()
	defer g.release()
	list := r.pyListNew(int64(len(items)))
	if list == 0 {
		return nil, r.currentError("NewList")
	}
	for i, item := range items {
		pyItem, err := r.goToPython(item)
		if err != nil {
			r.pyDecRef(list)
			return nil, fmt.Errorf("pyffi: NewList[%d]: %w", i, err)
		}
		// PyList_SetItem steals the reference.
		if r.pyListSetItem(list, int64(i), pyItem) != 0 {
			r.pyDecRef(list)
			return nil, r.currentError("NewList: PyList_SetItem")
		}
	}
	return r.newObject(list), nil
}

// NewTuple creates a Python tuple from the given Go values.
func (r *Runtime) NewTuple(items ...any) (*Object, error) {
	g := r.autoGIL()
	defer g.release()
	tuple := r.pyTupleNew(int64(len(items)))
	if tuple == 0 {
		return nil, r.currentError("NewTuple")
	}
	for i, item := range items {
		pyItem, err := r.goToPython(item)
		if err != nil {
			r.pyDecRef(tuple)
			return nil, fmt.Errorf("pyffi: NewTuple[%d]: %w", i, err)
		}
		// PyTuple_SetItem steals the reference.
		if r.pyTupleSetItem(tuple, int64(i), pyItem) != 0 {
			r.pyDecRef(tuple)
			return nil, r.currentError("NewTuple: PyTuple_SetItem")
		}
	}
	return r.newObject(tuple), nil
}

// NewDict creates a Python dict from alternating key-value pairs.
// Keys must be strings. Panics if an odd number of arguments is given.
func (r *Runtime) NewDict(kvs ...any) (*Object, error) {
	g := r.autoGIL()
	defer g.release()
	if len(kvs)%2 != 0 {
		return nil, fmt.Errorf("pyffi: NewDict requires even number of arguments (key-value pairs), got %d", len(kvs))
	}
	dict := r.pyDictNew()
	if dict == 0 {
		return nil, r.currentError("NewDict")
	}
	for i := 0; i < len(kvs); i += 2 {
		key, ok := kvs[i].(string)
		if !ok {
			r.pyDecRef(dict)
			return nil, fmt.Errorf("pyffi: NewDict key at position %d must be string, got %T", i, kvs[i])
		}
		pyVal, err := r.goToPython(kvs[i+1])
		if err != nil {
			r.pyDecRef(dict)
			return nil, fmt.Errorf("pyffi: NewDict[%q]: %w", key, err)
		}
		// PyDict_SetItemString does NOT steal the reference.
		if r.pyDictSetItemString(dict, key, pyVal) != 0 {
			r.pyDecRef(pyVal)
			r.pyDecRef(dict)
			return nil, r.currentError("NewDict: PyDict_SetItemString")
		}
		r.pyDecRef(pyVal)
	}
	return r.newObject(dict), nil
}

// NewSet creates a Python set from the given Go values.
func (r *Runtime) NewSet(items ...any) (*Object, error) {
	g := r.autoGIL()
	defer g.release()
	// PySet_New(NULL) creates an empty set.
	set := r.pySetNew(0)
	if set == 0 {
		return nil, r.currentError("NewSet")
	}
	for i, item := range items {
		pyItem, err := r.goToPython(item)
		if err != nil {
			r.pyDecRef(set)
			return nil, fmt.Errorf("pyffi: NewSet[%d]: %w", i, err)
		}
		// PySet_Add does NOT steal the reference.
		if r.pySetAdd(set, pyItem) != 0 {
			r.pyDecRef(pyItem)
			r.pyDecRef(set)
			return nil, r.currentError("NewSet: PySet_Add")
		}
		r.pyDecRef(pyItem)
	}
	return r.newObject(set), nil
}

// bindFunctions binds all required CPython C-API functions via purego.
func (r *Runtime) bindFunctions() (err error) {
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("pyffi: failed to bind C function: %v", v)
		}
	}()

	// Phase 1
	purego.RegisterLibFunc(&r.pyInitialize, r.handle, "Py_Initialize")
	purego.RegisterLibFunc(&r.pyFinalize, r.handle, "Py_Finalize")
	purego.RegisterLibFunc(&r.pyGetVersion, r.handle, "Py_GetVersion")
	purego.RegisterLibFunc(&r.pyRunSimpleString, r.handle, "PyRun_SimpleString")
	r.bindRunStringFlags()
	purego.RegisterLibFunc(&r.pyImportModule, r.handle, "PyImport_ImportModule")
	purego.RegisterLibFunc(&r.pyErrOccurred, r.handle, "PyErr_Occurred")
	purego.RegisterLibFunc(&r.pyErrPrint, r.handle, "PyErr_Print")

	// Reference counting
	purego.RegisterLibFunc(&r.pyIncRef, r.handle, "Py_IncRef")
	purego.RegisterLibFunc(&r.pyDecRef, r.handle, "Py_DecRef")

	// Integer
	purego.RegisterLibFunc(&r.pyLongFromLongLong, r.handle, "PyLong_FromLongLong")
	purego.RegisterLibFunc(&r.pyLongFromUnsignedLongLong, r.handle, "PyLong_FromUnsignedLongLong")
	purego.RegisterLibFunc(&r.pyLongAsLongLong, r.handle, "PyLong_AsLongLong")
	purego.RegisterLibFunc(&r.pyLongAsUnsignedLongLong, r.handle, "PyLong_AsUnsignedLongLong")

	// Float
	purego.RegisterLibFunc(&r.pyFloatFromDouble, r.handle, "PyFloat_FromDouble")
	purego.RegisterLibFunc(&r.pyFloatAsDouble, r.handle, "PyFloat_AsDouble")

	// String
	purego.RegisterLibFunc(&r.pyUnicodeFromString, r.handle, "PyUnicode_FromString")
	purego.RegisterLibFunc(&r.pyUnicodeAsUTF8, r.handle, "PyUnicode_AsUTF8")

	// Bytes
	purego.RegisterLibFunc(&r.pyBytesFromStringAndSize, r.handle, "PyBytes_FromStringAndSize")
	purego.RegisterLibFunc(&r.pyBytesAsString, r.handle, "PyBytes_AsString")
	purego.RegisterLibFunc(&r.pyBytesSize, r.handle, "PyBytes_Size")
	purego.RegisterLibFunc(&r.pyBytearrayAsString, r.handle, "PyByteArray_AsString")
	purego.RegisterLibFunc(&r.pyBytearraySize, r.handle, "PyByteArray_Size")

	// Bool
	purego.RegisterLibFunc(&r.pyBoolFromLong, r.handle, "PyBool_FromLong")
	purego.RegisterLibFunc(&r.pyObjectIsTrue, r.handle, "PyObject_IsTrue")

	// List
	purego.RegisterLibFunc(&r.pyListNew, r.handle, "PyList_New")
	purego.RegisterLibFunc(&r.pyListSetItem, r.handle, "PyList_SetItem")
	purego.RegisterLibFunc(&r.pyListGetItem, r.handle, "PyList_GetItem")
	purego.RegisterLibFunc(&r.pyListSize, r.handle, "PyList_Size")

	// Dict
	purego.RegisterLibFunc(&r.pyDictNew, r.handle, "PyDict_New")
	purego.RegisterLibFunc(&r.pyDictSetItemString, r.handle, "PyDict_SetItemString")
	purego.RegisterLibFunc(&r.pyDictGetItemString, r.handle, "PyDict_GetItemString")
	purego.RegisterLibFunc(&r.pyDictKeys, r.handle, "PyDict_Keys")
	purego.RegisterLibFunc(&r.pyDictSize, r.handle, "PyDict_Size")

	// Tuple
	purego.RegisterLibFunc(&r.pyTupleNew, r.handle, "PyTuple_New")
	purego.RegisterLibFunc(&r.pyTupleSetItem, r.handle, "PyTuple_SetItem")
	purego.RegisterLibFunc(&r.pyTupleSize, r.handle, "PyTuple_Size")
	purego.RegisterLibFunc(&r.pyTupleGetItem, r.handle, "PyTuple_GetItem")

	// Set
	purego.RegisterLibFunc(&r.pySetNew, r.handle, "PySet_New")
	purego.RegisterLibFunc(&r.pySetAdd, r.handle, "PySet_Add")
	purego.RegisterLibFunc(&r.pySetSize, r.handle, "PySet_Size")

	// Iterator
	purego.RegisterLibFunc(&r.pyObjectGetIter, r.handle, "PyObject_GetIter")
	purego.RegisterLibFunc(&r.pyIterNext, r.handle, "PyIter_Next")

	// Object protocol
	purego.RegisterLibFunc(&r.pyObjectCall, r.handle, "PyObject_Call")
	purego.RegisterLibFunc(&r.pyObjectGetAttrString, r.handle, "PyObject_GetAttrString")
	purego.RegisterLibFunc(&r.pyObjectSetAttrString, r.handle, "PyObject_SetAttrString")
	purego.RegisterLibFunc(&r.pyObjectSetItem, r.handle, "PyObject_SetItem")
	purego.RegisterLibFunc(&r.pyObjectGetItem, r.handle, "PyObject_GetItem")
	purego.RegisterLibFunc(&r.pyObjectDelItem, r.handle, "PyObject_DelItem")
	purego.RegisterLibFunc(&r.pySequenceContains, r.handle, "PySequence_Contains")
	purego.RegisterLibFunc(&r.pyObjectLength, r.handle, "PyObject_Length")
	purego.RegisterLibFunc(&r.pyObjectStr, r.handle, "PyObject_Str")
	purego.RegisterLibFunc(&r.pyObjectRepr, r.handle, "PyObject_Repr")
	purego.RegisterLibFunc(&r.pyObjectType, r.handle, "PyObject_Type")
	purego.RegisterLibFunc(&r.pyObjectRichCompareBool, r.handle, "PyObject_RichCompareBool")

	// Module
	purego.RegisterLibFunc(&r.pyImportAddModule, r.handle, "PyImport_AddModule")

	// CFunction
	purego.RegisterLibFunc(&r.pyCFunctionNew, r.handle, "PyCFunction_New")

	// Error handling
	purego.RegisterLibFunc(&r.pyErrSetString, r.handle, "PyErr_SetString")
	r.bindErrGetRaisedException()

	// GIL
	purego.RegisterLibFunc(&r.pyGILStateEnsure, r.handle, "PyGILState_Ensure")
	purego.RegisterLibFunc(&r.pyGILStateRelease, r.handle, "PyGILState_Release")
	purego.RegisterLibFunc(&r.pyEvalSaveThread, r.handle, "PyEval_SaveThread")
	purego.RegisterLibFunc(&r.pyEvalRestoreThread, r.handle, "PyEval_RestoreThread")

	// Global singletons
	var err2 error
	r.pyNone, err2 = purego.Dlsym(r.handle, "_Py_NoneStruct")
	if err2 != nil {
		return fmt.Errorf("pyffi: failed to find _Py_NoneStruct: %w", err2)
	}
	// pyExcRuntimeError is cached later in cacheTypePointers.

	return nil
}

// cacheTypePointers caches Python built-in type pointers for fast type detection.
func (r *Runtime) cacheTypePointers() {
	builtins := r.pyImportModule("builtins")
	if builtins == 0 {
		return
	}
	defer r.pyDecRef(builtins)

	cache := func(name string) uintptr {
		obj := r.pyObjectGetAttrString(builtins, name)
		if obj == 0 {
			return 0
		}
		// obj is the type object itself (e.g., <class 'int'>).
		// We keep a reference to it for the lifetime of the Runtime.
		return obj
	}

	r.cachedBoolType = cache("bool")
	r.cachedIntType = cache("int")
	r.cachedFloatType = cache("float")
	r.cachedStrType = cache("str")
	r.cachedBytesType = cache("bytes")
	r.cachedBytearrayType = cache("bytearray")
	r.cachedListType = cache("list")
	r.cachedTupleType = cache("tuple")
	r.cachedDictType = cache("dict")
	r.cachedSetType = cache("set")
	r.cachedFrozenSetType = cache("frozenset")
	r.pyExcRuntimeError = cache("RuntimeError")
}

// bindRunStringFlags tries to bind PyRun_StringFlags for structured Exec error handling.
// PyRun_String is a macro in CPython; the actual symbol is PyRun_StringFlags.
func (r *Runtime) bindRunStringFlags() {
	if _, err := purego.Dlsym(r.handle, "PyRun_StringFlags"); err != nil {
		return
	}
	purego.RegisterLibFunc(&r.pyRunStringFlags, r.handle, "PyRun_StringFlags")
}

// bindErrGetRaisedException tries to bind PyErr_GetRaisedException (Python 3.12+).
// On older Python versions, these symbols don't exist and we fall back
// to PyErr_Occurred + PyErr_Print for error extraction.
func (r *Runtime) bindErrGetRaisedException() {
	// Check if the symbol exists before binding.
	// purego.RegisterLibFunc panics if the symbol is not found,
	// but we only want to skip binding for missing symbols, not hide other errors.
	if _, err := purego.Dlsym(r.handle, "PyErr_GetRaisedException"); err != nil {
		return // Python < 3.12, fall back to PyErr_Print
	}
	purego.RegisterLibFunc(&r.pyErrGetRaisedException, r.handle, "PyErr_GetRaisedException")
	purego.RegisterLibFunc(&r.pyErrSetRaisedException, r.handle, "PyErr_SetRaisedException")
}

func (r *Runtime) finalize() {
	r.Close()
}
