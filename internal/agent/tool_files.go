package agent

import (
	"context"
	"fmt"
	"strings"

	"sshgo/internal/llm"
)

const (
	// MaxReadBytes is how much of a file one read returns.
	MaxReadBytes = 128 << 10
	// MaxWriteBytes is how large a file one write may replace.
	MaxWriteBytes = 512 << 10
)

// ReadFileArgs is what read_file was called with.
type ReadFileArgs struct {
	Path        string
	Description string
}

// ParseReadFileArgs reads a read_file call.
func ParseReadFileArgs(raw string) (ReadFileArgs, error) {
	m, err := parseArguments(raw)
	if err != nil {
		return ReadFileArgs{}, err
	}
	path, err := absolutePath(m)
	if err != nil {
		return ReadFileArgs{}, err
	}
	return ReadFileArgs{
		Path:        path,
		Description: stringArg(m, "description"),
	}, nil
}

// WriteFileArgs is what write_file was called with.
type WriteFileArgs struct {
	Path        string
	Content     string
	Description string
}

// ParseWriteFileArgs reads a write_file call. An empty content is a valid
// way to empty a file; a missing one is not.
func ParseWriteFileArgs(raw string) (WriteFileArgs, error) {
	m, err := parseArguments(raw)
	if err != nil {
		return WriteFileArgs{}, err
	}
	path, err := absolutePath(m)
	if err != nil {
		return WriteFileArgs{}, err
	}
	content, ok := contentArg(m)
	if !ok {
		return WriteFileArgs{}, fmt.Errorf("the call has no content")
	}
	return WriteFileArgs{
		Path:        path,
		Content:     content,
		Description: stringArg(m, "description"),
	}, nil
}

func readFileTool() Tool {
	return Tool{
		Definition: llm.Tool{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "read_file",
				Description: "Read a UTF-8 text file at an absolute path on the host this conversation is about. At most 128 KiB is read; the user reviews it unless read-only actions may run by themselves.",
				Parameters: map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"path", "description"},
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "The absolute path of the file to read.",
						},
						"description": map[string]any{
							"type":        "string",
							"description": "Why this file is needed.",
						},
					},
				},
			},
		},
		Prepare: func(raw string) (Prepared, error) {
			args, err := ParseReadFileArgs(raw)
			if err != nil {
				return Prepared{}, err
			}
			return Prepared{
				Summary:     args.Path,
				Description: args.Description,
				Path:        args.Path,
				Risk:        RiskReadOnly,
				Auto:        AutoReadTool,
				Run: func(ctx context.Context, env Env) (ToolResult, error) {
					if env.ReadFile == nil {
						return ToolResult{Path: args.Path, Error: "This app cannot read files."}, nil
					}
					file, err := env.ReadFile(ctx, args.Path, MaxReadBytes)
					res := ToolResult{
						Path:      args.Path,
						Bytes:     file.Size,
						Truncated: file.Truncated,
						Content:   string(file.Data),
					}
					if res.Bytes == 0 && len(file.Data) > 0 {
						res.Bytes = int64(len(file.Data))
					}
					if err != nil {
						if ctx.Err() != nil {
							return res, ctx.Err()
						}
						res.Error = err.Error()
						return res, nil
					}
					if res.Truncated {
						res.Summary = fmt.Sprintf("Read the first %d bytes of %s.", len(file.Data), args.Path)
					} else {
						res.Summary = fmt.Sprintf("Read %d bytes from %s.", res.Bytes, args.Path)
					}
					return res, ctx.Err()
				},
				Format: formatReadFileResult,
			}, nil
		},
	}
}

func writeFileTool() Tool {
	return Tool{
		Definition: llm.Tool{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "write_file",
				Description: "Replace a UTF-8 text file at an absolute path on the host this conversation is about. The whole file is replaced, so read it first when you can. The user reviews the content before it runs.",
				Parameters: map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"path", "content", "description"},
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "The absolute path of the file to create or replace.",
						},
						"content": map[string]any{
							"type":        "string",
							"description": "The complete UTF-8 content to write.",
						},
						"description": map[string]any{
							"type":        "string",
							"description": "What is being changed and why.",
						},
					},
				},
			},
		},
		Prepare: func(raw string) (Prepared, error) {
			args, err := ParseWriteFileArgs(raw)
			if err != nil {
				return Prepared{}, err
			}
			if len(args.Content) > MaxWriteBytes {
				return Prepared{}, fmt.Errorf("the content is larger than %d bytes", MaxWriteBytes)
			}
			return Prepared{
				Summary:     args.Path,
				Description: args.Description,
				Path:        args.Path,
				Content:     args.Content,
				Risk:        RiskCaution,
				Auto:        AutoNone,
				Run: func(ctx context.Context, env Env) (ToolResult, error) {
					res := ToolResult{Path: args.Path, Bytes: int64(len(args.Content))}
					if env.WriteFile == nil {
						res.Error = "This app cannot write files."
						return res, nil
					}
					if err := env.WriteFile(ctx, args.Path, []byte(args.Content)); err != nil {
						if ctx.Err() != nil {
							return res, ctx.Err()
						}
						res.Error = err.Error()
						return res, nil
					}
					res.Summary = fmt.Sprintf("Wrote %d bytes to %s.", res.Bytes, args.Path)
					return res, ctx.Err()
				},
				Format: formatWriteFileResult,
			}, nil
		},
	}
}

func formatReadFileResult(host Host, res ToolResult) string {
	var b strings.Builder
	writeHostHeader(&b, host)
	fmt.Fprintf(&b, "Path: %s\n", res.Path)
	fmt.Fprintf(&b, "Size: %d bytes\n", res.Bytes)
	if res.Error != "" {
		fmt.Fprintf(&b, "Error: %s\n", res.Error)
		return b.String()
	}
	b.WriteString("Content:\n")
	b.WriteString(res.Content)
	if !strings.HasSuffix(res.Content, "\n") {
		b.WriteString("\n")
	}
	if res.Truncated {
		b.WriteString("The file was truncated; only its start is shown.\n")
	}
	return b.String()
}

func formatWriteFileResult(host Host, res ToolResult) string {
	var b strings.Builder
	writeHostHeader(&b, host)
	fmt.Fprintf(&b, "Path: %s\n", res.Path)
	if res.Error != "" {
		fmt.Fprintf(&b, "Error: %s\n", res.Error)
		return b.String()
	}
	fmt.Fprintf(&b, "Wrote: %d bytes\n", res.Bytes)
	return b.String()
}
