package gen

import (
	"strings"
	"unicode"
)

// Mapper converts Python type annotations to Go types.
type Mapper struct {
	Overrides map[string]string
}

// NewMapper creates a mapper with optional type overrides.
func NewMapper(overrides map[string]string) *Mapper {
	if overrides == nil {
		overrides = make(map[string]string)
	}
	return &Mapper{Overrides: overrides}
}

var baseTypes = map[string]string{
	"int":      "int64",
	"float":    "float64",
	"str":      "string",
	"bool":     "bool",
	"bytes":    "[]byte",
	"NoneType": "",
	"None":     "",
}

// Map converts a Python annotation string to a TypeInfo.
func (m *Mapper) Map(pyAnnotation string) TypeInfo {
	if pyAnnotation == "" {
		return TypeInfo{Python: "", Go: "any", IsAny: true}
	}

	// Check overrides first.
	if goType, ok := m.Overrides[pyAnnotation]; ok {
		return TypeInfo{Python: pyAnnotation, Go: goType}
	}

	t := m.mapTypeDepth(pyAnnotation, 0)
	// Final safety: Go type must be a valid identifier (no dots, brackets, etc.)
	if strings.ContainsAny(t.Go, ".'\"") {
		t.Go = "any"
		t.IsAny = true
	}
	return t
}

const maxMapDepth = 10

func (m *Mapper) mapType(s string) TypeInfo {
	return m.mapTypeDepth(s, 0)
}

func (m *Mapper) mapTypeDepth(s string, depth int) TypeInfo {
	if depth > maxMapDepth {
		return TypeInfo{Python: s, Go: "any", IsAny: true}
	}
	s = strings.TrimSpace(s)

	// Strip <class '...'> wrapper.
	if strings.HasPrefix(s, "<class '") && strings.HasSuffix(s, "'>") {
		inner := s[8 : len(s)-2]
		if goType, ok := baseTypes[inner]; ok {
			return TypeInfo{Python: s, Go: goType}
		}
		return TypeInfo{Python: s, Go: "*" + GoFuncName(inner), IsPtr: true}
	}

	// Base types.
	if goType, ok := baseTypes[s]; ok {
		return TypeInfo{Python: s, Go: goType}
	}

	// typing.Optional[X] or Optional[X]
	if inner, ok := unwrapGeneric(s, "typing.Optional"); ok {
		t := m.mapTypeDepth(inner, depth+1)
		if t.Go != "" && t.Go != "any" && !strings.HasPrefix(t.Go, "*") {
			t.Go = "*" + t.Go
		}
		t.Python = s
		t.IsPtr = true
		return t
	}
	if inner, ok := unwrapGeneric(s, "Optional"); ok {
		t := m.mapTypeDepth(inner, depth+1)
		if t.Go != "" && t.Go != "any" && !strings.HasPrefix(t.Go, "*") {
			t.Go = "*" + t.Go
		}
		t.Python = s
		t.IsPtr = true
		return t
	}

	// X | Y (Python 3.10+ union syntax, including X | None for Optional)
	if strings.Contains(s, " | ") {
		parts := strings.Split(s, " | ")
		nonNone := make([]string, 0, len(parts))
		hasNone := false
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "None" || p == "NoneType" {
				hasNone = true
			} else {
				nonNone = append(nonNone, p)
			}
		}
		if hasNone && len(nonNone) == 1 {
			// Optional: X | None → *X
			t := m.mapTypeDepth(nonNone[0], depth+1)
			if t.Go != "" && t.Go != "any" && !strings.HasPrefix(t.Go, "*") {
				t.Go = "*" + t.Go
			}
			t.Python = s
			t.IsPtr = true
			return t
		}
		// Multi-type union → any
		return TypeInfo{Python: s, Go: "any", IsAny: true}
	}

	// list[X] or typing.List[X]
	if inner, ok := unwrapGeneric(s, "list"); ok {
		elem := m.mapTypeDepth(inner, depth+1)
		return TypeInfo{Python: s, Go: "[]" + elem.Go}
	}
	if inner, ok := unwrapGeneric(s, "typing.List"); ok {
		elem := m.mapTypeDepth(inner, depth+1)
		return TypeInfo{Python: s, Go: "[]" + elem.Go}
	}

	// dict[K, V] or typing.Dict[K, V]
	if inner, ok := unwrapGeneric(s, "dict"); ok {
		k, v := splitTwo(inner)
		kt := m.mapTypeDepth(k, depth+1)
		vt := m.mapTypeDepth(v, depth+1)
		return TypeInfo{Python: s, Go: "map[" + kt.Go + "]" + vt.Go}
	}
	if inner, ok := unwrapGeneric(s, "typing.Dict"); ok {
		k, v := splitTwo(inner)
		kt := m.mapTypeDepth(k, depth+1)
		vt := m.mapTypeDepth(v, depth+1)
		return TypeInfo{Python: s, Go: "map[" + kt.Go + "]" + vt.Go}
	}

	// tuple[X, ...] → []X (homogeneous) or any (heterogeneous)
	if inner, ok := unwrapGeneric(s, "tuple"); ok {
		parts := splitParams(inner)
		if len(parts) == 2 && strings.TrimSpace(parts[1]) == "..." {
			elem := m.mapTypeDepth(parts[0], depth+1)
			return TypeInfo{Python: s, Go: "[]" + elem.Go}
		}
		return TypeInfo{Python: s, Go: "any", IsAny: true}
	}

	// Union[X, Y] or typing.Union[X, Y]
	for _, prefix := range []string{"typing.Union", "Union"} {
		if inner, ok := unwrapGeneric(s, prefix); ok {
			parts := splitParams(inner)
			// If one part is None/NoneType, it's Optional.
			nonNone := make([]string, 0, len(parts))
			hasNone := false
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p == "None" || p == "NoneType" || p == "<class 'NoneType'>" {
					hasNone = true
				} else {
					nonNone = append(nonNone, p)
				}
			}
			if hasNone && len(nonNone) == 1 {
				t := m.mapTypeDepth(nonNone[0], depth+1)
				if t.Go != "" && t.Go != "any" && !strings.HasPrefix(t.Go, "*") {
					t.Go = "*" + t.Go
				}
				t.Python = s
				t.IsPtr = true
				return t
			}
			return TypeInfo{Python: s, Go: "any", IsAny: true}
		}
	}

	// typing.Any or Any
	if s == "typing.Any" || s == "Any" {
		return TypeInfo{Python: s, Go: "any", IsAny: true}
	}

	// Unknown generics, literals, or complex annotations → any
	if strings.Contains(s, "[") || strings.Contains(s, "'") || strings.Contains(s, "]") ||
		strings.Contains(s, " ") || strings.Contains(s, ",") || strings.Contains(s, "|") {
		return TypeInfo{Python: s, Go: "any", IsAny: true}
	}

	// Unknown: treat as custom class.
	name := s
	// Strip module prefix if present (e.g., "claude_agent_sdk.types.McpSdkServerConfig" → "McpSdkServerConfig")
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[idx+1:]
	}
	goName := GoFuncName(name)
	if goName == "" || !isValidGoIdent(goName) {
		return TypeInfo{Python: s, Go: "any", IsAny: true}
	}
	return TypeInfo{Python: s, Go: "*" + goName, IsPtr: true}
}

func isValidGoIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if i == 0 && !unicode.IsLetter(r) {
			return false
		}
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// unwrapGeneric extracts the inner part of "prefix[inner]".
func unwrapGeneric(s, prefix string) (string, bool) {
	if !strings.HasPrefix(s, prefix+"[") || !strings.HasSuffix(s, "]") {
		return "", false
	}
	inner := s[len(prefix)+1 : len(s)-1]
	return inner, true
}

// splitTwo splits "K, V" at the top-level comma.
func splitTwo(s string) (string, string) {
	parts := splitParams(s)
	if len(parts) >= 2 {
		return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	}
	return strings.TrimSpace(s), "any"
}

// splitParams splits comma-separated type params respecting bracket nesting.
func splitParams(s string) []string {
	var parts []string
	depth := 0
	start := 0
	for i, ch := range s {
		switch ch {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, s[start:i])
				start = i + 1
			}
		}
	}
	parts = append(parts, s[start:])
	return parts
}

// GoFuncName converts snake_case to PascalCase.
// Also strips module prefixes (dots) and handles PascalCase input.
func GoFuncName(pyName string) string {
	// Strip module prefix: "claude_agent_sdk.types.Foo" → "Foo"
	if idx := strings.LastIndex(pyName, "."); idx >= 0 {
		pyName = pyName[idx+1:]
	}
	parts := strings.Split(pyName, "_")
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		runes := []rune(p)
		runes[0] = unicode.ToUpper(runes[0])
		b.WriteString(string(runes))
	}
	result := b.String()
	if result == "" {
		return "X"
	}
	// Ensure starts with letter.
	if !unicode.IsLetter(rune(result[0])) {
		return "X" + result
	}
	return result
}

// Go reserved words that cannot be used as parameter names.
var goReserved = map[string]string{
	"break": "break_", "case": "case_", "chan": "chan_", "const": "const_",
	"continue": "continue_", "default": "default_", "defer": "defer_",
	"else": "else_", "fallthrough": "fallthrough_", "for": "for_",
	"func": "func_", "go": "go_", "goto": "goto_", "if": "if_",
	"import": "import_", "interface": "interface_", "map": "map_",
	"package": "package_", "range": "range_", "return": "return_",
	"select": "select_", "struct": "struct_", "switch": "switch_",
	"type": "type_", "var": "var_",
}

// GoParamName converts snake_case to camelCase, escaping Go reserved words.
func GoParamName(pyName string) string {
	parts := strings.Split(pyName, "_")
	var b strings.Builder
	for i, p := range parts {
		if p == "" {
			continue
		}
		runes := []rune(p)
		if i == 0 {
			runes[0] = unicode.ToLower(runes[0])
		} else {
			runes[0] = unicode.ToUpper(runes[0])
		}
		b.WriteString(string(runes))
	}
	result := b.String()
	if result == "" {
		return "x"
	}
	if escaped, ok := goReserved[result]; ok {
		return escaped
	}
	return result
}
