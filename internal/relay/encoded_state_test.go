package relay

import "testing"

func TestEncodeDecodeState(t *testing.T) {
	tests := []struct {
		name   string
		claims EncodedStateClaims
	}{
		{
			name: "basic",
			claims: EncodedStateClaims{
				Port:     52847,
				CLIState: "abc123",
				Space:    "myspace",
				Domain:   "backlog.jp",
			},
		},
		{
			name: "with project",
			claims: EncodedStateClaims{
				Port:     12345,
				CLIState: "xyz789",
				Space:    "testspace",
				Domain:   "backlog.com",
				Project:  "PROJECT1",
			},
		},
		{
			name: "empty project",
			claims: EncodedStateClaims{
				Port:     8080,
				CLIState: "state123",
				Space:    "space1",
				Domain:   "backlogtool.com",
				Project:  "",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := encodeState(tt.claims)
			if err != nil {
				t.Fatalf("encodeState failed: %v", err)
			}

			decoded, err := decodeState(encoded)
			if err != nil {
				t.Fatalf("decodeState failed: %v", err)
			}

			if decoded.Port != tt.claims.Port {
				t.Errorf("Port = %d, want %d", decoded.Port, tt.claims.Port)
			}
			if decoded.CLIState != tt.claims.CLIState {
				t.Errorf("CLIState = %s, want %s", decoded.CLIState, tt.claims.CLIState)
			}
			if decoded.Space != tt.claims.Space {
				t.Errorf("Space = %s, want %s", decoded.Space, tt.claims.Space)
			}
			if decoded.Domain != tt.claims.Domain {
				t.Errorf("Domain = %s, want %s", decoded.Domain, tt.claims.Domain)
			}
			if decoded.Project != tt.claims.Project {
				t.Errorf("Project = %s, want %s", decoded.Project, tt.claims.Project)
			}
		})
	}
}

func TestDecodeStateErrors(t *testing.T) {
	tests := []struct {
		name    string
		encoded string
	}{
		{
			name:    "invalid base64",
			encoded: "not-valid-base64!!!",
		},
		{
			name:    "invalid json",
			encoded: "bm90LWpzb24=", // "not-json" in base64
		},
		{
			name:    "missing required fields",
			encoded: "e30=", // "{}" in base64
		},
		{
			name:    "missing port",
			encoded: "eyJzIjoic3RhdGUiLCJzcCI6InNwYWNlIiwiZCI6ImRvbWFpbiJ9", // {"s":"state","sp":"space","d":"domain"}
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodeState(tt.encoded)
			if err == nil {
				t.Error("decodeState should have returned an error")
			}
		})
	}
}
