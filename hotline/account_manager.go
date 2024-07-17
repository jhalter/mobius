package hotline

type AccountManager interface {
	Create(account Account) error
	Update(account Account, newLogin string) error
	Get(login string) *Account
	List() []Account
	Delete(login string) error
}
