package lockheed

type LockerInterface interface {
	Acquire(*Lock) error
	Renew(*Lock) error
	Release(*Lock) error
	// List all locks this locker has access to
	List() ([]string, error)
}
