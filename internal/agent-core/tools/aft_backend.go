package tools

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
)

type AftBackend struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	reader  *bufio.Reader
	mu      sync.Mutex
	nextID  atomic.Int64
	closed  bool
	OnCrash func(msg string)
}

var (
	aftOnce sync.Once
	aftInst *AftBackend
	aftErr  error
)

var aftCrashReporter = func(msg string) {}

func SetAftCrashReporter(fn func(msg string)) {
	if fn == nil {
		fn = func(msg string) {}
	}
	aftCrashReporter = fn
	if aftInst != nil && aftInst.OnCrash == nil {
		aftInst.OnCrash = fn
	}
}

func initAftBackend() {
	if os.Getenv("AWP_NO_AFT") == "1" {
		aftErr = fmt.Errorf("AFT disabled via AWP_NO_AFT=1")
		return
	}
	binPath := os.Getenv("AWP_TEST_AFT")
	if binPath == "" {
		resolved, err := exec.LookPath("aft")
		if err != nil {
			aftErr = fmt.Errorf("aft not found in PATH: %w", err)
			return
		}
		binPath = resolved
	}
	aftInst, aftErr = NewAftBackend(binPath)
	if aftInst != nil && aftInst.OnCrash == nil {
		aftInst.OnCrash = aftCrashReporter
	}
}

func AftBackendForTest() (*AftBackend, bool) {
	aftOnce.Do(initAftBackend)
	return aftInst, aftErr == nil
}

func ResetAftBackendForTest() {
	aftOnce = sync.Once{}
	if aftInst != nil {
		_ = aftInst.Close()
	}
	aftInst = nil
	aftErr = nil
}

func NewAftBackend(binPath string) (*AftBackend, error) {
	cmd := exec.Command(binPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("aft stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("aft stdout pipe: %w", err)
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("aft start: %w", err)
	}
	return &AftBackend{
		cmd:    cmd,
		stdin:  stdin,
		reader: bufio.NewReader(stdout),
	}, nil
}

func (a *AftBackend) Call(name string, params map[string]any) (string, error) {
	return a.call(name, params, false)
}

func (a *AftBackend) CallNested(name string, params map[string]any) (string, error) {
	return a.call(name, params, true)
}

func (a *AftBackend) call(name string, params map[string]any, nested bool) (string, error) {
	id := a.nextID.Add(1)
	reqID := fmt.Sprintf("awp-%d", id)

	req := make(map[string]any, len(params)+3)
	req["id"] = reqID
	req["command"] = name
	if nested {
		req["params"] = params
	} else {
		for k, v := range params {
			if k == "id" || k == "command" {
				continue
			}
			req[k] = v
		}
	}
	b, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("aft marshal: %w", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return "", fmt.Errorf("aft backend closed")
	}
	if _, err := a.stdin.Write(append(b, '\n')); err != nil {
		if a.OnCrash != nil {
			a.OnCrash("aft write: " + err.Error())
		}
		return "", fmt.Errorf("aft write: %w", err)
	}

	for {
		line, err := a.reader.ReadBytes('\n')
		if err != nil {
			if a.OnCrash != nil {
				a.OnCrash("aft read: " + err.Error())
			}
			return "", fmt.Errorf("aft read: %w", err)
		}
		var resp map[string]any
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}
		gotID, _ := resp["id"].(string)
		if gotID != reqID {
			continue
		}
		if success, _ := resp["success"].(bool); !success {
			msg, _ := resp["message"].(string)
			return "", fmt.Errorf("aft %s failed: %s", name, msg)
		}
		out, err := json.Marshal(resp)
		if err != nil {
			return "", fmt.Errorf("aft re-marshal: %w", err)
		}
		return string(out), nil
	}
}

func (a *AftBackend) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	a.mu.Unlock()

	_ = a.stdin.Close()
	return a.cmd.Wait()
}
