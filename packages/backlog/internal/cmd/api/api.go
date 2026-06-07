package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	internalapi "github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/cmdutil"
)

var (
	method      string
	rawFields   []string
	typedFields []string
	inputFile   string
	silent      bool
	includeResp bool
)

// APICmd is the root command for api operations
var APICmd = &cobra.Command{
	Use:   "api <endpoint>",
	Short: "Make an authenticated API request",
	Long: `Make an authenticated API request to the Backlog API.

The endpoint argument should be a full API path starting with /api/v2 (or other versions).
The path will be prefixed with "https://{space}.{domain}".

Pass one or more -F/--field values in key=value format to add parameters.
The -F flag has type conversion based on value format:
  - integers and booleans (true/false) are sent as-is
  - values starting with @ are read from a file (use @- for stdin)
  - all other values are treated as strings
Use --raw-field to always send the value as a literal string.

Examples:
  # Get space information
  backlog api /api/v2/space

  # Get issues with query parameters
  backlog api /api/v2/issues -F "projectId[]=12345" -F "count=10"

  # Create an issue with POST
  backlog api /api/v2/issues -X POST -F "projectId=12345" -F "summary=New Issue" -F "issueTypeId=1" -F "priorityId=3"

  # Read a field value from file
  backlog api /api/v2/issues -X POST -F "projectId=12345" -F "summary=Bug" -F "description=@body.txt"

  # Pass request body from stdin
  echo '{"name":"test"}' | backlog api /api/v2/projects -X POST --input -`,
	Args: cobra.ExactArgs(1),
	RunE: runAPI,
}

func init() {
	APICmd.Flags().StringVarP(&method, "method", "X", "GET", "HTTP method to use (GET, POST, PATCH, DELETE)")
	APICmd.Flags().StringArrayVar(&rawFields, "raw-field", nil, "Add a string parameter in key=value format")
	APICmd.Flags().StringArrayVarP(&typedFields, "field", "F", nil, "Add a typed parameter in key=value format (integers, booleans auto-converted; @file reads value from file)")
	APICmd.Flags().StringVar(&inputFile, "input", "", "Read request body from file (use - for stdin)")
	APICmd.Flags().BoolVarP(&silent, "silent", "s", false, "Do not print response body")
	APICmd.Flags().BoolVarP(&includeResp, "include", "i", false, "Include response headers in output")
}

