package hotline

import (
	"encoding/binary"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestNewAccount(t *testing.T) {
	access := AccessBitmap{0xff, 0, 0, 0, 0, 0, 0, 0}
	acct := NewAccount("jdoe", "John Doe", "secret123", access)

	assert.Equal(t, "jdoe", acct.Login)
	assert.Equal(t, "John Doe", acct.Name)
	assert.Equal(t, access, acct.Access)

	// Password should be a bcrypt hash, not the plaintext.
	assert.NotEqual(t, "secret123", acct.Password)
	err := bcrypt.CompareHashAndPassword([]byte(acct.Password), []byte("secret123"))
	require.NoError(t, err, "password hash should validate against original plaintext")
}

func TestHashAndSalt(t *testing.T) {
	t.Run("produces valid bcrypt hash", func(t *testing.T) {
		hash := HashAndSalt([]byte("password"))
		err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("password"))
		require.NoError(t, err)
	})

	t.Run("different calls produce different hashes", func(t *testing.T) {
		h1 := HashAndSalt([]byte("same"))
		h2 := HashAndSalt([]byte("same"))
		assert.NotEqual(t, h1, h2, "bcrypt salts should differ between calls")

		// Both should still verify against the original input.
		require.NoError(t, bcrypt.CompareHashAndPassword([]byte(h1), []byte("same")))
		require.NoError(t, bcrypt.CompareHashAndPassword([]byte(h2), []byte("same")))
	})
}

func TestAccount_Read(t *testing.T) {
	t.Run("with password set", func(t *testing.T) {
		acct := NewAccount("admin", "Admin User", "pass", AccessBitmap{})

		data, err := io.ReadAll(acct)
		require.NoError(t, err)

		// First two bytes are the field count (big-endian uint16).
		require.GreaterOrEqual(t, len(data), 2)
		fieldCount := binary.BigEndian.Uint16(data[:2])
		assert.Equal(t, uint16(4), fieldCount, "should have 4 fields when password is set")

		// Verify the password marker "x" appears in the serialized data.
		assert.Contains(t, string(data), "x")

		// Verify the user name appears in the serialized data.
		assert.Contains(t, string(data), "Admin User")
	})

	t.Run("with empty password", func(t *testing.T) {
		acct := &Account{
			Login:    "guest",
			Name:     "Guest",
			Password: HashAndSalt([]byte("")),
			Access:   AccessBitmap{},
		}

		data, err := io.ReadAll(acct)
		require.NoError(t, err)

		require.GreaterOrEqual(t, len(data), 2)
		fieldCount := binary.BigEndian.Uint16(data[:2])
		assert.Equal(t, uint16(3), fieldCount, "should have 3 fields when password is empty")
	})

	t.Run("full read returns all serialized bytes", func(t *testing.T) {
		acct := NewAccount("test", "Test", "pw", AccessBitmap{0x01})

		data, err := io.ReadAll(acct)
		require.NoError(t, err)
		assert.Greater(t, len(data), 2, "serialized output should contain field data beyond the count header")
	})
}

func TestNewAccount_GuestConstant(t *testing.T) {
	assert.Equal(t, "guest", GuestAccount)
}
