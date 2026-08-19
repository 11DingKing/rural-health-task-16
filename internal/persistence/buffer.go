package persistence

import (
	"bufio"
	"io"
)

// bufferedWriter wraps a writer with a buffered writer for efficient
// sequential writes. Tests can swap this out.
func newBufferedWriter(w io.Writer) *bufio.Writer {
	return bufio.NewWriterSize(w, 64*1024)
}
