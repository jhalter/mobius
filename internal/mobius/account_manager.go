package mobius

import (
	"fmt"
	"github.com/jhalter/mobius/hotline"
	"github.com/stretchr/testify/mock"
	"gopkg.in/yaml.v3"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
)

// loadFromYAMLFile loads data from a YAML file into the provided data structure.
func loadFromYAMLFile(path string, data interface{}) error {
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()

	decoder := yaml.NewDecoder(fh)
	return decoder.Decode(data)
}

type YAMLAccountManager struct {
	accounts   map[string]hotline.Account
	accountDir string

	mu sync.Mutex
}

func NewYAMLAccountManager(accountDir string) (*YAMLAccountManager, error) {
	accountMgr := YAMLAccountManager{
		accountDir: accountDir,
		accounts:   make(map[string]hotline.Account),
	}

	matches, err := filepath.Glob(filepath.Join(accountDir, "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("list account files: %w", err)
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no accounts found in directory: %s", accountDir)
	}

	for _, filePath := range matches {
		var account hotline.Account
		fileContents, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read file: %v", err)
		}

		if err := yaml.Unmarshal(fileContents, &account); err != nil {
			return nil, fmt.Errorf("unmarshal: %v", err)
		}

		// Check the account file contents for a field name that only appears in the new AccessBitmap flag format.
		// If not present, re-save the file to migrate it from the old array of ints format to new bool flag format.
		if !strings.Contains(string(fileContents), "    DownloadFile:") {
			if err := accountMgr.Update(account, account.Login); err != nil {
				return nil, fmt.Errorf("migrate account to new access flag format: %v", err)
			}
		}

		accountMgr.accounts[account.Login] = account
	}

	return &accountMgr, nil
}

func (am *YAMLAccountManager) Create(account hotline.Account) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	// Create account file, returning an error if one already exists.
	file, err := os.OpenFile(
		filepath.Join(am.accountDir, path.Join("/", account.Login+".yaml")),
		os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644,
	)
	if err != nil {
		return fmt.Errorf("create account file: %w", err)
	}
	defer file.Close()

	b, err := yaml.Marshal(account)
	if err != nil {
		return fmt.Errorf("marshal account to YAML: %v", err)
	}

	_, err = file.Write(b)
	if err != nil {
		return fmt.Errorf("write account file: %w", err)
	}

	am.accounts[account.Login] = account

	return nil
}

func (am *YAMLAccountManager) Update(account hotline.Account, newLogin string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	// If the login has changed, rename the account file.
	if account.Login != newLogin {
		err := os.Rename(
			filepath.Join(am.accountDir, path.Join("/", account.Login)+".yaml"),
			filepath.Join(am.accountDir, path.Join("/", newLogin)+".yaml"),
		)
		if err != nil {
			return fmt.Errorf("error renaming account file: %w", err)
		}

		delete(am.accounts, account.Login)
		account.Login = newLogin
		am.accounts[newLogin] = account
	}

	out, err := yaml.Marshal(&account)
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(am.accountDir, newLogin+".yaml"), out, 0644); err != nil {
		return fmt.Errorf("error writing account file: %w", err)
	}

	am.accounts[account.Login] = account

	return nil
}

func (am *YAMLAccountManager) Get(login string) *hotline.Account {
	am.mu.Lock()
	defer am.mu.Unlock()

	account, ok := am.accounts[login]
	if !ok {
		return nil
	}

	return &account
}

func (am *YAMLAccountManager) List() []hotline.Account {
	am.mu.Lock()
	defer am.mu.Unlock()

	var accounts []hotline.Account
	for _, account := range am.accounts {
		accounts = append(accounts, account)
	}

	return accounts
}

func (am *YAMLAccountManager) Delete(login string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	err := os.Remove(filepath.Join(am.accountDir, path.Join("/", login+".yaml")))
	if err != nil {
		return fmt.Errorf("delete account file: %v", err)
	}

	delete(am.accounts, login)

	return nil
}

type MockAccountManager struct {
	mock.Mock
}

func (m *MockAccountManager) Create(account hotline.Account) error {
	args := m.Called(account)

	return args.Error(0)
}

func (m *MockAccountManager) Update(account hotline.Account, newLogin string) error {
	args := m.Called(account, newLogin)

	return args.Error(0)
}

func (m *MockAccountManager) Get(login string) *hotline.Account {
	args := m.Called(login)

	return args.Get(0).(*hotline.Account)
}

func (m *MockAccountManager) List() []hotline.Account {
	args := m.Called()

	return args.Get(0).([]hotline.Account)
}

func (m *MockAccountManager) Delete(login string) error {
	args := m.Called(login)

	return args.Error(0)
}
