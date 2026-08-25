package executor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDebPackageNameFromURL(t *testing.T) {
	t.Run("standard name_version_arch.deb", func(t *testing.T) {
		name, err := debPackageNameFromURL("https://repo.example.com/pool/m/myapp_1.2.3_amd64.deb")
		require.NoError(t, err)
		assert.Equal(t, "myapp", name)
	})

	t.Run("hyphenated name before first underscore is preserved", func(t *testing.T) {
		name, err := debPackageNameFromURL("https://x/y/myapp-utils_1.2.3_amd64.deb")
		require.NoError(t, err)
		assert.Equal(t, "myapp-utils", name)
	})

	t.Run("not a .deb is rejected", func(t *testing.T) {
		_, err := debPackageNameFromURL("https://x/y/myapp_1.0_amd64.rpm")
		assert.Error(t, err)
	})

	t.Run("no underscore (non-standard filename) is rejected so we don't guess wrong", func(t *testing.T) {

		_, err := debPackageNameFromURL("https://x/y/myapp.deb")
		assert.Error(t, err)
	})

	t.Run("empty / no filename segment is rejected", func(t *testing.T) {
		_, err := debPackageNameFromURL("https://x/")
		assert.Error(t, err)
	})

	t.Run("invalid debian name characters rejected", func(t *testing.T) {
		_, err := debPackageNameFromURL("https://x/y/Bad@Name_1_amd64.deb")
		assert.Error(t, err)
	})
}
