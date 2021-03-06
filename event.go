package lockheed

import "fmt"

type Event struct {
	Code    int
	Message string
	Err     error
}

func (l *Lock) Emit(e Event) error {
	l.eventChan <- e
	return e.Err
}

func (l *Lock) EmitRenewSuccessful() {
	l.Emit(Event{
		Code:    211,
		Message: fmt.Sprintf("Lock %s(%s) renewal successful", l.Name, l.InstanceID),
		Err:     nil,
	})
}

func (l *Lock) EmitRenewFailed(err error) {
	l.Emit(Event{
		Code:    511,
		Message: fmt.Sprintf("Lock %s(%s) renewal failed", l.Name, l.InstanceID),
		Err:     err,
	})
}

func (l *Lock) EmitReleaseSuccessful() {
	l.Emit(Event{
		Code:    212,
		Message: fmt.Sprintf("Lock %s(%s) release successful", l.Name, l.InstanceID),
		Err:     nil,
	})
}

func (l *Lock) EmitReleaseFailed(err error) {
	l.Emit(Event{
		Code:    512,
		Message: fmt.Sprintf("Lock %s(%s) release failed", l.Name, l.InstanceID),
		Err:     err,
	})
}

func (l *Lock) EmitAcquireSuccessful() {
	l.Emit(Event{
		Code:    213,
		Message: fmt.Sprintf("Lock %s(%s) acquire successful", l.Name, l.InstanceID),
		Err:     nil,
	})
}

func (l *Lock) EmitAcquireFailed(err error) {
	l.Emit(Event{
		Code:    513,
		Message: fmt.Sprintf("Lock %s(%s) acquire failed", l.Name, l.InstanceID),
		Err:     err,
	})
}

func (l *Lock) EmitMaintainStarted() {
	l.Emit(Event{
		Code:    214,
		Message: fmt.Sprintf("Lock %s(%s) maintain loop started", l.Name, l.InstanceID),
		Err:     nil,
	})
}

func (l *Lock) EmitMaintainStopped() {
	l.Emit(Event{
		Code:    215,
		Message: fmt.Sprintf("Lock %s(%s) maintain loop stopped", l.Name, l.InstanceID),
		Err:     nil,
	})
}

func (l *Lock) EmitDebug(msg string) {
	l.Emit(Event{
		Code:    299,
		Message: fmt.Sprintf("Lock %s(%s) debug: %s", l.Name, l.InstanceID, msg),
		Err:     nil,
	})
}
