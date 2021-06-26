package hotline

import (
	"encoding/binary"
)

// GuestAccount is the account used when no login is provided for a connection
const GuestAccount = "guest"

// User flags field is a bitmap with the following values:
// 0	1	User is away
// 1	2	User is admin
// 2	4	User refuses private messages
// 3	8	User refuses private chat
const userFlagAway = 0
const userFlagAdmin = 1
const userFlagRefusePM = 2
const userFLagRefusePChat = 3

type User struct {
	ID    []byte //Size 2
	Icon  []byte //Size 2
	Flags []byte //Size 2
	Name  string
}

func (u User) Payload() []byte {
	name := []byte(u.Name)
	nameLen := make([]byte, 2)
	binary.BigEndian.PutUint16(nameLen, uint16(len(name)))

	if len(u.Icon) == 4 {
		u.Icon = u.Icon[2:]
	}

	if len(u.Flags) == 4 {
		u.Flags = u.Flags[2:]
	}

	out := append(u.ID[:2], u.Icon[:2]...)
	out = append(out, u.Flags[:2]...)
	out = append(out, nameLen...)
	out = append(out, name...)

	return out
}

func DecodeUserString(encodedString []byte) (decodedString string) {
	for _, char := range encodedString {
		decodedString += string(rune(255 - uint(char)))
	}

	return decodedString
}

// Take a []byte of uncoded ascii as input and encode it
func NegatedUserString(encodedString []byte) string {
	var decodedString string
	for _, char := range encodedString {
		decodedString += string(255 - uint8(char))[1:]
	}

	return decodedString
}
