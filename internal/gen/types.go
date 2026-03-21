// Package gen implements Python module introspection and Go code generation.
package gen

// ModuleInfo holds the complete introspection result for a Python module.
type ModuleInfo struct {
	PythonModule string
	GoPackage    string
	Functions    []FuncInfo
	Classes      []ClassInfo
	Enums        []EnumInfo
	Doc          string
	ClassNames   map[string]bool // set of Go class names generated in this package
}

// FuncInfo describes a Python function or method.
type FuncInfo struct {
	PythonName string
	GoName     string // PascalCase
	Params     []ParamInfo
	Returns    TypeInfo
	Doc        string
}

// ParamInfo describes a function parameter.
type ParamInfo struct {
	PythonName string
	GoName     string // camelCase
	Type       TypeInfo
	HasDefault bool
	IsKwOnly   bool
}

// TypeInfo describes a mapped type.
type TypeInfo struct {
	Python string // original annotation, e.g. "list[int]"
	Go     string // mapped Go type, e.g. "[]int64"
	IsPtr  bool   // Optional → pointer
	IsAny  bool   // unresolvable → any
}

// ClassInfo describes a Python class.
type ClassInfo struct {
	PythonName string
	GoName     string
	Init       *FuncInfo // __init__ → NewXxx; nil if absent
	Methods    []FuncInfo
	Properties []PropInfo
	Doc        string
}

// PropInfo describes a Python property.
type PropInfo struct {
	PythonName string
	GoName     string
	Type       TypeInfo
}

// EnumInfo describes a Python Enum class.
type EnumInfo struct {
	PythonName string
	GoName     string
	Members    []EnumMember
}

// EnumMember is a single enum value.
type EnumMember struct {
	PythonName string
	GoName     string
	Value      string // Go literal
}
