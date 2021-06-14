package hotline

import (
	"encoding/binary"
)

type Account struct {
	Login    string  `yaml:"Login"`
	Name     string  `yaml:"Name"`
	Password string  `yaml:"Password"`
	Access   *[]byte `yaml:"Access"` // 8 byte bitmap
}

// Example:
//	00 04 // fieldCount?
//	00 66 // 102 - fieldUserName
//	00 0d // 13
//	61 64 6d 69 6e 69 73 74 72 61 74 6f 72 // administrator
//	00 69 // 105 fieldUserLogin (encoded)
//	00 05 // len
//	9e 9b 92 96 91 // encoded login name
//	00 6a // 106 fieldUserPassword
//	00 01  // len
//	78
//	00 6e  // fieldUserAccess
//	00 08
//	ff d3 cf ef ff 80 00 00
func (a *Account) Payload() []byte {
	var out []byte

	nameLen := make([]byte, 2)
	binary.BigEndian.PutUint16(nameLen, uint16(len(a.Name)))

	loginLen := make([]byte, 2)
	binary.BigEndian.PutUint16(loginLen, uint16(len(a.Login)))

	out = append(out, []byte{0x00, 0x3}...) // fieldCount ?

	out = append(out, []byte{0x00, 0x66}...) // fieldUserName
	out = append(out, nameLen...)
	out = append(out, []byte(a.Name)...)

	out = append(out, []byte{0x00, 0x69}...) // fieldUserLogin
	out = append(out, loginLen...)
	out = append(out, []byte(NegatedUserString([]byte(a.Login)))...) // TODO: Ugggh

	out = append(out, []byte{0x00, 0x6e}...) // fieldUserAccess
	out = append(out, []byte{0x00, 0x08}...)
	out = append(out, *a.Access...)

	return out
}
