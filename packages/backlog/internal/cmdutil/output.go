package cmdutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"text/template"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
)

// OutputJSON outputs data as JSON with optional Go template formatting.
// If format is empty, outputs pretty-printed JSON.
// If format is specified, applies Go template to the data.
//
// For single objects:
//
//	OutputJSON(project, format) // applies template to project
//
// For slices/arrays:
//
//	OutputJSON(issues, format)  // applies template to each element
func OutputJSON(data any, format string) error {
	if format == "" {
		// Pretty-print JSON
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	}

	// Apply Go template
	return outputWithTemplate(data, format)
}

// OutputJSONFromProfile outputs data using the profile's format setting.
// This is a convenience function that extracts format from the profile.
func OutputJSONFromProfile(data any, profile *config.ResolvedProfile) error {
	format := ""
	if profile != nil {
		format = profile.Format
	}
	return OutputJSON(data, format)
}

// outputWithTemplate applies a Go template to data.
// For slices, applies template to each element.
func outputWithTemplate(data any, format string) error {
	tmpl, err := template.New("output").Parse(format)
	if err != nil {
		return fmt.Errorf("invalid format template: %w", err)
	}

	// Convert to map for template access
	// This allows accessing fields with both .FieldName and .field_name
	v := reflect.ValueOf(data)

	// Handle pointer
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	// Handle slice/array - apply template to each element
	if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
		for i := 0; i < v.Len(); i++ {
			elem := v.Index(i).Interface()
			if err := executeTemplate(tmpl, elem); err != nil {
				return err
			}
		}
		return nil
	}

	// Single object
	return executeTemplate(tmpl, data)
}

// executeTemplate executes template on a single data item
func executeTemplate(tmpl *template.Template, data any) error {
	// Convert struct to map for flexible field access
	dataMap, err := toMap(data)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, dataMap); err != nil {
		return fmt.Errorf("failed to execute template: %w", err)
	}

	// Output with newline
	fmt.Println(buf.String())
	return nil
}

// toMap converts a struct to a map[string]any
// Supports both JSON field names and struct field names
func toMap(data any) (map[string]any, error) {
	// First, marshal to JSON and unmarshal to map
	// This ensures we use JSON field names
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return result, nil
}
