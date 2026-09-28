package main

import (
	"time"
)

func (f *feedCache) get() ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	if time.Now().Before(f.validUntil) {
		return f.value, true
	}

	if f.value != nil {
		return f.value, true
	}

	return nil, false
}

func (f *feedCache) set(val []byte, ttl time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	
	f.value = val
	f.validUntil = time.Now().Add(ttl)
}

func (f *feedCache) clear() {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.value = nil
	f.validUntil = time.Time{}
}