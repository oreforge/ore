package software

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSpec(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		input       string
		wantName    string
		wantVersion string
		wantErr     string
	}{
		{
			name:        "valid",
			input:       "paper:1.21",
			wantName:    "paper",
			wantVersion: "1.21",
		},
		{
			name:        "valid_with_latest",
			input:       "my-software:latest",
			wantName:    "my-software",
			wantVersion: "latest",
		},
		{name: "missing_colon", input: "paper", wantErr: "invalid software spec"},
		{name: "empty_name", input: ":1.21", wantErr: "invalid software spec"},
		{name: "empty_version", input: "paper:", wantErr: "invalid software spec"},
		{name: "empty_string", input: "", wantErr: "invalid software spec"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			name, version, err := ParseSpec(tt.input)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, name)
			assert.Equal(t, tt.wantVersion, version)
		})
	}
}

func TestLookupUnknown(t *testing.T) {
	t.Parallel()

	_, err := lookup("nonexistent_provider_xyz")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnknownSoftware)
}
