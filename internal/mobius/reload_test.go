package mobius

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The concrete Reload implementations (FlatNews, Agreement, BanFile, ThreadedNewsYAML) are
// exercised in their own *_test.go files. This covers the ReloaderFunc adapter, which is the
// only unit defined in reload.go without direct coverage.
func TestReloaderFunc_Reload(t *testing.T) {
	t.Run("invokes the wrapped func", func(t *testing.T) {
		called := false
		var r Reloader = ReloaderFunc(func() error {
			called = true
			return nil
		})

		require.NoError(t, r.Reload())
		assert.True(t, called)
	})

	t.Run("propagates the wrapped error", func(t *testing.T) {
		sentinel := errors.New("reload failed")
		var r Reloader = ReloaderFunc(func() error { return sentinel })

		assert.ErrorIs(t, r.Reload(), sentinel)
	})
}
