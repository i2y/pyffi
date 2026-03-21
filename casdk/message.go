package casdk

import (
	"github.com/i2y/pyffi"
)

// Message represents a message from the Claude Agent SDK.
// All fields are eagerly extracted from the Python object during construction,
// so Message does not require Close().
type Message struct {
	typ       string
	text      string
	model     string
	sessionID string
	isError   bool
	duration  int
	numTurns  int
	costUSD   float64
	usage     *Usage
	blocks    []ContentBlock
}

// Type returns the message type: "user", "assistant", "system", "result".
func (m *Message) Type() string { return m.typ }

// Text returns the text content of the message.
func (m *Message) Text() string { return m.text }

// Model returns the model name (for assistant messages).
func (m *Message) Model() string { return m.model }

// SessionID returns the session ID (for system and result messages).
func (m *Message) SessionID() string { return m.sessionID }

// IsError returns whether the result is an error (for result messages).
func (m *Message) IsError() bool { return m.isError }

// DurationMs returns the duration in milliseconds (for result messages).
func (m *Message) DurationMs() int { return m.duration }

// NumTurns returns the number of agentic turns (for result messages).
func (m *Message) NumTurns() int { return m.numTurns }

// TotalCostUSD returns the total cost in USD (for result messages).
func (m *Message) TotalCostUSD() float64 { return m.costUSD }

// Usage returns token usage information (for result messages).
func (m *Message) Usage() *Usage { return m.usage }

// ContentBlocks returns the content blocks (for assistant messages).
func (m *Message) ContentBlocks() []ContentBlock { return m.blocks }


// Usage holds token usage information.
type Usage struct {
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
}

// ContentBlock is an interface for message content blocks.
type ContentBlock interface {
	contentBlock()
}

// TextBlock represents a text content block.
type TextBlock struct {
	Text string
}

func (TextBlock) contentBlock() {}

// ToolUseBlock represents a tool use content block.
type ToolUseBlock struct {
	ID    string
	Name  string
	Input map[string]any
}

func (ToolUseBlock) contentBlock() {}

// ToolResultBlock represents a tool result content block.
type ToolResultBlock struct {
	ToolUseID string
	Content   string
	IsError   bool
}

func (ToolResultBlock) contentBlock() {}

// ThinkingBlock represents an extended thinking content block.
type ThinkingBlock struct {
	Thinking string
}

func (ThinkingBlock) contentBlock() {}

// extractMessage extracts a Go Message from a Python message object.
// The Python object is closed before returning; the caller does not need
// to close the returned Message.
func extractMessage(obj *pyffi.Object) *Message {
	defer obj.Close()
	msg := &Message{}

	cls := obj.Attr("__class__")
	if cls != nil {
		nameObj := cls.Attr("__name__")
		if nameObj != nil {
			name, _ := nameObj.GoString()
			switch name {
			case "UserMessage":
				msg.typ = "user"
			case "AssistantMessage":
				msg.typ = "assistant"
				extractAssistantFields(obj, msg)
			case "SystemMessage":
				msg.typ = "system"
				extractSystemFields(obj, msg)
			case "ResultMessage":
				msg.typ = "result"
				extractResultFields(obj, msg)
			default:
				msg.typ = name
			}
			nameObj.Close()
		}
		cls.Close()
	}

	msg.text = extractText(obj)
	return msg
}

func extractAssistantFields(obj *pyffi.Object, msg *Message) {
	if m := obj.Attr("model"); m != nil {
		msg.model, _ = m.GoString()
		m.Close()
	}
	msg.blocks = extractContentBlocks(obj)
}

func extractSystemFields(obj *pyffi.Object, msg *Message) {
	if s := obj.Attr("session_id"); s != nil && !s.IsNone() {
		msg.sessionID, _ = s.GoString()
		s.Close()
	}
}

