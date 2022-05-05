package data

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/rs/zerolog"
)

var (
	DatabasePath string = "."
	log          *zerolog.Logger
	Initialized  = false
)

func SetLogger(logger *zerolog.Logger) {
	log = logger
}

func Initialize() {
	_, err := os.Stat(filepath.Join(DatabasePath, ".git"))
	if os.IsNotExist(err) {
		panic(fmt.Errorf("git not initialized on git database at %s", DatabasePath))
	}

	if err := gitPull(); err != nil {
		log.Warn().Err(err).Msg("failed to git pull")
	}

	os.MkdirAll(filepath.Join(DatabasePath, "accounts"), 0o700)
	os.MkdirAll(filepath.Join(DatabasePath, "contracts"), 0o700)

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

	go gitPush()
}
