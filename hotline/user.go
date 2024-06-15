package hotline

import (
	"encoding/binary"
	"io"
	"slices"
)

// User flags are stored as a 2 byte bitmap and represent various user states
const (
	UserFlagAway        = iota // User is away
	UserFlagAdmin              // User is admin
	UserFlagRefusePM           // User refuses private messages
	UserFlagRefusePChat        // User refuses private chat
)

// FieldOptions flags are sent from v1.5+ clients as part of TranAgreed
const (
	UserOptRefusePM     = iota // User has "Refuse private messages" pref set
	UserOptRefuseChat          // User has "Refuse private chat" pref set
	UserOptAutoResponse        // User has "Automatic response" pref set
)

type User struct {
	ID    []byte // Size 2
	Icon  []byte // Size 2
	Flags []byte // Size 2
	Name  string // Variable length user name

	readOffset int // Internal offset to track read progress
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

	b := slices.Concat(
		u.ID,
		u.Icon,
		u.Flags,
		nameLen,
		[]byte(u.Name),
	)

	if u.readOffset >= len(b) {
		return 0, io.EOF // All bytes have been read
	}

	n := copy(p, b)

	return n, io.EOF
}

func (u *User) Write(p []byte) (int, error) {
	namelen := int(binary.BigEndian.Uint16(p[6:8]))
	u.ID = p[0:2]
	u.Icon = p[2:4]
	u.Flags = p[4:6]
	u.Name = string(p[8 : 8+namelen])

	return 8 + namelen, nil
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
