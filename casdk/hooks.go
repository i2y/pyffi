package casdk

// HookEvent represents a lifecycle event type.
type HookEvent string

const (
	HookPreToolUse         HookEvent = "PreToolUse"
	HookPostToolUse        HookEvent = "PostToolUse"
	HookPostToolUseFailure HookEvent = "PostToolUseFailure"
	HookUserPromptSubmit   HookEvent = "UserPromptSubmit"
	HookStop               HookEvent = "Stop"
	HookSessionStart       HookEvent = "SessionStart"
	HookSessionEnd         HookEvent = "SessionEnd"
	HookPermissionRequest  HookEvent = "PermissionRequest"
	HookNotification       HookEvent = "Notification"
)

// HookMatcher matches hook events and dispatches to a handler.
type HookMatcher struct {
	Matcher string                                             // tool name regex (for PreToolUse etc.)
	Handler func(input map[string]any) (map[string]any, error) // Go handler
	Timeout float64                                            // seconds; 0 uses the default (60s)
}
