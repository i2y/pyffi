package casdk

// MCPTool defines a tool for an in-process MCP server.
type MCPTool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     func(args map[string]any) (string, error)
	Annotations *ToolAnnotations
}

// ToolAnnotations provides optional metadata hints for MCP tools.
type ToolAnnotations struct {
	ReadOnly    *bool
	Destructive *bool
	OpenWorld   *bool
}

// SDKMCPServer defines an in-process MCP server with Go tool handlers.
type SDKMCPServer struct {
	Name    string
	Version string // defaults to "1.0.0" if empty
	Tools   []MCPTool
}

// MCPServerStatus represents the status of an MCP server.
type MCPServerStatus struct {
	Name   string
	Status string // "connected", "disconnected", "error"
}
