package controller

import (
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
)

const adtyLoginFailureLimit = 8

type adtyLoginFailure struct {
	count int
	until time.Time
}

var (
	adtyLoginFailuresMu sync.Mutex
	adtyLoginFailures   = map[string]adtyLoginFailure{}
)

func adtyLoginKey(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

func adtyLoginLocked(username string) bool {
	key := adtyLoginKey(username)
	if key == "" {
		return false
	}
	adtyLoginFailuresMu.Lock()
	defer adtyLoginFailuresMu.Unlock()
	entry, ok := adtyLoginFailures[key]
	if !ok || time.Now().After(entry.until) {
		delete(adtyLoginFailures, key)
		return false
	}
	return entry.count >= adtyLoginFailureLimit
}

func adtyRecordLoginFailure(username string) {
	key := adtyLoginKey(username)
	if key == "" {
		return
	}
	adtyLoginFailuresMu.Lock()
	defer adtyLoginFailuresMu.Unlock()
	entry := adtyLoginFailures[key]
	if time.Now().After(entry.until) {
		entry = adtyLoginFailure{}
	}
	entry.count++
	entry.until = time.Now().Add(15 * time.Minute)
	adtyLoginFailures[key] = entry
	if len(adtyLoginFailures) > 10000 {
		for name, stored := range adtyLoginFailures {
			if time.Now().After(stored.until) {
				delete(adtyLoginFailures, name)
			}
		}
	}
}

func adtyClearLoginFailures(username string) {
	key := adtyLoginKey(username)
	adtyLoginFailuresMu.Lock()
	delete(adtyLoginFailures, key)
	adtyLoginFailuresMu.Unlock()
}

func adtyLoginFailureResponse() *common.Message {
	return common.NewMessage("Username or password is incorrect, or user has been banned")
}
