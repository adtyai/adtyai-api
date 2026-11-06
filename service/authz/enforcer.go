package authz

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/casbin/casbin/v2"
	casbinmodel "github.com/casbin/casbin/v2/model"
	"gorm.io/gorm"
)

var (
	enforcerMu sync.RWMutex
	enforcer   *casbin.SyncedEnforcer
)

const modelText = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act, eft

[policy_effect]
e = some(where (p.eft == allow))

[matchers]
m = r.sub == p.sub && r.obj == p.obj && r.act == p.act && p.eft == "allow"
`

func Init(db *gorm.DB) error {
	if common.IsMasterNode {
		if err := seedBuiltInRoles(db); err != nil {
			return err
		}
		if err := resetBuiltInRolePolicies(db); err != nil {
			return err
		}
	}

	m, err := casbinmodel.NewModelFromString(modelText)
	if err != nil {
		return err
	}
	e, err := casbin.NewSyncedEnforcer(m, newGormAdapter(db))
	if err != nil {
		return err
	}
	e.EnableAutoSave(true)

	enforcerMu.Lock()
	enforcer = e
	enforcerMu.Unlock()

	if !common.IsMasterNode {
		return nil
	}
	return seedDefaultPolicies()
}

func currentEnforcer() *casbin.SyncedEnforcer {
	enforcerMu.RLock()
	defer enforcerMu.RUnlock()
	return enforcer
}

func ReloadPolicy() error {
	enforcerMu.Lock()
	defer enforcerMu.Unlock()
	if enforcer == nil {
		return fmt.Errorf("authz enforcer is not initialized")
	}
	return enforcer.LoadPolicy()
}

// StartPolicySync periodically reloads the authorization policy from the database.
// The enforcer keeps an in-memory snapshot, and permission changes are written
// straight to the DB (see SetUserPermissionsInTx) with only the local node's
// snapshot refreshed afterwards. Without this loop other instances in a
// multi-node deployment would keep serving stale permissions (including not
// honoring a revoked grant) until restart. Mirrors model.SyncOptions polling.
// NotifyPolicyChanged asks other nodes to reload. Redis absence keeps the poll.
func NotifyPolicyChanged() {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	if err := common.RDB.Publish(context.Background(), "adty:authz:reload", "1").Err(); err != nil {
		common.SysError(common.LogText("failed to publish authz reload: %s", err.Error()))
	}
}

func subscribePolicyReload() {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	pubsub := common.RDB.Subscribe(context.Background(), "adty:authz:reload")
	for range pubsub.Channel() {
		if err := ReloadPolicy(); err != nil {
			common.SysError(common.LogText("failed to reload authz policy: %s", err.Error()))
		}
	}
}

func StartPolicySync(frequency int) {
	go subscribePolicyReload()
	if frequency <= 0 {
		return
	}
	for {
		time.Sleep(time.Duration(frequency) * time.Second)
		if err := ReloadPolicy(); err != nil {
			common.SysError(common.LogText("failed to reload authz policy: %s", err.Error()))
		}
	}
}
