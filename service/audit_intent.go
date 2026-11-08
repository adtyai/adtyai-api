package service

import (
	"context"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

type auditDeliveryHandler struct{}

func (auditDeliveryHandler) Type() string            { return model.SystemTaskTypeAuditDelivery }
func (auditDeliveryHandler) Enabled() bool           { return model.HasPendingAuditIntents() }
func (auditDeliveryHandler) Interval() time.Duration { return 15 * time.Second }
func (auditDeliveryHandler) NewPayload() any         { return nil }

func (auditDeliveryHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	delivered, err := model.DeliverPendingAuditIntents(ctx, 100)
	status, errorCategory := model.SystemTaskStatusSucceeded, ""
	if err != nil {
		status, errorCategory = model.SystemTaskStatusFailed, "audit delivery incomplete"
	}
	// Task rows contain counters and a fixed category, never event payloads.
	if err := model.FinishSystemTask(task.TaskID, runnerID, status, map[string]int{"delivered": delivered}, errorCategory); err != nil {
		common.SysLog(common.LogText("audit delivery task result could not be persisted"))
	}
}

func init() { RegisterSystemTaskHandler(auditDeliveryHandler{}) }
