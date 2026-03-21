package pyffi

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
)

// EventLoop manages a persistent asyncio event loop running in a dedicated
// goroutine. Coroutines can be submitted from any goroutine via Go or Submit,
// and results are delivered through channels.
//
// The event loop goroutine holds the GIL while executing Python code, but
// releases it during I/O waits, allowing other goroutines to acquire the GIL
// for their own Python operations.
type EventLoop struct {
	rt      *Runtime
	loop    uintptr // asyncio event loop (Python object, IncRef'd)
	stopped atomic.Bool
	done    chan struct{} // closed when the event loop goroutine exits

	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan AsyncResult
}

// StartEventLoop creates an asyncio event loop and runs it in a background
// goroutine. The GIL must be held by the caller (i.e., do NOT call ReleaseGIL
// before this).
//
// Call Stop to shut down the event loop when done.
func (r *Runtime) StartEventLoop() (*EventLoop, error) {
	g := r.autoGIL()
	defer g.release()
	if r.closed.Load() {
		return nil, ErrAlreadyClosed
	}

	// Import asyncio and create a new event loop.
	asyncio, err := r.Import("asyncio")
	if err != nil {
		return nil, fmt.Errorf("pyffi: StartEventLoop: %w", err)
	}
	defer asyncio.Close()

	newLoopFn := asyncio.Attr("new_event_loop")
	if newLoopFn == nil {
		return nil, fmt.Errorf("pyffi: StartEventLoop: asyncio.new_event_loop not found")
	}
	defer newLoopFn.Close()

	loopObj, err := newLoopFn.Call()
	if err != nil {
		return nil, fmt.Errorf("pyffi: StartEventLoop: %w", err)
	}

	// Keep a raw pointer and IncRef for the event loop goroutine.
	r.pyIncRef(loopObj.ptr)
	loopPtr := loopObj.ptr
	loopObj.Close()

	// Register a Go callback to receive completion notifications.
	el := &EventLoop{
		rt:      r,
		loop:    loopPtr,
		done:    make(chan struct{}),
		pending: make(map[int64]chan AsyncResult),
	}

	if err := r.RegisterFunc("__pyffi_el_done", el.onDone); err != nil {
		r.pyDecRef(loopPtr)
		return nil, fmt.Errorf("pyffi: StartEventLoop: register callback: %w", err)
	}

	// Set up the Python-side done callback wrapper.
	if err := r.Exec(`
import go_bridge
def __pyffi_el_notify(task_id, loop):
    def _cb(future):
        try:
            exc = future.exception()
            if exc is not None:
                go_bridge.__pyffi_el_done(task_id, "", str(exc))
            else:
                go_bridge.__pyffi_el_done(task_id, "ok", "")
        except Exception as e:
            go_bridge.__pyffi_el_done(task_id, "", str(e))
    return _cb
`); err != nil {
		r.pyDecRef(loopPtr)
		return nil, fmt.Errorf("pyffi: StartEventLoop: setup notify: %w", err)
	}

	// Launch the event loop in a dedicated goroutine.
	go el.run()

	return el, nil
}

// run executes the event loop. Runs in a dedicated goroutine.
func (el *EventLoop) run() {
	defer close(el.done)

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	state := el.rt.pyGILStateEnsure()
	defer el.rt.pyGILStateRelease(state)

	// asyncio.set_event_loop(loop) for this thread.
	asyncio := el.rt.pyImportModule("asyncio")
	if asyncio != 0 {
		setLoop := el.rt.pyObjectGetAttrString(asyncio, "set_event_loop")
		if setLoop != 0 {
			args := el.rt.pyTupleNew(1)
			el.rt.pyIncRef(el.loop)
			el.rt.pyTupleSetItem(args, 0, el.loop)
			result := el.rt.pyObjectCall(setLoop, args, 0)
			if result != 0 {
				el.rt.pyDecRef(result)
			}
			el.rt.pyDecRef(args)
			el.rt.pyDecRef(setLoop)
		}
		el.rt.pyDecRef(asyncio)
	}

	// loop.run_forever()
	runForever := el.rt.pyObjectGetAttrString(el.loop, "run_forever")
	if runForever != 0 {
		emptyArgs := el.rt.pyTupleNew(0)
		result := el.rt.pyObjectCall(runForever, emptyArgs, 0)
		if result != 0 {
			el.rt.pyDecRef(result)
		} else {
			el.rt.clearError()
		}
		el.rt.pyDecRef(emptyArgs)
		el.rt.pyDecRef(runForever)
	}

	// loop.close()
	closeFn := el.rt.pyObjectGetAttrString(el.loop, "close")
	if closeFn != 0 {
		emptyArgs := el.rt.pyTupleNew(0)
		result := el.rt.pyObjectCall(closeFn, emptyArgs, 0)
		if result != 0 {
			el.rt.pyDecRef(result)
		} else {
			el.rt.clearError()
		}
		el.rt.pyDecRef(emptyArgs)
		el.rt.pyDecRef(closeFn)
	}

	el.rt.pyDecRef(el.loop)
}

// onDone is the Go callback invoked when a submitted task completes.
// Called from the event loop thread (with GIL held).
func (el *EventLoop) onDone(taskID int, status string, errMsg string) {
	el.mu.Lock()
	ch, ok := el.pending[int64(taskID)]
	if ok {
		delete(el.pending, int64(taskID))
	}
	el.mu.Unlock()

	if !ok {
		return
	}

	if status == "ok" {
		ch <- AsyncResult{Err: nil}
	} else {
		ch <- AsyncResult{Err: fmt.Errorf("pyffi: async task failed: %s", errMsg)}
	}
}

