package task

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ZicBoard/ZicNode/common/reload"
	log "github.com/sirupsen/logrus"
)

type Task struct {
	Name     string
	NodeTag  string
	Interval time.Duration
	Execute  func(context.Context) error
	Access   sync.RWMutex
	Running  bool
	ReloadCh chan reload.Event
	Stop     chan struct{}
	changed  chan struct{}
}

func (t *Task) Start(first bool) error {
	t.Access.Lock()
	if t.Running {
		t.Access.Unlock()
		return nil
	}
	t.Running = true
	t.Stop = make(chan struct{})
	t.changed = make(chan struct{}, 1)
	t.Access.Unlock()
	go func() {
		if first {
			if err := t.ExecuteWithTimeout(); err != nil {
				log.Errorf("Task %s execution error: %v", t.Name, err)
			}
		}

		interval := t.currentInterval()
		timer := time.NewTimer(interval)
		defer timer.Stop()
		for {
			select {
			case <-timer.C:
				// continue
			case <-t.Stop:
				return
			case <-t.changed:
				interval = t.currentInterval()
				resetTimer(timer, interval)
				continue
			}

			if err := t.ExecuteWithTimeout(); err != nil {
				log.Errorf("Task %s execution error: %v", t.Name, err)
			}
			interval = t.currentInterval()
			resetTimer(timer, interval)
		}
	}()

	return nil
}

func (t *Task) ExecuteWithTimeout() error {
	interval := t.currentInterval()
	timeout := min(5*interval, 5*time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	done := make(chan error, 1)

	go func() {
		done <- t.Execute(ctx)
	}()

	select {
	case <-ctx.Done():
		fields := log.Fields{
			"reason":   reload.ReasonTaskTimeout,
			"action":   "skip_and_keep_running",
			"task":     t.Name,
			"timeout":  timeout.String(),
			"interval": interval.String(),
		}
		if t.NodeTag != "" {
			fields["tag"] = t.NodeTag
		}
		// A timeout usually means the panel is temporarily unreachable. Do not
		// reload the core: keep serving with the last known data and retry on
		// the next tick.
		log.WithFields(fields).Warn("task execution timed out, keep running with last known data")
		return nil
	case err := <-done:
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		return err
	}
}

func (t *Task) SetInterval(interval time.Duration) {
	t.Access.Lock()
	changed := t.Interval != interval
	t.Interval = interval
	running := t.Running
	changedCh := t.changed
	t.Access.Unlock()
	if !changed || !running || changedCh == nil {
		return
	}
	select {
	case changedCh <- struct{}{}:
	default:
	}
}

func (t *Task) currentInterval() time.Duration {
	t.Access.RLock()
	defer t.Access.RUnlock()
	return t.Interval
}

func resetTimer(timer *time.Timer, interval time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(interval)
}

func (t *Task) safeStop() {
	t.Access.Lock()
	if t.Running {
		t.Running = false
		close(t.Stop)
	}
	t.Access.Unlock()
}

func (t *Task) Close() {
	t.safeStop()
	log.Warningf("Task %s stopped", t.Name)
}
