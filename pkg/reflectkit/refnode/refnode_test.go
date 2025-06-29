package refnode_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/reflectkit/refnode"
	"go.llib.dev/testcase/assert"
)

func TestPath_Contains(t *testing.T) {
	t.Run("empty ntp returns true", func(t *testing.T) {
		p := refnode.Path{}
		assert.True(t, p.Contains())
	})

	t.Run("ntp matches exactly path", func(t *testing.T) {
		p := refnode.Path{refnode.Array, refnode.ArrayElem}
		ntp := []refnode.Type{refnode.Array, refnode.ArrayElem}
		assert.True(t, p.Contains(ntp...))
	})

	t.Run("ntp shorter than path", func(t *testing.T) {
		p := refnode.Path{refnode.Array, refnode.ArrayElem, refnode.Interface, refnode.InterfaceElem}
		ntp := []refnode.Type{refnode.Array, refnode.ArrayElem}
		assert.True(t, p.Contains(ntp...))
	})

	t.Run("ntp missing an element from fully being contained 1:1 in the Path", func(t *testing.T) {
		p := refnode.Path{refnode.Array, refnode.ArrayElem, refnode.Interface, refnode.InterfaceElem}
		ntp := []refnode.Type{refnode.Array, refnode.ArrayElem, refnode.InterfaceElem}
		assert.False(t, p.Contains(ntp...))
	})

	t.Run("ntp longer than path", func(t *testing.T) {
		p := refnode.Path{refnode.Array, refnode.ArrayElem}
		ntp := []refnode.Type{refnode.Array, refnode.ArrayElem, refnode.Interface, refnode.InterfaceElem}
		assert.False(t, p.Contains(ntp...))
	})

	t.Run("some elements mismatch", func(t *testing.T) {
		p := refnode.Path{refnode.Array, refnode.ArrayElem, refnode.Interface, refnode.InterfaceElem}
		ntp := []refnode.Type{refnode.Array, refnode.ArrayElem, refnode.Interface, refnode.PointerElem}
		assert.False(t, p.Contains(ntp...))
	})

	t.Run("p empty and ntp non-empty", func(t *testing.T) {
		p := refnode.Path{}
		ntp := []refnode.Type{refnode.Struct}
		assert.False(t, p.Contains(ntp...))
	})
}
