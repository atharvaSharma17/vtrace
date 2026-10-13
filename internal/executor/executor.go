package executor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"sync"
)

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}
type Handle struct {
	cmd    *exec.Cmd
	out    bytes.Buffer
	err    bytes.Buffer
	done   chan struct{}
	result Result
	once   sync.Once
}

func Start(ctx context.Context, name string, args ...string) (*Handle, error) {
	h := &Handle{done: make(chan struct{})}
	h.cmd = exec.CommandContext(ctx, name, args...)
	configure(h.cmd)
	h.cmd.Stdout = &h.out
	h.cmd.Stderr = &h.err
	if e := h.cmd.Start(); e != nil {
		return nil, e
	}
	go func() {
		e := h.cmd.Wait()
		code := 0
		if e != nil {
			code = -1
			if h.cmd.ProcessState != nil {
				code = h.cmd.ProcessState.ExitCode()
			}
		}
		h.result = Result{h.out.String(), h.err.String(), code, e}
		close(h.done)
	}()
	return h, nil
}
func (h *Handle) PID() int { return h.cmd.Process.Pid }
func (h *Handle) Alive() bool {
	select {
	case <-h.done:
		return false
	default:
		return true
	}
}
func (h *Handle) Wait() Result { <-h.done; return h.result }
func (h *Handle) Stop()        { h.once.Do(func() { kill(h.cmd) }); <-h.done }
func Run(ctx context.Context, name string, args ...string) Result {
	h, e := Start(ctx, name, args...)
	if e != nil {
		return Result{ExitCode: -1, Err: fmt.Errorf("start %s: %w", name, e)}
	}
	return h.Wait()
}
