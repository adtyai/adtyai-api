package controller

import (
	"github.com/QuantumNous/new-api/common"
)

// 口令失败计数的实现在 common/login_lock.go：登录和别的「拿口令换凭据」的入口
// 必须共用同一份计数，否则后者就是绕开锁定的密码预言机（ISS-R4-0105）。
// 这里只保留 controller 里既有的名字，登录链路和它的测试不用改。

const adtyLoginFailureLimit = common.LoginLockLimit

func adtyLoginLocked(username string) bool {
	return common.LoginLocked(username)
}

func adtyRecordLoginFailure(username string) error {
	return common.RecordLoginFailure(username)
}

func adtyClearLoginFailures(username string) {
	common.ClearLoginFailures(username)
}

func adtyLoginFailureResponse() *common.Message {
	return common.NewMessage("Username or password is incorrect, or user has been banned")
}