// Go submits a coroutine created by calling asyncFn(args...) to the event loop.
// Returns a channel that receives the result when the coroutine completes.
//
// The caller must hold the GIL when calling this method (e.g., inside WithGIL).
// The result Object in the channel must also be accessed with the GIL held.
func (el *EventLoop) Go(asyncFn *Object, args ...any) (<-chan AsyncResult, error) {
	g := el.rt.autoGIL()
	defer g.release()
	if el.stopped.Load() {
		return nil, fmt.Errorf("pyffi: EventLoop is stopped")
	}

	// Call the async function to get the coroutine.
	coro, err := asyncFn.Call(args...)
	if err != nil {
		return nil, err
	}

	return el.Submit(coro)
}

// Submit submits an existing coroutine object to the event loop.
// The coroutine will be closed after submission.
//
// The caller must hold the GIL when calling this method.
func (el *EventLoop) Submit(coro *Object) (<-chan AsyncResult, error) {
	g := el.rt.autoGIL()
	defer g.release()
	if el.stopped.Load() {
		coro.Close()
		return nil, fmt.Errorf("pyffi: EventLoop is stopped")
	}

	rt := el.rt

	// Allocate a task ID and result channel.
	el.mu.Lock()
	taskID := el.nextID
	el.nextID++
	ch := make(chan AsyncResult, 1)
	el.pending[taskID] = ch
	el.mu.Unlock()

	// asyncio.run_coroutine_threadsafe(coro, loop) → future
	asyncio, err := rt.Import("asyncio")
	if err != nil {
		coro.Close()
		el.removePending(taskID, ch)
		return nil, fmt.Errorf("pyffi: Submit: %w", err)
	}
	defer asyncio.Close()

	rctsFn := asyncio.Attr("run_coroutine_threadsafe")
	if rctsFn == nil {
		coro.Close()
		el.removePending(taskID, ch)
		return nil, fmt.Errorf("pyffi: Submit: asyncio.run_coroutine_threadsafe not found")
	}
	defer rctsFn.Close()

	loopObj := rt.newObject(el.loop)
	rt.pyIncRef(el.loop) // newObject takes ownership, but we want to keep our ref
	defer loopObj.Close()

	future, err := rctsFn.Call(coro, loopObj)
	coro.Close()
	if err != nil {
		el.removePending(taskID, ch)
		return nil, fmt.Errorf("pyffi: Submit: %w", err)
	}
	defer future.Close()

	// future.add_done_callback(__pyffi_el_notify(task_id, loop))
	mainMod, err := rt.Import("__main__")
	if err != nil {
		el.removePending(taskID, ch)
		return nil, fmt.Errorf("pyffi: Submit: %w", err)
	}
	defer mainMod.Close()

	notifyFn := mainMod.Attr("__pyffi_el_notify")
	if notifyFn == nil {
		el.removePending(taskID, ch)
		return nil, fmt.Errorf("pyffi: Submit: __pyffi_el_notify not found")
	}
	defer notifyFn.Close()

	cb, err := notifyFn.Call(int(taskID), loopObj)
	if err != nil {
		el.removePending(taskID, ch)
		return nil, fmt.Errorf("pyffi: Submit: create callback: %w", err)
	}
	defer cb.Close()

	addCbFn := future.Attr("add_done_callback")
	if addCbFn == nil {
		el.removePending(taskID, ch)
		return nil, fmt.Errorf("pyffi: Submit: add_done_callback not found")
	}
	defer addCbFn.Close()

	_, err = addCbFn.Call(cb)
	if err != nil {
		el.removePending(taskID, ch)
		return nil, fmt.Errorf("pyffi: Submit: %w", err)
	}

	return ch, nil
}

func (el *EventLoop) removePending(id int64, ch chan AsyncResult) {
	el.mu.Lock()
	delete(el.pending, id)
	el.mu.Unlock()
	close(ch)
}

// Stop signals the event loop to stop and waits for it to finish.
func (el *EventLoop) Stop() error {
	if el.stopped.Swap(true) {
		return nil // already stopped
	}

	// Phase 1: tell the loop to stop (needs GIL).
	func() {
		g := el.rt.autoGIL()
		defer g.release()

		callSoon := el.rt.pyObjectGetAttrString(el.loop, "call_soon_threadsafe")
		if callSoon == 0 {
			el.rt.clearError()
			return
		}
		defer el.rt.pyDecRef(callSoon)

		stopFn := el.rt.pyObjectGetAttrString(el.loop, "stop")
		if stopFn == 0 {
			el.rt.clearError()
			return
		}
		defer el.rt.pyDecRef(stopFn)

		args := el.rt.pyTupleNew(1)
		el.rt.pyIncRef(stopFn)
		el.rt.pyTupleSetItem(args, 0, stopFn)
		result := el.rt.pyObjectCall(callSoon, args, 0)
		el.rt.pyDecRef(args)
		if result == 0 {
			el.rt.clearError()
		} else {
			el.rt.pyDecRef(result)
		}
	}()

	// Phase 2: wait for the event loop goroutine to finish (no GIL needed).
	<-el.done
	return nil
}
