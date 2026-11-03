package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sshgo/internal/agent"
)

const (
	chatsDirName  = "chats"
	chatIndexName = "index.json"
)

// ChatMeta is what the history list needs of a conversation, without
// reading its messages.
type ChatMeta struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	HostID    string `json:"hostId,omitempty"`
	CreatedAt int64  `json:"createdAt,omitempty"`
	UpdatedAt int64  `json:"updatedAt,omitempty"`
}

// ChatIndex is the Agent's conversations, and the one in view.
type ChatIndex struct {
	Current string     `json:"current,omitempty"`
	Chats   []ChatMeta `json:"chats,omitempty"`
}

// LoadChatIndex reads the list of conversations.
func LoadChatIndex() (ChatIndex, error) {
	path, err := chatIndexPath()
	if err != nil {
		return ChatIndex{}, err
	}
	idx, err := loadChatIndexFrom(path)
	if errors.Is(err, os.ErrNotExist) {
		return ChatIndex{}, nil
	}
	if err != nil {
		return ChatIndex{}, err
	}
	return idx, nil
}

func loadChatIndexFrom(path string) (ChatIndex, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return ChatIndex{}, err
	}
	var idx ChatIndex
	if err := json.Unmarshal(b, &idx); err != nil {
		return ChatIndex{}, err
	}
	return idx, nil
}

// Save writes the list of conversations.
func (idx ChatIndex) Save() error {
	path, err := chatIndexPath()
	if err != nil {
		return err
	}
	return idx.saveTo(path)
}

func (idx ChatIndex) saveTo(path string) error {
	if idx.Chats == nil {
		idx.Chats = []ChatMeta{}
	}
	b, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return writeFileAtomic(path, b)
}

// LoadChat reads one conversation.
func LoadChat(id string) (*agent.Chat, error) {
	path, err := chatPath(id)
	if err != nil {
		return nil, err
	}
	return loadChatFrom(path)
}

func loadChatFrom(path string) (*agent.Chat, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c agent.Chat
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// SaveChat writes one conversation.
func SaveChat(c *agent.Chat) error {
	if c == nil {
		return errors.New("store: no chat")
	}
	path, err := chatPath(c.ID)
	if err != nil {
		return err
	}
	return saveChatTo(path, c)
}

func saveChatTo(path string, c *agent.Chat) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return writeFileAtomic(path, b)
}

// DeleteChat removes one conversation's file.
func DeleteChat(id string) error {
	path, err := chatPath(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func chatsDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, chatsDirName), nil
}

func chatIndexPath() (string, error) {
	dir, err := chatsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, chatIndexName), nil
}

func chatPath(id string) (string, error) {
	dir, err := chatsDir()
	if err != nil {
		return "", err
	}
	return chatPathIn(dir, id)
}

func chatPathIn(dir, id string) (string, error) {
	if !validChatID(id) {
		return "", fmt.Errorf("store: invalid chat id %q", id)
	}
	return filepath.Join(dir, id+".json"), nil
}

func validChatID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if !(r == '-' || r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}

func writeFileAtomic(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	fail := func(err error) error {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if _, err := f.Write(b); err != nil {
		return fail(err)
	}
	if err := f.Sync(); err != nil {
		return fail(err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

const attachmentsDirName = "attachments"

// SaveAttachment keeps an image the Agent was given and returns the id its
// conversation refers to.
func SaveAttachment(data []byte, ext string) (string, error) {
	switch ext {
	case ".png", ".jpg", ".webp", ".gif":
	default:
		return "", fmt.Errorf("store: invalid attachment extension %q", ext)
	}
	id := agent.NewID("att") + ext
	path, err := attachmentPath(id)
	if err != nil {
		return "", err
	}
	if err := writeFileAtomic(path, data); err != nil {
		return "", err
	}
	return id, nil
}

// LoadAttachment reads an image kept for the Agent.
func LoadAttachment(id string) ([]byte, error) {
	path, err := attachmentPath(id)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// AttachmentPath is where an image kept for the Agent is.
func AttachmentPath(id string) (string, error) { return attachmentPath(id) }

// DeleteAttachment removes an image kept for the Agent.
func DeleteAttachment(id string) error {
	path, err := attachmentPath(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func attachmentPath(id string) (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return attachmentPathIn(dir, id)
}

func attachmentPathIn(dir, id string) (string, error) {
	if !validAttachmentID(id) {
		return "", fmt.Errorf("store: invalid attachment id %q", id)
	}
	return filepath.Join(dir, attachmentsDirName, id), nil
}

func validAttachmentID(id string) bool {
	if id == "" || len(id) > 128 || strings.HasPrefix(id, ".") || strings.Contains(id, "..") {
		return false
	}
	for _, r := range id {
		if !(r == '-' || r == '_' || r == '.' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return false
		}
	}
	return true
}
