package agent

import (
	"errors"
	"strconv"
	"strings"
)

// Policy says which read-only calls may run without asking.
//
// ReadTools covers tools that can only read; ReadCommands covers shell
// commands whose shape the local check recognises as read-only. A command
// also has to be marked safe by the model; a tool that only reads does not.
type Policy struct {
	ReadTools    bool
	ReadCommands bool
}

// Allows says whether the call may run without asking.
func (p Policy) Allows(prep Prepared) bool {
	if prep.Risk != RiskReadOnly {
		return false
	}
	switch prep.Auto {
	case AutoReadTool:
		return p.ReadTools
	case AutoReadCommand:
		return p.ReadCommands && prep.ModelSafe
	}
	return false
}

func stringArg(m map[string]any, names ...string) string {
	for _, name := range names {
		if v, ok := m[name]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
}

func boolArg(m map[string]any, names ...string) bool {
	for _, name := range names {
		if v, ok := m[name]; ok {
			switch t := v.(type) {
			case bool:
				return t
			case string:
				b, _ := strconv.ParseBool(strings.ToLower(strings.TrimSpace(t)))
				return b
			case float64:
				return t != 0
			}
		}
	}
	return false
}

// contentArg reads a required string that may be empty, such as a file's
// new content; an absent one is an error, an empty one is not.
func contentArg(m map[string]any) (string, bool) {
	v, ok := m["content"]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// absolutePath reads and checks a remote path the model named.
func absolutePath(m map[string]any) (string, error) {
	path := stringArg(m, "path")
	if path == "" {
		return "", errors.New("the call has no path")
	}
	if !strings.HasPrefix(path, "/") {
		return "", errors.New("the path must be absolute")
	}
	return path, nil
}
