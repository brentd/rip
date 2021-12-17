package rip

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRead(t *testing.T) {
	assert := assert.New(t)

	t.Run("with ChunkSize that unevenly splits the boundary", func(t *testing.T) {
		r := NewParallelReader()
		r.ChunkSize = 6

		chunks := make(chan string, 128)
		r.Read(strings.NewReader("abc\ndef\n"), func(chunk []byte) {
			chunks <- string(chunk)
		})
		close(chunks)

		results := drain(chunks)

		assert.Len(results, 2)
		assert.Contains(results, "abc\n", "def\n")
	})

	t.Run("with ChunkSize greater than input", func(t *testing.T) {
		r := NewParallelReader()
		r.ChunkSize = 1 << 16

		chunks := make(chan string, 128)
		r.Read(strings.NewReader("abc\ndef\n"), func(chunk []byte) {
			chunks <- string(chunk)
		})
		close(chunks)

		results := drain(chunks)

		assert.Len(results, 1)
		assert.Contains(results, "abc\ndef\n")
	})

	t.Run("with a multichar ChunkBoundary", func(t *testing.T) {
		r := NewParallelReader()
		r.ChunkSize = 16
		r.ChunkBoundary = "END"

		chunks := make(chan string, 128)
		r.Read(strings.NewReader("abcdefgENDhijklmnopEND"), func(chunk []byte) {
			chunks <- string(chunk)
		})
		close(chunks)

		results := drain(chunks)

		assert.Len(results, 2)
		assert.Contains(results, "abcdefgEND", "hijklmnopEND")
	})

	t.Run("when the last chunk does not end with a ChunkBoundary", func(t *testing.T) {
		r := NewParallelReader()
		r.ChunkSize = 1 << 16
		r.ChunkBoundary = "|SPLIT|"

		chunks := make(chan string, 128)
		r.Read(strings.NewReader("abcdefg|SPLIT|hijklmnop|SPLIT|hello"), func(chunk []byte) {
			chunks <- string(chunk)
		})
		close(chunks)

		results := drain(chunks)

		assert.Len(results, 2)
		assert.Contains(results, "abcdefg|SPLIT|hijklmnop|SPLIT|", "hello")

	})

	t.Run("when using RequireBoundary", func(t *testing.T) {
		r := NewParallelReader()
		r.ChunkSize = 100
		r.ChunkBoundary = "|SPLIT|"
		r.RequireBoundary = true

		chunks := make(chan string, 128)
		r.Read(strings.NewReader("abcdefg|SPLIT|hijklmnop|SPLIT|hello"), func(chunk []byte) {
			chunks <- string(chunk)
		})
		close(chunks)

		results := drain(chunks)

		assert.Len(results, 1)
		assert.EqualValues(results, []string{"abcdefg|SPLIT|hijklmnop|SPLIT|"})
	})

	t.Run("ChunkBoundaryStart and ChunkBoundaryEnd", func(t *testing.T) {
		r := NewParallelReader()
		r.ChunkSize = 100
		r.ChunkBoundaryStart = "<FOO>"
		r.ChunkBoundary = "</FOO>"
		r.RequireBoundary = true

		chunks := make(chan string, 128)
		r.Read(strings.NewReader("abcdefg<FOO>hijklmnop</FOO>hello"), func(chunk []byte) {
			chunks <- string(chunk)
		})
		close(chunks)

		results := drain(chunks)

		assert.Len(results, 1)
		assert.EqualValues([]string{"<FOO>hijklmnop</FOO>"}, results)
	})
}

func drain(c <-chan string) []string {
	var results []string
	for s := range c {
		results = append(results, string(s))
	}
	return results
}
