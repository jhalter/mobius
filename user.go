package hotline

import (
	"encoding/binary"
)

// User flags are stored as a 2 byte bitmap with the following values:
const (
	userFlagAway        = 0 // User is away
	userFlagAdmin       = 1 // User is admin
	userFlagRefusePM    = 2 // User refuses private messages
	userFLagRefusePChat = 3 // User refuses private chat
)

type User struct {
	ID    []byte // Size 2
	Icon  []byte // Size 2
	Flags []byte // Size 2
	Name  string // Variable length user name
}

func (u User) Payload() []byte {
	nameLen := make([]byte, 2)
	binary.BigEndian.PutUint16(nameLen, uint16(len(u.Name)))

	if len(u.Icon) == 4 {
		u.Icon = u.Icon[2:]
	}

	if len(u.Flags) == 4 {
		u.Flags = u.Flags[2:]
	}

	out := append(u.ID[:2], u.Icon[:2]...)
	out = append(out, u.Flags[:2]...)
	out = append(out, nameLen...)
	out = append(out, u.Name...)

	return out
}

func ReadUser(b []byte) (*User, error) {
	u := &User{
		ID:    b[0:2],
		Icon:  b[2:4],
		Flags: b[4:6],
		Name:  string(b[8:]),
	}
	return u, nil
}

// DecodeUserString decodes an obfuscated user string from a client
// e.g. 98 8a 9a 8c 8b => "guest"
func DecodeUserString(encodedString []byte) (decodedString string) {
	for _, char := range encodedString {
		decodedString += string(rune(255 - uint(char)))
	}
	return decodedString
}

// Take a []byte of uncoded ascii as input and encode it
// TODO: change the method signature to take a string and return []byte
func NegatedUserString(encodedString []byte) string {
	var decodedString string
	for _, char := range encodedString {
		decodedString += string(255 - uint8(char))[1:]
	}
	return decodedString
}
