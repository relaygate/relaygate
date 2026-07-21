package ops

import (
	"io"
	"os"
	"sync"
)

var captureMu sync.Mutex

// CaptureStdout runs fn while capturing os.Stdout (serialized).
func CaptureStdout(fn func() error) (string, error) {
	captureMu.Lock()
	defer captureMu.Unlock()

	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	fnErr := fn()
	_ = w.Close()
	os.Stdout = old
	out := <-done
	_ = r.Close()
	return out, fnErr
}

// DrainCapture runs Drain and returns captured stdout.
func DrainCapture(root, action string) (string, error) {
	return CaptureStdout(func() error { return Drain(root, action) })
}

// SmokeCapture runs Smoke and returns captured stdout.
func SmokeCapture(root, host string) (string, error) {
	return CaptureStdout(func() error { return Smoke(root, host) })
}

// CanaryCapture runs Canary and returns captured stdout.
func CanaryCapture(root, host string) (string, error) {
	return CaptureStdout(func() error { return Canary(root, host) })
}

// RollbackCapture runs Rollback and returns captured stdout.
func RollbackCapture(root, stamp string) (string, error) {
	return CaptureStdout(func() error { return Rollback(root, stamp) })
}

// FirewallCapture runs Firewall and returns captured stdout.
func FirewallCapture(root string, apply bool) (string, error) {
	return CaptureStdout(func() error { return Firewall(root, apply) })
}

// FirewallApplyCapture runs FirewallApplyConfirmed and returns captured stdout.
func FirewallApplyCapture(root string) (string, error) {
	return CaptureStdout(func() error { return FirewallApplyConfirmed(root) })
}
