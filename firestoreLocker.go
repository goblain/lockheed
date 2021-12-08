package lockheed

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"runtime"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	// "google.golang.org/genproto/googleapis/firestore/v1"
)

type FirestoreLocker struct {
	Client         *firestore.Client
	CollectionPath string
}

type FirestoreLockState struct {
	Lock []byte `firestore:"lock"`
}

func (fls *FirestoreLockState) SetLock(l *Lock) {
	var err error
	fls.Lock, err = json.Marshal(l)
	if err != nil {
		panic(err.Error())
	}
}

func (fls *FirestoreLockState) GetLock() *Lock {
	var err error
	l := &Lock{}
	err = json.Unmarshal(fls.Lock, l)
	if err != nil {
		panic(err.Error())
	}
	return l
}

func NewFirestoreLocker(fsc *firestore.Client, path string) *FirestoreLocker {
	locker := &FirestoreLocker{
		Client:         fsc,
		CollectionPath: path,
	}
	return locker
}

type LockState struct {
	Lock            *Lock
	ReservedBy      *string
	ReservedExpires *time.Time
	Release         func() error
	Metadata        map[string]interface{}
}

func NewLockState() *LockState {
	mdta := make(map[string]interface{})
	return &LockState{
		Metadata: mdta,
	}
}

func (locker *FirestoreLocker) SaveLockState(ctx context.Context, ls *LockState) error {
	preconds := []firestore.Precondition{}
	log.Printf("sls")
	updateTime, ok := ls.Metadata["updateTime"]
	if ok {
		preconds = append(preconds, firestore.LastUpdateTime(updateTime.(time.Time)))
	}
	log.Printf("sls2")
	fls := &FirestoreLockState{}
	if ls != nil && ls.Lock != nil {
		log.Printf("sls4")
		fls.SetLock(ls.Lock)
		ref := locker.Client.Doc(locker.CollectionPath + "/" + ls.Lock.Name)
		_, err := ref.Update(
			ctx,
			[]firestore.Update{
				firestore.Update{Path: "lock", Value: fls.Lock},
			},
			preconds...,
		)
		log.Printf("sls3")
		if err != nil {
			return err
		}
		log.Printf("sls5")
	}
	return nil
}

func (locker *FirestoreLocker) GetLockState(ctx context.Context, lockName string, reserve bool) (*LockState, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	log.Printf("gls0 %v", m.Alloc)
	lockState := NewLockState()
	snap, err := locker.Client.Doc(locker.CollectionPath + "/" + lockName).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			lock := &Lock{Name: lockName}
			fls := &FirestoreLockState{}
			fls.SetLock(lock)
			_, err = locker.Client.Doc(locker.CollectionPath+"/"+lockName).Set(ctx, fls)
			snap, err = locker.Client.Doc(locker.CollectionPath + "/" + lockName).Get(ctx)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, err
		}

	}
	lockState.Metadata["updateTime"] = snap.UpdateTime
	lockState.Lock = &Lock{}

	fls := &FirestoreLockState{}
	if err := snap.DataTo(fls); err == nil {
		lockState.Lock = fls.GetLock()
	} else {
		return nil, err
	}
	log.Printf("gls1 %v", m.Alloc)
	return lockState, nil
}

func (locker *FirestoreLocker) GetAllLocks(ctx context.Context) ([]*Lock, error) {
	var result []*Lock
	refs := locker.Client.Collection(locker.CollectionPath).DocumentRefs(ctx)
	for {
		item, err := refs.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return result, err
		}

		snap, err := item.Get(ctx)

		fls := &FirestoreLockState{}
		if err := snap.DataTo(fls); err == nil {
			lock := fls.GetLock()
			result = append(result, lock)
		}
	}
	return result, nil
}

// TODO: extract to locker independent code
func (locker *FirestoreLocker) Acquire(ctx context.Context, l *Lock) error {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	log.Printf("dupa0 %v", m.Alloc)
	lockState, err := locker.GetLockState(ctx, l.Name, true)
	if err != nil {
		return err
	}
	defer lockState.Release()

	force := false
	if l.forceCondition != nil {
		force, err = lockState.Lock.Evaluate(l.forceCondition)
		if err != nil {
			return err
		}
	}

	if lockState.Lock.LockType == "" {
		lockState.Lock.LockType = LockTypeMutex
	}

	leaseCount := len(lockState.Lock.Leases)
	if lockState.Lock.LockType == LockTypeMutex && leaseCount > 0 {
		if leaseCount > 1 {
			return fmt.Errorf("Invalid number of leases for mutex lock: %d", leaseCount)
		}
		for key, lease := range lockState.Lock.Leases {
			if key != l.InstanceID && !lease.Expired() && !force {
				return fmt.Errorf("Mutex lock is already held by %s", lease.InstanceID)
			}
		}
	}

	if lockState.Lock.LockType == LockTypeMutex {
		lockState.Lock.Leases = map[string]LockLease{
			l.InstanceID: LockLease{InstanceID: l.InstanceID, Expires: l.NewExpiryTime()},
		}
	} else {
		return fmt.Errorf("Non-mutex locks not implemented yet")
	}

	syncLockFields(l, lockState.Lock)

	log.Printf("dupa1 %v", m.Alloc)
	err = locker.SaveLockState(ctx, lockState)
	log.Printf("dupa2 %v", m.Alloc)
	return err
}

func (locker *FirestoreLocker) Renew(ctx context.Context, l *Lock) error {
	lockState, err := locker.GetLockState(ctx, l.Name, true)
	if err != nil {
		return err
	}
	defer lockState.Release()

	lease, exists := lockState.Lock.Leases[l.InstanceID]
	if !exists {
		return fmt.Errorf("No lease to renew for %s", l.InstanceID)
	}
	if lease.Expired() {
		return fmt.Errorf("Lease on lock %s for %s already expired", l.Name, l.InstanceID)
	}
	lease.Expires = l.NewExpiryTime()
	lockState.Lock.Leases[l.InstanceID] = lease

	return locker.SaveLockState(ctx, lockState)
}

func (locker *FirestoreLocker) Release(ctx context.Context, l *Lock) error {
	lockState, err := locker.GetLockState(ctx, l.Name, true)
	if err != nil {
		return err
	}
	defer lockState.Release()

	delete(lockState.Lock.Leases, l.InstanceID)
	syncLockFields(l, lockState.Lock)

	return locker.SaveLockState(ctx, lockState)
}
