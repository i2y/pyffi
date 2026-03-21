package casdk

// QueryOption configures a Query or Session call.
type QueryOption func(*queryConfig)

type queryConfig struct {
	model                   string
	systemPrompt            string
	maxTurns                int
	permissionMode          string
	cwd                     string
	allowedTools            []string
	disallowedTools         []string
	maxBudgetUSD            float64
	resume                  string
	thinking                *ThinkingConfig
	sandbox                 *SandboxConfig
	mcpServers              map[string]MCPServerConfig
	sdkMCPServers           []SDKMCPServer
	agents                  map[string]AgentDefinition
	hooks                   map[HookEvent][]HookMatcher
	canUseTool              CanUseToolFunc
	plugins                 []PluginConfig
	includePartialMessages  bool
	enableFileCheckpointing bool
	settingSources          []string
	cliPath                 string
}

// WithModel sets the model (e.g. "sonnet", "opus", "haiku", "claude-sonnet-4-5-20250514").
func WithModel(model string) QueryOption {
	return func(c *queryConfig) { c.model = model }
}

// WithSystemPrompt sets a custom system prompt.
func WithSystemPrompt(prompt string) QueryOption {
	return func(c *queryConfig) { c.systemPrompt = prompt }
}

// WithMaxTurns limits the number of agentic turns.
func WithMaxTurns(n int) QueryOption {
	return func(c *queryConfig) { c.maxTurns = n }
}

// WithPermissionMode sets the permission mode.
// Valid values: "default", "acceptEdits", "plan", "bypassPermissions".
func WithPermissionMode(mode string) QueryOption {
	return func(c *queryConfig) { c.permissionMode = mode }
}

// WithCwd sets the working directory for the agent.
func WithCwd(dir string) QueryOption {
	return func(c *queryConfig) { c.cwd = dir }
}

// WithAllowedTools sets tools that are auto-approved without prompting.
func WithAllowedTools(tools ...string) QueryOption {
	return func(c *queryConfig) { c.allowedTools = tools }
}

// WithDisallowedTools sets tools that are blocked.
func WithDisallowedTools(tools ...string) QueryOption {
	return func(c *queryConfig) { c.disallowedTools = tools }
}

// WithMaxBudget sets the max budget in USD.
func WithMaxBudget(usd float64) QueryOption {
	return func(c *queryConfig) { c.maxBudgetUSD = usd }
}

// WithResume resumes an existing session by ID.
func WithResume(sessionID string) QueryOption {
	return func(c *queryConfig) { c.resume = sessionID }
}

// WithThinking configures extended thinking.
func WithThinking(config ThinkingConfig) QueryOption {
	return func(c *queryConfig) { c.thinking = &config }
}

// WithSandbox configures sandbox settings.
func WithSandbox(config SandboxConfig) QueryOption {
	return func(c *queryConfig) { c.sandbox = &config }
}

// WithMCPServers configures MCP server connections.
func WithMCPServers(servers map[string]MCPServerConfig) QueryOption {
	return func(c *queryConfig) { c.mcpServers = servers }
}

// WithAgents defines custom subagents.
func WithAgents(agents map[string]AgentDefinition) QueryOption {
	return func(c *queryConfig) { c.agents = agents }
}

// WithHooks sets lifecycle event hooks.
func WithHooks(hooks map[HookEvent][]HookMatcher) QueryOption {
	return func(c *queryConfig) { c.hooks = hooks }
}

// WithCanUseTool sets a custom tool permission callback.
func WithCanUseTool(fn CanUseToolFunc) QueryOption {
	return func(c *queryConfig) { c.canUseTool = fn }
}

// WithIncludePartialMessages enables streaming partial message updates.
func WithIncludePartialMessages(v bool) QueryOption {
	return func(c *queryConfig) { c.includePartialMessages = v }
}

// WithEnableFileCheckpointing enables file change tracking.
func WithEnableFileCheckpointing(v bool) QueryOption {
	return func(c *queryConfig) { c.enableFileCheckpointing = v }
}

// WithSettingSources loads settings from the given sources ("user", "project", "local").
func WithSettingSources(sources ...string) QueryOption {
	return func(c *queryConfig) { c.settingSources = sources }
}

// WithCLIPath sets the path to the Claude Code CLI binary.
func WithCLIPath(path string) QueryOption {
	return func(c *queryConfig) { c.cliPath = path }
}

// ThinkingConfig configures extended thinking.
type ThinkingConfig struct {
	Type         string // "enabled", "adaptive", "disabled"
	BudgetTokens int    // only used when Type is "enabled"
}

// SandboxConfig configures sandbox settings.
type SandboxConfig struct {
	Enabled                  bool
	AutoAllowBashIfSandboxed bool
	Network                  *NetworkConfig
	ExcludedCommands         []string
}

// NetworkConfig configures network access within the sandbox.
type NetworkConfig struct {
	AllowLocalBinding bool
}

// MCPServerConfig configures an MCP server connection.
type MCPServerConfig struct {
	Command string
	Args    []string
	Env     map[string]string
}

// AgentDefinition defines a custom subagent.
type AgentDefinition struct {
	Description string
	Prompt      string
	Tools       []string
}

// WithPlugins configures SDK plugins.
func WithPlugins(configs ...PluginConfig) QueryOption {
	return func(c *queryConfig) { c.plugins = configs }
}

// WithSDKMCPServers configures in-process MCP servers with Go tool handlers.
func WithSDKMCPServers(servers ...SDKMCPServer) QueryOption {
	return func(c *queryConfig) { c.sdkMCPServers = servers }
}

// PluginConfig configures an SDK plugin.
type PluginConfig struct {
	Type string // "local"
	Path string
}

// CanUseToolFunc is a callback for custom tool permission decisions.
type CanUseToolFunc func(toolName string, input map[string]any) PermissionResult

// PermissionResult is returned by CanUseToolFunc.
type PermissionResult struct {
	Allow        bool
	UpdatedInput map[string]any // only if Allow=true
	Message      string         // only if Allow=false
	Interrupt    bool           // only if Allow=false
}
