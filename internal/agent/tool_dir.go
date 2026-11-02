package agent

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"sshgo/internal/llm"
)

// MaxDirEntries is how many entries one directory listing returns.
const MaxDirEntries = 500

// ListDirArgs is what list_dir was called with.
type ListDirArgs struct {
	Path        string
	Description string
}

// ParseListDirArgs reads a list_dir call.
func ParseListDirArgs(raw string) (ListDirArgs, error) {
	m, err := parseArguments(raw)
	if err != nil {
		return ListDirArgs{}, err
	}
	path, err := absolutePath(m)
	if err != nil {
		return ListDirArgs{}, err
	}
	return ListDirArgs{Path: path, Description: stringArg(m, "description")}, nil
}

func listDirTool() Tool {
	return Tool{
		Definition: llm.Tool{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "list_dir",
				Description: "List the entries of a directory at an absolute path on the host this conversation is about. At most 500 entries are returned.",
				Parameters: map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"path", "description"},
					"properties": map[string]any{
						"path": map[string]any{
							"type":        "string",
							"description": "The absolute path of the directory to list.",
						},
						"description": map[string]any{
							"type":        "string",
							"description": "Why the directory is needed.",
						},
					},
				},
			},
		},
		Prepare: func(raw string) (Prepared, error) {
			args, err := ParseListDirArgs(raw)
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
					res := ToolResult{Path: args.Path}
					if env.ListDir == nil {
						res.Error = "This app cannot list directories."
						return res, nil
					}
					listing, err := env.ListDir(ctx, args.Path, MaxDirEntries)
					if err != nil {
						if ctx.Err() != nil {
							return res, ctx.Err()
						}
						res.Error = err.Error()
						return res, nil
					}
					sort.SliceStable(listing.Entries, func(i, j int) bool {
						a, b := listing.Entries[i], listing.Entries[j]
						if a.IsDir != b.IsDir {
							return a.IsDir
						}
						return a.Name < b.Name
					})
					var b strings.Builder
					for _, entry := range listing.Entries {
						name := entry.Name
						if entry.IsDir {
							name += "/"
						}
						mode := entry.Mode
						if mode == "" {
							mode = "----------"
							if entry.IsDir {
								mode = "d---------"
							}
						}
						size := humanSize(entry.Size)
						if entry.IsDir {
							size = "-"
						}
						fmt.Fprintf(&b, "%-10s %8s  %s", mode, size, name)
						if !entry.Modified.IsZero() {
							fmt.Fprintf(&b, "  %s", entry.Modified.Format("2006-01-02 15:04"))
						}
						b.WriteString("\n")
					}
					res.Content = b.String()
					res.Summary = fmt.Sprintf("Listed %d entries.", len(listing.Entries))
					res.Truncated = listing.Truncated
					if listing.Truncated {
						res.Summary += " More entries were left out."
					}
					return res, ctx.Err()
				},
				Format: formatListDirResult,
			}, nil
		},
	}
}

func formatListDirResult(host Host, res ToolResult) string {
	var b strings.Builder
	writeHostHeader(&b, host)
	fmt.Fprintf(&b, "Path: %s\n", res.Path)
	if res.Error != "" {
		fmt.Fprintf(&b, "Error: %s\n", res.Error)
		return b.String()
	}
	b.WriteString("Entries:\n")
	b.WriteString(res.Content)
	if res.Truncated {
		fmt.Fprintf(&b, "The listing was cut to the first %d entries.\n", MaxDirEntries)
	}
	return b.String()
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
