package hotline

// AccountManager provides an interface for managing user accounts,
// including creation, retrieval, updates, and deletion operations.
type AccountManager interface {
	Create(account Account) error
	Update(account Account, newLogin string) error
	Get(login string) *Account
	List() []Account
	Delete(login string) error
}
