package model

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

const AuditActionAdminLogin = "admin_login"

// HasPriorAdminLoginIP reports whether this operator already has an admin_login
// audit row from the given IP (used to compute ip_first_seen).
func HasPriorAdminLoginIP(userId int, ip string) (bool, error) {
	if LOG_DB == nil || userId <= 0 || ip == "" {
		return false, nil
	}
	var n int64
	err := LOG_DB.Model(&AuditLog{}).
		Where("user_id = ? AND action = ? AND ip = ?", userId, AuditActionAdminLogin, ip).
		Count(&n).Error
	return n > 0, err
}

// RecordAdminLoginAudit writes action=admin_login with ip_first_seen in Other.
// Call only for role >= Admin. Does not create a separate table.
func RecordAdminLoginAudit(userId, actorRole int, username, content, ip, loginMethod string, params map[string]any, request ...*gin.Context) {
	firstSeen := true
	if prior, err := HasPriorAdminLoginIP(userId, ip); err == nil && prior {
		firstSeen = false
	} else if err != nil {
		common.SysError(fmt.Sprintf("admin_login ip_first_seen lookup failed: %v", err))
	}
	other := AuditOther{
		LoginMethod: loginMethod,
		IpFirstSeen: &firstSeen,
	}
	if len(request) > 0 && request[0] != nil {
		other.UserAgent = request[0].Request.UserAgent()
	}
	RecordLoginLog(userId, actorRole, username, content, ip, AuditActionAdminLogin, params, other, request...)
}
