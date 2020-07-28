package lockheed

import (
	"context"
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
	Name         string                            `json:"name"`
	InstanceID   string                            `json:"-"`
	StopChan     chan interface{}                  `json:"-"`
	EventChan    chan Event                        `json:"-"`
	EventHandler func(context.Context, chan Event) `json:"-"`
	Context      context.Context                   `json:"-"`
	Cancel       func()                            `json:"-"`
	Options      Options                           `json:"-"`
	Locker       LockerInterface                   `json:"-"`
	State        *LockState                        `json:"-"`
	StateStore   interface{}                       `json:"-"`
	mutex        sync.Mutex
}

type LockState struct {
	LockType LockType             `json:"lockType"`
	Leases   map[string]LockLease `json:"leases"`
}

type LockLease struct {
	InstanceID string    `json:"instanceID"`
	Expires    time.Time `json:"expires"`
}

type Options struct {
	Duration      time.Duration `json:"duration"`
	RenewInterval time.Duration `json:"renewInterval"`
	MaxLeases     *int          `json:"maxLeases"`
	Takeover      *bool         `json:"takeover"`
}

func DefaultEventHandler(ctx context.Context, echan chan Event) {
	for {
		select {
		case event := <-echan:
			log.Printf("Event: %s\n", event.Message)
		case <-ctx.Done():
			return
		}
	}
}

func NewLock(name string, ctx context.Context, locker LockerInterface, opts Options) *Lock {
	l := &Lock{
		Name:         name,
		InstanceID:   uuid.New().String(),
		StopChan:     make(chan interface{}),
		EventChan:    make(chan Event),
		EventHandler: DefaultEventHandler,
		Locker:       locker,
		Options:      opts,
	}
	l.Context, l.Cancel = context.WithCancel(ctx)
	go l.EventHandler(l.Context, l.EventChan)
	return l
}

func (l *Lock) Acquire() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if err := l.Locker.Acquire(l); err != nil {
		l.EmitAcquireFailed(err)
		return err
	}
	if l.Options.RenewInterval.Seconds() != 0 {
		go l.Maintain(l.StopChan)
	}
	l.EmitAcquireSuccessful()
	return nil
}

func (l *Lock) Release() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if err := l.Locker.Release(l); err != nil {
		l.EmitReleaseFailed(err)
		return err
	}
	l.EmitReleaseSuccessful()
	return nil
}

func (l *Lock) Renew() error {
	l.mutex.Lock()
	defer l.mutex.Unlock()
	if err := l.Locker.Renew(l); err != nil {
		l.EmitRenewFailed(err)
		return err
	}
	l.EmitRenewSuccessful()
	return nil
}

func (l *Lock) Maintain(stopchan chan interface{}) {
	l.EmitMaintainStarted()
	ticks := time.Tick(l.Options.RenewInterval)
	for {
		select {
		case <-l.Context.Done():
			l.Release()
			break
		case <-ticks:
			if err := l.Renew(); err != nil {
				continue
			}
		case <-stopchan:
			break
		}
	}
	l.EmitMaintainStopped()
}

func (l *Lock) NewExpiryTime() time.Time {
	return time.Now().Add(l.Options.Duration)
}

// func (l *Lock) GetEventChan() chan LockEvent {
// 	return l.EventChan
// }

// func (l *Lock) GetID() string {
// 	return l.ID
// }

// func (l *Lock) GetContext() context.Context {
// 	return l.Context
// }

// func (l *Lock) GetAutoRenew() *time.Duration {
// 	return l.Options.AutoRenew
// }

// func Acquire(ctx context.Context, l LockInterface) error {
// 	l.Init()
// 	l.Acquire()
// 	if l.GetAutoRenew() != nil {
// 		go maintain(l)
// 	}
// 	return nil
// }

// func maintain(l LockInterface) {
// 	for{
// 		select {
// 		case <-time.Tick(*l.GetAutoRenew()):
// 			if err := l.Renew(); err != nil {
// 				l.GetEventChan() <-EventRenewFailed(l, err)
// 			}
// 		case <-l.GetContext().Done():
// 			l.Release()
// 		}
// 	}
// }

// func Renew(l LockInterface) error {
// 	return l.Renew()
// }

// func Release(l LockInterface) error {
// 	return l.Release()
// }

// func maintain(l LockInterface) {
// 	for{
// 		select {
// 		case <-time.Tick(*l.GetAutoRenew()):
// 			if err := l.Renew(); err != nil {
// 				l.GetEventChan() <-EventRenewFailed(l, err)
// 			}
// 		case <-l.GetContext().Done():
// 			l.Release()
// 		}
// 	}
// }
