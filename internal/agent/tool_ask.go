package agent

import (
	"context"
	"fmt"
	"strings"

	"sshgo/internal/llm"
)

// MaxAskQuestions is how many questions one ask_user call may ask.
const MaxAskQuestions = 4

// AskArgs is what ask_user was called with.
type AskArgs struct {
	Questions []AskQuestion
}

// ParseAskArgs reads an ask_user call.
func ParseAskArgs(raw string) (AskArgs, error) {
	m, err := parseArguments(raw)
	if err != nil {
		return AskArgs{}, err
	}
	items, ok := m["questions"].([]any)
	if !ok || len(items) == 0 {
		return AskArgs{}, fmt.Errorf("the call has no questions")
	}
	if len(items) > MaxAskQuestions {
		return AskArgs{}, fmt.Errorf("at most %d questions are allowed", MaxAskQuestions)
	}
	questions := make([]AskQuestion, 0, len(items))
	seen := map[string]bool{}
	for i, item := range items {
		fields, ok := item.(map[string]any)
		if !ok {
			return AskArgs{}, fmt.Errorf("question %d is not an object", i+1)
		}
		id := stringArg(fields, "id")
		if id == "" {
			id = fmt.Sprintf("q%d", i+1)
		}
		if seen[id] {
			return AskArgs{}, fmt.Errorf("question id %q is repeated", id)
		}
		seen[id] = true
		question := stringArg(fields, "question")
		if question == "" {
			return AskArgs{}, fmt.Errorf("question %d has no text", i+1)
		}
		options, err := parseAskOptions(fields["options"])
		if err != nil {
			return AskArgs{}, fmt.Errorf("question %d: %w", i+1, err)
		}
		secret := boolArg(fields, "secret")
		multiple := boolArg(fields, "multiple")
		if secret && len(options) > 0 {
			return AskArgs{}, fmt.Errorf("question %d is secret and cannot have options", i+1)
		}
		if multiple && len(options) == 0 {
			return AskArgs{}, fmt.Errorf("question %d allows multiple answers but has no options", i+1)
		}
		questions = append(questions, AskQuestion{
			ID:          id,
			Question:    question,
			Detail:      stringArg(fields, "detail"),
			Options:     options,
			Multiple:    multiple,
			Secret:      secret,
			Placeholder: stringArg(fields, "placeholder"),
		})
	}
	return AskArgs{Questions: questions}, nil
}

func parseAskOptions(value any) ([]AskOption, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("options must be an array")
	}
	if len(items) > 20 {
		return nil, fmt.Errorf("too many options")
	}
	out := make([]AskOption, 0, len(items))
	seen := map[string]bool{}
	for i, item := range items {
		var option AskOption
		switch fields := item.(type) {
		case string:
			option.Label = strings.TrimSpace(fields)
			option.Value = option.Label
		case map[string]any:
			option.Label = stringArg(fields, "label")
			option.Value = stringArg(fields, "value")
			option.Description = stringArg(fields, "description")
			if option.Value == "" {
				option.Value = option.Label
			}
		default:
			return nil, fmt.Errorf("option %d is not a string or object", i+1)
		}
		if option.Label == "" {
			return nil, fmt.Errorf("option %d has no label", i+1)
		}
		if seen[option.Value] {
			return nil, fmt.Errorf("option value %q is repeated", option.Value)
		}
		seen[option.Value] = true
		out = append(out, option)
	}
	return out, nil
}

