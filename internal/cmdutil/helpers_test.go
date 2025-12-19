package cmdutil

import "testing"

func TestParseIssueKey(t *testing.T) {
	tests := []struct {
		name             string
		issueKey         string
		wantProjectKey   string
		wantIssueNumber  string
		wantHasProject   bool
	}{
		{
			name:            "full key",
			issueKey:        "PROJ-123",
			wantProjectKey:  "PROJ",
			wantIssueNumber: "123",
			wantHasProject:  true,
		},
		{
			name:            "number only",
			issueKey:        "123",
			wantProjectKey:  "",
			wantIssueNumber: "123",
			wantHasProject:  false,
		},
		{
			name:            "project with hyphen",
			issueKey:        "MY-PROJECT-456",
			wantProjectKey:  "MY-PROJECT",
			wantIssueNumber: "456",
			wantHasProject:  true,
		},
		{
			name:            "lowercase project",
			issueKey:        "proj-789",
			wantProjectKey:  "proj",
			wantIssueNumber: "789",
			wantHasProject:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectKey, issueNumber, hasProject := ParseIssueKey(tt.issueKey)
			if projectKey != tt.wantProjectKey {
				t.Errorf("ParseIssueKey(%q) projectKey = %q, want %q", tt.issueKey, projectKey, tt.wantProjectKey)
			}
			if issueNumber != tt.wantIssueNumber {
				t.Errorf("ParseIssueKey(%q) issueNumber = %q, want %q", tt.issueKey, issueNumber, tt.wantIssueNumber)
			}
			if hasProject != tt.wantHasProject {
				t.Errorf("ParseIssueKey(%q) hasProject = %v, want %v", tt.issueKey, hasProject, tt.wantHasProject)
			}
		})
	}
}

func TestResolveIssueKey(t *testing.T) {
	tests := []struct {
		name           string
		issueKey       string
		configProject  string
		wantResolved   string
		wantProjectKey string
	}{
		{
			name:           "full key without config",
			issueKey:       "PROJ-123",
			configProject:  "",
			wantResolved:   "PROJ-123",
			wantProjectKey: "PROJ",
		},
		{
			name:           "full key with different config",
			issueKey:       "PROJ-123",
			configProject:  "OTHER",
			wantResolved:   "PROJ-123",
			wantProjectKey: "PROJ",
		},
		{
			name:           "number only with config",
			issueKey:       "123",
			configProject:  "PROJ",
			wantResolved:   "PROJ-123",
			wantProjectKey: "PROJ",
		},
		{
			name:           "number only without config",
			issueKey:       "123",
			configProject:  "",
			wantResolved:   "123",
			wantProjectKey: "",
		},
		{
			name:           "project key with hyphen",
			issueKey:       "MY-PROJECT-456",
			configProject:  "",
			wantResolved:   "MY-PROJECT-456",
			wantProjectKey: "MY-PROJECT",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolved, projectKey := ResolveIssueKey(tt.issueKey, tt.configProject)
			if resolved != tt.wantResolved {
				t.Errorf("ResolveIssueKey(%q, %q) resolved = %q, want %q", tt.issueKey, tt.configProject, resolved, tt.wantResolved)
			}
			if projectKey != tt.wantProjectKey {
				t.Errorf("ResolveIssueKey(%q, %q) projectKey = %q, want %q", tt.issueKey, tt.configProject, projectKey, tt.wantProjectKey)
			}
		})
	}
}
