package hotline

import (
	"encoding/binary"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"io"
	"slices"
)

const GuestAccount = "guest" // default account used when no login is provided for a connection

type Account struct {
	Login    string       `yaml:"Login"`
	Name     string       `yaml:"Name"`
	Password string       `yaml:"Password"`
	Access   accessBitmap `yaml:"Access,flow"`

	readOffset int // Internal offset to track read progress
}

func NewAccount(login, name, password string, access accessBitmap) *Account {
	return &Account{
		Login:    login,
		Name:     name,
		Password: hashAndSalt([]byte(password)),
		Access:   access,
	}
}

// Read implements io.Reader interface for Account
func (a *Account) Read(p []byte) (int, error) {
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
		b, err := io.ReadAll(&field)
		if err != nil {
			return 0, fmt.Errorf("error reading field: %w", err)
		}
		fieldBytes = append(fieldBytes, b...)
	}

	buf := slices.Concat(fieldCount, fieldBytes)
	if a.readOffset >= len(buf) {
		return 0, io.EOF // All bytes have been read
	}

	n := copy(p, buf[a.readOffset:])
	a.readOffset += n

	return n, nil
}

// hashAndSalt generates a password hash from a users obfuscated plaintext password
func hashAndSalt(pwd []byte) string {
	hash, _ := bcrypt.GenerateFromPassword(pwd, bcrypt.MinCost)

	return string(hash)
}
