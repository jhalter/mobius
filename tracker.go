package hotline

import (
	"net"
)

// Tracker registration sequence:
// Server sends UDP to tracker on port 5499
//
// 0000   02 00 00 00 45 00 00 47 d0 9f 00 00 40 11 00 00   ....E..GÐ...@...
// 0010   c0 a8 56 f4 c0 a8 56 f4 ec 4b 15 7b 00 33 2f 7e   À¨VôÀ¨VôìK.{.3/~
// 0020   00 01 15 7c 00 00 00 00 d5 8b b9 c2 0f 48 45 4c   ...|....Õ.¹Â.HEL
// 0030   4c 4f 20 48 4f 54 4c 49 4e 45 21 21 0d 74 65 73   LO HOTLINE!!.tes
// 0040   74 20 73 65 72 76 65 72 7a 7a 00                  t serverzz.

func (s *Server) RegisterWithTracker(tracker string) error {
	name := []byte(s.Config.Name)
	description := []byte(s.Config.Description)

	payload := []byte{
		0x00, 0x01,
		0x15, 0x7c, // TCP port
		0x00, 0x00, // Number of Users
		0x00, 0x00, // ??
		0xd3, 0x8b, 0xb9, 0xc2, // ??
	}

	payload = append(payload, []byte{uint8(len(name))}...)
	payload = append(payload, name...)
	payload = append(payload, []byte{uint8(len(description))}...)
	payload = append(payload, description...)

	conn, err := net.Dial("udp", tracker)
	if err != nil {
		return err
	}

	if _, err := conn.Write(payload); err != nil {
		return err
	}

	return nil
}
