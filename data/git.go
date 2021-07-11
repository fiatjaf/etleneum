package data

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func execute(name string, args ...string) error {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	cmd := exec.Command(name, args...)
	cmd.Dir = DatabasePath
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if err != nil {
		stdout.WriteTo(os.Stderr)
		stderr.WriteTo(os.Stderr)
		return err
	}

	return nil
}

func gitAdd(path string) error {
	if err := execute("git", "add", "."); err != nil {
		return err
	}

	return nil
}

func gitCommit(message string) error {
	if err := execute("git", "commit", "-m", message, "--no-edit"); err != nil {
		return err
	}

	return nil
}

func gitReset() error {
	if err := execute("git", "reset", "--hard", "HEAD"); err != nil {
		return err
	}

	return nil
}

func gitPull() error {
	if err := execute("git", "pull", "origin", "master", "--rebase"); err != nil {
		return err
	}

	return nil
}

func gitPush() error {
	if err := execute("git", "push", "origin", "master"); err != nil {
		return err
	}

	return nil
}

func gitGetLastCommitFileTimestamp(path string) time.Time {
	stdout := &bytes.Buffer{}
	cmd := exec.Command("git", "log", "-n", "1", "--pretty=format:%at", "--", path)
	cmd.Dir = DatabasePath
	cmd.Stdout = stdout

	cmd.Run()

	timestamp, _ := strconv.ParseInt(strings.TrimSpace(stdout.String()), 10, 64)
	if timestamp > 0 {
		return time.Unix(timestamp, 0)
	} else {
		panic(fmt.Errorf("failed to get call timestamp at %s", path))
	}
}
