package hotline

import (
	"encoding/binary"
	"github.com/jhalter/mobius/concat"
	"golang.org/x/crypto/bcrypt"
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
	fields := []Field{
		NewField(fieldUserName, []byte(a.Name)),
		NewField(fieldUserLogin, negateString([]byte(a.Login))),
		NewField(fieldUserAccess, *a.Access),
	}

	if bcrypt.CompareHashAndPassword([]byte(a.Password), []byte("")) != nil {
		fields = append(fields, NewField(fieldUserPassword, []byte("x")))
	}

	fieldCount := make([]byte, 2)
	binary.BigEndian.PutUint16(fieldCount, uint16(len(fields)))

	var fieldPayload []byte
	for _, field := range fields {
		fieldPayload = append(fieldPayload, field.Payload()...)
	}

	return concat.Slices(
		fieldCount,
		fieldPayload,
	)
}
