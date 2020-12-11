package main

import "os"

// PluginLogger prefixes the log output with 'etleneum'
// and writes to stderr
type PluginLogger struct{}

func (pl PluginLogger) Write(p []byte) (n int, err error) {
	_, err = os.Stderr.Write([]byte("\x1B[01;41metleneum\x1B[0m " + string(p)))
	return len(p), err
}
