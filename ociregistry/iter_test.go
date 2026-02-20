package ociregistry

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSliceSeq(t *testing.T) {
	slice := []int{3, 1, 4}
	var got []int
	for x, err := range SliceSeq(slice) {
		require.NoError(t, err)
		got = append(got, x)
	}
	require.Equal(t, slice, got)
}

func TestErrorSeq(t *testing.T) {
	err := errors.New("foo")
	i := 0
	for s, gotErr := range ErrorSeq[string](err) {
		assert.Equal(t, 0, i)
		assert.Equal(t, "", s)
		assert.Equal(t, err, gotErr)
		i++
	}
	require.Equal(t, 1, i)
}
