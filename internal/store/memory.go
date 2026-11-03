package store

import (
	"errors"
	"os"
	"path/filepath"
)

const agentMemoryName = "agent-memory.md"

// AgentMemoryPath is the file the Agent's cross-chat notes live in.
func AgentMemoryPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, agentMemoryName), nil
}

// LoadAgentMemory reads the Agent's notes; a missing file is no notes.
func LoadAgentMemory() (string, error) {
	path, err := AgentMemoryPath()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// SaveAgentMemory replaces the Agent's notes.
func SaveAgentMemory(content string) error {
	path, err := AgentMemoryPath()
	if err != nil {
		return err
	}
	return writeFileAtomic(path, []byte(content))
}
