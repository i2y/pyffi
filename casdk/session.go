package casdk

import (
	"fmt"
	"iter"

	"github.com/i2y/pyffi"
)

// Session wraps the Python ClaudeSDKClient for bidirectional conversations.
// Create with NewSession, use Query/ReceiveMessages for interaction, and
// Close when done.
type Session struct {
	rt      *pyffi.Runtime
	wrapper *pyffi.Object // Python _SessionWrapper instance
	ownsRT  bool          // true if Session created its own Runtime
	closed  bool
}

// NewSession creates a new interactive session with the Claude Agent SDK.
// The SDK is auto-installed via uv if not present.
func NewSession(opts ...QueryOption) (*Session, error) {
	rt, err := pyffi.New(pyffi.Dependencies("claude-agent-sdk"))
	if err != nil {
		return nil, fmt.Errorf("casdk: %w", err)
	}
	s, err := newSession(rt, true, opts...)
	if err != nil {
		rt.Close()
		return nil, err
	}
	return s, nil
}

// Session creates a new interactive session using the client's existing Runtime.
func (c *Client) Session(opts ...QueryOption) (*Session, error) {
	return newSession(c.rt, false, opts...)
}

func newSession(rt *pyffi.Runtime, ownsRT bool, opts ...QueryOption) (*Session, error) {
	var cfg queryConfig
	for _, o := range opts {
		o(&cfg)
	}

	// Define the Python wrapper class.
	if err := rt.Exec(sessionWrapperCode); err != nil {
		return nil, fmt.Errorf("casdk: define session wrapper: %w", err)
	}

	// Build options code.
	client := &Client{rt: rt}
	optionsCode, err := client.buildOptionsCode(cfg)
	if err != nil {
		rt.Close()
		return nil, fmt.Errorf("casdk: build options: %w", err)
	}

	// Create the wrapper instance.
	code := fmt.Sprintf(`
%s
_pyffi_session = _SessionWrapper(_pyffi_options)
`, optionsCode)
	if err := rt.Exec(code); err != nil {
		rt.Close()
		return nil, fmt.Errorf("casdk: create session: %w", err)
	}

	mainMod, _ := rt.Import("__main__")
	defer mainMod.Close()
	wrapper := mainMod.Attr("_pyffi_session")
	if wrapper == nil {
		rt.Close()
		return nil, fmt.Errorf("casdk: session wrapper not found")
	}

	return &Session{rt: rt, wrapper: wrapper, ownsRT: ownsRT}, nil
}

// Connect establishes the connection with an optional initial prompt.
func (s *Session) Connect(prompt string) error {
	if s.closed {
		return ErrSessionClosed
	}
	var args []any
	if prompt != "" {
		args = append(args, prompt)
	}
	result, err := s.wrapper.Attr("connect").Call(args...)
	if result != nil {
		result.Close()
	}
	return err
}

// Query sends a prompt to the session.
func (s *Session) Query(prompt string) error {
	if s.closed {
		return ErrSessionClosed
	}
	fn := s.wrapper.Attr("query")
	if fn == nil {
		return fmt.Errorf("casdk: session.query not found")
	}
	defer fn.Close()
	result, err := fn.Call(prompt)
	if result != nil {
		result.Close()
	}
	return err
}

// ReceiveMessages returns an iterator over response messages from the session.
func (s *Session) ReceiveMessages() iter.Seq2[*Message, error] {
	return func(yield func(*Message, error) bool) {
		if s.closed {
			yield(nil, ErrSessionClosed)
			return
		}

		fn := s.wrapper.Attr("receive_messages")
		if fn == nil {
			yield(nil, fmt.Errorf("casdk: session.receive_messages not found"))
			return
		}
		defer fn.Close()

		resultObj, err := fn.Call()
		if err != nil {
			yield(nil, fmt.Errorf("casdk: receive_messages: %w", err))
			return
		}
		if resultObj == nil {
			return
		}
		defer resultObj.Close()

		n, _ := resultObj.Len()
		for i := int64(0); i < n; i++ {
			itemObj, err := resultObj.GetItem(int(i))
			if err != nil {
				yield(nil, err)
				return
			}
			msg := extractMessage(itemObj)
			if !yield(msg, nil) {
				msg.Close()
				return
			}
		}
	}
}

// Interrupt interrupts the current operation.
func (s *Session) Interrupt() error {
	if s.closed {
		return ErrSessionClosed
	}
	fn := s.wrapper.Attr("interrupt")
	if fn == nil {
		return fmt.Errorf("casdk: session.interrupt not found")
	}
	defer fn.Close()
	result, err := fn.Call()
	if result != nil {
		result.Close()
	}
	return err
}

// SetModel changes the model mid-session.
func (s *Session) SetModel(model string) error {
	if s.closed {
		return ErrSessionClosed
	}
	fn := s.wrapper.Attr("set_model")
	if fn == nil {
		return fmt.Errorf("casdk: session.set_model not found")
	}
	defer fn.Close()
	result, err := fn.Call(model)
	if result != nil {
		result.Close()
	}
	return err
}

// SetPermissionMode changes the permission mode mid-session.
func (s *Session) SetPermissionMode(mode string) error {
	if s.closed {
		return ErrSessionClosed
	}
	fn := s.wrapper.Attr("set_permission_mode")
	if fn == nil {
		return fmt.Errorf("casdk: session.set_permission_mode not found")
	}
	defer fn.Close()
	result, err := fn.Call(mode)
	if result != nil {
		result.Close()
	}
	return err
}

// Close disconnects the session and releases resources.
func (s *Session) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true

	fn := s.wrapper.Attr("close")
	if fn != nil {
		result, _ := fn.Call()
		if result != nil {
			result.Close()
		}
		fn.Close()
	}
	s.wrapper.Close()
	if s.ownsRT {
		return s.rt.Close()
	}
	return nil
}

const sessionWrapperCode = `
import asyncio
from claude_agent_sdk import ClaudeSDKClient

class _SessionWrapper:
    def __init__(self, options):
        self._options = options
        self._client = ClaudeSDKClient(options=options) if options else ClaudeSDKClient()
        self._loop = asyncio.new_event_loop()
        self._connected = False

    def connect(self, prompt=None):
        self._loop.run_until_complete(self._client.connect(prompt))
        self._connected = True

    def query(self, prompt):
        if not self._connected:
            self.connect()
        self._loop.run_until_complete(self._client.query(prompt))

    def receive_messages(self):
        async def _collect():
            msgs = []
            async for msg in self._client.receive_response():
                msgs.append(msg)
            return msgs
        return self._loop.run_until_complete(_collect())

    def interrupt(self):
        self._loop.run_until_complete(self._client.interrupt())

    def set_model(self, model):
        self._loop.run_until_complete(self._client.set_model(model))

    def set_permission_mode(self, mode):
        self._loop.run_until_complete(self._client.set_permission_mode(mode))

    def close(self):
        try:
            self._loop.run_until_complete(self._client.disconnect())
        except Exception:
            pass
        self._loop.close()
`
