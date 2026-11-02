package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"sshgo/internal/agent"
)

// ChatFile is the Agent's conversations as they are saved.
type ChatFile struct {
	Current string        `json:"current,omitempty"`
	Chats   []*agent.Chat `json:"chats,omitempty"`
}

// ChatsPath is the file the Agent's conversations are saved in.
func ChatsPath() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "chats.json"), nil
}

// LoadChats reads the Agent's conversations, and gives an empty set when
// the file is not there yet.
func LoadChats() (ChatFile, error) {
	path, err := ChatsPath()
	if err != nil {
		return ChatFile{}, err
	}
	return loadChatsFrom(path)
}

func loadChatsFrom(path string) (ChatFile, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ChatFile{}, nil
	}
	if err != nil {
		return ChatFile{}, err
	}
	var f ChatFile
	if err := json.Unmarshal(b, &f); err != nil {
		return ChatFile{}, err
	}
	return f, nil
}

// Save writes the conversations.
func (f ChatFile) Save() error {
	path, err := ChatsPath()
	if err != nil {
		return err
	}
	return f.saveTo(path)
}

func (f ChatFile) saveTo(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if f.Chats == nil {
		f.Chats = []*agent.Chat{}
	}
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}
