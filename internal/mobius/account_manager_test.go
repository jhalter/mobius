package mobius

import (
	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path"
	"testing"
)

// copyTestFiles copies test config files to a temporary directory
func copyTestFiles(t *testing.T, srcDir, dstDir string) {
	files := []string{"admin.yaml", "guest.yaml", "user-with-old-access-format.yaml"}

	for _, file := range files {
		srcPath := path.Join(srcDir, file)
		dstPath := path.Join(dstDir, file)

		data, err := os.ReadFile(srcPath)
		require.NoError(t, err, "Failed to read %s", srcPath)

		err = os.WriteFile(dstPath, data, 0644)
		require.NoError(t, err, "Failed to write %s", dstPath)
	}
}

func TestNewYAMLAccountManager(t *testing.T) {
	t.Run("loads accounts from a directory", func(t *testing.T) {
		// Create temporary directory and copy test files
		tempDir := t.TempDir()
		copyTestFiles(t, "test/config/Users", tempDir)

		// Use temp directory instead of original test directory
		got, err := NewYAMLAccountManager(tempDir)
		require.NoError(t, err)

		assert.Equal(t,
			&hotline.Account{
				Name:     "admin",
				Login:    "admin",
				Password: "$2a$04$2itGEYx8C1N5bsfRSoC9JuonS3I4YfnyVPZHLSwp7kEInRX0yoB.a",
				Access:   hotline.AccessBitmap{0xff, 0xff, 0xef, 0xff, 0xff, 0x80, 0x00, 0x00},
			},
			got.Get("admin"),
		)

		assert.Equal(t,
			&hotline.Account{
				Login:    "guest",
				Name:     "guest",
				Password: "$2a$04$6Yq/TIlgjSD.FbARwtYs9ODnkHawonu1TJ5W2jJKfhnHwBIQTk./y",
				Access:   hotline.AccessBitmap{0x60, 0x70, 0x0c, 0x20, 0x03, 0x80, 0x00, 0x00},
			},
			got.Get("guest"),
		)

		assert.Equal(t,
			&hotline.Account{
				Login:    "test-user",
				Name:     "Test User Name",
				Password: "$2a$04$9P/jgLn1fR9TjSoWL.rKxuN6g.1TSpf2o6Hw.aaRuBwrWIJNwsKkS",
				Access:   hotline.AccessBitmap{0x7d, 0xf0, 0x0c, 0xef, 0xab, 0x80, 0x00, 0x00},
			},
			got.Get("test-user"),
		)
	})
}

func TestYAMLAccountManager_Delete(t *testing.T) {
	t.Run("successful deletions", func(t *testing.T) {
		type args struct {
			login string
		}
		tests := []struct {
			name string
			args args
		}{
			{
				name: "deletes existing guest account",
				args: args{
					login: "guest",
				},
			},
			{
				name: "deletes existing admin account",
				args: args{
					login: "admin",
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Create temporary directory and copy test files
				tempDir := t.TempDir()
				copyTestFiles(t, "test/config/Users", tempDir)

				accountMgr, err := NewYAMLAccountManager(tempDir)
				require.NoError(t, err)

				// Get initial state
				initialAccounts := accountMgr.List()
				initialCount := len(initialAccounts)

				// Verify account exists before deletion
				account := accountMgr.Get(tt.args.login)
				require.NotNilf(t, account, "Account %s should exist before deletion", tt.args.login)

				// Perform deletion
				err = accountMgr.Delete(tt.args.login)
				assert.NoErrorf(t, err, "Delete(%v)", tt.args.login)

				// Verify account no longer exists in memory
				account = accountMgr.Get(tt.args.login)
				assert.Nilf(t, account, "Delete(%v)", tt.args.login)

				// Verify file was deleted
				filePath := path.Join(tempDir, tt.args.login+".yaml")
				_, fileErr := os.Stat(filePath)
				assert.Truef(t, os.IsNotExist(fileErr), "Delete(%v) - file should be deleted", tt.args.login)

				// Verify list count decreased
				updatedAccounts := accountMgr.List()
				expectedCount := initialCount - 1
				assert.Equalf(t, expectedCount, len(updatedAccounts), "Delete(%v)", tt.args.login)

				// Verify deleted account is not in the list
				for _, account := range updatedAccounts {
					assert.NotEqualf(t, tt.args.login, account.Login, "Delete(%v)", tt.args.login)
				}
			})
		}
	})

	t.Run("error cases", func(t *testing.T) {
		type args struct {
			login string
		}
		tests := []struct {
			name string
			args args
		}{
			{
				name: "returns error when deleting non-existent account",
				args: args{
					login: "nonexistent",
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Create temporary directory and copy test files
				tempDir := t.TempDir()
				copyTestFiles(t, "test/config/Users", tempDir)

				accountMgr, err := NewYAMLAccountManager(tempDir)
				require.NoError(t, err)

				// Perform deletion
				err = accountMgr.Delete(tt.args.login)
				assert.Errorf(t, err, "Delete(%v)", tt.args.login)
			})
		}
	})
}

