package ops

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"
)

// RunCmd runs a command in root with inherited env; streams to stdout/stderr.
func RunCmd(root string, name string, args ...string) error {
	return RunCmdIO(root, os.Stdout, os.Stderr, name, args...)
}

func RunCmdIO(root string, stdout, stderr io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.Env = os.Environ()
	return cmd.Run()
}

func RunCmdCapture(root string, name string, args ...string) (string, error) {
	var buf bytes.Buffer
	err := RunCmdIO(root, &buf, &buf, name, args...)
	return buf.String(), err
}

func LookPath(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func IsRoot() bool {
	return os.Geteuid() == 0
}

func HTTPPost(url string) error {
	cmd := exec.Command("curl", "-fsS", "-X", "POST", url)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run()
}

func HTTPGet(url string) (string, error) {
	out, err := exec.Command("curl", "-fsS", url).CombinedOutput()
	return string(out), err
}

func HTTPGetOK(url string) bool {
	cmd := exec.Command("curl", "-fsS", "--max-time", "3", url)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

func WaitHTTP(url string, attempts int, sleep time.Duration) error {
	if attempts <= 0 {
		attempts = 30
	}
	if sleep <= 0 {
		sleep = 2 * time.Second
	}
	for i := 0; i < attempts; i++ {
		if HTTPGetOK(url) {
			return nil
		}
		time.Sleep(sleep)
	}
	return fmt.Errorf("超时等待就绪: %s", url)
}

func ComposeArgs(root string, withEnvFile bool) []string {
	args := []string{"compose", "-f", "packaging/compose.yaml"}
	if withEnvFile {
		envPath := root + "/.env"
		if _, err := os.Stat(envPath); err == nil {
			args = append(args, "--env-file", ".env")
		}
	}
	return args
}

func Compose(root string, stdout, stderr io.Writer, args ...string) error {
	base := ComposeArgs(root, true)
	return RunCmdIO(root, stdout, stderr, "docker", append(base, args...)...)
}

func ComposeQuiet(root string, args ...string) error {
	return Compose(root, io.Discard, os.Stderr, args...)
}

func DockerInspectOK(name string) bool {
	cmd := exec.Command("docker", "inspect", name)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	return cmd.Run() == nil
}

func logf(w io.Writer, format string, args ...any) {
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprintf(w, format+"\n", args...)
}

func warnf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "WARN: "+format+"\n", args...)
}
