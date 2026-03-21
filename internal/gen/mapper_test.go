package gen

import "testing"

func TestMapBaseTypes(t *testing.T) {
	m := NewMapper(nil)
	tests := []struct {
		input string
		want  string
	}{
		{"int", "int64"},
		{"float", "float64"},
		{"str", "string"},
		{"bool", "bool"},
		{"bytes", "[]byte"},
		{"NoneType", ""},
		{"None", ""},
	}
	for _, tt := range tests {
		ti := m.Map(tt.input)
		if ti.Go != tt.want {
			t.Errorf("Map(%q) = %q, want %q", tt.input, ti.Go, tt.want)
		}
	}
}

func TestMapClassWrapper(t *testing.T) {
	m := NewMapper(nil)
	tests := []struct {
		input string
		want  string
	}{
		{"<class 'int'>", "int64"},
		{"<class 'str'>", "string"},
		{"<class 'bool'>", "bool"},
		{"<class 'MyClass'>", "*MyClass"},
	}
	for _, tt := range tests {
		ti := m.Map(tt.input)
		if ti.Go != tt.want {
			t.Errorf("Map(%q) = %q, want %q", tt.input, ti.Go, tt.want)
		}
	}
}

func TestMapOptional(t *testing.T) {
	m := NewMapper(nil)
	tests := []struct {
		input string
		want  string
	}{
		{"typing.Optional[str]", "*string"},
		{"Optional[int]", "*int64"},
		{"str | None", "*string"},
	}
	for _, tt := range tests {
		ti := m.Map(tt.input)
		if ti.Go != tt.want {
			t.Errorf("Map(%q) = %q, want %q", tt.input, ti.Go, tt.want)
		}
		if !ti.IsPtr {
			t.Errorf("Map(%q).IsPtr = false, want true", tt.input)
		}
	}
}

func TestMapGenericContainers(t *testing.T) {
	m := NewMapper(nil)
	tests := []struct {
		input string
		want  string
	}{
		{"list[int]", "[]int64"},
		{"typing.List[str]", "[]string"},
		{"list[float]", "[]float64"},
		{"dict[str, int]", "map[string]int64"},
		{"typing.Dict[str, float]", "map[string]float64"},
		{"tuple[int, ...]", "[]int64"},
	}
	for _, tt := range tests {
		ti := m.Map(tt.input)
		if ti.Go != tt.want {
			t.Errorf("Map(%q) = %q, want %q", tt.input, ti.Go, tt.want)
		}
	}
}

func TestMapUnion(t *testing.T) {
	m := NewMapper(nil)

	// Union with None → Optional
	ti := m.Map("typing.Union[str, None]")
	if ti.Go != "*string" {
		t.Errorf("Map(Union[str, None]) = %q, want *string", ti.Go)
	}

	// Union without None → any
	ti = m.Map("typing.Union[int, str]")
	if ti.Go != "any" {
		t.Errorf("Map(Union[int, str]) = %q, want any", ti.Go)
	}
}

func TestMapAny(t *testing.T) {
	m := NewMapper(nil)
	for _, input := range []string{"typing.Any", "Any", ""} {
		ti := m.Map(input)
		if ti.Go != "any" {
			t.Errorf("Map(%q) = %q, want any", input, ti.Go)
		}
		if !ti.IsAny {
			t.Errorf("Map(%q).IsAny = false, want true", input)
		}
	}
}

func TestMapCustomClass(t *testing.T) {
	m := NewMapper(nil)
	ti := m.Map("sklearn.ensemble.RandomForestClassifier")
	if ti.Go != "*RandomForestClassifier" {
		t.Errorf("got %q, want *RandomForestClassifier", ti.Go)
	}
}

func TestMapOverrides(t *testing.T) {
	m := NewMapper(map[string]string{
		"numpy.ndarray": "[][]float64",
	})
	ti := m.Map("numpy.ndarray")
	if ti.Go != "[][]float64" {
		t.Errorf("got %q, want [][]float64", ti.Go)
	}
}

func TestMapComplexGenerics(t *testing.T) {
	m := NewMapper(nil)
	// Heterogeneous tuple → any
	ti := m.Map("tuple[int, str]")
	if ti.Go != "any" {
		t.Errorf("Map(tuple[int, str]) = %q, want any", ti.Go)
	}
}

func TestGoFuncName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"train_test_split", "TrainTestSplit"},
		{"dumps", "Dumps"},
		{"__init__", "Init"},
		{"x", "X"},
		{"", "X"},
		{"RandomForestClassifier", "RandomForestClassifier"},
	}
	for _, tt := range tests {
		got := GoFuncName(tt.input)
		if got != tt.want {
			t.Errorf("GoFuncName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGoParamName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"n_estimators", "nEstimators"},
		{"x", "x"},
		{"sort_keys", "sortKeys"},
		{"default", "default_"},
		{"type", "type_"},
		{"map", "map_"},
	}
	for _, tt := range tests {
		got := GoParamName(tt.input)
		if got != tt.want {
			t.Errorf("GoParamName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