func runAPI(cmd *cobra.Command, args []string) error {
	client, cfg, err := cmdutil.GetAPIClient(cmd)
	if err != nil {
		return err
	}

	profile := cfg.CurrentProfile()

	endpoint := args[0]
	if !strings.HasPrefix(endpoint, "/") {
		endpoint = "/" + endpoint
	}

	// Parse fields for query parameters or form data
	queryParams := url.Values{}
	formData := url.Values{}

	addField := func(key, value string) {
		if method == "GET" {
			queryParams.Add(key, value)
		} else {
			formData.Add(key, value)
		}
	}

	// -f/--raw-field: string values as-is
	for _, field := range rawFields {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid field format: %s (expected key=value)", field)
		}
		addField(parts[0], parts[1])
	}

	// -F/--field: type conversion (integers, booleans; @file reads from file)
	for _, field := range typedFields {
		parts := strings.SplitN(field, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid field format: %s (expected key=value)", field)
		}
		key, value := parts[0], parts[1]

		resolved, err := resolveTypedValue(value)
		if err != nil {
			return fmt.Errorf("field %s: %w", key, err)
		}
		addField(key, resolved)
	}

	// Handle input from file/stdin for request body
	var requestBody io.Reader
	if inputFile != "" {
		if inputFile == "-" {
			requestBody = os.Stdin
		} else {
			f, err := os.Open(inputFile)
			if err != nil {
				return fmt.Errorf("failed to open input file: %w", err)
			}
			defer func() { _ = f.Close() }()
			requestBody = f
		}
	}

	ctx := cmd.Context()

	// Make the request
	var resp *httpResponse
	switch strings.ToUpper(method) {
	case "GET":
		resp, err = doGetRequest(ctx, client, endpoint, queryParams)
	case "POST":
		if requestBody != nil {
			resp, err = doJSONRequest(ctx, client, "POST", endpoint, requestBody)
		} else {
			resp, err = doFormRequest(ctx, client, "POST", endpoint, formData)
		}
	case "PATCH":
		if requestBody != nil {
			resp, err = doJSONRequest(ctx, client, "PATCH", endpoint, requestBody)
		} else {
			resp, err = doFormRequest(ctx, client, "PATCH", endpoint, formData)
		}
	case "DELETE":
		resp, err = doDeleteRequest(ctx, client, endpoint)
	default:
		return fmt.Errorf("unsupported HTTP method: %s", method)
	}

	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}

	// Output response
	if includeResp {
		fmt.Printf("HTTP/1.1 %d\n", resp.StatusCode)
		for key, values := range resp.Headers {
			for _, v := range values {
				fmt.Printf("%s: %s\n", key, v)
			}
		}
		fmt.Println()
	}

	if !silent && len(resp.Body) > 0 {
		var jsonData interface{}
		isJSON := json.Unmarshal(resp.Body, &jsonData) == nil

		if isJSON && (profile.Output == "json" || profile.JQ != "" || profile.Template != "") {
			return cmdutil.OutputJSONFromProfile(jsonData, profile.JSONFields, profile.JQ, profile.Template)
		}

		if isJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			_ = enc.Encode(jsonData)
		} else {
			_, _ = os.Stdout.Write(resp.Body)
			fmt.Println()
		}
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("API returned status %d", resp.StatusCode)
	}

	return nil
}

type httpResponse struct {
	StatusCode int
	Headers    map[string][]string
	Body       []byte
}

func doGetRequest(ctx context.Context, client *internalapi.Client, path string, query url.Values) (*httpResponse, error) {
	resp, err := client.RawRequest(ctx, "GET", path, query, nil, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &httpResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       respBody,
	}, nil
}

func doFormRequest(ctx context.Context, client *internalapi.Client, method, path string, data url.Values) (*httpResponse, error) {
	resp, err := client.RawRequest(ctx, method, path, nil, strings.NewReader(data.Encode()), "application/x-www-form-urlencoded")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &httpResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       respBody,
	}, nil
}

func doJSONRequest(ctx context.Context, client *internalapi.Client, method, path string, body io.Reader) (*httpResponse, error) {
	// Read the body content
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return nil, err
	}

	// Parse JSON to ensure it's valid
	var jsonData interface{}
	if err := json.Unmarshal(bodyBytes, &jsonData); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	resp, err := client.RawRequest(ctx, method, path, nil, bytes.NewReader(bodyBytes), "application/json")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &httpResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       respBody,
	}, nil
}

func doDeleteRequest(ctx context.Context, client *internalapi.Client, path string) (*httpResponse, error) {
	resp, err := client.RawRequest(ctx, "DELETE", path, nil, nil, "")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	return &httpResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       respBody,
	}, nil
}

// resolveTypedValue applies gh-compatible type conversion for -F/--field values.
// @file reads from file, @- reads from stdin, booleans and integers pass through as-is.
func resolveTypedValue(value string) (string, error) {
	if strings.HasPrefix(value, "@") {
		path := value[1:]
		var r io.Reader
		if path == "-" {
			r = os.Stdin
		} else {
			f, err := os.Open(path)
			if err != nil {
				return "", fmt.Errorf("failed to open %s: %w", path, err)
			}
			defer func() { _ = f.Close() }()
			r = f
		}
		data, err := io.ReadAll(r)
		if err != nil {
			return "", fmt.Errorf("failed to read %s: %w", path, err)
		}
		return strings.TrimRight(string(data), "\r\n"), nil
	}

	if value == "true" || value == "false" || value == "null" {
		return value, nil
	}
	if _, err := strconv.Atoi(value); err == nil {
		return value, nil
	}
	return value, nil
}
