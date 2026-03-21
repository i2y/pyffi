package gen

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/i2y/pyffi"
)

// Introspector runs Python to extract module metadata.
type Introspector struct {
	rt      *pyffi.Runtime
	introFn *pyffi.Object // _pyffi_introspect_module
}

const helperScript = `
import inspect, typing, enum

def _pyffi_introspect_func(obj):
    try:
        sig = inspect.signature(obj)
    except (ValueError, TypeError):
        return None
    params = []
    for name, p in sig.parameters.items():
        if name == 'self':
            continue
        kind = int(p.kind)
        if kind in (2, 4):  # VAR_POSITIONAL, VAR_KEYWORD
            continue
        info = {'name': name, 'kind': kind,
                'has_default': p.default is not inspect.Parameter.empty}
        if p.annotation is not inspect.Parameter.empty:
            info['annotation'] = str(p.annotation)
        params.append(info)
    ret = None
    if sig.return_annotation is not inspect.Signature.empty:
        ret = str(sig.return_annotation)
    hints = {}
    try:
        hints = {k: str(v) for k, v in typing.get_type_hints(obj).items()}
    except Exception:
        pass
    return {'params': params, 'return': ret, 'hints': hints, 'doc': inspect.getdoc(obj) or ''}

def _pyffi_introspect_module(module_name):
    """Introspect a module and return a plain dict with all info."""
    import importlib
    mod = importlib.import_module(module_name)
    result = {'doc': inspect.getdoc(mod) or '', 'functions': [], 'classes': [], 'enums': []}

    for name, obj in inspect.getmembers(mod):
        if name.startswith('_') and not name.startswith('__'):
            continue
        if name.startswith('__'):
            continue

        try:
            if isinstance(obj, type):
                if issubclass(obj, enum.Enum):
                    members = []
                    for m in obj:
                        members.append({'name': m.name, 'value': str(m.value)})
                    result['enums'].append({'name': name, 'members': members, 'doc': inspect.getdoc(obj) or ''})
                else:
                    cls_info = {'name': name, 'doc': inspect.getdoc(obj) or '', 'init': None, 'methods': []}
                    # __init__
                    try:
                        init_info = _pyffi_introspect_func(obj.__init__)
                        if init_info:
                            cls_info['init'] = init_info
                    except Exception:
                        pass
                    # methods (deduplicate by name)
                    seen_methods = set()
                    for mname, mobj in inspect.getmembers(obj, predicate=inspect.isfunction):
                        if mname.startswith('_') and mname != '__init__':
                            continue
                        if mname == '__init__':
                            continue
                        if mname in seen_methods:
                            continue
                        seen_methods.add(mname)
                        minfo = _pyffi_introspect_func(mobj)
                        if minfo:
                            minfo['name'] = mname
                            cls_info['methods'].append(minfo)
                    result['classes'].append(cls_info)
            elif callable(obj):
                finfo = _pyffi_introspect_func(obj)
                if finfo:
                    finfo['name'] = name
                    result['functions'].append(finfo)
        except Exception:
            continue

    return result
`

// NewIntrospector creates a Python runtime and loads helper functions.
func NewIntrospector(opts ...pyffi.Option) (*Introspector, error) {
	rt, err := pyffi.New(opts...)
	if err != nil {
		return nil, err
	}

	if err := rt.Exec(helperScript); err != nil {
		rt.Close()
		return nil, fmt.Errorf("pyffi-gen: failed to load introspection helpers: %w", err)
	}

	main, err := rt.Import("__main__")
	if err != nil {
		rt.Close()
		return nil, err
	}
	defer main.Close()

	introFn := main.Attr("_pyffi_introspect_module")
	if introFn == nil {
		rt.Close()
		return nil, fmt.Errorf("pyffi-gen: helper function not found")
	}

	return &Introspector{rt: rt, introFn: introFn}, nil
}

// Close releases the Python runtime.
func (i *Introspector) Close() error {
	i.introFn.Close()
	return i.rt.Close()
}