func TestYAMLAccountManager_Create(t *testing.T) {
	t.Run("successful creation", func(t *testing.T) {
		type args struct {
			account hotline.Account
		}
		tests := []struct {
			name string
			args args
		}{
			{
				name: "creates new account with valid data",
				args: args{
					account: hotline.Account{
						Login:    "newuser",
						Name:     "New User",
						Password: "hashedpassword",
						Access:   hotline.AccessBitmap{0x60, 0x70, 0x0c, 0x20, 0x03, 0x80, 0x00, 0x00},
					},
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				tempDir := t.TempDir()
				copyTestFiles(t, "test/config/Users", tempDir)

				accountMgr, err := NewYAMLAccountManager(tempDir)
				require.NoError(t, err)

				initialCount := len(accountMgr.List())

				err = accountMgr.Create(tt.args.account)
				assert.NoErrorf(t, err, "Create(%v)", tt.args.account)

				// Verify account exists in memory
				retrievedAccount := accountMgr.Get(tt.args.account.Login)
				assert.NotNilf(t, retrievedAccount, "Create(%v)", tt.args.account)
				assert.Equalf(t, &tt.args.account, retrievedAccount, "Create(%v)", tt.args.account)

				// Verify file was created
				filePath := path.Join(tempDir, tt.args.account.Login+".yaml")
				_, err = os.Stat(filePath)
				assert.NoErrorf(t, err, "Create(%v) - file should exist", tt.args.account)

				// Verify list count increased
				assert.Equalf(t, initialCount+1, len(accountMgr.List()), "Create(%v)", tt.args.account)
			})
		}
	})

	t.Run("error cases", func(t *testing.T) {
		type args struct {
			account hotline.Account
		}
		tests := []struct {
			name string
			args args
		}{
			{
				name: "returns error when creating account with existing login",
				args: args{
					account: hotline.Account{
						Login:    "admin",
						Name:     "Duplicate Admin",
						Password: "password",
						Access:   hotline.AccessBitmap{},
					},
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				tempDir := t.TempDir()
				copyTestFiles(t, "test/config/Users", tempDir)

				accountMgr, err := NewYAMLAccountManager(tempDir)
				require.NoError(t, err)

				err = accountMgr.Create(tt.args.account)
				assert.Errorf(t, err, "Create(%v)", tt.args.account)
			})
		}
	})
}

