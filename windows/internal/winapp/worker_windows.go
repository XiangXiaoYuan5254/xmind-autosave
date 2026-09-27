package winapp

import "runtime"

// worker runs slow jobs — walking XMind's accessibility tree, reading
// shortcuts, writing diagnostics — on its own COM thread, so the UI thread
// (tray menu, toggle panel, input) never waits on XMind.
type worker struct {
	jobs         chan func()
	completions  chan func()
	notifyWindow uintptr
}

func startWorker(notifyWindow uintptr) *worker {
	w := &worker{
		jobs:         make(chan func(), 8),
		completions:  make(chan func(), 32),
		notifyWindow: notifyWindow,
	}
	go func() {
		runtime.LockOSThread()
		_ = initializeCOM()
		for job := range w.jobs {
			job()
		}
	}()
	return w
}

// submit queues a job; it returns false when the queue is full.
func (w *worker) submit(job func()) bool {
	select {
	case w.jobs <- job:
		return true
	default:
		return false
	}
}

// complete hands a callback back to the UI thread. Called on the worker.
func (w *worker) complete(callback func()) {
	w.completions <- callback
	postMessage(w.notifyWindow, wmAppWorkerDone, 0, 0)
}

// runCompletions runs the callbacks handed back so far. Called on the UI thread.
func (w *worker) runCompletions() {
	for {
		select {
		case callback := <-w.completions:
			callback()
		default:
			return
		}
	}
}
