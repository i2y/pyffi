package pyffi

import "context"

// BoundModule wraps an imported Python module for convenient function calls.
type BoundModule struct {
	rt     *Runtime
	module *Object
	name   string
}

// Bind imports a Python module and wraps it for dynamic function calls.
func Bind(rt *Runtime, moduleName string) (*BoundModule, error) {
	mod, err := rt.Import(moduleName)
	if err != nil {
		return nil, err
	}
	return &BoundModule{rt: rt, module: mod, name: moduleName}, nil
}

// Call calls a named function on the module with positional arguments.
// If the last argument is of type KW, it is used as keyword arguments.
func (m *BoundModule) Call(name string, args ...any) (*Object, error) {
	fn := m.module.Attr(name)
	if fn == nil {
		return nil, &PythonError{
			Context: "Bind.Call",
			Type:    "AttributeError",
			Message: m.name + " has no attribute " + name,
		}
	}
	defer fn.Close()
	return fn.Call(args...)
}

// CallKw calls a named function with positional and keyword arguments.
func (m *BoundModule) CallKw(name string, args []any, kwargs map[string]any) (*Object, error) {
	fn := m.module.Attr(name)
	if fn == nil {
		return nil, &PythonError{
			Context: "Bind.CallKw",
			Type:    "AttributeError",
			Message: m.name + " has no attribute " + name,
		}
	}
	defer fn.Close()
	return fn.CallKw(args, kwargs)
}

// CallContext calls a named function on the module, respecting context
// cancellation and deadlines. See Object.CallContext for details.
func (m *BoundModule) CallContext(ctx context.Context, name string, args ...any) (*Object, error) {
	fn := m.module.Attr(name)
	if fn == nil {
		return nil, &PythonError{
			Context: "Bind.CallContext",
			Type:    "AttributeError",
			Message: m.name + " has no attribute " + name,
		}
	}
	defer fn.Close()
	return fn.CallContext(ctx, args...)
}

// CallKwContext calls a named function with positional and keyword arguments,
// respecting context cancellation and deadlines. See Object.CallKwContext for details.
func (m *BoundModule) CallKwContext(ctx context.Context, name string, args []any, kwargs map[string]any) (*Object, error) {
	fn := m.module.Attr(name)
	if fn == nil {
		return nil, &PythonError{
			Context: "Bind.CallKwContext",
			Type:    "AttributeError",
			Message: m.name + " has no attribute " + name,
		}
	}
	defer fn.Close()
	return fn.CallKwContext(ctx, args, kwargs)
}

// Has reports whether the module has an attribute with the given name.
func (m *BoundModule) Has(name string) bool {
	obj := m.module.Attr(name)
	if obj == nil {
		return false
	}
	obj.Close()
	return true
}

// Names returns the list of public attribute names on the module.
func (m *BoundModule) Names() ([]string, error) {
	builtins, err := m.rt.Import("builtins")
	if err != nil {
		return nil, err
	}
	defer builtins.Close()

	dirFn := builtins.Attr("dir")
	if dirFn == nil {
		return nil, &PythonError{Context: "Names", Message: "builtins.dir not found"}
	}
	defer dirFn.Close()

	result, err := dirFn.Call(m.module)
	if err != nil {
		return nil, err
	}
	defer result.Close()

	items, err := result.GoSlice()
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			names = append(names, s)
		}
	}
	return names, nil
}

// Attr returns a named attribute from the module.
func (m *BoundModule) Attr(name string) *Object {
	return m.module.Attr(name)
}

// Close releases the underlying module reference.
func (m *BoundModule) Close() error {
	return m.module.Close()
}
