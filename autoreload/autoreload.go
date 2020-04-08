package autoreload

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
)

func restartOnUpdate() {
	s, _ := os.Readlink("/proc/self/exe")
	fmt.Printf("This program will restart itself with the same flags and environment variables when %s changes.\n", s)

	for {
		time.Sleep(1 * time.Second)
		s, _ := os.Readlink("/proc/self/exe")
		if strings.HasSuffix(s, " (deleted)") {
			p := s[:len(s)-10]
			if _, err := os.Stat(p); os.IsNotExist(err) {
				// Looks like the file just got deleted, or it doesn't exist yet?
				continue
			}
			fmt.Printf("Restarting %s\n", s)
			if err := syscall.Exec(p, os.Args, os.Environ()); err != nil {
				fmt.Printf("Autoreload failed! %s: %v", p, err)
			}
		}

	}
}

func init() {
	go restartOnUpdate()
}
