package cmdutil

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
)

// ParsePatchOps parses a JSON string into PatchOp slice.
// Accepts either a single object {"find":"...","replace":"..."} or an array of such objects.
func ParsePatchOps(jsonStr string) ([]api.PatchOp, error) {
	jsonStr = strings.TrimSpace(jsonStr)
	if jsonStr == "" {
		return nil, fmt.Errorf("empty patch JSON")
	}

	type patchJSON struct {
		Find    string `json:"find"`
		Replace string `json:"replace"`
	}

	if jsonStr[0] == '[' {
		var ops []patchJSON
		if err := json.Unmarshal([]byte(jsonStr), &ops); err != nil {
			return nil, fmt.Errorf("invalid patch JSON array: %w", err)
		}
		if len(ops) == 0 {
			return nil, fmt.Errorf("empty patch array")
		}
		result := make([]api.PatchOp, len(ops))
		for i, op := range ops {
			if op.Find == "" {
				return nil, fmt.Errorf("patch[%d]: \"find\" must not be empty", i)
			}
			result[i] = api.PatchOp{Find: op.Find, Replace: op.Replace}
		}
		return result, nil
	}

	var op patchJSON
	if err := json.Unmarshal([]byte(jsonStr), &op); err != nil {
		return nil, fmt.Errorf("invalid patch JSON: %w", err)
	}
	if op.Find == "" {
		return nil, fmt.Errorf("patch: \"find\" must not be empty")
	}
	return []api.PatchOp{{Find: op.Find, Replace: op.Replace}}, nil
}

// BuildPatchFn builds a combined patchFn from patch operations, append, prepend,
// and optional safe full replacement content.
func BuildPatchFn(patchOps []api.PatchOp, prependText, appendText, fullReplace string) (func(string) (string, error), error) {
	var fns []func(string) (string, error)

	if len(patchOps) > 0 {
		fns = append(fns, api.PatchFnReplace(patchOps))
	}
	if prependText != "" {
		fns = append(fns, api.PatchFnPrepend(prependText))
	}
	if appendText != "" {
		fns = append(fns, api.PatchFnAppend(appendText))
	}
	if fullReplace != "" {
		fns = append(fns, api.PatchFnFullReplace(fullReplace))
	}

	if len(fns) == 0 {
		return nil, fmt.Errorf("no patch operations specified")
	}

	return func(current string) (string, error) {
		result := current
		for _, fn := range fns {
			var err error
			result, err = fn(result)
			if err != nil {
				return "", err
			}
		}
		return result, nil
	}, nil
}
