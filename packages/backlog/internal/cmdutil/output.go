package cmdutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"

	"github.com/cli/go-gh/v2/pkg/jq"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

// JSONOutputOptions holds options for JSON output with optional jq filtering.
type JSONOutputOptions struct {
	Fields   []string // Fields to include (empty = all fields)
	JQFilter string   // jq filter expression
	Template string   // Go template expression (e.g. "{{.summary}}")
	Pretty   bool     // Pretty-print output
}

// OutputJSON outputs data as JSON with optional field selection and jq filtering.
func OutputJSON(w io.Writer, data any, opts JSONOutputOptions) error {
	// Convert to JSON bytes first
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	// If fields are specified, filter to only those fields
	if len(opts.Fields) > 0 {
		jsonBytes, err = filterFields(jsonBytes, opts.Fields)
		if err != nil {
			return fmt.Errorf("failed to filter fields: %w", err)
		}
	}

	// Apply Go template if specified
	if opts.Template != "" {
		return applyGoTemplate(w, jsonBytes, opts.Template)
	}

	// Apply jq filter if specified
	if opts.JQFilter != "" {
		return applyJQFilter(w, jsonBytes, opts.JQFilter, opts.Pretty)
	}

	// Output JSON
	if opts.Pretty {
		var buf bytes.Buffer
		if err := json.Indent(&buf, jsonBytes, "", "  "); err != nil {
			return fmt.Errorf("failed to indent JSON: %w", err)
		}
		_, err = buf.WriteTo(w)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w)
		return err
	}

	_, err = w.Write(jsonBytes)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(w)
	return err
}

// applyGoTemplate applies a Go template to JSON data.
func applyGoTemplate(w io.Writer, jsonBytes []byte, tmplStr string) error {
	var data any
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return fmt.Errorf("failed to parse JSON for template: %w", err)
	}

	tmpl, err := template.New("format").Parse(tmplStr)
	if err != nil {
		return fmt.Errorf("invalid template: %w", err)
	}

	if err := tmpl.Execute(w, data); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}
	_, err = fmt.Fprintln(w)
	return err
}

// applyJQFilter applies a jq filter to JSON data.
func applyJQFilter(w io.Writer, jsonBytes []byte, filter string, colorize bool) error {
	input := bytes.NewReader(jsonBytes)
	useColor := colorize && ui.IsColorEnabled()
	return jq.EvaluateFormatted(input, w, filter, "  ", useColor)
}

// filterFields filters JSON to only include specified fields.
func filterFields(jsonBytes []byte, fields []string) ([]byte, error) {
	// Parse JSON
	var data any
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		return nil, err
	}

	// Build field set for quick lookup
	fieldSet := make(map[string]bool)
	for _, f := range fields {
		fieldSet[strings.ToLower(f)] = true
	}

	// Filter based on data type
	filtered := filterValue(data, fieldSet)
	return json.Marshal(filtered)
}

// filterValue recursively filters a value to only include specified fields.
func filterValue(data any, fieldSet map[string]bool) any {
	switch v := data.(type) {
	case []any:
		// Array: filter each element
		result := make([]any, len(v))
		for i, elem := range v {
			result[i] = filterValue(elem, fieldSet)
		}
		return result
	case map[string]any:
		// Object: filter to only specified fields
		result := make(map[string]any)
		for key, val := range v {
			if fieldSet[strings.ToLower(key)] {
				result[key] = val
			}
		}
		return result
	default:
		return v
	}
}

// AvailableIssueFields returns the list of available fields for issue JSON output.
func AvailableIssueFields() []string {
	return []string{
		"id", "issueKey", "keyId", "projectId",
		"issueType", "summary", "description",
		"resolution", "priority", "status",
		"assignee", "category", "versions", "milestone",
		"startDate", "dueDate", "estimatedHours", "actualHours",
		"parentIssueId", "createdUser", "created", "updatedUser", "updated",
		"customFields", "attachments", "sharedFiles", "stars",
	}
}

// OutputJSONToStdout is a convenience function that outputs to stdout.
func OutputJSONToStdout(data any, opts JSONOutputOptions) error {
	return OutputJSON(os.Stdout, data, opts)
}

// OutputJSONFromProfile outputs JSON using profile settings for fields and jq filter.
// This is a convenience function that extracts JSONFields, JQ, and Template from the profile.
func OutputJSONFromProfile(data any, jsonFields, jqFilter string, templateStr ...string) error {
	opts := JSONOutputOptions{Pretty: true}
	if jsonFields != "" {
		opts.Fields = strings.Split(jsonFields, ",")
	}
	if len(templateStr) > 0 && templateStr[0] != "" {
		opts.Template = templateStr[0]
	} else if jqFilter != "" {
		opts.JQFilter = jqFilter
	}
	return OutputJSONToStdout(data, opts)
}
