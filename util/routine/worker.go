package routine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync"

	"csm/util/atomic"
	"csm/util/log"
	"csm/util/multierror"
)

const (
	_asyncWorkName  = "[async]"
	_daemonWorkName = "[daemon]"
)

var (
	errUnavailable = errors.New("service is unavailable")
	globalWorker   *worker
)

func init() {
	globalWorker = newWorker()
}

// Recover recover panic function
func Recover(handlers ...func(interface{})) {
	if r := recover(); r != nil {
		if len(handlers) > 0 {
			for _, fn := range handlers {
				fn(r)
			}
			return
		}

		LogPanic(true)
	}
}

// LogPanic recover handler, log the panic message
func LogPanic(throw bool) func(interface{}) {
	return func(r interface{}) {
		callers := ""
		for i := 0; true; i++ {
			_, file, line, ok := runtime.Caller(i)
			if !ok {
				break
			}
			callers = callers + fmt.Sprintf("%v:%v\n", file, line)
		}
		log.Errorf("Recovered from panic: %#v (%v)\n%v", r, r, callers)

		if throw {
			panic(r)
		}
	}
}

// RunWork run the func in the same routine
func RunWork(name string, f func() error, panicHandler ...func(interface{})) error {
	if !globalWorker.workPrelude(name, false) {
		return errUnavailable
	}

	defer Recover(panicHandler...)
	defer globalWorker.workPostlude(name, false)

	return f()
}

// RunWorkAsync run the func in new routine
func RunWorkAsync(name string, f func(), panicHandler ...func(interface{})) error {
	name = _asyncWorkName + name
	if !globalWorker.workPrelude(name, false) {
		return errUnavailable
	}

	GoWork(func() {
		defer Recover(panicHandler...)
		defer globalWorker.workPostlude(name, false)

		f()
	})

	return nil
}

// RunWorkDaemon run the func in new routine and run until stop
func RunWorkDaemon(name string, f func(), quit <-chan struct{}) error {
	name = _daemonWorkName + name
	if !globalWorker.workPrelude(name, true) {
		return errUnavailable
	}

	go func() {
		defer globalWorker.workPostlude(name, true)

		for {
			select {
			case <-globalWorker.stopping:
				return
			case <-quit:
				return

			default:
				func() {
					defer Recover(LogPanic(false))
					f()
				}()
			}
		}
	}()

	return nil
}

// Stop stop the service and wait all worker exit
func Stop() error {
	globalWorker.rwMu.Lock()
	select {
	case <-globalWorker.stopping:
		globalWorker.rwMu.Unlock()
		return nil
	default:
		close(globalWorker.stopping)
		for _, cancel := range globalWorker.cancels {
			cancel()
		}
	}
	globalWorker.rwMu.Unlock()

	globalWorker.stopWG.Wait()
	globalWorker.rwMu.RLock()
	defer globalWorker.rwMu.RUnlock()

	merr := &multierror.MultiError{}
	for _, c := range globalWorker.closers {
		merr.Append(c.Close())
	}
	close(globalWorker.stopped)
	return merr.ErrorOrNil()
}

// ShouldStop return the service is stopping
func ShouldStop() <-chan struct{} {
	return globalWorker.stopping
}

// IsStopped return the service has stopped
func IsStopped() <-chan struct{} {
	return globalWorker.stopped
}

// WorkNum return current number of works
func WorkNum() int64 {
	return globalWorker.numWorks.Get()
}

// AddCloser add close hook
func AddCloser(c io.Closer) {
	globalWorker.rwMu.Lock()
	globalWorker.closers = append(globalWorker.closers, c)
	globalWorker.rwMu.Unlock()
}

// AddCancel addcancel hook
func AddCancel(c context.Context) (ctx context.Context) {
	var cancel func()
	ctx, cancel = context.WithCancel(c)

	globalWorker.rwMu.Lock()
	globalWorker.cancels = append(globalWorker.cancels, cancel)
	globalWorker.rwMu.Unlock()
	return
}

// DebugString return the debug string
func DebugString() string {
	num := 0
	works := make([]string, 0, 16)
	globalWorker.workMap.Range(func(key, value interface{}) bool {
		works = append(works, fmt.Sprintf("[%s , %d]", key, value.(*atomic.AtomicInt64).Get()))
		num++
		return true
	})

	return fmt.Sprintf("[%d]works:\n%s", num, strings.Join(works, "\n"))
}

type worker struct {
	stopping chan struct{}
	stopped  chan struct{}
	stopWG   sync.WaitGroup
	workMap  *sync.Map
	numWorks *atomic.AtomicInt64

	rwMu    sync.RWMutex
	closers []io.Closer
	cancels []func()
}

func newWorker() *worker {
	return &worker{
		stopping: make(chan struct{}),
		stopped:  make(chan struct{}),
		workMap:  &sync.Map{},
		numWorks: atomic.NewAtomicInt64(0),
	}
}

func (s *worker) workPrelude(name string, daemon bool) bool {
	select {
	case <-s.stopping:
		return false

	default:
		if !daemon {
			s.stopWG.Add(1)
		}

		wnum, _ := s.workMap.Load(name)
		if wnum == nil {
			wnum, _ = s.workMap.LoadOrStore(name, atomic.NewAtomicInt64(0))
		}
		wnum.(*atomic.AtomicInt64).Incr()
		s.numWorks.Incr()
		return true
	}
}

func (s *worker) workPostlude(name string, daemon bool) {
	if !daemon {
		s.stopWG.Done()
	}

	if wnum, _ := s.workMap.Load(name); wnum != nil {
		if wnum.(*atomic.AtomicInt64).Decr() <= 0 {
			s.workMap.Delete(name)
		}
	}
	s.numWorks.Decr()
}