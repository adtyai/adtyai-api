package oauth

import (
	"maps"
	"sync"
)

var (
	providers = make(map[string]Provider)
	mu        sync.RWMutex
)

// Register registers an OAuth provider with the given name
func Register(name string, provider Provider) {
	mu.Lock()
	defer mu.Unlock()
	providers[name] = provider
}

// Unregister removes a provider from the registry
func Unregister(name string) {
	mu.Lock()
	defer mu.Unlock()
	delete(providers, name)
}

// GetProvider returns the OAuth provider for the given name
func GetProvider(name string) Provider {
	mu.RLock()
	defer mu.RUnlock()
	return providers[name]
}

// GetAllProviders returns all registered OAuth providers
func GetAllProviders() map[string]Provider {
	mu.RLock()
	defer mu.RUnlock()
	result := make(map[string]Provider, len(providers))
	maps.Copy(result, providers)
	return result
}

// IsProviderRegistered checks if a provider is registered
func IsProviderRegistered(name string) bool {
	mu.RLock()
	defer mu.RUnlock()
	_, ok := providers[name]
	return ok
}
