package hotline

import (
	"github.com/jhalter/mobius/concat"
)

const GuestAccount = "guest" // default account used when no login is provided for a connection

type Account struct {
	Login    string  `yaml:"Login"`
	Name     string  `yaml:"Name"`
	Password string  `yaml:"Password"`
	Access   *[]byte `yaml:"Access"` // 8 byte bitmap
}

// MarshalBinary marshals an Account to byte slice
func (a *Account) MarshalBinary() (out []byte) {
	return concat.Slices(
		[]byte{0x00, 0x3}, // param count -- always 3
		NewField(fieldUserName, []byte(a.Name)).Payload(),
		NewField(fieldUserLogin, negateString([]byte(a.Login))).Payload(),
		NewField(fieldUserAccess, *a.Access).Payload(),
	)
}
