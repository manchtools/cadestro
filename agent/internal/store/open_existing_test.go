package store

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenExistingDoesNotCreateDatabase(t *testing.T) {
	_, err := OpenExisting(t.TempDir())
	require.ErrorContains(t, err, "does not exist")
}
