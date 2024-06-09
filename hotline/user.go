package hotline

import (
	"encoding/binary"
	"io"
	"slices"
)

// User flags are stored as a 2 byte bitmap and represent various user states
const (
	UserFlagAway        = 0 // User is away
	UserFlagAdmin       = 1 // User is admin
	UserFlagRefusePM    = 2 // User refuses private messages
	UserFlagRefusePChat = 3 // User refuses private chat
)

// FieldOptions flags are sent from v1.5+ clients as part of TranAgreed
const (
	refusePM     = 0 // User has "Refuse private messages" pref set
	refuseChat   = 1 // User has "Refuse private chat" pref set
	autoResponse = 2 // User has "Automatic response" pref set
)

type User struct {
	ID    []byte // Size 2
	Icon  []byte // Size 2
	Flags []byte // Size 2
	Name  string // Variable length user name
}

func (u *User) Read(p []byte) (int, error) {
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

	return copy(p, slices.Concat(
		u.ID,
		u.Icon,
		u.Flags,
		nameLen,
		[]byte(u.Name),
	)), io.EOF
}

func (u *User) Write(p []byte) (int, error) {
	namelen := int(binary.BigEndian.Uint16(p[6:8]))
	u.ID = p[0:2]
	u.Icon = p[2:4]
	u.Flags = p[4:6]
	u.Name = string(p[8 : 8+namelen])

	return 8 + namelen, nil
}

// decodeString decodes an obfuscated user string from a client
// e.g. 98 8a 9a 8c 8b => "guest"
func decodeString(obfuText []byte) (clearText string) {
	for _, char := range obfuText {
		clearText += string(rune(255 - uint(char)))
	}
	return clearText
}

// encodeString takes []byte s containing cleartext and rotates by 255 into obfuscated cleartext.
// The Hotline protocol uses this format for sending passwords over network.
// Not secure, but hey, it was the 90s!
func encodeString(clearText []byte) []byte {
	obfuText := make([]byte, len(clearText))
	for i := 0; i < len(clearText); i++ {
		obfuText[i] = 255 - clearText[i]
	}
	return obfuText
}
