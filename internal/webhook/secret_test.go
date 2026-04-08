package webhook

import (
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeriveSecret(t *testing.T) {
	t.Parallel()

	t.Run("deterministic", func(t *testing.T) {
		t.Parallel()
		s1 := DeriveSecret("token", "project")
		s2 := DeriveSecret("token", "project")
		assert.Equal(t, s1, s2)
	})

	t.Run("valid_hex_64_chars", func(t *testing.T) {
		t.Parallel()
		s := DeriveSecret("token", "project")
		assert.Len(t, s, 64)
		_, err := hex.DecodeString(s)
		require.NoError(t, err)
	})

	t.Run("different_token", func(t *testing.T) {
		t.Parallel()
		s1 := DeriveSecret("token1", "project")
		s2 := DeriveSecret("token2", "project")
		assert.NotEqual(t, s1, s2)
	})

	t.Run("different_project", func(t *testing.T) {
		t.Parallel()
		s1 := DeriveSecret("token", "project1")
		s2 := DeriveSecret("token", "project2")
		assert.NotEqual(t, s1, s2)
	})
}

func TestValidateSecret(t *testing.T) {
	t.Parallel()

	t.Run("correct", func(t *testing.T) {
		t.Parallel()
		secret := DeriveSecret("token", "project")
		assert.True(t, ValidateSecret("token", "project", secret))
	})

	t.Run("wrong_secret", func(t *testing.T) {
		t.Parallel()
		assert.False(t, ValidateSecret("token", "project", "wrong"))
	})

	t.Run("empty_inputs", func(t *testing.T) {
		t.Parallel()
		s := DeriveSecret("", "")
		assert.True(t, ValidateSecret("", "", s))
	})
}
