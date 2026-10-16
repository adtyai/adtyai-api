package smartrouting

import "sync/atomic"

// RequestCaps describes the current request's capability / stream needs for SmartWeights filters.
type RequestCaps struct {
	NeedTools   bool
	NeedVision  bool
	NeedJSON    bool
	NeedContext int
	IsStream    bool
}

var requestCapsAtomic atomic.Value // RequestCaps

func init() {
	requestCapsAtomic.Store(RequestCaps{})
}

func SetRequestCaps(c RequestCaps) {
	requestCapsAtomic.Store(c)
}

func GetRequestCaps() RequestCaps {
	v, _ := requestCapsAtomic.Load().(RequestCaps)
	return v
}

func ClearRequestCaps() {
	requestCapsAtomic.Store(RequestCaps{})
}
