package oasdk

import (
	"encoding/json"

	"github.com/i2y/pyffi"
)

// RunResult contains the output of an agent run.
// All fields are eagerly extracted; RunResult does not require Close().
type RunResult struct {
	finalOutput   string
	lastAgent     string
	items         []RunItem
	inputListJSON string // JSON for multi-turn
}

// FinalOutput returns the final text output of the run.
func (r *RunResult) FinalOutput() string { return r.finalOutput }

// LastAgent returns the name of the last agent that produced output.
func (r *RunResult) LastAgent() string { return r.lastAgent }

// Items returns the run items (messages, tool calls, tool results, etc.).
func (r *RunResult) Items() []RunItem { return r.items }

// RunItem represents a single item in the agent run (message, tool call, etc.).
type RunItem struct {
	Type  string         // e.g. "MessageOutputItem", "ToolCallItem", "ToolCallOutputItem", "HandoffOutputItem"
	Agent string         // agent name
	Raw   map[string]any // all fields
}

// extractRunResult extracts a RunResult from a Python RunResult object.
func extractRunResult(rt *pyffi.Runtime, obj *pyffi.Object) *RunResult {
	defer obj.Close()
	result := &RunResult{}

	// final_output
	if fo := obj.Attr("final_output"); fo != nil {
		if !fo.IsNone() {
			result.finalOutput = fo.String()
		}
		fo.Close()
	}

	// last_agent.name
	if la := obj.Attr("last_agent"); la != nil {
		if n := la.Attr("name"); n != nil {
			result.lastAgent, _ = n.GoString()
			n.Close()
		}
		la.Close()
	}

	// to_input_list() — serialize to JSON for multi-turn
	result.inputListJSON = extractInputListJSON(rt, obj)

	// new_items
	result.items = extractRunItems(obj)

	return result
}

func extractInputListJSON(rt *pyffi.Runtime, resultObj *pyffi.Object) string {
	toInputList := resultObj.Attr("to_input_list")
	if toInputList == nil {
		return "[]"
	}
	defer toInputList.Close()

	listObj, err := toInputList.Call()
	if err != nil || listObj == nil {
		return "[]"
	}
	defer listObj.Close()

	// Serialize via json.dumps on the Python side
	jsonMod, err := rt.Import("json")
	if err != nil {
		return "[]"
	}
	defer jsonMod.Close()

	dumpsFn := jsonMod.Attr("dumps")
	if dumpsFn == nil {
		return "[]"
	}
	defer dumpsFn.Close()

	jsonObj, err := dumpsFn.Call(listObj, pyffi.KW{"default": "str"})
	if err != nil || jsonObj == nil {
		return "[]"
	}
	defer jsonObj.Close()

	s, _ := jsonObj.GoString()
	return s
}

func extractRunItems(resultObj *pyffi.Object) []RunItem {
	items := resultObj.Attr("new_items")
	if items == nil {
		return nil
	}
	defer items.Close()

	n, _ := items.Len()
	if n <= 0 {
		return nil
	}

	var result []RunItem
	for i := int64(0); i < n; i++ {
		item, err := items.GetItem(int(i))
		if err != nil || item == nil {
			continue
		}

		ri := RunItem{}

		// type from class name
		if cls := item.Attr("__class__"); cls != nil {
			if nameObj := cls.Attr("__name__"); nameObj != nil {
				ri.Type, _ = nameObj.GoString()
				nameObj.Close()
			}
			cls.Close()
		}

		// agent name
		if a := item.Attr("agent"); a != nil && !a.IsNone() {
			if nameObj := a.Attr("name"); nameObj != nil {
				ri.Agent, _ = nameObj.GoString()
				nameObj.Close()
			}
			a.Close()
		}

		// raw as string repr (lightweight, avoids complex object walking)
		ri.Raw = map[string]any{"repr": item.String()}

		item.Close()
		result = append(result, ri)
	}

	return result
}

// InputListJSON returns the JSON-encoded input list for multi-turn use.
// This is used internally by WithPreviousResult.
func (r *RunResult) InputListJSON() string { return r.inputListJSON }

// InputList returns the input list as a slice of maps.
func (r *RunResult) InputList() []map[string]any {
	var list []map[string]any
	json.Unmarshal([]byte(r.inputListJSON), &list)
	return list
}
