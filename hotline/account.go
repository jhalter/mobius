package hotline

import (
	"encoding/binary"
	"golang.org/x/crypto/bcrypt"
)

const GuestAccount = "guest" // default account used when no login is provided for a connection

type Account struct {
	Login    string  `yaml:"Login"`
	Name     string  `yaml:"Name"`
	Password string  `yaml:"Password"`
	Access   *[]byte `yaml:"Access"` // 8 byte bitmap
}

// Read implements io.Reader interface for Account
func (a *Account) Read(p []byte) (n int, err error) {
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

	p = append(p, fieldCount...)

	for _, field := range fields {
		p = append(p, field.Payload()...)
	}

	return len(p), nil
}
