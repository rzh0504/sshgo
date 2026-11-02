package sshx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/pkg/sftp"
)

// FileData is a file read, cut to the caller's limit. Size is the file's
// size on the server, which may be larger than Data.
type FileData struct {
	Data      []byte
	Size      int64
	Truncated bool
}

// DirEntry is one entry of a directory.
type DirEntry struct {
	Name     string
	IsDir    bool
	Size     int64
	Mode     string
	Modified time.Time
}

// DirListing is a directory read, cut to the caller's limit.
type DirListing struct {
	Entries   []DirEntry
	Truncated bool
}

// ReadFile reads at most max bytes of path. It leaves one byte of margin
// when reading, so a file of exactly max bytes is not called truncated.
func (s *Session) ReadFile(ctx context.Context, filePath string, max int64) (FileData, error) {
	if max <= 0 {
		max = 128 << 10
	}
	if s.client == nil {
		return FileData{}, errors.New("sshx: session closed")
	}
	if err := ctx.Err(); err != nil {
		return FileData{}, err
	}
	client, err := sftp.NewClient(s.client)
	if err != nil {
		return FileData{}, err
	}
	type result struct {
		file FileData
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		file, err := readFile(client, filePath, max)
		ch <- result{file: file, err: err}
	}()
	select {
	case <-ctx.Done():
		client.Close()
		return FileData{}, ctx.Err()
	case r := <-ch:
		client.Close()
		return r.file, r.err
	}
}

func readFile(client *sftp.Client, filePath string, max int64) (FileData, error) {
	info, err := client.Stat(filePath)
	if err != nil {
		return FileData{}, fileError(filePath, err)
	}
	if info.IsDir() {
		return FileData{}, fmt.Errorf("%s is a directory, not a file", filePath)
	}
	file, err := client.Open(filePath)
	if err != nil {
		return FileData{}, fileError(filePath, err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, max+1))
	if err != nil {
		return FileData{}, fileError(filePath, err)
	}
	truncated := int64(len(data)) > max
	if truncated {
		data = data[:max]
	}
	return FileData{Data: data, Size: info.Size(), Truncated: truncated}, nil
}

// WriteFile replaces path with data. It writes beside the target and
// renames onto it, so a write that fails halfway leaves the old file.
func (s *Session) WriteFile(ctx context.Context, filePath string, data []byte) error {
	if s.client == nil {
		return errors.New("sshx: session closed")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	client, err := sftp.NewClient(s.client)
	if err != nil {
		return err
	}
	ch := make(chan error, 1)
	go func() {
		ch <- writeFile(client, filePath, data)
	}()
	select {
	case <-ctx.Done():
		client.Close()
		return ctx.Err()
	case err := <-ch:
		client.Close()
		return err
	}
}

func writeFile(client *sftp.Client, filePath string, data []byte) error {
	info, statErr := client.Stat(filePath)
	if statErr == nil && info.IsDir() {
		return fmt.Errorf("%s is a directory, not a file", filePath)
	}
	temp := path.Join(path.Dir(filePath),
		fmt.Sprintf(".%s.%d.tmp", path.Base(filePath), time.Now().UnixNano()))
	file, err := client.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC)
	if err != nil {
		return fileError(temp, err)
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		client.Remove(temp)
		return fileError(temp, err)
	}
	if err := file.Close(); err != nil {
		client.Remove(temp)
		return fileError(temp, err)
	}
	if statErr == nil {
		if err := client.Chmod(temp, info.Mode().Perm()); err != nil {
			client.Remove(temp)
			return fileError(temp, err)
		}
	}
	if err := client.PosixRename(temp, filePath); err != nil {
		// Servers without the extension cannot replace an existing file,
		// so the target is removed first; there is no atomic way there.
		if !unsupported(err) {
			client.Remove(temp)
			return fileError(filePath, err)
		}
		if rmErr := client.Remove(filePath); rmErr != nil && !noSuchFile(rmErr) {
			client.Remove(temp)
			return fileError(filePath, rmErr)
		}
		if err := client.Rename(temp, filePath); err != nil {
			client.Remove(temp)
			return fileError(filePath, err)
		}
	}
	return nil
}

// ListDir reads at most max entries of a directory, directories first.
func (s *Session) ListDir(ctx context.Context, dirPath string, max int) (DirListing, error) {
	if max <= 0 {
		max = 500
	}
	if s.client == nil {
		return DirListing{}, errors.New("sshx: session closed")
	}
	if err := ctx.Err(); err != nil {
		return DirListing{}, err
	}
	client, err := sftp.NewClient(s.client)
	if err != nil {
		return DirListing{}, err
	}
	type result struct {
		listing DirListing
		err     error
	}
	ch := make(chan result, 1)
	go func() {
		listing, err := listDir(client, dirPath, max)
		ch <- result{listing: listing, err: err}
	}()
	select {
	case <-ctx.Done():
		client.Close()
		return DirListing{}, ctx.Err()
	case r := <-ch:
		client.Close()
		return r.listing, r.err
	}
}

func listDir(client *sftp.Client, dirPath string, max int) (DirListing, error) {
	info, err := client.Stat(dirPath)
	if err != nil {
		return DirListing{}, fileError(dirPath, err)
	}
	if !info.IsDir() {
		return DirListing{}, fmt.Errorf("%s is not a directory", dirPath)
	}
	entries, err := client.ReadDir(dirPath)
	if err != nil {
		return DirListing{}, fileError(dirPath, err)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].IsDir() != entries[j].IsDir() {
			return entries[i].IsDir()
		}
		return entries[i].Name() < entries[j].Name()
	})
	truncated := len(entries) > max
	if truncated {
		entries = entries[:max]
	}
	out := DirListing{Entries: make([]DirEntry, 0, len(entries)), Truncated: truncated}
	for _, entry := range entries {
		out.Entries = append(out.Entries, DirEntry{
			Name:     entry.Name(),
			IsDir:    entry.IsDir(),
			Size:     entry.Size(),
			Mode:     entry.Mode().String(),
			Modified: entry.ModTime(),
		})
	}
	return out, nil
}

func fileError(filePath string, err error) error {
	if noSuchFile(err) {
		return fmt.Errorf("no such file: %s", filePath)
	}
	if denied(err) {
		return fmt.Errorf("permission denied: %s", filePath)
	}
	return fmt.Errorf("%s: %w", filePath, err)
}

func noSuchFile(err error) bool {
	var status *sftp.StatusError
	return errors.As(err, &status) && status.FxCode() == sftp.ErrSSHFxNoSuchFile
}

func denied(err error) bool {
	var status *sftp.StatusError
	return errors.As(err, &status) && status.FxCode() == sftp.ErrSSHFxPermissionDenied
}

func unsupported(err error) bool {
	var status *sftp.StatusError
	if errors.As(err, &status) {
		return status.FxCode() == sftp.ErrSSHFxOpUnsupported
	}
	return strings.Contains(err.Error(), "unimplemented")
}
