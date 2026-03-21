package gen

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/i2y/pyffi"
)

// Generator orchestrates code generation.
type Generator struct {
	Config *Config
	Mapper *Mapper
}

// NewGenerator creates a generator from config.
func NewGenerator(cfg *Config) *Generator {
	return &Generator{
		Config: cfg,
		Mapper: NewMapper(cfg.TypeOverrides),
	}
}

// Generate runs the full pipeline: introspect → map → template → write.
func (g *Generator) Generate() error {
	info, err := g.introspect()
	if err != nil {
		return err
	}

	code, err := g.render(info, "templates/module.go.tmpl")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(g.Config.Out, 0755); err != nil {
		return fmt.Errorf("pyffi-gen: mkdir: %w", err)
	}

	outFile := filepath.Join(g.Config.Out, g.Config.Package+".go")
	if err := os.WriteFile(outFile, code, 0644); err != nil {
		return fmt.Errorf("pyffi-gen: write: %w", err)
	}
	fmt.Printf("wrote %s\n", outFile)

	// Generate test file if requested.
	if g.Config.Test {
		testCode, err := g.render(info, "templates/test.go.tmpl")
		if err != nil {
			return err
		}
		testFile := filepath.Join(g.Config.Out, g.Config.Package+"_test.go")
		if err := os.WriteFile(testFile, testCode, 0644); err != nil {
			return fmt.Errorf("pyffi-gen: write test: %w", err)
		}
		fmt.Printf("wrote %s\n", testFile)
	}

	return nil
}

// DryRun prints what would be generated to stdout.
func (g *Generator) DryRun() error {
	info, err := g.introspect()
	if err != nil {
		return err
	}

	code, err := g.render(info, "templates/module.go.tmpl")
	if err != nil {
		return err
	}

	fmt.Print(string(code))
	return nil
}

func (g *Generator) introspect() (*ModuleInfo, error) {
	var opts []pyffi.Option
	if len(g.Config.Dependencies) > 0 {
		opts = append(opts, pyffi.Dependencies(g.Config.Dependencies...))
	}

	intro, err := NewIntrospector(opts...)
	if err != nil {
		return nil, err
	}
	// Do NOT call intro.Close() here.
	// Py_Finalize followed by Py_Initialize in the same process is unreliable
	// (CPython limitation). Since pyffi-gen is a short-lived CLI tool,
	// we let the OS reclaim resources at process exit.

	info, err := intro.Introspect(g.Config.Module, g.Config.Include, g.Config.Exclude, g.Mapper)
	if err != nil {
		return nil, err
	}

	info.GoPackage = g.Config.Package

	// Build the set of class names for type-safe return/param resolution.
	info.ClassNames = make(map[string]bool)
	for _, c := range info.Classes {
		info.ClassNames[c.GoName] = true
	}

	return info, nil
}

