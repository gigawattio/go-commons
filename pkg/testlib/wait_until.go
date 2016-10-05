package testlib

import (
	"fmt"
	"time"

	log "github.com/Sirupsen/logrus"
)

func WaitUntil(what string, waitTimeout time.Duration, fn func() bool, pause ...time.Duration) error {
	if len(pause) == 0 {
		pause = []time.Duration{50 * time.Millisecond}
	}
	var (
		waitingSince = time.Now()
		waitUntil    = waitingSince.Add(waitTimeout)
	)
	for {
		if time.Now().After(waitUntil) {
			break
		}
		if fn() {
			log.Infof("%v succeeded after %v", what, time.Now().Sub(waitingSince))
			return nil
		}
		time.Sleep(pause[0])
	}
	log.Infof("Timed out after %v waiting for %v", waitTimeout, what)
	return fmt.Errorf("timed out after %v waiting for %v", waitTimeout, what)
}
