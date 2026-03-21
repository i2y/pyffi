// Package casdk provides a Go-idiomatic wrapper around the
// [Claude Agent SDK] for building applications powered by Claude Code.
//
// The SDK is auto-installed via uv on first use. Set the ANTHROPIC_API_KEY
// environment variable for authentication.
//
// # One-Off Queries
//
// Use [Client.Query] for single-turn interactions:
//
//	client, _ := casdk.New()
//	defer client.Close()
//
//	for msg, err := range client.Query(ctx, "Explain Go interfaces",
//	    casdk.WithMaxTurns(1),
//	    casdk.WithModel("sonnet"),
//	) {
//	    if err != nil { log.Fatal(err) }
//	    if msg.Type() == "assistant" {
//	        for _, block := range msg.ContentBlocks() {
//	            if tb, ok := block.(casdk.TextBlock); ok {
//	                fmt.Println(tb.Text)
//	            }
//	        }
//	    }
//	}
//
// # Interactive Sessions
//
// Use [NewSession] for multi-turn conversations:
//
//	session, _ := casdk.NewSession(casdk.WithModel("sonnet"))
//	defer session.Close()
//
//	session.Query("What files are in this directory?")
//	for msg, _ := range session.ReceiveMessages() {
//	    fmt.Println(msg.Text())
//	}
//
//	session.Query("Now explain the main.go file")
//	for msg, _ := range session.ReceiveMessages() {
//	    fmt.Println(msg.Text())
//	}
//
// # Session Management
//
// List and inspect past Claude Code sessions (no API key needed):
//
//	sessions, _ := client.ListSessions(casdk.WithLimit(10))
//	msgs, _ := client.GetSessionMessages(sessions[0].SessionID)
//
// # Configuration
//
// All [ClaudeAgentOptions] fields are exposed as [QueryOption] functions:
// [WithModel], [WithSystemPrompt], [WithMaxTurns], [WithPermissionMode],
// [WithAllowedTools], [WithThinking], [WithSandbox], [WithMCPServers],
// [WithResume], [WithMaxBudget], and more.
//
// [Claude Agent SDK]: https://github.com/anthropics/claude-agent-sdk-python
package casdk

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"strings"
	"sync/atomic"
	"time"

	"github.com/i2y/pyffi"
	"github.com/i2y/pyffi/casdk/internal/sdk"
)

// callbackSeq generates unique names for Go→Python callbacks.
var callbackSeq atomic.Int64

func nextCallbackID(prefix string) string {
	return fmt.Sprintf("_casdk_%s_%d", prefix, callbackSeq.Add(1))
}

// Client wraps the Claude Agent SDK for one-off queries and session management.
type Client struct {
	rt  *pyffi.Runtime
	mod *sdk.Module
}

// New creates a new Claude Agent SDK client.
// The SDK is auto-installed via uv if not present.
// Set ANTHROPIC_API_KEY environment variable for authentication.
func New() (*Client, error) {
	rt, err := pyffi.New(pyffi.Dependencies("claude-agent-sdk"))
	if err != nil {
		return nil, fmt.Errorf("casdk: %w", err)
	}

	mod, err := sdk.New(rt)
	if err != nil {
		rt.Close()
		return nil, fmt.Errorf("casdk: %w", err)
	}

	return &Client{rt: rt, mod: mod}, nil
}

// Close releases all resources.
func (c *Client) Close() error {
	c.mod.Close()
	return c.rt.Close()
}

// Runtime returns the underlying pyffi Runtime for advanced use.
func (c *Client) Runtime() *pyffi.Runtime {
	return c.rt
}