func extractResultFields(obj *pyffi.Object, msg *Message) {
	if s := obj.Attr("session_id"); s != nil && !s.IsNone() {
		msg.sessionID, _ = s.GoString()
		s.Close()
	}
	if v := obj.Attr("is_error"); v != nil {
		msg.isError, _ = v.Bool()
		v.Close()
	}
	if v := obj.Attr("duration_ms"); v != nil && !v.IsNone() {
		n, _ := v.Int64()
		msg.duration = int(n)
		v.Close()
	}
	if v := obj.Attr("num_turns"); v != nil && !v.IsNone() {
		n, _ := v.Int64()
		msg.numTurns = int(n)
		v.Close()
	}
	if v := obj.Attr("total_cost_usd"); v != nil && !v.IsNone() {
		msg.costUSD, _ = v.Float64()
		v.Close()
	}
	if u := obj.Attr("usage"); u != nil && !u.IsNone() {
		msg.usage = extractUsage(u)
		u.Close()
	}
	if v := obj.Attr("result"); v != nil && !v.IsNone() {
		msg.text, _ = v.GoString()
		v.Close()
	}
}

func extractUsage(obj *pyffi.Object) *Usage {
	u := &Usage{}
	getInt := func(name string) int {
		v := obj.Attr(name)
		if v == nil || v.IsNone() {
			if v != nil {
				v.Close()
			}
			return 0
		}
		n, _ := v.Int64()
		v.Close()
		return int(n)
	}
	u.InputTokens = getInt("input_tokens")
	u.OutputTokens = getInt("output_tokens")
	u.CacheCreationInputTokens = getInt("cache_creation_input_tokens")
	u.CacheReadInputTokens = getInt("cache_read_input_tokens")
	return u
}

func extractContentBlocks(obj *pyffi.Object) []ContentBlock {
	content := obj.Attr("content")
	if content == nil {
		return nil
	}
	defer content.Close()

	n, _ := content.Len()
	if n <= 0 {
		return nil
	}

	var blocks []ContentBlock
	for i := int64(0); i < n; i++ {
		block, err := content.GetItem(int(i))
		if err != nil || block == nil {
			continue
		}

		cls := block.Attr("__class__")
		if cls == nil {
			block.Close()
			continue
		}
		nameObj := cls.Attr("__name__")
		cls.Close()
		if nameObj == nil {
			block.Close()
			continue
		}
		name, _ := nameObj.GoString()
		nameObj.Close()

		switch name {
		case "TextBlock":
			if t := block.Attr("text"); t != nil {
				s, _ := t.GoString()
				blocks = append(blocks, TextBlock{Text: s})
				t.Close()
			}
		case "ToolUseBlock":
			tb := ToolUseBlock{}
			if v := block.Attr("id"); v != nil {
				tb.ID, _ = v.GoString()
				v.Close()
			}
			if v := block.Attr("name"); v != nil {
				tb.Name, _ = v.GoString()
				v.Close()
			}
			if v := block.Attr("input"); v != nil {
				m, err := v.GoMap()
				if err == nil {
					tb.Input = m
				}
				v.Close()
			}
			blocks = append(blocks, tb)
		case "ToolResultBlock":
			tb := ToolResultBlock{}
			if v := block.Attr("tool_use_id"); v != nil {
				tb.ToolUseID, _ = v.GoString()
				v.Close()
			}
			if v := block.Attr("content"); v != nil {
				tb.Content = v.String()
				v.Close()
			}
			if v := block.Attr("is_error"); v != nil {
				tb.IsError, _ = v.Bool()
				v.Close()
			}
			blocks = append(blocks, tb)
		case "ThinkingBlock":
			if t := block.Attr("thinking"); t != nil {
				s, _ := t.GoString()
				blocks = append(blocks, ThinkingBlock{Thinking: s})
				t.Close()
			}
		}
		block.Close()
	}
	return blocks
}

func extractText(obj *pyffi.Object) string {
	// Try .result first (ResultMessage)
	if r := obj.Attr("result"); r != nil && !r.IsNone() {
		s, _ := r.GoString()
		r.Close()
		if s != "" {
			return s
		}
	}
	// Try .content as string (UserMessage)
	content := obj.Attr("content")
	if content != nil {
		s := content.String()
		content.Close()
		if s != "None" && s != "" {
			return s
		}
	}
	return obj.String()
}
