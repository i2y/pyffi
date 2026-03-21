package pyffi

import (
	"fmt"
	"reflect"
	"runtime"
	"unsafe"
)

// PyObjecter is implemented by types that wrap a Python object.
// pyffi-gen generated types satisfy this interface automatically.
// When passed to Python, the underlying object is used directly.
type PyObjecter interface {
	Object() *Object
}

// goToPython converts a Go value to a new-reference PyObject*.
// The caller owns the returned reference.
func (r *Runtime) goToPython(v any) (uintptr, error) {
	switch val := v.(type) {
	case *Object:
		if val == nil || val.ptr == 0 {
			r.pyIncRef(r.pyNone)
			return r.pyNone, nil
		}
		r.pyIncRef(val.ptr)
		runtime.KeepAlive(val)
		return val.ptr, nil
	case PyObjecter:
		rv := reflect.ValueOf(val)
		if rv.Kind() == reflect.Ptr && rv.IsNil() {
			r.pyIncRef(r.pyNone)
			return r.pyNone, nil
		}
		obj := val.Object()
		if obj == nil || obj.ptr == 0 {
			r.pyIncRef(r.pyNone)
			return r.pyNone, nil
		}
		r.pyIncRef(obj.ptr)
		runtime.KeepAlive(obj)
		return obj.ptr, nil
	case bool:
		// bool must come before int types because Go's type switch is exact.
		var n int64
		if val {
			n = 1
		}
		ptr := r.pyBoolFromLong(n)
		if ptr == 0 {
			return 0, r.currentError("goToPython(bool)")
		}
		return ptr, nil
	case int:
		return r.pyLongFromLongLong(int64(val)), nil
	case int8:
		return r.pyLongFromLongLong(int64(val)), nil
	case int16:
		return r.pyLongFromLongLong(int64(val)), nil
	case int32:
		return r.pyLongFromLongLong(int64(val)), nil
	case int64:
		return r.pyLongFromLongLong(val), nil
	case uint:
		return r.pyLongFromUnsignedLongLong(uint64(val)), nil
	case uint8:
		return r.pyLongFromLongLong(int64(val)), nil
	case uint16:
		return r.pyLongFromLongLong(int64(val)), nil
	case uint32:
		return r.pyLongFromLongLong(int64(val)), nil
	case uint64:
		return r.pyLongFromUnsignedLongLong(val), nil
	case float32:
		return r.pyFloatFromDouble(float64(val)), nil
	case float64:
		return r.pyFloatFromDouble(val), nil
	case string:
		ptr := r.pyUnicodeFromString(val)
		if ptr == 0 {
			return 0, r.currentError("goToPython(string)")
		}
		return ptr, nil
	case []byte:
		var dataPtr uintptr
		if len(val) > 0 {
			dataPtr = uintptr(unsafe.Pointer(&val[0]))
		}
		ptr := r.pyBytesFromStringAndSize(dataPtr, int64(len(val)))
		if ptr == 0 {
			return 0, r.currentError("goToPython([]byte)")
		}
		return ptr, nil
	case []any:
		return r.goSliceToPyList(val)
	case map[string]any:
		return r.goMapToPyDict(val)
	case nil:
		r.pyIncRef(r.pyNone)
		return r.pyNone, nil
	default:
		return 0, fmt.Errorf("pyffi: unsupported Go type %T", v)
	}
}

func (r *Runtime) goSliceToPyList(items []any) (uintptr, error) {
	list := r.pyListNew(int64(len(items)))
	if list == 0 {
		return 0, r.currentError("goSliceToPyList")
	}
	for i, item := range items {
		pyItem, err := r.goToPython(item)
		if err != nil {
			r.pyDecRef(list)
			return 0, err
		}
		// PyList_SetItem steals the reference.
		if r.pyListSetItem(list, int64(i), pyItem) != 0 {
			r.pyDecRef(list)
			return 0, r.currentError("PyList_SetItem")
		}
	}
	return list, nil
}

func (r *Runtime) goMapToPyDict(m map[string]any) (uintptr, error) {
	dict := r.pyDictNew()
	if dict == 0 {
		return 0, r.currentError("goMapToPyDict")
	}
	for k, v := range m {
		pyVal, err := r.goToPython(v)
		if err != nil {
			r.pyDecRef(dict)
			return 0, err
		}
		// PyDict_SetItemString does NOT steal the reference.
		if r.pyDictSetItemString(dict, k, pyVal) != 0 {
			r.pyDecRef(pyVal)
			r.pyDecRef(dict)
			return 0, r.currentError("PyDict_SetItemString")
		}
		r.pyDecRef(pyVal)
	}
	return dict, nil
}

