package hotline

// ClientIDFromBytes converts client-supplied bytes to a ClientID, returning
// ok=false if the input is not exactly the expected length. Use this instead of
// a direct [2]byte(...) conversion on field data, which panics on a length
// mismatch.
func ClientIDFromBytes(b []byte) (id ClientID, ok bool) {
	if len(b) != len(id) {
		return id, false
	}
	return ClientID(b), true
}

// ChatIDFromBytes converts client-supplied bytes to a ChatID, returning ok=false
// if the input is not exactly the expected length. Use this instead of a direct
// [4]byte(...) conversion on field data, which panics on a length mismatch.
func ChatIDFromBytes(b []byte) (id ChatID, ok bool) {
	if len(b) != len(id) {
		return id, false
	}
	return ChatID(b), true
}
