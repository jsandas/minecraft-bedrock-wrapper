package runner

import (
	"testing"
)

func TestCheckServerReady(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{
			name:     "Server started message",
			line:     "Server started",
			expected: true,
		},
		{
			name:     "IPv4 port message",
			line:     "Server started. IPv4 port: 19132",
			expected: true,
		},
		{
			name:     "IPv6 port message",
			line:     "IPv6 port: [::1]:19132",
			expected: true,
		},
		{
			name:     "Listening on message",
			line:     "Listening on port 19132",
			expected: true,
		},
		{
			name:     "Other message",
			line:     "Starting server...",
			expected: false,
		},
		{
			name:     "Empty message",
			line:     "",
			expected: false,
		},
		{
			name:     "Error message",
			line:     "[ERR] Connection failed",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := New("echo", "test")
			r.checkServerReady(tt.line)

			if r.IsReady() != tt.expected {
				t.Errorf("checkServerReady(%q) set ready=%v, expected %v", tt.line, r.IsReady(), tt.expected)
			}
		})
	}
}