func askUserTool() Tool {
	return Tool{
		Definition: llm.Tool{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "ask_user",
				Description: "Ask the user up to four questions when you need a decision or information tools cannot get. A question may offer choices, allow multiple answers, or ask for text; a secret answer comes back as a handle and is never stored in the chat.",
				Parameters: map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"questions"},
					"properties": map[string]any{
						"questions": map[string]any{
							"type":     "array",
							"minItems": 1,
							"maxItems": MaxAskQuestions,
							"items": map[string]any{
								"type":                 "object",
								"additionalProperties": false,
								"required":             []string{"question"},
								"properties": map[string]any{
									"id": map[string]any{
										"type":        "string",
										"description": "A short id for this question; q1, q2... when empty.",
									},
									"question": map[string]any{
										"type":        "string",
										"description": "The question to show the user.",
									},
									"detail": map[string]any{
										"type":        "string",
										"description": "Optional context under the question.",
									},
									"options": map[string]any{
										"type":        "array",
										"description": "Choices. Each is a label, or an object with label, value and description.",
										"items": map[string]any{
											"type":                 "object",
											"additionalProperties": false,
											"required":             []string{"label"},
											"properties": map[string]any{
												"label":       map[string]any{"type": "string"},
												"value":       map[string]any{"type": "string"},
												"description": map[string]any{"type": "string"},
											},
										},
									},
									"multiple": map[string]any{
										"type":        "boolean",
										"description": "Let the user choose more than one option.",
									},
									"secret": map[string]any{
										"type":        "boolean",
										"description": "Ask for a secret in a masked field. The answer becomes a handle.",
									},
									"placeholder": map[string]any{
										"type":        "string",
										"description": "Placeholder for a text or secret answer.",
									},
								},
							},
						},
					},
				},
			},
		},
		Prepare: func(raw string) (Prepared, error) {
			args, err := ParseAskArgs(raw)
			if err != nil {
				return Prepared{}, err
			}
			return Prepared{
				Summary: args.Questions[0].Question,
				Content: AskQuestionsText(args.Questions),
				Risk:    RiskReadOnly,
				Direct:  true,
				Run: func(ctx context.Context, env Env) (ToolResult, error) {
					if env.Ask == nil {
						return ToolResult{Error: "This app cannot ask the user."}, nil
					}
					result, err := env.Ask(ctx, AskRequest{Questions: args.Questions})
					if err != nil {
						if ctx.Err() != nil {
							return ToolResult{}, ctx.Err()
						}
						return ToolResult{Error: err.Error()}, nil
					}
					if result.Cancelled {
						return ToolResult{Summary: "The user cancelled."}, nil
					}
					res := ToolResult{Content: formatAskAnswers(args.Questions, result.Answers)}
					res.Summary = fmt.Sprintf("Answered %d questions.", len(result.Answers))
					return res, ctx.Err()
				},
				Format: formatAskResult,
			}, nil
		},
	}
}

// AskQuestionsText is the questions as the approval card and tool preview
// show them.
func AskQuestionsText(questions []AskQuestion) string {
	var b strings.Builder
	for i, question := range questions {
		fmt.Fprintf(&b, "%d. %s\n", i+1, question.Question)
		if question.Detail != "" {
			fmt.Fprintf(&b, "   %s\n", question.Detail)
		}
		for _, option := range question.Options {
			fmt.Fprintf(&b, "   · %s\n", option.Label)
		}
	}
	return strings.TrimSpace(b.String())
}

func formatAskAnswers(questions []AskQuestion, answers []AskAnswer) string {
	labels := make(map[string]string, len(questions))
	for _, question := range questions {
		labels[question.ID] = question.Question
	}
	var b strings.Builder
	for _, answer := range answers {
		label := labels[answer.ID]
		if label == "" {
			label = answer.ID
		}
		values := answer.Values
		if answer.Text != "" {
			values = append(values, answer.Text)
		}
		fmt.Fprintf(&b, "- %s: %s\n", label, strings.Join(values, ", "))
	}
	return strings.TrimSpace(b.String())
}

func formatAskResult(host Host, res ToolResult) string {
	var b strings.Builder
	writeHostHeader(&b, host)
	if res.Error != "" {
		fmt.Fprintf(&b, "Error: %s\n", res.Error)
		return b.String()
	}
	b.WriteString(res.Summary)
	b.WriteString("\n")
	if res.Content != "" {
		b.WriteString("Answers:\n")
		b.WriteString(res.Content)
		b.WriteString("\n")
	}
	return b.String()
}
