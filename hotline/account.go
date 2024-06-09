package hotline

import (
	"encoding/binary"
	"golang.org/x/crypto/bcrypt"
	"io"
	"log"
	"slices"
)

const GuestAccount = "guest" // default account used when no login is provided for a connection

type Account struct {
	Login    string       `yaml:"Login"`
	Name     string       `yaml:"Name"`
	Password string       `yaml:"Password"`
	Access   accessBitmap `yaml:"Access"`
}

// Read implements io.Reader interface for Account
func (a *Account) Read(p []byte) (n int, err error) {
	fields := []Field{
		NewField(FieldUserName, []byte(a.Name)),
		NewField(FieldUserLogin, encodeString([]byte(a.Login))),
		NewField(FieldUserAccess, a.Access[:]),
	}

	if bcrypt.CompareHashAndPassword([]byte(a.Password), []byte("")) != nil {
		fields = append(fields, NewField(FieldUserPassword, []byte("x")))
	}

	fieldCount := make([]byte, 2)
	binary.BigEndian.PutUint16(fieldCount, uint16(len(fields)))

	var fieldBytes []byte
	for _, field := range fields {
		fieldBytes = append(fieldBytes, field.Payload()...)
	}

	return copy(p, slices.Concat(fieldCount, fieldBytes)), io.EOF
}

// hashAndSalt generates a password hash from a users obfuscated plaintext password
func hashAndSalt(pwd []byte) string {
	hash, err := bcrypt.GenerateFromPassword(pwd, bcrypt.MinCost)
	if err != nil {
		log.Println(err)
	}

	return string(hash)
}
