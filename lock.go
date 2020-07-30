package lockheed

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	LockTypeMutex LockType = "mutex"
)

type LockType string

type Lock struct {
	Name       string               `json:"name"`
	LockType   LockType             `json:"lockType"`
	Leases     map[string]LockLease `json:"leases"`
	InstanceID string               `json:"-"`
	Context    context.Context      `json:"-"`
	Cancel     func()               `json:"-"`
	Locker     LockerInterface      `json:"-"`
	Options
	stopChan     chan interface{}
	eventChan    chan Event
	eventHandler func(context.Context, chan Event)
	initialized  bool
	maintained   bool
	mutex        sync.Mutex
}

type LockLease struct {
	InstanceID string    `json:"instanceID"`
	Expires    time.Time `json:"expires"`
}

func (lease *LockLease) Expired() bool {
	if time.Now().Before(lease.Expires) {
		return false
	}
	return true
}

type Options struct {
	Tags           []string      `json:"tags,omitempty"`
	Duration       time.Duration `json:"-"`
	RenewInterval  time.Duration `json:"-"`
	MaxLeases      *int          `json:"-"`
	Takeover       *bool         `json:"-"`
	resetTags      bool
	forceCondition *Condition
}

func DefaultEventHandler(ctx context.Context, echan chan Event) {
	for {
		select {
		case event := <-echan:
			log.Printf("Event: %s\n", event.Message)
			if event.Err != nil {
				log.Printf("Error: %s\n", event.Err.Error())
			}
		case <-ctx.Done():
			return
		}
	}
}

func NewLock(name string, locker LockerInterface) *Lock {
	l := &Lock{Name: name}
	l.Locker = locker
	l = l.WithContext(context.Background())
	l.Init()
	return l
}

func (l *Lock) WithContext(ctx context.Context) *Lock {
	l.Context, l.Cancel = context.WithCancel(ctx)
	return l
}

func (l *Lock) WithDuration(duration time.Duration) *Lock {
	l.Duration = duration
	return l
}

func (l *Lock) WithTags(tags []string) *Lock {
	l.Tags = tags
	return l
}

func (l *Lock) WithResetTags() *Lock {
	l.resetTags = true
	return l
}

func (l *Lock) WithForce(c Condition) *Lock {
	l.forceCondition = &c
	return l
}

func (l *Lock) WithRenewInterval(interval time.Duration) *Lock {
	l.RenewInterval = interval
	return l
}

func (l *Lock) Init() {
	l.InstanceID = uuid.New().String()
	l.stopChan = make(chan interface{})
	l.eventChan = make(chan Event)
	l.eventHandler = DefaultEventHandler
	go l.eventHandler(l.Context, l.eventChan)
	l.initialized = true
}

func (l *Lock) Acquire() error {
	if !l.initialized {
		return fmt.Errorf("Lock needs to be properly initialized first")
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if err := l.Locker.Acquire(l); err != nil {
		l.EmitAcquireFailed(err)
		return err
	}
	if l.RenewInterval.Seconds() != 0 {
		go l.Maintain()
	}
	l.EmitAcquireSuccessful()
	return nil
}

func (l *Lock) AcquireRetry(retries int, delay time.Duration) error {
	if !l.initialized {
		return fmt.Errorf("Lock needs to be properly initialized first")
	}
	var err error
	attempt := 0
	for {
		attempt++
		err = l.Acquire()
		if err != nil {
			if attempt <= retries {
				time.Sleep(delay)
				continue
			}
		}
		break
	}
	return err
}

func (l *Lock) Release() error {
	if !l.initialized {
		return fmt.Errorf("Lock needs to be properly initialized first")
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if err := l.Locker.Release(l); err != nil {
		l.EmitReleaseFailed(err)
		return err
	}
	l.maintained = false
	l.EmitReleaseSuccessful()
	return nil
}

func (l *Lock) Renew() error {
	if !l.initialized {
		return fmt.Errorf("Lock needs to be properly initialized first")
	}
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if err := l.Locker.Renew(l); err != nil {
		l.EmitRenewFailed(err)
		return err
	}
	l.EmitRenewSuccessful()
	return nil
}

func (l *Lock) Maintain() {
	l.maintained = true
	l.EmitMaintainStarted()
	ticks := time.Tick(l.RenewInterval)
	for {
		select {
		case <-l.Context.Done():
			l.maintained = false
			l.EmitMaintainStopped()
			return
		case <-ticks:
			if !l.maintained {
				l.EmitMaintainStopped()
				return
			}
			l.Renew()
		}
	}
}

func (l *Lock) NewExpiryTime() time.Time {
	if l.Duration == 0 {
		return time.Date(9999, time.December, 31, 23, 59, 59, 0, nil)
	}
	return time.Now().Add(l.Duration)
}

func stringInSlice(pool []string, item string) bool {
	for _, elem := range pool {
		if elem == item {
			return true
		}
	}
	return false
}

func syncLockFields(src *Lock, dst *Lock) {
	// tags are only added to locks, not removed
	if src.resetTags {
		dst.Tags = src.Tags
	} else {
		for _, tag := range src.Tags {
			if !stringInSlice(dst.Tags, tag) {
				dst.Tags = append(dst.Tags, tag)
			}
		}
	}
}