// Query sends a prompt to Claude and returns an iterator over response messages.
//
//	for msg, err := range client.Query(ctx, "Hello") {
//	    if err != nil { log.Fatal(err) }
//	    fmt.Println(msg.Text())
//	}
func (c *Client) Query(ctx context.Context, prompt string, opts ...QueryOption) iter.Seq2[*Message, error] {
	var cfg queryConfig
	for _, o := range opts {
		o(&cfg)
	}

	// Hooks, SDK MCP servers, and can_use_tool require ClaudeSDKClient
	// (not the simpler sdk.query() function).
	needsClient := cfg.canUseTool != nil || len(cfg.hooks) > 0 || len(cfg.sdkMCPServers) > 0

	return func(yield func(*Message, error) bool) {
		optionsCode, err := c.buildOptionsCode(cfg)
		if err != nil {
			yield(nil, err)
			return
		}

		var code string
		if needsClient {
			code = fmt.Sprintf(`
import asyncio
import claude_agent_sdk as sdk

%s

async def _pyffi_run_query(prompt):
    msgs = []
    async with sdk.ClaudeSDKClient(options=_pyffi_options) as client:
        await client.query(prompt)
        async for msg in client.receive_response():
            msgs.append(msg)
    return msgs

_pyffi_query_result = asyncio.run(_pyffi_run_query(_pyffi_prompt))
`, optionsCode)
		} else {
			code = fmt.Sprintf(`
import asyncio
import claude_agent_sdk as sdk

%s

async def _pyffi_run_query(prompt):
    kwargs = {"prompt": prompt}
    if _pyffi_options is not None:
        kwargs["options"] = _pyffi_options
    msgs = []
    async for msg in sdk.query(**kwargs):
        msgs.append(msg)
    return msgs

_pyffi_query_result = asyncio.run(_pyffi_run_query(_pyffi_prompt))
`, optionsCode)
		}

		mainMod, _ := c.rt.Import("__main__")
		defer mainMod.Close()

		promptObj := c.rt.FromString(prompt)
		defer promptObj.Close()
		mainMod.SetAttr("_pyffi_prompt", promptObj)

		if ctx.Err() != nil {
			yield(nil, ctx.Err())
			return
		}

		if err := c.rt.Exec(code); err != nil {
			yield(nil, fmt.Errorf("casdk: query failed: %w", err))
			return
		}

		resultObj := mainMod.Attr("_pyffi_query_result")
		if resultObj == nil {
			yield(nil, fmt.Errorf("casdk: no result"))
			return
		}
		defer resultObj.Close()

		n, _ := resultObj.Len()
		for i := int64(0); i < n; i++ {
			if ctx.Err() != nil {
				yield(nil, ctx.Err())
				return
			}
			itemObj, err := resultObj.GetItem(int(i))
			if err != nil {
				yield(nil, err)
				return
			}
			msg := extractMessage(itemObj)
			if !yield(msg, nil) {
				return
			}
		}
	}
}

