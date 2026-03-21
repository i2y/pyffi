package oasdk

// Agent represents an AI agent configured with instructions, tools, and handoffs.
// Agent is a pure Go struct; the corresponding Python object is created at Run() time.
type Agent struct {
	name            string
	instructions    string
	model           string
	modelSettings   *ModelSettings
	tools           []ToolDef
	handoffs        []*Agent
	toolUseBehavior string // "run_llm_again" (default), "stop_on_first_tool"
}

// ModelSettings configures model-specific tuning parameters.
type ModelSettings struct {
	Temperature *float64
	TopP        *float64
	MaxTokens   *int
}

// AgentOption configures an Agent.
type AgentOption func(*Agent)

// NewAgent creates a new Agent with the given name and options.
func NewAgent(name string, opts ...AgentOption) *Agent {
	a := &Agent{name: name}
	for _, o := range opts {
		o(a)
	}
	return a
}

// WithInstructions sets the agent's system prompt.
func WithInstructions(s string) AgentOption {
	return func(a *Agent) { a.instructions = s }
}

// WithModel sets the model to use (e.g. "gpt-4o", "gpt-4o-mini").
func WithModel(model string) AgentOption {
	return func(a *Agent) { a.model = model }
}

// WithModelSettings configures model tuning parameters.
func WithModelSettings(ms ModelSettings) AgentOption {
	return func(a *Agent) { a.modelSettings = &ms }
}

// WithTools adds tools to the agent.
func WithTools(tools ...ToolDef) AgentOption {
	return func(a *Agent) { a.tools = append(a.tools, tools...) }
}

// WithHandoffs sets agents that this agent can delegate to.
func WithHandoffs(agents ...*Agent) AgentOption {
	return func(a *Agent) { a.handoffs = agents }
}

// WithToolUseBehavior sets how tool results are handled.
// Valid values: "run_llm_again" (default), "stop_on_first_tool".
func WithToolUseBehavior(b string) AgentOption {
	return func(a *Agent) { a.toolUseBehavior = b }
}
