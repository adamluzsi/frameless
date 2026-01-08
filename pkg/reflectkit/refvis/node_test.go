package refvis_test

import (
	"testing"

	"go.llib.dev/frameless/pkg/reflectkit/refvis"
	"go.llib.dev/testcase/assert"
)

func TestPath_Contains(t *testing.T) {
	t.Run("empty ntp returns true", func(t *testing.T) {
		p := refvis.Path{}
		assert.True(t, p.Contains())
	})

	t.Run("ntp matches exactly path", func(t *testing.T) {
		p := refvis.Path{refvis.Array, refvis.ArrayElem}
		ntp := []refvis.NodeType{refvis.Array, refvis.ArrayElem}
		assert.True(t, p.Contains(ntp...))
	})

	t.Run("ntp shorter than path", func(t *testing.T) {
		p := refvis.Path{refvis.Array, refvis.ArrayElem, refvis.Interface, refvis.InterfaceElem}
		ntp := []refvis.NodeType{refvis.Array, refvis.ArrayElem}
		assert.True(t, p.Contains(ntp...))
	})

	t.Run("ntp missing an element from fully being contained 1:1 in the Path", func(t *testing.T) {
		p := refvis.Path{refvis.Array, refvis.ArrayElem, refvis.Interface, refvis.InterfaceElem}
		ntp := []refvis.NodeType{refvis.Array, refvis.ArrayElem, refvis.InterfaceElem}
		assert.False(t, p.Contains(ntp...))
	})

	t.Run("ntp longer than path", func(t *testing.T) {
		p := refvis.Path{refvis.Array, refvis.ArrayElem}
		ntp := []refvis.NodeType{refvis.Array, refvis.ArrayElem, refvis.Interface, refvis.InterfaceElem}
		assert.False(t, p.Contains(ntp...))
	})

	t.Run("some elements mismatch", func(t *testing.T) {
		p := refvis.Path{refvis.Array, refvis.ArrayElem, refvis.Interface, refvis.InterfaceElem}
		ntp := []refvis.NodeType{refvis.Array, refvis.ArrayElem, refvis.Interface, refvis.PointerElem}
		assert.False(t, p.Contains(ntp...))
	})

	t.Run("p empty and ntp non-empty", func(t *testing.T) {
		p := refvis.Path{}
		ntp := []refvis.NodeType{refvis.Struct}
		assert.False(t, p.Contains(ntp...))
	})
}