func (g *Generator) render(info *ModuleInfo, mainTemplate string) ([]byte, error) {
	// Closures that capture the class name set for type-safe resolution.
	classNames := info.ClassNames

	funcMap := template.FuncMap{
		"zeroVal":             zeroVal,
		"returnConvert":       func(t TypeInfo) string { return returnConvert(t, classNames, "m") },
		"returnConvertMethod": func(t TypeInfo) string { return returnConvert(t, classNames, "w") },
		"requiredParams": func(params []ParamInfo) string {
			return requiredParams(params, classNames)
		},
		"callExpr":       callExpr,
		"safeReturnType": func(t TypeInfo) string { return resolveGoType(t.Go, classNames) },
		"safeParamType":  func(t TypeInfo) string { return resolveGoType(t.Go, classNames) },
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.go.tmpl")
	if err != nil {
		return nil, fmt.Errorf("pyffi-gen: parse templates: %w", err)
	}

	var buf bytes.Buffer
	tmplName := filepath.Base(mainTemplate)
	if err := tmpl.ExecuteTemplate(&buf, tmplName, info); err != nil {
		return nil, fmt.Errorf("pyffi-gen: execute template: %w", err)
	}

	// Format with gofmt.
	formatted, err := format.Source(buf.Bytes())
	if err != nil {
		// Return unformatted on failure so user can debug.
		fmt.Fprintf(os.Stderr, "warning: gofmt failed: %v\n", err)
		return buf.Bytes(), nil
	}
	return formatted, nil
}

// zeroVal returns the zero value expression for a Go type.
func zeroVal(goType string) string {
	if goType == "" {
		return ""
	}
	switch goType {
	case "int64", "int", "float64", "float32":
		return "0, "
	case "string":
		return "\"\", "
	case "bool":
		return "false, "
	case "[]byte":
		return "nil, "
	case "any":
		return "nil, "
	}
	if strings.HasPrefix(goType, "*") || strings.HasPrefix(goType, "[]") || strings.HasPrefix(goType, "map[") {
		return "nil, "
	}
	return "nil, "
}

// requiredParams generates the function parameter list.
func requiredParams(params []ParamInfo, classNames map[string]bool) string {
	var required []string
	hasOptional := false
	for _, p := range params {
		if p.HasDefault || p.IsKwOnly {
			hasOptional = true
			continue
		}
		required = append(required, p.GoName+" "+resolveGoType(p.Type.Go, classNames))
	}
	if hasOptional {
		required = append(required, "opts ...pyffi.KW")
	}
	return strings.Join(required, ", ")
}

// callExpr generates the Call/CallKw expression for a function.
func callExpr(params []ParamInfo) string {
	var required []string
	var optional []ParamInfo
	for _, p := range params {
		if p.HasDefault || p.IsKwOnly {
			optional = append(optional, p)
			continue
		}
		required = append(required, p.GoName)
	}

	if len(optional) == 0 {
		// Simple case: all required.
		args := strings.Join(required, ", ")
		return "\n\tresult, err := fn.Call(" + args + ")"
	}

	// Has optional: use CallKw when opts provided.
	args := strings.Join(required, ", ")
	var b strings.Builder
	b.WriteString("\n\targs := []any{")
	b.WriteString(args)
	b.WriteString("}")
	b.WriteString("\n\tvar kw pyffi.KW")
	b.WriteString("\n\tif len(opts) > 0 {")
	b.WriteString("\n\t\tkw = opts[0]")
	b.WriteString("\n\t}")
	b.WriteString("\n\tresult, err := fn.CallKw(args, kw)")
	return b.String()
}

// resolveGoType returns a Go type string that is valid in generated code.
// If the type is a pointer to a class generated in the same package, it is kept as-is.
// Unknown pointer types are downgraded to "any".
func resolveGoType(goType string, classNames map[string]bool) string {
	if goType == "" {
		return "" // void (error-only)
	}
	switch goType {
	case "int64", "float64", "string", "bool", "any", "[]byte", "error":
		return goType
	}
	// Pointer to a class generated in this package → keep type-safe.
	if strings.HasPrefix(goType, "*") {
		className := goType[1:]
		if classNames[className] {
			return goType // e.g. *McpSdkServerConfig — generated in same package
		}
		return "any" // unknown external class
	}
	return "any"
}

// returnConvert generates the return conversion + cleanup code.
// For value types (int64, string, etc.), it defers Close and extracts.
// For struct-wrapped types, ownership transfers to the struct (no Close).
func returnConvert(t TypeInfo, classNames map[string]bool, receiver string) string {
	goType := resolveGoType(t.Go, classNames)
	if goType == "" {
		return "\n\tresult.Close()\n\treturn nil"
	}

	// Pointer to generated class → wrap, transfer ownership (don't Close).
	if strings.HasPrefix(goType, "*") {
		className := goType[1:]
		if classNames[className] {
			return fmt.Sprintf("\n\treturn &%s{obj: result, rt: %s.rt}, nil", className, receiver)
		}
	}

	// For all other types, Close the result after extracting the Go value.
	const closeAndReturn = "\n\tdefer result.Close()"
	switch goType {
	case "int64":
		return closeAndReturn + "\n\treturn result.Int64()"
	case "float64":
		return closeAndReturn + "\n\treturn result.Float64()"
	case "string":
		return closeAndReturn + "\n\treturn result.GoString()"
	case "bool":
		return closeAndReturn + "\n\treturn result.Bool()"
	default:
		return closeAndReturn + "\n\treturn result.GoValue()"
	}
}
