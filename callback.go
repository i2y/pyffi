package pyffi

import (
	"fmt"
	"reflect"
	"unsafe"

	"github.com/ebitengine/purego"
)

// callbackEntry holds a registered Go function.
type callbackEntry struct {
	name string
	fn   reflect.Value
}

// RegisterFunc registers a Go function that can be called from Python.
// The function is accessible via `import go_bridge; go_bridge.name(args)`.
//
// Supported function signatures:
//   - func()
//   - func(args...) result
//   - func(args...) (result, error)
//   - func(args...) error
func (r *Runtime) RegisterFunc(name string, fn any) error {
	g := r.autoGIL()
	defer g.release()
	if r.closed.Load() {
		return ErrAlreadyClosed
	}

	fnVal := reflect.ValueOf(fn)
	if fnVal.Kind() != reflect.Func {
		return fmt.Errorf("pyffi: RegisterFunc expects a function, got %T", fn)
	}

	if err := r.ensureBridge(); err != nil {
		return err
	}

	id := int64(len(r.callbacks))
	r.callbacks = append(r.callbacks, callbackEntry{name: name, fn: fnVal})

	// Create a Python lambda wrapper that calls the dispatcher.
	code := fmt.Sprintf(
		`import go_bridge; go_bridge.%s = lambda *args, **kwargs: go_bridge._dispatch(%d, args, kwargs)`,
		name, id)
	return r.Exec(code)
}

// ensureBridge creates the go_bridge module and the _dispatch PyCFunction once.
func (r *Runtime) ensureBridge() error {
	if r.bridgeReady {
		return nil
	}

	// Create the go_bridge module in sys.modules.
	bridgeMod := r.pyImportAddModule("go_bridge") // borrowed ref
	if bridgeMod == 0 {
		return r.currentError("ensureBridge: PyImport_AddModule")
	}

	// Create the C dispatch callback.
	dispatchPtr := purego.NewCallback(func(self, args, kwargs uintptr) uintptr {
		return r.dispatchCallback(self, args, kwargs)
	})

	// Build a PyMethodDef struct in Go memory.
	nameBytes := []byte("_dispatch\x00")
	methodDef := make([]byte, 32)
	*(*uintptr)(unsafe.Pointer(&methodDef[0])) = uintptr(unsafe.Pointer(&nameBytes[0]))
	*(*uintptr)(unsafe.Pointer(&methodDef[8])) = dispatchPtr
	*(*int32)(unsafe.Pointer(&methodDef[16])) = 0x0003 // METH_VARARGS | METH_KEYWORDS
	*(*uintptr)(unsafe.Pointer(&methodDef[24])) = 0    // no doc

	// Keep allocations alive for the lifetime of the Runtime.
	r.bridgeKeep = append(r.bridgeKeep, nameBytes, methodDef)

	// Create PyCFunction from the PyMethodDef.
	methodDefPtr := uintptr(unsafe.Pointer(&methodDef[0]))
	pyCFunc := r.pyCFunctionNew(methodDefPtr, 0)
	if pyCFunc == 0 {
		return r.currentError("ensureBridge: PyCFunction_New")
	}

	// Set _dispatch on the module.
	if r.pyObjectSetAttrString(bridgeMod, "_dispatch", pyCFunc) != 0 {
		r.pyDecRef(pyCFunc)
		return r.currentError("ensureBridge: set _dispatch")
	}
	r.pyDecRef(pyCFunc)

	r.bridgeReady = true
	return nil
}