// Introspect extracts metadata from the named Python module.
func (i *Introspector) Introspect(moduleName string, include, exclude []string, mapper *Mapper) (*ModuleInfo, error) {
	// Serialize the result to JSON in Python to avoid SIGSEGV from
	// deep recursive GoMap conversion on large modules.
	if err := i.rt.Exec(`import json as _json`); err != nil {
		return nil, err
	}
	resultObj, err := i.introFn.Call(moduleName)
	if err != nil {
		return nil, err
	}
	defer resultObj.Close()

	// Convert to JSON string in Python, then parse in Go.
	jsonMod, _ := i.rt.Import("json")
	defer jsonMod.Close()
	dumpsFn := jsonMod.Attr("dumps")
	defer dumpsFn.Close()
	jsonObj, err := dumpsFn.Call(resultObj)
	if err != nil {
		return nil, fmt.Errorf("pyffi-gen: JSON serialization failed: %w", err)
	}
	defer jsonObj.Close()
	jsonStr, err := jsonObj.GoString()
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("pyffi-gen: JSON parse failed: %w", err)
	}

	info := &ModuleInfo{PythonModule: moduleName}
	if doc, ok := result["doc"].(string); ok {
		info.Doc = firstLine(doc)
	}

	// Functions.
	if funcs, ok := result["functions"].([]any); ok {
		for _, f := range funcs {
			fm, ok := f.(map[string]any)
			if !ok {
				continue
			}
			name, _ := fm["name"].(string)
			if !matchFilter(name, include, exclude) {
				continue
			}
			if fi := parseFuncInfo(name, fm, mapper); fi != nil {
				info.Functions = append(info.Functions, *fi)
			}
		}
	}

	// Classes.
	if classes, ok := result["classes"].([]any); ok {
		for _, c := range classes {
			cm, ok := c.(map[string]any)
			if !ok {
				continue
			}
			name, _ := cm["name"].(string)
			if !matchFilter(name, include, exclude) {
				continue
			}
			ci := parseClassInfo(name, cm, mapper)
			if ci != nil {
				info.Classes = append(info.Classes, *ci)
			}
		}
	}

	// Enums.
	if enums, ok := result["enums"].([]any); ok {
		for _, e := range enums {
			em, ok := e.(map[string]any)
			if !ok {
				continue
			}
			name, _ := em["name"].(string)
			if !matchFilter(name, include, exclude) {
				continue
			}
			ei := parseEnumInfo(name, em)
			if ei != nil {
				info.Enums = append(info.Enums, *ei)
			}
		}
	}

	return info, nil
}

func parseFuncInfo(name string, m map[string]any, mapper *Mapper) *FuncInfo {
	fi := &FuncInfo{
		PythonName: name,
		GoName:     GoFuncName(name),
	}

	if doc, ok := m["doc"].(string); ok {
		fi.Doc = firstLine(doc)
	}

	hints, _ := m["hints"].(map[string]any)
	if retStr, ok := m["return"].(string); ok && retStr != "" {
		fi.Returns = mapper.Map(retStr)
	} else if retHint, ok := hints["return"].(string); ok {
		fi.Returns = mapper.Map(retHint)
	} else {
		fi.Returns = TypeInfo{Go: "any", IsAny: true}
	}

	if params, ok := m["params"].([]any); ok {
		for _, p := range params {
			pm, ok := p.(map[string]any)
			if !ok {
				continue
			}
			pname, _ := pm["name"].(string)
			if pname == "" {
				continue
			}

			pi := ParamInfo{
				PythonName: pname,
				GoName:     GoParamName(pname),
			}

			if hint, ok := hints[pname].(string); ok {
				pi.Type = mapper.Map(hint)
			} else if ann, ok := pm["annotation"].(string); ok {
				pi.Type = mapper.Map(ann)
			} else {
				pi.Type = TypeInfo{Go: "any", IsAny: true}
			}

			if hd, ok := pm["has_default"].(bool); ok {
				pi.HasDefault = hd
			}
			if kind, ok := pm["kind"].(float64); ok && int(kind) == 3 {
				pi.IsKwOnly = true
			}

			// Ensure no empty Go type.
			if pi.Type.Go == "" {
				pi.Type.Go = "any"
				pi.Type.IsAny = true
			}
			fi.Params = append(fi.Params, pi)
		}
	}

	return fi
}

func parseClassInfo(name string, m map[string]any, mapper *Mapper) *ClassInfo {
	ci := &ClassInfo{
		PythonName: name,
		GoName:     GoFuncName(name),
	}

	if doc, ok := m["doc"].(string); ok {
		ci.Doc = firstLine(doc)
	}

	if initMap, ok := m["init"].(map[string]any); ok {
		ci.Init = parseFuncInfo("__init__", initMap, mapper)
		if ci.Init != nil {
			ci.Init.GoName = "New" + ci.GoName
		}
	}

	if methods, ok := m["methods"].([]any); ok {
		for _, meth := range methods {
			mm, ok := meth.(map[string]any)
			if !ok {
				continue
			}
			mname, _ := mm["name"].(string)
			if fi := parseFuncInfo(mname, mm, mapper); fi != nil {
				// Skip methods that collide with generated Close/Object methods.
				if fi.GoName == "Close" || fi.GoName == "Object" {
					continue
				}
				ci.Methods = append(ci.Methods, *fi)
			}
		}
	}

	return ci
}

func parseEnumInfo(name string, m map[string]any) *EnumInfo {
	ei := &EnumInfo{
		PythonName: name,
		GoName:     GoFuncName(name),
	}

	if members, ok := m["members"].([]any); ok {
		for _, mem := range members {
			mm, ok := mem.(map[string]any)
			if !ok {
				continue
			}
			mname, _ := mm["name"].(string)
			mval, _ := mm["value"].(string)

			ei.Members = append(ei.Members, EnumMember{
				PythonName: mname,
				GoName:     ei.GoName + GoFuncName(strings.ToLower(mname)),
				Value:      mval,
			})
		}
	}

	return ei
}

func matchFilter(name string, include, exclude []string) bool {
	if name == "" {
		return false
	}
	if len(include) > 0 {
		matched := false
		for _, pat := range include {
			if ok, _ := path.Match(pat, name); ok {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	for _, pat := range exclude {
		if ok, _ := path.Match(pat, name); ok {
			return false
		}
	}
	return true
}

func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}
