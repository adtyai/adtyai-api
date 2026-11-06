package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPasswordLoginLocksAccountWithoutChangingTheError(t *testing.T) {
	const name = "lock-target"
	adtyClearLoginFailures(name)
	t.Cleanup(func() { adtyClearLoginFailures(name) })
	assert.False(t, adtyLoginLocked(name))
	for i := 0; i < adtyLoginFailureLimit; i++ {
		adtyRecordLoginFailure(name)
	}
	assert.True(t, adtyLoginLocked(name))
	assert.Equal(t, "Username or password is incorrect, or user has been banned", adtyLoginFailureResponse().Key)
	adtyClearLoginFailures(name)
	assert.False(t, adtyLoginLocked(name))
}
