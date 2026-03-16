package hotline

import "io"

// readFrom is a shared helper for types that implement io.Reader via an
// offset-based copy from a serialized byte slice. It copies bytes from data
// starting at *offset into p, advances *offset, and returns io.EOF when done.
func readFrom(p []byte, offset *int, data []byte) (int, error) {
	if *offset >= len(data) {
		return 0, io.EOF
	}
	n := copy(p, data[*offset:])
	*offset += n
	return n, nil
}
