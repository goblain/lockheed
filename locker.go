package lockheed

import "context"

type LockerInterface interface {
	Acquire(context.Context, *Lock) error
	Renew(context.Context, *Lock) error
	Release(context.Context, *Lock) error
	// List all locks, this locker has access to
	GetAllLocks(context.Context) ([]*Lock, error)
	// ForcefulRemoval(string, []Condition)
}

func GetLocks(ctx context.Context, locker LockerInterface, c *Condition) ([]*Lock, error) {
	var result []*Lock
	locks, err := locker.GetAllLocks(ctx)
	if err != nil {
		return nil, err
	}
	if c == nil {
		result = locks
	} else {
		for _, lock := range locks {
			matching, err := lock.Evaluate(c)
			if err != nil {
				return nil, err
			}
			if matching {
				result = append(result, lock)
			}
		}
	}
	return result, nil
}
