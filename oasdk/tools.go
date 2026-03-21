package oasdk

// ToolDef defines a tool that an agent can use.
type ToolDef struct {
	Name        string
	Description string
	Schema      map[string]any                            // JSON Schema for parameters
	Handler     func(args map[string]any) (string, error) // Go handler
}

// FuncTool creates a tool definition with an explicit JSON schema.
func FuncTool(name, description string, schema map[string]any, handler func(args map[string]any) (string, error)) ToolDef {
	return ToolDef{
		Name:        name,
		Description: description,
		Schema:      schema,
		Handler:     handler,
	}
}
