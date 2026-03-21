package pyffi

import "runtime"

// gilGuard is returned by autoGIL. Call release() when done.
type gilGuard struct {
	rt    *Runtime
	state int32
	held  bool
}

// autoGIL acquires the GIL for a public API method call.
// PyGILState_Ensure supports nesting, so calling autoGIL from a method
// that is called by another method with autoGIL is safe.
func (r *Runtime) autoGIL() gilGuard {
	if r.closed.Load() {
		return gilGuard{}
	}
	if !r.freeThreaded {
		runtime.LockOSThread()
	}
	state := r.pyGILStateEnsure()
	return gilGuard{rt: r, state: state, held: true}
}

func (g *gilGuard) release() {
	if !g.held {
		return
	}
	g.rt.pyGILStateRelease(g.state)
	if !g.rt.freeThreaded {
		runtime.UnlockOSThread()
	}
	g.held = false
}

// GILState represents a held GIL. It must be released by calling Release().
type GILState struct {
	state int32
	rt    *Runtime
}

// Release releases the GIL and unlocks the OS thread.
func (g *GILState) Release() {
	g.rt.pyGILStateRelease(g.state)
	if !g.rt.freeThreaded {
		runtime.UnlockOSThread()
	}
}

// GILEnsure acquires the GIL and locks the current goroutine to its OS thread.
// The caller must call Release() when done.
//
// With auto-GIL, most users do not need to call this directly.
// It is still useful for batching multiple operations under a single GIL acquisition.
func (r *Runtime) GILEnsure() *GILState {
	if !r.freeThreaded {
		runtime.LockOSThread()
	}
	state := r.pyGILStateEnsure()
	return &GILState{state: state, rt: r}
}

// WithGIL acquires the GIL, calls fn, then releases the GIL.
// The current goroutine is locked to its OS thread for the duration.
//
// With auto-GIL, most users do not need to call this directly.
// It is still useful for batching multiple operations under a single GIL acquisition.
func (r *Runtime) WithGIL(fn func() error) error {
	if !r.freeThreaded {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
	}
	state := r.pyGILStateEnsure()
	defer r.pyGILStateRelease(state)
	return fn()
}

// ThreadState is the saved thread state from releasing the init GIL.
type ThreadState struct {
	state uintptr
	rt    *Runtime
	noop  bool
}

// Restore is a no-op. It is retained for backward compatibility.
//
// Deprecated: With auto-GIL, the init GIL is released automatically by New().
// ReleaseGIL and Restore are no longer needed.
func (ts *ThreadState) Restore() {
	if ts.noop {
		return
	}
	ts.rt.pyEvalRestoreThread(ts.state)
}

// ReleaseGIL is a no-op that returns a dummy ThreadState.
// It is retained for backward compatibility.
//
// Deprecated: With auto-GIL, the init GIL is released automatically by New().
// All public methods acquire the GIL as needed.
func (r *Runtime) ReleaseGIL() *ThreadState {
	return &ThreadState{rt: r, noop: true}
}
