// OneAppFactory - Android app factory
// License: MIT
//
// Copyright (c) 2026 OneAppFactory contributors

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecurityConfig(t *testing.T) {
	t.Run("LoadNonExistent", func(t *testing.T) {
		sec, err := loadSecurityConfig("/nonexistent/.security.yml")
		require.NoError(t, err)
		assert.NotNil(t, sec)
		assert.Empty(t, sec.ModelList)
	})
}

func TestSecurityPath(t *testing.T) {
	tests := []struct {
		name      string
		configDir string
		want      string
	}{
		{
			name:      "standard path",
			configDir: "/home/user/.appfactory/config.json",
			want:      "/home/user/.appfactory/.security.yml",
		},
		{
			name:      "nested path",
			configDir: "/path/to/config/myconfig.json",
			want:      "/path/to/config/.security.yml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := securityPath(tt.configDir)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSaveAndLoadSecurityConfig(t *testing.T) {
	tmpDir := t.TempDir()
	secPath := filepath.Join(tmpDir, SecurityConfigFile)

	original := &SecurityConfig{
		ModelList: map[string]ModelSecurityEntry{
			"model1:0": {
				APIKeys: []string{"key1", "key2"},
			},
		},
	}

	// Save
	err := saveSecurityConfig(secPath, original)
	require.NoError(t, err)

	// Verify file was created with correct permissions
	info, err := os.Stat(secPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode())

	// Load
	loaded, err := loadSecurityConfig(secPath)
	require.NoError(t, err)

	assert.Equal(t, original.ModelList, loaded.ModelList)
}
