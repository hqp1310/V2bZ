package task

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ZicBoard/ZicNode/common/reload"
)

func TestTaskTimeoutDoesNotRequestReload(t *testing.T) {
	reloadCh := make(chan reload.Event, 1)
	task := &Task{
		Name:     "timeout-task",
		Interval: 10 * time.Millisecond,
		ReloadCh: reloadCh,
		Execute: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}

	if err := task.ExecuteWithTimeout(); err != nil {
		t.Fatalf("ExecuteWithTimeout returned error: %v", err)
	}
	select {
	case ev := <-reloadCh:
		t.Fatalf("unexpected reload event: %#v", ev)
	default:
	}
}

func TestTaskKeepsRunningAfterExecuteError(t *testing.T) {
	var runs atomic.Int32
	var once sync.Once
	ranAgain := make(chan struct{})
	task := &Task{
		Name:     "retry-task",
		Interval: 10 * time.Millisecond,
		Execute: func(context.Context) error {
			if runs.Add(1) == 1 {
				return errors.New("temporary failure")
			}
			once.Do(func() { close(ranAgain) })
			return nil
		},
	}

	if err := task.Start(false); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer task.Close()

	select {
	case <-ranAgain:
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("task did not retry after temporary error; runs=%d", runs.Load())
	}
}

func TestTaskSetIntervalResetsRunningTimer(t *testing.T) {
	ran := make(chan struct{})
	var once sync.Once
	task := &Task{
		Name:     "interval-task",
		Interval: time.Hour,
		Execute: func(context.Context) error {
			once.Do(func() { close(ran) })
			return nil
		},
	}

	if err := task.Start(false); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	defer task.Close()
	task.SetInterval(10 * time.Millisecond)

	select {
	case <-ran:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("task did not run after interval update")
	}
}
