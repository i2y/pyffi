package oasdk

// RunOption configures a Run call.
type RunOption func(*runConfig)

type runConfig struct {
	maxTurns          int
	modelOverride     string
	previousInputList string // JSON-encoded input list from previous RunResult
}

// WithMaxTurns limits the number of agentic turns.
func WithMaxTurns(n int) RunOption {
	return func(c *runConfig) { c.maxTurns = n }
}

// WithModelOverride overrides the model for this run.
func WithModelOverride(model string) RunOption {
	return func(c *runConfig) { c.modelOverride = model }
}

// WithPreviousResult continues a conversation from a previous result.
func WithPreviousResult(r *RunResult) RunOption {
	return func(c *runConfig) { c.previousInputList = r.inputListJSON }
}