// pythonToGo converts a PyObject* (borrowed reference) to a Go value.
// It does NOT consume the reference.
func (r *Runtime) pythonToGo(ptr uintptr) (any, error) {
	if ptr == 0 || ptr == r.pyNone {
		return nil, nil
	}

	objType := r.pyObjectType(ptr)
	if objType == 0 {
		return nil, r.currentError("pythonToGo: PyObject_Type")
	}
	defer r.pyDecRef(objType)

	// Check bool before int because Python bool is a subclass of int.
	switch objType {
	case r.cachedBoolType:
		v := r.pyObjectIsTrue(ptr)
		if v == -1 {
			return nil, r.currentError("pythonToGo(bool)")
		}
		return v != 0, nil
	case r.cachedIntType:
		v := r.pyLongAsLongLong(ptr)
		if v == -1 && r.pyErrOccurred() != 0 {
			return nil, r.currentError("pythonToGo(int)")
		}
		return v, nil
	case r.cachedFloatType:
		v := r.pyFloatAsDouble(ptr)
		if v == -1.0 && r.pyErrOccurred() != 0 {
			return nil, r.currentError("pythonToGo(float)")
		}
		return v, nil
	case r.cachedStrType:
		return r.pyUnicodeAsUTF8(ptr), nil
	case r.cachedBytesType:
		return r.pyBytesToGoBytes(ptr), nil
	case r.cachedBytearrayType:
		return r.pyBytearrayToGoBytes(ptr), nil
	case r.cachedListType:
		return r.pyListToGoSlice(ptr)
	case r.cachedTupleType:
		return r.pyTupleToGoSlice(ptr)
	case r.cachedDictType:
		return r.pyDictToGoMap(ptr)
	case r.cachedSetType, r.cachedFrozenSetType:
		return r.pyIterableToGoSlice(ptr)
	default:
		// Return as *Object with a new reference.
		r.pyIncRef(ptr)
		return r.newObject(ptr), nil
	}
}

func (r *Runtime) pyListToGoSlice(ptr uintptr) ([]any, error) {
	n := r.pyListSize(ptr)
	result := make([]any, n)
	for i := int64(0); i < n; i++ {
		item := r.pyListGetItem(ptr, i) // borrowed reference
		v, err := r.pythonToGo(item)
		if err != nil {
			return nil, fmt.Errorf("pyffi: list[%d]: %w", i, err)
		}
		result[i] = v
	}
	return result, nil
}

func (r *Runtime) pyDictToGoMap(ptr uintptr) (map[string]any, error) {
	keys := r.pyDictKeys(ptr) // new reference
	if keys == 0 {
		return nil, r.currentError("pyDictToGoMap: PyDict_Keys")
	}
	defer r.pyDecRef(keys)

	n := r.pyListSize(keys)
	result := make(map[string]any, n)
	for i := int64(0); i < n; i++ {
		keyObj := r.pyListGetItem(keys, i) // borrowed reference
		keyStr := r.pyUnicodeAsUTF8(keyObj)
		valObj := r.pyDictGetItemString(ptr, keyStr) // borrowed reference
		v, err := r.pythonToGo(valObj)
		if err != nil {
			return nil, fmt.Errorf("pyffi: dict[%q]: %w", keyStr, err)
		}
		result[keyStr] = v
	}
	return result, nil
}

func (r *Runtime) pyTupleToGoSlice(ptr uintptr) ([]any, error) {
	n := r.pyTupleSize(ptr)
	result := make([]any, n)
	for i := int64(0); i < n; i++ {
		item := r.pyTupleGetItem(ptr, i) // borrowed reference
		v, err := r.pythonToGo(item)
		if err != nil {
			return nil, fmt.Errorf("pyffi: tuple[%d]: %w", i, err)
		}
		result[i] = v
	}
	return result, nil
}

func (r *Runtime) pyIterableToGoSlice(ptr uintptr) ([]any, error) {
	iter := r.pyObjectGetIter(ptr)
	if iter == 0 {
		return nil, r.currentError("pyIterableToGoSlice: PyObject_GetIter")
	}
	defer r.pyDecRef(iter)

	var result []any
	for {
		item := r.pyIterNext(iter) // new reference, 0 on end or error
		if item == 0 {
			if r.pyErrOccurred() != 0 {
				return nil, r.currentError("pyIterableToGoSlice: PyIter_Next")
			}
			break // StopIteration
		}
		v, err := r.pythonToGo(item)
		r.pyDecRef(item)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	if result == nil {
		result = []any{}
	}
	return result, nil
}

func (r *Runtime) pyBytesToGoBytes(ptr uintptr) []byte {
	size := r.pyBytesSize(ptr)
	if size <= 0 {
		return []byte{}
	}
	dataPtr := r.pyBytesAsString(ptr)
	// dataPtr is a C pointer returned by PyBytes_AsString; the uintptr→Pointer
	// conversion is safe because the source PyObject is alive (borrowed ref).
	src := unsafe.Slice((*byte)(unsafe.Pointer(dataPtr)), size) //nolint:govet
	dst := make([]byte, size)
	copy(dst, src)
	return dst
}

func (r *Runtime) pyBytearrayToGoBytes(ptr uintptr) []byte {
	size := r.pyBytearraySize(ptr)
	if size <= 0 {
		return []byte{}
	}
	dataPtr := r.pyBytearrayAsString(ptr)
	// Same as above: C pointer from PyByteArray_AsString.
	src := unsafe.Slice((*byte)(unsafe.Pointer(dataPtr)), size) //nolint:govet
	dst := make([]byte, size)
	copy(dst, src)
	return dst
}