// buildOptionsCode generates Python code to construct ClaudeAgentOptions.
func (c *Client) buildOptionsCode(cfg queryConfig) (string, error) {
	var preCode strings.Builder // imports, callback wrappers, variables
	var parts []string

	// --- simple scalar options ---

	if cfg.model != "" {
		parts = append(parts, fmt.Sprintf("model=%q", cfg.model))
	}
	if cfg.systemPrompt != "" {
		parts = append(parts, fmt.Sprintf("system_prompt=%q", cfg.systemPrompt))
	}
	if cfg.maxTurns > 0 {
		parts = append(parts, fmt.Sprintf("max_turns=%d", cfg.maxTurns))
	}
	if cfg.permissionMode != "" {
		parts = append(parts, fmt.Sprintf("permission_mode=%q", cfg.permissionMode))
	}
	if cfg.cwd != "" {
		parts = append(parts, fmt.Sprintf("cwd=%q", cfg.cwd))
	}
	if cfg.maxBudgetUSD > 0 {
		parts = append(parts, fmt.Sprintf("max_budget_usd=%f", cfg.maxBudgetUSD))
	}
	if cfg.resume != "" {
		parts = append(parts, fmt.Sprintf("resume=%q", cfg.resume))
	}
	if cfg.includePartialMessages {
		parts = append(parts, "include_partial_messages=True")
	}
	if cfg.enableFileCheckpointing {
		parts = append(parts, "enable_file_checkpointing=True")
	}
	if cfg.cliPath != "" {
		parts = append(parts, fmt.Sprintf("cli_path=%q", cfg.cliPath))
	}
	if len(cfg.allowedTools) > 0 {
		b, _ := json.Marshal(cfg.allowedTools)
		parts = append(parts, fmt.Sprintf("allowed_tools=%s", string(b)))
	}
	if len(cfg.disallowedTools) > 0 {
		b, _ := json.Marshal(cfg.disallowedTools)
		parts = append(parts, fmt.Sprintf("disallowed_tools=%s", string(b)))
	}
	if len(cfg.settingSources) > 0 {
		b, _ := json.Marshal(cfg.settingSources)
		parts = append(parts, fmt.Sprintf("setting_sources=%s", string(b)))
	}
	if cfg.thinking != nil {
		tc := fmt.Sprintf(`{"type": %q`, cfg.thinking.Type)
		if cfg.thinking.BudgetTokens > 0 {
			tc += fmt.Sprintf(`, "budget_tokens": %d`, cfg.thinking.BudgetTokens)
		}
		tc += "}"
		parts = append(parts, fmt.Sprintf("thinking=%s", tc))
	}
	if cfg.sandbox != nil {
		sc := fmt.Sprintf("{'enabled': %s", pythonBool(cfg.sandbox.Enabled))
		if cfg.sandbox.AutoAllowBashIfSandboxed {
			sc += ", 'autoAllowBashIfSandboxed': True"
		}
		if cfg.sandbox.Network != nil {
			sc += fmt.Sprintf(", 'network': {'allowLocalBinding': %s}", pythonBool(cfg.sandbox.Network.AllowLocalBinding))
		}
		if len(cfg.sandbox.ExcludedCommands) > 0 {
			b, _ := json.Marshal(cfg.sandbox.ExcludedCommands)
			sc += fmt.Sprintf(", 'excludedCommands': %s", string(b))
		}
		sc += "}"
		parts = append(parts, fmt.Sprintf("sandbox=%s", sc))
	}
	if len(cfg.agents) > 0 {
		m := "{"
		first := true
		for name, agent := range cfg.agents {
			if !first {
				m += ", "
			}
			toolsJSON, _ := json.Marshal(agent.Tools)
			m += fmt.Sprintf("%q: {'description': %q, 'prompt': %q, 'tools': %s}",
				name, agent.Description, agent.Prompt, string(toolsJSON))
			first = false
		}
		m += "}"
		parts = append(parts, fmt.Sprintf("agents=%s", m))
	}

	// --- plugins (pure data, no callbacks) ---

	if len(cfg.plugins) > 0 {
		p := "["
		for i, pc := range cfg.plugins {
			if i > 0 {
				p += ", "
			}
			p += fmt.Sprintf(`{"type": %q, "path": %q}`, pc.Type, pc.Path)
		}
		p += "]"
		parts = append(parts, fmt.Sprintf("plugins=%s", p))
	}

	// --- can_use_tool callback ---

	if cfg.canUseTool != nil {
		cbName := nextCallbackID("can_use_tool")
		goFn := cfg.canUseTool
		if err := c.rt.RegisterFunc(cbName, func(toolName string, input map[string]any) map[string]any {
			result := goFn(toolName, input)
			ret := map[string]any{"allow": result.Allow}
			if result.Allow && result.UpdatedInput != nil {
				ret["updated_input"] = result.UpdatedInput
			}
			if !result.Allow {
				if result.Message != "" {
					ret["message"] = result.Message
				}
				if result.Interrupt {
					ret["interrupt"] = true
				}
			}
			return ret
		}); err != nil {
			return "", fmt.Errorf("casdk: register can_use_tool: %w", err)
		}
		wrapperName := cbName + "_wrapper"
		preCode.WriteString("import go_bridge\n")
		preCode.WriteString(fmt.Sprintf(`
async def %s(tool_name, tool_input, context=None):
    result = go_bridge.%s(tool_name, tool_input)
    if result.get("allow"):
        from claude_agent_sdk.types import PermissionResultAllow
        return PermissionResultAllow(updated_input=result.get("updated_input"))
    from claude_agent_sdk.types import PermissionResultDeny
    return PermissionResultDeny(
        message=result.get("message", ""),
        interrupt=result.get("interrupt", False),
    )
`, wrapperName, cbName))
		parts = append(parts, fmt.Sprintf("can_use_tool=%s", wrapperName))
	}

	// --- hooks ---

	if len(cfg.hooks) > 0 {
		if preCode.Len() == 0 {
			preCode.WriteString("import go_bridge\n")
		}
		preCode.WriteString("import dataclasses as _dc\n")
		preCode.WriteString("from claude_agent_sdk import HookMatcher as _HookMatcher\n")

		var hookEntries []string
		for event, matchers := range cfg.hooks {
			var matcherCodes []string
			for _, matcher := range matchers {
				cbName := nextCallbackID("hook")
				handler := matcher.Handler
				if err := c.rt.RegisterFunc(cbName, func(input map[string]any) (map[string]any, error) {
					return handler(input)
				}); err != nil {
					return "", fmt.Errorf("casdk: register hook: %w", err)
				}
				wrapperName := cbName + "_wrapper"
				preCode.WriteString(fmt.Sprintf(`
async def %s(hook_input, tool_use_id=None, context=None):
    d = _dc.asdict(hook_input) if _dc.is_dataclass(hook_input) else (dict(hook_input) if isinstance(hook_input, dict) else {})
    return go_bridge.%s(d)
`, wrapperName, cbName))

				mc := fmt.Sprintf("_HookMatcher(hooks=[%s]", wrapperName)
				if matcher.Matcher != "" {
					mc += fmt.Sprintf(", matcher=%q", matcher.Matcher)
				}
				if matcher.Timeout > 0 {
					mc += fmt.Sprintf(", timeout=%g", matcher.Timeout)
				}
				mc += ")"
				matcherCodes = append(matcherCodes, mc)
			}
			hookEntries = append(hookEntries, fmt.Sprintf("%q: [%s]", string(event), strings.Join(matcherCodes, ", ")))
		}
		preCode.WriteString(fmt.Sprintf("_pyffi_hooks = {%s}\n", strings.Join(hookEntries, ", ")))
		parts = append(parts, "hooks=_pyffi_hooks")
	}

	// --- mcp_servers (external + SDK in-process) ---

	hasSDKMCP := len(cfg.sdkMCPServers) > 0
	hasExtMCP := len(cfg.mcpServers) > 0

	if hasSDKMCP {
		if preCode.Len() == 0 {
			preCode.WriteString("import go_bridge\n")
		}
		preCode.WriteString("from claude_agent_sdk import create_sdk_mcp_server as _create_mcp, SdkMcpTool as _SdkMcpTool\n")

		// Build external servers dict as a variable so we can merge SDK servers.
		preCode.WriteString("_pyffi_mcp_servers = {")
		if hasExtMCP {
			first := true
			for name, srv := range cfg.mcpServers {
				if !first {
					preCode.WriteString(", ")
				}
				argsJSON, _ := json.Marshal(srv.Args)
				preCode.WriteString(fmt.Sprintf("%q: {'command': %q, 'args': %s", name, srv.Command, string(argsJSON)))
				if len(srv.Env) > 0 {
					envJSON, _ := json.Marshal(srv.Env)
					preCode.WriteString(fmt.Sprintf(", 'env': %s", string(envJSON)))
				}
				preCode.WriteString("}")
				first = false
			}
		}
		preCode.WriteString("}\n")

		for srvIdx, srv := range cfg.sdkMCPServers {
			var toolCodes []string
			for toolIdx, tool := range srv.Tools {
				cbName := nextCallbackID(fmt.Sprintf("mcp_%d_%d", srvIdx, toolIdx))
				handler := tool.Handler
				if err := c.rt.RegisterFunc(cbName, func(args map[string]any) (string, error) {
					return handler(args)
				}); err != nil {
					return "", fmt.Errorf("casdk: register mcp tool %s: %w", tool.Name, err)
				}
				wrapperName := cbName + "_wrapper"
				preCode.WriteString(fmt.Sprintf(`
async def %s(args):
    try:
        d = dict(args) if not isinstance(args, dict) else args
        result = go_bridge.%s(d)
        return {"content": [{"type": "text", "text": str(result)}]}
    except Exception as e:
        return {"content": [{"type": "text", "text": str(e)}], "isError": True}
`, wrapperName, cbName))

				schemaJSON, _ := json.Marshal(tool.InputSchema)
				tc := fmt.Sprintf("_SdkMcpTool(name=%q, description=%q, input_schema=%s, handler=%s",
					tool.Name, tool.Description, string(schemaJSON), wrapperName)
				if tool.Annotations != nil {
					tc += ", annotations={"
					annParts := []string{}
					if tool.Annotations.ReadOnly != nil {
						annParts = append(annParts, fmt.Sprintf("'readOnlyHint': %s", pythonBool(*tool.Annotations.ReadOnly)))
					}
					if tool.Annotations.Destructive != nil {
						annParts = append(annParts, fmt.Sprintf("'destructiveHint': %s", pythonBool(*tool.Annotations.Destructive)))
					}
					if tool.Annotations.OpenWorld != nil {
						annParts = append(annParts, fmt.Sprintf("'openWorldHint': %s", pythonBool(*tool.Annotations.OpenWorld)))
					}
					tc += strings.Join(annParts, ", ") + "}"
				}
				tc += ")"
				toolCodes = append(toolCodes, tc)
			}

			version := srv.Version
			if version == "" {
				version = "1.0.0"
			}
			varName := fmt.Sprintf("_pyffi_sdk_mcp_%d", srvIdx)
			preCode.WriteString(fmt.Sprintf("%s = _create_mcp(%q, version=%q, tools=[%s])\n",
				varName, srv.Name, version, strings.Join(toolCodes, ", ")))
			preCode.WriteString(fmt.Sprintf("_pyffi_mcp_servers[%q] = %s\n", srv.Name, varName))
		}

		parts = append(parts, "mcp_servers=_pyffi_mcp_servers")
	} else if hasExtMCP {
		// External servers only (original inline dict approach).
		m := "{"
		first := true
		for name, srv := range cfg.mcpServers {
			if !first {
				m += ", "
			}
			argsJSON, _ := json.Marshal(srv.Args)
			m += fmt.Sprintf("%q: {'command': %q, 'args': %s", name, srv.Command, string(argsJSON))
			if len(srv.Env) > 0 {
				envJSON, _ := json.Marshal(srv.Env)
				m += fmt.Sprintf(", 'env': %s", string(envJSON))
			}
			m += "}"
			first = false
		}
		m += "}"
		parts = append(parts, fmt.Sprintf("mcp_servers=%s", m))
	}

	// --- assemble final code ---

	if len(parts) > 0 || preCode.Len() > 0 {
		code := preCode.String()
		code += "from claude_agent_sdk import ClaudeAgentOptions\n"
		code += "_pyffi_options = ClaudeAgentOptions(" + strings.Join(parts, ", ") + ")\n"
		return code, nil
	}

	return "_pyffi_options = None\n", nil
}

