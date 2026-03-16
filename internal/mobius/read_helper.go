package mobius

import "io"

// readFrom is a shared helper for types that implement io.Reader via an
// offset-based copy from a serialized byte slice.
func readFrom(p []byte, offset *int, data []byte) (int, error) {
	if *offset >= len(data) {
		return 0, io.EOF
	}
	n := copy(p, data[*offset:])
	*offset += n
	return n, nil
}
