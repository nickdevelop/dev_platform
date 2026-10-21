package deepseek

import (
	"bytes"
	"testing"
)

func TestParsePPID(t *testing.T) {
	tests := []struct {
		name     string
		stat     string
		wantPPID int
	}{
		{
			name:     "standard proc stat",
			stat:     "1234 (node) S 1 1234 1234 0 -1 4194304 ...",
			wantPPID: 1,
		},
		{
			name:     "comm with spaces and parentheses",
			stat:     "5678 (node (worker) dsh) S 999 5678 5678 ...",
			wantPPID: 999,
		},
		{
			name:     "malformed stat",
			stat:     "invalid",
			wantPPID: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePPID([]byte(tt.stat))
			if got != tt.wantPPID {
				t.Errorf("parsePPID() = %d, want %d", got, tt.wantPPID)
			}
		})
	}
}

func TestIsDshWebProcess(t *testing.T) {
	tests := []struct {
		name    string
		cmdline []string
		want    bool
	}{
		{
			name:    "dsh web process",
			cmdline: []string{"node", "/usr/local/bin/dsh", "--profile", "web", "--port", "0", "--no-open"},
			want:    true,
		},
		{
			name:    "dsh web process with equal sign",
			cmdline: []string{"dsh", "--profile=web", "--port", "0"},
			want:    true,
		},
		{
			name:    "other node process",
			cmdline: []string{"node", "/app/server.js", "--port", "3000"},
			want:    false,
		},
		{
			name:    "dsh non-web process",
			cmdline: []string{"dsh", "--version"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			for _, arg := range tt.cmdline {
				buf.WriteString(arg)
				buf.WriteByte(0)
			}
			got := isDshWebProcess(buf.Bytes())
			if got != tt.want {
				t.Errorf("isDshWebProcess() = %v, want %v", got, tt.want)
			}
		})
	}
}
