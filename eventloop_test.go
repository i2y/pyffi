package pyffi_test

import (
	"sync"
	"testing"

	"github.com/i2y/pyffi"
)

func TestEventLoop(t *testing.T) {
	rt := newOrSkip(t)

	// Define async functions.
	if err := rt.Exec(`
import asyncio

async def async_double(x):
    await asyncio.sleep(0)
    return x * 2
`); err != nil {
		t.Fatalf("Exec failed: %v", err)
	}

	t.Run("StartStop", func(t *testing.T) {
		el, err := rt.StartEventLoop()
		if err != nil {
			t.Fatalf("StartEventLoop failed: %v", err)
		}
		if err := el.Stop(); err != nil {
			t.Fatalf("Stop failed: %v", err)
		}
	})

	t.Run("SubmitTask", func(t *testing.T) {
		el, err := rt.StartEventLoop()
		if err != nil {
			t.Fatalf("StartEventLoop failed: %v", err)
		}

		mod, _ := rt.Import("__main__")
		defer mod.Close()
		fn := mod.Attr("async_double")
		defer fn.Close()

		ch, err := el.Go(fn, 21)
		if err != nil {
			t.Fatalf("Go failed: %v", err)
		}

		ar := <-ch
		if ar.Err != nil {
			t.Fatalf("async task failed: %v", ar.Err)
		}

		if err := el.Stop(); err != nil {
			t.Fatalf("Stop failed: %v", err)
		}
	})

	t.Run("MultipleSubmits", func(t *testing.T) {
		el, err := rt.StartEventLoop()
		if err != nil {
			t.Fatalf("StartEventLoop failed: %v", err)
		}

		mod, _ := rt.Import("__main__")
		defer mod.Close()
		fn := mod.Attr("async_double")
		defer fn.Close()

		var channels []<-chan pyffi.AsyncResult
		for i := range 5 {
			ch, err := el.Go(fn, i+1)
			if err != nil {
				t.Fatalf("Go(%d) failed: %v", i, err)
			}
			channels = append(channels, ch)
		}

		for i, ch := range channels {
			ar := <-ch
			if ar.Err != nil {
				t.Errorf("task %d failed: %v", i, ar.Err)
			}
		}

		if err := el.Stop(); err != nil {
			t.Fatalf("Stop failed: %v", err)
		}
	})

	t.Run("ConcurrentGoroutines", func(t *testing.T) {
		el, err := rt.StartEventLoop()
		if err != nil {
			t.Fatalf("StartEventLoop failed: %v", err)
		}

		mod, _ := rt.Import("__main__")
		defer mod.Close()
		fn := mod.Attr("async_double")
		defer fn.Close()

		var wg sync.WaitGroup
		errs := make([]error, 5)

		for i := range 5 {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				ch, submitErr := el.Go(fn, idx+1)
				if submitErr != nil {
					errs[idx] = submitErr
					return
				}
				ar := <-ch
				errs[idx] = ar.Err
			}(i)
		}

		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Errorf("goroutine %d: %v", i, err)
			}
		}

		if err := el.Stop(); err != nil {
			t.Fatalf("Stop failed: %v", err)
		}
	})
}