func pythonBool(v bool) string {
	if v {
		return "True"
	}
	return "False"
}

// --- Sessions ---

// SessionInfo represents metadata about a Claude Code session.
type SessionInfo struct {
	SessionID    string
	Summary      string
	LastModified time.Time
	CustomTitle  string
	FirstPrompt  string
	CWD          string
	Tag          string
}

// ListSessions returns metadata for recent Claude Code sessions.
func (c *Client) ListSessions(opts ...ListSessionsOption) ([]SessionInfo, error) {
	var cfg listSessionsConfig
	for _, o := range opts {
		o(&cfg)
	}

	code := `
import json, claude_agent_sdk as sdk, dataclasses
_ls_args = {}
`
	if cfg.limit > 0 {
		code += fmt.Sprintf("_ls_args['limit'] = %d\n", cfg.limit)
	}
	if cfg.directory != "" {
		code += fmt.Sprintf("_ls_args['directory'] = %q\n", cfg.directory)
	}
	code += `
_ls_result = sdk.list_sessions(**_ls_args)
_ls_json = json.dumps([dataclasses.asdict(s) for s in _ls_result])
`
	if err := c.rt.Exec(code); err != nil {
		return nil, fmt.Errorf("casdk: list_sessions: %w", err)
	}

	mainMod, _ := c.rt.Import("__main__")
	defer mainMod.Close()
	jsonObj := mainMod.Attr("_ls_json")
	if jsonObj == nil {
		return nil, fmt.Errorf("casdk: no result from list_sessions")
	}
	defer jsonObj.Close()

	jsonStr, _ := jsonObj.GoString()

	var raw []map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("casdk: parse sessions: %w", err)
	}

	var sessions []SessionInfo
	for _, m := range raw {
		si := SessionInfo{}
		if v, ok := m["session_id"].(string); ok {
			si.SessionID = v
		}
		if v, ok := m["summary"].(string); ok {
			si.Summary = v
		}
		if v, ok := m["last_modified"].(float64); ok {
			si.LastModified = time.Unix(int64(v), 0)
		}
		if v, ok := m["custom_title"].(string); ok {
			si.CustomTitle = v
		}
		if v, ok := m["first_prompt"].(string); ok {
			si.FirstPrompt = v
		}
		if v, ok := m["cwd"].(string); ok {
			si.CWD = v
		}
		if v, ok := m["tag"].(string); ok {
			si.Tag = v
		}
		sessions = append(sessions, si)
	}

	return sessions, nil
}

