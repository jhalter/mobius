package hotline

// HandlerFunc is the signature of a func to handle a Hotline transaction.
type HandlerFunc func(*ClientConn, *Transaction) []Transaction

func (s *Server) HandleFunc(tranType [2]byte, handler HandlerFunc) {
	s.handlers[tranType] = handler
}

// The total size of a chat message data field is 8192 bytes.
const LimitChatMsg = 8192
