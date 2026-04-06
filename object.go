package pyffi

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
)

// noCopy may be embedded into structs which must not be copied after first use.
// See https://golang.org/issues/8005#issuecomment-190753527
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

// Object wraps a CPython PyObject* with explicit lifecycle management.
// Call Close() to release the reference when done. A GC finalizer provides
// a safety net, but explicit Close() is strongly recommended.
//
// The zero value and nil receiver are safe — all methods return ErrNilObject
// or nil as appropriate.
//
// Objects are not safe for concurrent use from multiple goroutines.
// Use [Runtime.WithGIL] to access Python objects from different goroutines.
type Object struct {
	noCopy noCopy
	ptr    uintptr
	rt     *Runtime
	closed atomic.Bool
}

// newObject creates an Object that owns the given new reference.
// Returns nil if ptr is 0 (NULL).
func (r *Runtime) newObject(ptr uintptr) *Object {
	if ptr == 0 {
		return nil
	}
	o := &Object{ptr: ptr, rt: r}
	runtime.SetFinalizer(o, (*Object).release)
	return o
}

// Close releases the Python reference. It is safe to call multiple times
// or on a nil receiver.
func (o *Object) Close() error {
	if o == nil || o.closed.Swap(true) {
		return nil
	}
	runtime.SetFinalizer(o, nil)
	g := o.rt.autoGIL()
	defer g.release()
	o.rt.pyDecRef(o.ptr)
	o.ptr = 0
	return nil
}

