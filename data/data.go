package data

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/rs/zerolog"
)

var DatabasePath string = "."
var log *zerolog.Logger
var Initialized = false

func SetLogger(logger *zerolog.Logger) {
	log = logger
}

func Initialize() {
	_, err := os.Stat(filepath.Join(DatabasePath, ".git"))
	if os.IsNotExist(err) {
		panic(fmt.Errorf("git not initialized on git database at %s", DatabasePath))
	}

	// TODO: fetch from git remote?

	os.MkdirAll(filepath.Join(DatabasePath, "accounts"), 0700)
	os.MkdirAll(filepath.Join(DatabasePath, "contracts"), 0700)

	Initialized = true
}

var mutex = &sync.Mutex{}

func Start() {
	mutex.Lock()
}

func Abort() {
	err := gitReset()
	if err != nil {
		panic(err)
	}
	mutex.Unlock()
}

func Finish(message string) {
	err := gitCommit(message)
	if err != nil {
		panic(err)
	}
	mutex.Unlock()
}