func TestYAMLAccountManager_Update(t *testing.T) {
	t.Run("successful updates", func(t *testing.T) {
		type args struct {
			account  hotline.Account
			newLogin string
		}
		tests := []struct {
			name string
			args args
		}{
			{
				name: "updates account without changing login",
				args: args{
					account: hotline.Account{
						Login:    "guest",
						Name:     "Updated Guest",
						Password: "newpassword",
						Access:   hotline.AccessBitmap{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88},
					},
					newLogin: "guest",
				},
			},
			{
				name: "updates account with login change",
				args: args{
					account: hotline.Account{
						Login:    "guest",
						Name:     "Renamed Guest",
						Password: "password",
						Access:   hotline.AccessBitmap{0x60, 0x70, 0x0c, 0x20, 0x03, 0x80, 0x00, 0x00},
					},
					newLogin: "renamedguest",
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				tempDir := t.TempDir()
				copyTestFiles(t, "test/config/Users", tempDir)

				accountMgr, err := NewYAMLAccountManager(tempDir)
				require.NoError(t, err)

				err = accountMgr.Update(tt.args.account, tt.args.newLogin)
				assert.NoErrorf(t, err, "Update(%v, %v)", tt.args.account, tt.args.newLogin)

				// Verify updated account exists with new login
				retrievedAccount := accountMgr.Get(tt.args.newLogin)
				assert.NotNilf(t, retrievedAccount, "Update(%v, %v)", tt.args.account, tt.args.newLogin)

				expectedAccount := tt.args.account
				expectedAccount.Login = tt.args.newLogin
				assert.Equalf(t, &expectedAccount, retrievedAccount, "Update(%v, %v)", tt.args.account, tt.args.newLogin)

				// If login changed, verify old login no longer exists
				if tt.args.account.Login != tt.args.newLogin {
					oldAccount := accountMgr.Get(tt.args.account.Login)
					assert.Nilf(t, oldAccount, "Update(%v, %v) - old login should not exist", tt.args.account, tt.args.newLogin)

					// Verify old file was renamed
					oldFilePath := path.Join(tempDir, tt.args.account.Login+".yaml")
					_, err = os.Stat(oldFilePath)
					assert.Truef(t, os.IsNotExist(err), "Update(%v, %v) - old file should not exist", tt.args.account, tt.args.newLogin)
				}

				// Verify new file exists
				newFilePath := path.Join(tempDir, tt.args.newLogin+".yaml")
				_, err = os.Stat(newFilePath)
				assert.NoErrorf(t, err, "Update(%v, %v) - new file should exist", tt.args.account, tt.args.newLogin)
			})
		}
	})
}

func TestYAMLAccountManager_Get(t *testing.T) {
	type args struct {
		login string
	}
	tests := []struct {
		name string
		args args
		want *hotline.Account
	}{
		{
			name: "returns existing account",
			args: args{
				login: "admin",
			},
			want: &hotline.Account{
				Name:     "admin",
				Login:    "admin",
				Password: "$2a$04$2itGEYx8C1N5bsfRSoC9JuonS3I4YfnyVPZHLSwp7kEInRX0yoB.a",
				Access:   hotline.AccessBitmap{0xff, 0xff, 0xef, 0xff, 0xff, 0x80, 0x00, 0x00},
			},
		},
		{
			name: "returns nil for non-existent account",
			args: args{
				login: "nonexistent",
			},
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			copyTestFiles(t, "test/config/Users", tempDir)

			accountMgr, err := NewYAMLAccountManager(tempDir)
			require.NoError(t, err)

			got := accountMgr.Get(tt.args.login)
			assert.Equalf(t, tt.want, got, "Get(%v)", tt.args.login)
		})
	}
}

func TestYAMLAccountManager_List(t *testing.T) {
	tests := []struct {
		name       string
		wantCount  int
		wantLogins []string
	}{
		{
			name:       "returns all accounts",
			wantCount:  3,
			wantLogins: []string{"admin", "guest", "test-user"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			copyTestFiles(t, "test/config/Users", tempDir)

			accountMgr, err := NewYAMLAccountManager(tempDir)
			require.NoError(t, err)

			got := accountMgr.List()
			assert.Equalf(t, tt.wantCount, len(got), "List() count")

			// Check that all expected logins are present
			gotLogins := make([]string, len(got))
			for i, account := range got {
				gotLogins[i] = account.Login
			}

			for _, expectedLogin := range tt.wantLogins {
				assert.Containsf(t, gotLogins, expectedLogin, "List() should contain login %s", expectedLogin)
			}
		})
	}
}