// release is the GC finalizer. It acquires the GIL before calling DecRef
// because finalizers run on an arbitrary goroutine/thread.
func (o *Object) release() {
	if o == nil || o.closed.Swap(true) {
		return
	}
	if o.rt.closed.Load() {
		// Runtime already finalized; leak rather than crash.
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	state := o.rt.pyGILStateEnsure()
	o.rt.pyDecRef(o.ptr)
	o.rt.pyGILStateRelease(state)
	o.ptr = 0
}

// Ptr returns the raw PyObject* pointer.
func (o *Object) Ptr() uintptr {
	if o == nil {
		return 0
	}
	return o.ptr
}

// IsNone reports whether this object is Python's None.
func (o *Object) IsNone() bool {
	return o != nil && o.ptr == o.rt.pyNone
}

// String returns the Python str() representation.
func (o *Object) String() string {
	if o == nil || o.ptr == 0 {
		return "<nil>"
	}
	g := o.rt.autoGIL()
	defer g.release()
	strObj := o.rt.pyObjectStr(o.ptr)
	runtime.KeepAlive(o)
	if strObj == 0 {
		return "<error>"
	}
	defer o.rt.pyDecRef(strObj)
	return o.rt.pyUnicodeAsUTF8(strObj)
}

// Attr returns the named attribute. Returns nil if the attribute does not exist,
// which allows chaining: obj.Attr("path").Attr("join").Call(...)
// Use AttrErr if you need the error.
func (o *Object) Attr(name string) *Object {
	if o == nil || o.ptr == 0 {
		return nil
	}
	g := o.rt.autoGIL()
	defer g.release()
	ptr := o.rt.pyObjectGetAttrString(o.ptr, name)
	runtime.KeepAlive(o)
	if ptr == 0 {
		// Clear Python error indicator.
		o.rt.clearError()
		return nil
	}
	return o.rt.newObject(ptr)
}

// AttrErr returns the named attribute or an error.
func (o *Object) AttrErr(name string) (*Object, error) {
	if o == nil || o.ptr == 0 {
		return nil, ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	ptr := o.rt.pyObjectGetAttrString(o.ptr, name)
	runtime.KeepAlive(o)
	if ptr == 0 {
		return nil, o.rt.currentError("Attr(%q)", name)
	}
	return o.rt.newObject(ptr), nil
}

// SetAttr sets the named attribute to the given Go value.
func (o *Object) SetAttr(name string, value any) error {
	if o == nil || o.ptr == 0 {
		return ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	pyVal, err := o.rt.goToPython(value)
	if err != nil {
		return fmt.Errorf("pyffi: SetAttr(%q): %w", name, err)
	}
	// PyObject_SetAttrString does NOT steal the reference.
	if o.rt.pyObjectSetAttrString(o.ptr, name, pyVal) != 0 {
		o.rt.pyDecRef(pyVal)
		runtime.KeepAlive(o)
		return o.rt.currentError("SetAttr(%q)", name)
	}
	o.rt.pyDecRef(pyVal)
	runtime.KeepAlive(o)
	return nil
}

// GetItem returns obj[key] using the Python __getitem__ protocol.
func (o *Object) GetItem(key any) (*Object, error) {
	if o == nil || o.ptr == 0 {
		return nil, ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	pyKey, err := o.rt.goToPython(key)
	if err != nil {
		return nil, fmt.Errorf("pyffi: GetItem key: %w", err)
	}
	result := o.rt.pyObjectGetItem(o.ptr, pyKey)
	o.rt.pyDecRef(pyKey)
	runtime.KeepAlive(o)
	if result == 0 {
		return nil, o.rt.currentError("GetItem")
	}
	return o.rt.newObject(result), nil
}

// DelItem deletes obj[key] using the Python __delitem__ protocol.
func (o *Object) DelItem(key any) error {
	if o == nil || o.ptr == 0 {
		return ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	pyKey, err := o.rt.goToPython(key)
	if err != nil {
		return fmt.Errorf("pyffi: DelItem key: %w", err)
	}
	rc := o.rt.pyObjectDelItem(o.ptr, pyKey)
	o.rt.pyDecRef(pyKey)
	runtime.KeepAlive(o)
	if rc != 0 {
		return o.rt.currentError("DelItem")
	}
	return nil
}

// Contains checks if item is in the object (equivalent to Python's "in" operator).
func (o *Object) Contains(item any) (bool, error) {
	if o == nil || o.ptr == 0 {
		return false, ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	pyItem, err := o.rt.goToPython(item)
	if err != nil {
		return false, fmt.Errorf("pyffi: Contains: %w", err)
	}
	rc := o.rt.pySequenceContains(o.ptr, pyItem)
	o.rt.pyDecRef(pyItem)
	runtime.KeepAlive(o)
	if rc == -1 {
		return false, o.rt.currentError("Contains")
	}
	return rc == 1, nil
}

// Len returns the length of the object (equivalent to Python's len()).
func (o *Object) Len() (int64, error) {
	if o == nil || o.ptr == 0 {
		return 0, ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	n := o.rt.pyObjectLength(o.ptr)
	runtime.KeepAlive(o)
	if n == -1 && o.rt.pyErrOccurred() != 0 {
		return 0, o.rt.currentError("Len")
	}
	return n, nil
}

// Python comparison operator constants for use with Compare.
const (
	PyLT = 0 // <
	PyLE = 1 // <=
	PyEQ = 2 // ==
	PyNE = 3 // !=
	PyGT = 4 // >
	PyGE = 5 // >=
)

// Equals returns true if this object equals other (Python ==).
func (o *Object) Equals(other *Object) (bool, error) {
	return o.Compare(other, PyEQ)
}

// Compare performs a rich comparison between this object and other.
// Use PyLT, PyLE, PyEQ, PyNE, PyGT, PyGE as the op argument.
func (o *Object) Compare(other *Object, op int) (bool, error) {
	if o == nil || o.ptr == 0 {
		return false, ErrNilObject
	}
	if other == nil || other.ptr == 0 {
		return false, ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	rc := o.rt.pyObjectRichCompareBool(o.ptr, other.ptr, int32(op))
	runtime.KeepAlive(o)
	runtime.KeepAlive(other)
	if rc == -1 {
		return false, o.rt.currentError("Compare")
	}
	return rc == 1, nil
}

// Iter returns an iterator over the object's elements.
// The caller should call Next() in a loop until it returns nil, nil.
func (o *Object) Iter() (*Iterator, error) {
	if o == nil || o.ptr == 0 {
		return nil, ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	iter := o.rt.pyObjectGetIter(o.ptr)
	runtime.KeepAlive(o)
	if iter == 0 {
		return nil, o.rt.currentError("Iter")
	}
	return &Iterator{rt: o.rt, ptr: iter}, nil
}

// Iterator wraps a Python iterator object.
type Iterator struct {
	rt  *Runtime
	ptr uintptr
}

// Next returns the next item from the iterator.
// Returns (nil, nil) when the iterator is exhausted.
func (it *Iterator) Next() (*Object, error) {
	g := it.rt.autoGIL()
	defer g.release()
	item := it.rt.pyIterNext(it.ptr) // new reference
	if item == 0 {
		if it.rt.pyErrOccurred() != 0 {
			return nil, it.rt.currentError("Iterator.Next")
		}
		return nil, nil // StopIteration
	}
	return it.rt.newObject(item), nil
}

// Close releases the iterator.
func (it *Iterator) Close() {
	if it.ptr != 0 {
		g := it.rt.autoGIL()
		it.rt.pyDecRef(it.ptr)
		it.ptr = 0
		g.release()
	}
}

// Enter calls __enter__ on the object (context manager protocol).
func (o *Object) Enter() (*Object, error) {
	if o == nil || o.ptr == 0 {
		return nil, ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	enterFn, err := o.AttrErr("__enter__")
	if err != nil {
		return nil, fmt.Errorf("pyffi: Enter: %w", err)
	}
	defer enterFn.Close()
	return enterFn.Call()
}

// Exit calls __exit__ on the object (context manager protocol).
// If err is nil, None is passed for all three arguments.
// If err is a *PythonError, the exception info is passed (as None values,
// since reconstructing the original exception is not supported).
func (o *Object) Exit(err error) error {
	if o == nil || o.ptr == 0 {
		return ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	exitFn, exitErr := o.AttrErr("__exit__")
	if exitErr != nil {
		return fmt.Errorf("pyffi: Exit: %w", exitErr)
	}
	defer exitFn.Close()
	_, callErr := exitFn.Call(nil, nil, nil)
	return callErr
}

// Repr returns the Python repr() of the object.
func (o *Object) Repr() (string, error) {
	if o == nil || o.ptr == 0 {
		return "", ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	reprObj := o.rt.pyObjectRepr(o.ptr)
	runtime.KeepAlive(o)
	if reprObj == 0 {
		return "", o.rt.currentError("Repr")
	}
	s := o.rt.pyUnicodeAsUTF8(reprObj)
	o.rt.pyDecRef(reprObj)
	return s, nil
}

// SetItem sets obj[key] = value using the Python __setitem__ protocol.
func (o *Object) SetItem(key, value any) error {
	if o == nil || o.ptr == 0 {
		return ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	pyKey, err := o.rt.goToPython(key)
	if err != nil {
		return fmt.Errorf("pyffi: SetItem key: %w", err)
	}
	pyVal, err := o.rt.goToPython(value)
	if err != nil {
		o.rt.pyDecRef(pyKey)
		return fmt.Errorf("pyffi: SetItem value: %w", err)
	}
	// PyObject_SetItem does NOT steal references.
	rc := o.rt.pyObjectSetItem(o.ptr, pyKey, pyVal)
	o.rt.pyDecRef(pyKey)
	o.rt.pyDecRef(pyVal)
	runtime.KeepAlive(o)
	if rc != 0 {
		return o.rt.currentError("SetItem")
	}
	return nil
}

// Call calls this object with the given arguments.
// If the last argument is of type KW, it is used as keyword arguments.
func (o *Object) Call(args ...any) (*Object, error) {
	if o == nil || o.ptr == 0 {
		return nil, ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	if len(args) > 0 {
		if kw, ok := args[len(args)-1].(KW); ok {
			return o.CallKw(args[:len(args)-1], kw)
		}
	}
	return o.CallKw(args, nil)
}

// CallKw calls this object with positional and keyword arguments.
func (o *Object) CallKw(args []any, kwargs map[string]any) (*Object, error) {
	if o == nil || o.ptr == 0 {
		return nil, ErrNilObject
	}
	return o.rt.callKw(o.ptr, args, kwargs)
}

// callKw is the internal implementation shared by CallKw and CallKwContext.
// It acquires the GIL, builds the args tuple and kwargs dict, and calls the
// Python callable identified by the raw pointer.
func (r *Runtime) callKw(callable uintptr, args []any, kwargs map[string]any) (*Object, error) {
	g := r.autoGIL()
	defer g.release()

	// Build args tuple.
	tuple := r.pyTupleNew(int64(len(args)))
	if tuple == 0 {
		return nil, r.currentError("Call: PyTuple_New")
	}
	for i, arg := range args {
		pyArg, err := r.goToPython(arg)
		if err != nil {
			r.pyDecRef(tuple)
			return nil, fmt.Errorf("pyffi: Call arg %d: %w", i, err)
		}
		// PyTuple_SetItem steals the reference.
		if r.pyTupleSetItem(tuple, int64(i), pyArg) != 0 {
			r.pyDecRef(tuple)
			return nil, r.currentError("Call: PyTuple_SetItem")
		}
	}

	// Build kwargs dict.
	var kw uintptr
	if len(kwargs) > 0 {
		kw = r.pyDictNew()
		if kw == 0 {
			r.pyDecRef(tuple)
			return nil, r.currentError("Call: PyDict_New")
		}
		for k, v := range kwargs {
			pyVal, err := r.goToPython(v)
			if err != nil {
				r.pyDecRef(tuple)
				r.pyDecRef(kw)
				return nil, fmt.Errorf("pyffi: Call kwarg %q: %w", k, err)
			}
			// PyDict_SetItemString does NOT steal the reference.
			if r.pyDictSetItemString(kw, k, pyVal) != 0 {
				r.pyDecRef(pyVal)
				r.pyDecRef(tuple)
				r.pyDecRef(kw)
				return nil, r.currentError("Call: PyDict_SetItemString")
			}
			r.pyDecRef(pyVal)
		}
	}

	result := r.pyObjectCall(callable, tuple, kw)
	r.pyDecRef(tuple)
	if kw != 0 {
		r.pyDecRef(kw)
	}

	if result == 0 {
		return nil, r.currentError("Call")
	}
	return r.newObject(result), nil
}

// CallContext calls this object with the given arguments, respecting
// context cancellation and deadlines. If the context is cancelled before the
// Python call completes, the Go caller returns ctx.Err() immediately.
//
// Caveat: the Python call itself cannot be interrupted from Go. When the
// context is cancelled, the background goroutine (and the OS thread pinned
// by the GIL) remains blocked until the Python function returns naturally.
// If the function never returns, the goroutine and OS thread are leaked.
// This method is best suited for Python functions that are slow but
// eventually complete; it is not a safeguard against functions that block
// indefinitely.
//
// If the last argument is of type KW, it is used as keyword arguments.
func (o *Object) CallContext(ctx context.Context, args ...any) (*Object, error) {
	if o == nil || o.ptr == 0 {
		return nil, ErrNilObject
	}
	if len(args) > 0 {
		if kw, ok := args[len(args)-1].(KW); ok {
			return o.CallKwContext(ctx, args[:len(args)-1], kw)
		}
	}
	return o.CallKwContext(ctx, args, nil)
}

// CallKwContext calls this object with positional and keyword arguments,
// respecting context cancellation and deadlines. If the context is cancelled
// before the Python call completes, the Go caller returns ctx.Err() immediately.
//
// Caveat: the Python call itself cannot be interrupted from Go. When the
// context is cancelled, the background goroutine (and the OS thread pinned
// by the GIL) remains blocked until the Python function returns naturally.
// If the function never returns, the goroutine and OS thread are leaked.
// This method is best suited for Python functions that are slow but
// eventually complete; it is not a safeguard against functions that block
// indefinitely.
func (o *Object) CallKwContext(ctx context.Context, args []any, kwargs map[string]any) (*Object, error) {
	if o == nil || o.ptr == 0 {
		return nil, ErrNilObject
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	rt := o.rt

	// IncRef the callable so it stays alive even if the caller closes the
	// Object after we return due to context cancellation.
	g := rt.autoGIL()
	rt.pyIncRef(o.ptr)
	callablePtr := o.ptr
	g.release()

	type callResult struct {
		obj *Object
		err error
	}
	ch := make(chan callResult, 1)

	go func() {
		result, err := rt.callKw(callablePtr, args, kwargs)

		cg := rt.autoGIL()
		rt.pyDecRef(callablePtr)
		cg.release()

		ch <- callResult{obj: result, err: err}
	}()

	select {
	case <-ctx.Done():
		// Drain the result in a background goroutine to clean up properly.
		go func() {
			r := <-ch
			if r.obj != nil {
				r.obj.Close()
			}
		}()
		return nil, ctx.Err()
	case r := <-ch:
		return r.obj, r.err
	}
}

// CallAsync calls an async Python function with the given arguments,
// running the resulting coroutine with asyncio.run().
func (o *Object) CallAsync(args ...any) (*Object, error) {
	if o == nil || o.ptr == 0 {
		return nil, ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	// Call the function to get the coroutine.
	coro, err := o.Call(args...)
	if err != nil {
		return nil, err
	}
	defer coro.Close()

	// Run the coroutine with asyncio.run().
	asyncio, err := o.rt.Import("asyncio")
	if err != nil {
		return nil, fmt.Errorf("pyffi: CallAsync: %w", err)
	}
	defer asyncio.Close()

	runFn := asyncio.Attr("run")
	if runFn == nil {
		return nil, fmt.Errorf("pyffi: CallAsync: asyncio.run not found")
	}
	defer runFn.Close()

	return runFn.Call(coro)
}

// CallAsyncGo calls an async Python function in a background goroutine.
// The caller's goroutine is not blocked. The GIL must have been released
// via ReleaseGIL before calling this method.
//
// The returned Object (if any) must be accessed with the GIL held.
func (o *Object) CallAsyncGo(args ...any) <-chan AsyncResult {
	ch := make(chan AsyncResult, 1)
	go func() {
		var ar AsyncResult
		ar.Value, ar.Err = o.CallAsync(args...)
		ch <- ar
	}()
	return ch
}

// Int64 returns the Python int value as a Go int64.
func (o *Object) Int64() (int64, error) {
	if o == nil || o.ptr == 0 {
		return 0, ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	v := o.rt.pyLongAsLongLong(o.ptr)
	runtime.KeepAlive(o)
	if v == -1 && o.rt.pyErrOccurred() != 0 {
		return 0, o.rt.currentError("Int64")
	}
	return v, nil
}

// Float64 returns the Python float value as a Go float64.
func (o *Object) Float64() (float64, error) {
	if o == nil || o.ptr == 0 {
		return 0, ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	v := o.rt.pyFloatAsDouble(o.ptr)
	runtime.KeepAlive(o)
	if v == -1.0 && o.rt.pyErrOccurred() != 0 {
		return 0, o.rt.currentError("Float64")
	}
	return v, nil
}

// GoString returns the Python str value as a Go string.
func (o *Object) GoString() (string, error) {
	if o == nil || o.ptr == 0 {
		return "", ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	s := o.rt.pyUnicodeAsUTF8(o.ptr)
	runtime.KeepAlive(o)
	return s, nil
}

// Bool returns the Python bool value as a Go bool.
func (o *Object) Bool() (bool, error) {
	if o == nil || o.ptr == 0 {
		return false, ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	v := o.rt.pyObjectIsTrue(o.ptr)
	runtime.KeepAlive(o)
	if v == -1 {
		return false, o.rt.currentError("Bool")
	}
	return v != 0, nil
}

// GoBytes converts a Python bytes or bytearray to a Go []byte.
func (o *Object) GoBytes() ([]byte, error) {
	if o == nil || o.ptr == 0 {
		return nil, ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	objType := o.rt.pyObjectType(o.ptr)
	if objType == 0 {
		return nil, o.rt.currentError("GoBytes: PyObject_Type")
	}
	defer o.rt.pyDecRef(objType)
	runtime.KeepAlive(o)

	switch objType {
	case o.rt.cachedBytesType:
		return o.rt.pyBytesToGoBytes(o.ptr), nil
	case o.rt.cachedBytearrayType:
		return o.rt.pyBytearrayToGoBytes(o.ptr), nil
	default:
		return nil, fmt.Errorf("pyffi: GoBytes: expected bytes or bytearray, got %T", o)
	}
}

// GoSlice converts a Python list to a Go []any.
func (o *Object) GoSlice() ([]any, error) {
	if o == nil || o.ptr == 0 {
		return nil, ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	result, err := o.rt.pyListToGoSlice(o.ptr)
	runtime.KeepAlive(o)
	return result, err
}

// GoMap converts a Python dict to a Go map[string]any.
func (o *Object) GoMap() (map[string]any, error) {
	if o == nil || o.ptr == 0 {
		return nil, ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	result, err := o.rt.pyDictToGoMap(o.ptr)
	runtime.KeepAlive(o)
	return result, err
}

// GoValue converts the Python object to the most appropriate Go type.
func (o *Object) GoValue() (any, error) {
	if o == nil || o.ptr == 0 {
		return nil, ErrNilObject
	}
	g := o.rt.autoGIL()
	defer g.release()
	result, err := o.rt.pythonToGo(o.ptr)
	runtime.KeepAlive(o)
	return result, err
}
