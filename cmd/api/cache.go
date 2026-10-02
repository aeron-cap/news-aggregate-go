package main

import (
	"time"
)

func (f *feedCache) get() ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.value == nil {
		return nil, false
	}

	if time.Now().After(f.validUntil) {
		return nil, false
	}

	return f.value, true
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