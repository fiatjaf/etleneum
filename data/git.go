package data

import (
	"bytes"
	"os"
	"os/exec"
)

func execute(name string, args ...string) error {
	stderr := &bytes.Buffer{}

	cmd := exec.Command(name, args...)
	cmd.Dir = DatabasePath
	cmd.Stderr = stderr

	err := cmd.Run()
	if err != nil {
		stderr.WriteTo(os.Stderr)
		return err
	}

	return nil
}

func gitCommit(message string) error {
	if err := execute("git", "add", "."); err != nil {
		return err
	}

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