// dispatchCallback is the C-level callback that dispatches to registered Go functions.
// Signature matches PyCFunctionWithKeywords: (self, args, kwargs) -> result
// The Python lambda calls: _dispatch(func_id, args_tuple, kwargs_dict)
func (r *Runtime) dispatchCallback(self, args, kwargs uintptr) uintptr {
	// args is a tuple: (func_id, actual_args, actual_kwargs)
	nArgs := r.pyTupleSize(args)
	if nArgs < 2 {
		r.pyIncRef(r.pyNone)
		return r.pyNone
	}

	// Get func_id.
	idObj := r.pyTupleGetItem(args, 0) // borrowed ref
	funcID := r.pyLongAsLongLong(idObj)

	if funcID < 0 || int(funcID) >= len(r.callbacks) {
		r.pyIncRef(r.pyNone)
		return r.pyNone
	}

	entry := r.callbacks[funcID]

	// Get actual args tuple.
	actualArgs := r.pyTupleGetItem(args, 1) // borrowed ref
	actualArgCount := r.pyTupleSize(actualArgs)

	// Get actual kwargs dict (may be None or a dict).
	var actualKwargs uintptr
	if nArgs >= 3 {
		kw := r.pyTupleGetItem(args, 2) // borrowed ref
		if kw != 0 && kw != r.pyNone {
			actualKwargs = kw
		}
	}

	fnType := entry.fn.Type()
	numIn := fnType.NumIn()

	// Check if last parameter accepts kwargs (map[string]any).
	// Only treat it as kwargs when the caller actually passed non-empty
	// keyword arguments. This distinguishes f(a=1) (kwargs) from
	// f({"a": 1}) (positional dict).
	hasNonEmptyKwargs := actualKwargs != 0 && r.pyDictSize(actualKwargs) > 0
	lastParamIsKwargs := numIn > 0 &&
		fnType.In(numIn-1) == reflect.TypeFor[map[string]any]() &&
		hasNonEmptyKwargs
	positionalParams := numIn
	if lastParamIsKwargs {
		positionalParams = numIn - 1
	}

	// Build Go arguments using reflection.
	goArgs := make([]reflect.Value, numIn)
	for i := 0; i < positionalParams; i++ {
		if int64(i) < actualArgCount {
			pyArg := r.pyTupleGetItem(actualArgs, int64(i)) // borrowed ref
			goVal, err := r.pythonToGo(pyArg)
			if err != nil {
				r.pyIncRef(r.pyNone)
				return r.pyNone
			}
			goArgs[i] = convertToType(goVal, fnType.In(i))
		} else {
			goArgs[i] = reflect.Zero(fnType.In(i))
		}
	}

	// Pass kwargs to last parameter if it accepts map[string]any.
	if lastParamIsKwargs {
		if actualKwargs != 0 {
			kwargsMap, err := r.pyDictToGoMap(actualKwargs)
			if err != nil {
				r.pyIncRef(r.pyNone)
				return r.pyNone
			}
			goArgs[numIn-1] = reflect.ValueOf(kwargsMap)
		} else {
			goArgs[numIn-1] = reflect.ValueOf(map[string]any{})
		}
	}

	// Call the Go function.
	results := entry.fn.Call(goArgs)

	// Handle return values.
	numOut := fnType.NumOut()
	if numOut == 0 {
		r.pyIncRef(r.pyNone)
		return r.pyNone
	}

	// Check if the last return value is an error.
	lastType := fnType.Out(numOut - 1)
	if lastType.Implements(reflect.TypeFor[error]()) {
		errVal := results[numOut-1]
		if !errVal.IsNil() {
			// Set Python exception from Go error.
			errMsg := errVal.Interface().(error).Error()
			if r.pyExcRuntimeError != 0 {
				r.pyErrSetString(r.pyExcRuntimeError, errMsg)
			}
			return 0 // signal exception
		}
		results = results[:numOut-1]
	}

	if len(results) == 0 {
		r.pyIncRef(r.pyNone)
		return r.pyNone
	}

	// Convert the first return value to Python.
	ret, err := r.goToPython(results[0].Interface())
	if err != nil {
		r.pyIncRef(r.pyNone)
		return r.pyNone
	}
	return ret
}

// convertToType converts an any value to a reflect.Value of the target type.
func convertToType(val any, target reflect.Type) reflect.Value {
	if val == nil {
		return reflect.Zero(target)
	}
	v := reflect.ValueOf(val)
	if v.Type().ConvertibleTo(target) {
		return v.Convert(target)
	}
	// Special case: int64 → int
	if v.Kind() == reflect.Int64 && target.Kind() == reflect.Int {
		return reflect.ValueOf(int(v.Int()))
	}
	// Special case: float64 → float32
	if v.Kind() == reflect.Float64 && target.Kind() == reflect.Float32 {
		return reflect.ValueOf(float32(v.Float()))
	}
	return v
}