// ListSessionsOption configures ListSessions.
type ListSessionsOption func(*listSessionsConfig)

type listSessionsConfig struct {
	limit     int
	directory string
}

// WithLimit limits the number of sessions returned.
func WithLimit(n int) ListSessionsOption {
	return func(c *listSessionsConfig) { c.limit = n }
}

// WithDirectory sets the project directory to search for sessions.
func WithDirectory(dir string) ListSessionsOption {
	return func(c *listSessionsConfig) { c.directory = dir }
}

// SessionMessage represents a message within a session.
type SessionMessage struct {
	Type      string // "user" or "assistant"
	UUID      string
	SessionID string
	Content   string
}

// GetSessionMessages returns messages for a given session.
func (c *Client) GetSessionMessages(sessionID string, opts ...GetMessagesOption) ([]SessionMessage, error) {
	var cfg getMessagesConfig
	for _, o := range opts {
		o(&cfg)
	}

	code := fmt.Sprintf(`
import json, claude_agent_sdk as sdk, dataclasses
_gm_args = {'session_id': %q}
`, sessionID)
	if cfg.limit > 0 {
		code += fmt.Sprintf("_gm_args['limit'] = %d\n", cfg.limit)
	}
	if cfg.offset > 0 {
		code += fmt.Sprintf("_gm_args['offset'] = %d\n", cfg.offset)
	}
	code += `
_gm_result = sdk.get_session_messages(**_gm_args)
def _serialize_msg(m):
    d = dataclasses.asdict(m)
    if 'message' in d and d['message'] is not None:
        d['message'] = str(d['message'])
    return d
_gm_json = json.dumps([_serialize_msg(m) for m in _gm_result])
`
	if err := c.rt.Exec(code); err != nil {
		return nil, fmt.Errorf("casdk: get_session_messages: %w", err)
	}

	mainMod, _ := c.rt.Import("__main__")
	defer mainMod.Close()
	jsonObj := mainMod.Attr("_gm_json")
	if jsonObj == nil {
		return nil, fmt.Errorf("casdk: no result")
	}
	defer jsonObj.Close()
	jsonStr, _ := jsonObj.GoString()

	var raw []map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return nil, fmt.Errorf("casdk: parse messages: %w", err)
	}

	var messages []SessionMessage
	for _, m := range raw {
		sm := SessionMessage{}
		if v, ok := m["type"].(string); ok {
			sm.Type = v
		}
		if v, ok := m["uuid"].(string); ok {
			sm.UUID = v
		}
		if v, ok := m["session_id"].(string); ok {
			sm.SessionID = v
		}
		if v, ok := m["message"].(string); ok {
			sm.Content = v
		}
		messages = append(messages, sm)
	}

	return messages, nil
}

// GetMessagesOption configures GetSessionMessages.
type GetMessagesOption func(*getMessagesConfig)

type getMessagesConfig struct {
	limit  int
	offset int
}

// WithMessageLimit limits the number of messages returned.
func WithMessageLimit(n int) GetMessagesOption {
	return func(c *getMessagesConfig) { c.limit = n }
}

// WithOffset sets the message offset.
func WithOffset(n int) GetMessagesOption {
	return func(c *getMessagesConfig) { c.offset = n }
}

// RenameSession renames a session.
func (c *Client) RenameSession(sessionID, title string) error {
	return c.mod.RenameSession(sessionID, title)
}

// TagSession tags or untags a session. Pass "" to clear the tag.
func (c *Client) TagSession(sessionID, tag string) error {
	var tagVal any = tag
	if tag == "" {
		tagVal = nil
	}
	return c.mod.TagSession(sessionID, tagVal)
}
