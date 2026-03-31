package envfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		expected  map[string]string
		expectErr bool
	}{
		{
			name: "basic env file",
			content: "API_KEY=secret123\nDATABASE_URL=postgres://localhost/db\nPORT=8080",
			expected: map[string]string{
				"API_KEY":      "secret123",
				"DATABASE_URL": "postgres://localhost/db",
				"PORT":         "8080",
			},
		},
		{
			name: "with comments and quotes",
			content: "# comment\nAPI_KEY=\"secret with spaces\"\nNAME='single quoted'\n",
			expected: map[string]string{
				"API_KEY": "secret with spaces",
				"NAME":    "single quoted",
			},
		},
		{
			name:      "invalid line",
			content:   "INVALID_LINE",
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), ".env")
			if err := os.WriteFile(path, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("WriteFile() error = %v", err)
			}

			got, err := Load(path)
			if tt.expectErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if len(got) != len(tt.expected) {
				t.Fatalf("len(got) = %d, want %d", len(got), len(tt.expected))
			}
			for key, want := range tt.expected {
				if got[key] != want {
					t.Fatalf("got[%q] = %q, want %q", key, got[key], want)
				}
			}
		})
	}
}

func TestLoadClosest(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	path := filepath.Join(root, ".env")
	if err := os.WriteFile(path, []byte("TEST_ENVFILE_VALUE=from-root\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	t.Setenv("TEST_ENVFILE_VALUE", "")
	if err := os.Unsetenv("TEST_ENVFILE_VALUE"); err != nil {
		t.Fatalf("Unsetenv() error = %v", err)
	}

	loadedPath, err := LoadClosest(nested, false)
	if err != nil {
		t.Fatalf("LoadClosest() error = %v", err)
	}
	if loadedPath != path {
		t.Fatalf("loadedPath = %q, want %q", loadedPath, path)
	}
	if got := os.Getenv("TEST_ENVFILE_VALUE"); got != "from-root" {
		t.Fatalf("TEST_ENVFILE_VALUE = %q, want from-root", got)
	}
}

func TestLoadClosestNotFound(t *testing.T) {
	_, err := LoadClosest(t.TempDir(), false)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("LoadClosest() err = %v, want ErrNotFound", err)
	}
}