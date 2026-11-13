package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

const (
	dataRetentionCleanupBatchSize  = 100
	dataRetentionSchedulerInterval = 24 * time.Hour

	dataRetentionSkipUnconfigured = "unconfigured; no automatic deletion"
	dataRetentionSkipNotification = "permanent retention; automatic cleanup must not delete notification history"
	dataRetentionSkipAccount      = "no safe cancelled-account population to retain, de-identify, or delete without risking active accounts or financial history"
	dataRetentionSkipOrders       = "orders and payment events are not safely automatically deletable with the current schema"
	dataRetentionSkipFinancial    = "financial ledgers are not safely automatically deletable with the current schema"
	dataRetentionSkipTickets      = "ticket images are stored on disk and automatic deletion is not safely mapped without a complete file-cleanup path"
	dataRetentionSkipArtifacts    = "task inputs, outputs, and generated file associations are not safely automatically deletable with the current schema"
	dataRetentionSkipDeidentify   = "de-identification is not implemented for this category; no automatic deletion was performed"
)

// DataRetentionCategoryView is the Root admin representation of one category.
type DataRetentionCategoryView struct {
	Category         string   `json:"category"`
	Title            string   `json:"title"`
	AutoCleanable    bool     `json:"auto_cleanable"`
	Configured       bool     `json:"configured"`
	RetentionDays    *int     `json:"retention_days"`
	Action           string   `json:"action"`
	MinRetentionDays *int     `json:"min_retention_days,omitempty"`
	AllowedActions   []string `json:"allowed_actions"`
	DefaultBehavior  string   `json:"default_behavior"`
	Notes            string   `json:"notes"`
}

type DataRetentionPolicyInput struct {
	Category      string `json:"category"`
	RetentionDays *int   `json:"retention_days"`
	Action        string `json:"action"`
	Clear         bool   `json:"clear"`
}

type DataRetentionRunView struct {
	Id           int64                            `json:"id"`
	StartedAt    int64                            `json:"started_at"`
	FinishedAt   int64                            `json:"finished_at"`
	Trigger      string                           `json:"trigger"`
	Scope        string                           `json:"scope"`
	SystemTaskID string                           `json:"system_task_id,omitempty"`
	Items        []model.AdtyDataRetentionRunItem `json:"items"`
}

type DataRetentionPayload struct {
	Trigger string `json:"trigger"`
}

type DataRetentionResult struct {
	RunID int64 `json:"run_id"`
}

type dataRetentionCategorySpec struct {
	Category        string
	Title           string
	AutoCleanable   bool
	AllowedActions  []string
	DefaultBehavior string
	Notes           string
	MinDays         int
	SkipConfigured  string
}

func dataRetentionCatalog() []dataRetentionCategorySpec {
	return []dataRetentionCategorySpec{
		{
			Category:        model.DataRetentionCategoryUserAccountProfile,
			Title:           "用户账号及个人资料",
			AutoCleanable:   true,
			AllowedActions:  []string{model.DataRetentionActionDelete, model.DataRetentionActionDeidentify},
			DefaultBehavior: "no_automatic_deletion",
			Notes:           "Root may set a retain, de-identify, or delete action. The task does not use account cancellation as a trigger to wipe orders, ledgers, or audit facts, and it will not act until a safe cancelled-account population exists.",
			SkipConfigured:  dataRetentionSkipAccount,
		},
		{
			Category:        model.DataRetentionCategoryLoginSecurity,
			Title:           "登录/安全记录",
			AutoCleanable:   true,
			AllowedActions:  []string{model.DataRetentionActionDelete},
			DefaultBehavior: "no_automatic_deletion",
			Notes:           "When Root sets a period, the official task may delete expired or revoked sessions, expired access tokens, login logs, and login/security audit rows older than that period. The USER_SESSION_REVOKED_RETENTION_DAYS environment setting remains an operational default for the existing session cleaner and is not treated as a signed Root period.",
		},
		{
			Category:        model.DataRetentionCategoryAPIUsageLogs,
			Title:           "API 调用日志和用量记录",
			AutoCleanable:   true,
			AllowedActions:  []string{model.DataRetentionActionDelete},
			DefaultBehavior: "no_automatic_deletion",
			Notes:           "Unconfigured usage logs are not auto-deleted. A configured period still honors a 90-day safety floor. Top-up, refund, adjustment, and login rows are not removed by usage cleanup.",
			MinDays:         90,
		},
		{
			Category:        model.DataRetentionCategoryOrdersPayments,
			Title:           "订单及支付事件",
			AutoCleanable:   true,
			AllowedActions:  []string{model.DataRetentionActionDelete},
			DefaultBehavior: "no_automatic_deletion",
			Notes:           "Listed for Root configuration. Automatic deletion is skipped until a safe mapping exists. Historical orders are never deleted because an account was cancelled.",
			SkipConfigured:  dataRetentionSkipOrders,
		},
		{
			Category:        model.DataRetentionCategoryFinancialLedgers,
			Title:           "资金流水、钱包、保证金、收益、佣金和退款",
			AutoCleanable:   true,
			AllowedActions:  []string{model.DataRetentionActionDelete},
			DefaultBehavior: "no_automatic_deletion",
			Notes:           "Listed for Root configuration. Automatic deletion is skipped until a safe mapping exists. Financial facts are never deleted because an account was cancelled.",
			SkipConfigured:  dataRetentionSkipFinancial,
		},
		{
			Category:        model.DataRetentionCategorySupportTickets,
			Title:           "客服工单及图片",
			AutoCleanable:   true,
			AllowedActions:  []string{model.DataRetentionActionDelete},
			DefaultBehavior: "no_automatic_deletion",
			Notes:           "Listed for Root configuration. Automatic deletion is skipped until ticket images on disk can be removed safely with the database rows.",
			SkipConfigured:  dataRetentionSkipTickets,
		},
		{
			Category:        model.DataRetentionCategoryNotificationHistory,
			Title:           "通知和公告历史",
			AutoCleanable:   false,
			AllowedActions:  []string{model.DataRetentionActionExclude},
			DefaultBehavior: "permanent_retention",
			Notes:           "In-site notification history is permanently retained against automatic cleanup. Only the user may manually delete their own deletable messages. Root-configured retention tasks must not delete it.",
		},
		{
			Category:        model.DataRetentionCategoryOperationAudit,
			Title:           "操作审计",
			AutoCleanable:   true,
			AllowedActions:  []string{model.DataRetentionActionDelete},
			DefaultBehavior: "no_automatic_deletion",
			Notes:           "Unconfigured audit facts are not auto-deleted. When Root sets a period, later runs may delete operation and access-token audit rows older than that period. A config change never deletes history by itself.",
		},
		{
			Category:        model.DataRetentionCategoryTaskArtifacts,
			Title:           "任务输入、输出及生成文件关联信息",
			AutoCleanable:   true,
			AllowedActions:  []string{model.DataRetentionActionDelete},
			DefaultBehavior: "no_automatic_deletion",
			Notes:           "Listed for Root configuration. Automatic deletion is skipped until generated files and their associations can be removed safely.",
			SkipConfigured:  dataRetentionSkipArtifacts,
		},
	}
}

func dataRetentionSpec(category string) (dataRetentionCategorySpec, bool) {
	for _, spec := range dataRetentionCatalog() {
		if spec.Category == category {
			return spec, true
		}
	}
	return dataRetentionCategorySpec{}, false
}

// ListDataRetentionSettings returns every A.17.2 category and the current Root config.
func ListDataRetentionSettings() (gin.H, error) {
	policies, err := model.GetAdtyDataRetentionPolicies()
	if err != nil {
		return nil, err
	}
	views := make([]DataRetentionCategoryView, 0, len(dataRetentionCatalog()))
	for _, spec := range dataRetentionCatalog() {
		view := DataRetentionCategoryView{
			Category:        spec.Category,
			Title:           spec.Title,
			AutoCleanable:   spec.AutoCleanable,
			AllowedActions:  append([]string(nil), spec.AllowedActions...),
			DefaultBehavior: spec.DefaultBehavior,
			Notes:           spec.Notes,
		}
		if spec.MinDays > 0 {
			days := spec.MinDays
			view.MinRetentionDays = &days
		}
		if !spec.AutoCleanable {
			view.Action = model.DataRetentionActionExclude
		}
		if policy, ok := policies[spec.Category]; ok && policy.RetentionDays != nil && *policy.RetentionDays > 0 && spec.AutoCleanable {
			days := *policy.RetentionDays
			view.Configured = true
			view.RetentionDays = &days
			view.Action = policy.Action
		}
		views = append(views, view)
	}
	return gin.H{
		"categories":                      views,
		"login_security_env_default_days": common.UserSessionRevokedRetentionDays,
		"usage_log_min_retention_days":    int(LogCleanupRetention / (24 * time.Hour)),
	}, nil
}

// SaveDataRetentionPolicies updates Root-configured periods. It does not start cleanup.
func SaveDataRetentionPolicies(c *gin.Context, actorID int, actorName string, inputs []DataRetentionPolicyInput) error {
	if len(inputs) == 0 {
		return common.NewMessage("Invalid parameters")
	}
	seen := map[string]bool{}
	changes := make([]string, 0, len(inputs))
	for _, input := range inputs {
		category := strings.TrimSpace(input.Category)
		if !model.IsDataRetentionCategory(category) || seen[category] {
			return common.NewMessage("Invalid parameters")
		}
		seen[category] = true
		spec, ok := dataRetentionSpec(category)
		if !ok {
			return common.NewMessage("Invalid parameters")
		}
		action := strings.TrimSpace(strings.ToLower(input.Action))
		if category == model.DataRetentionCategoryNotificationHistory || !spec.AutoCleanable {
			if input.Clear {
				continue
			}
			if input.RetentionDays != nil || (action != "" && action != model.DataRetentionActionExclude) {
				return common.NewMessage("Notification history cannot be given an automatic cleanup period")
			}
			continue
		}
		previous, _, err := model.GetAdtyDataRetentionPolicy(category)
		if err != nil {
			return err
		}
		if input.Clear || (input.RetentionDays == nil && action == "") {
			if err := model.ClearAdtyDataRetentionPolicy(category); err != nil {
				return err
			}
			changes = append(changes, fmt.Sprintf("%s:cleared", category))
			continue
		}
		if input.RetentionDays == nil || *input.RetentionDays < 1 {
			return common.NewMessage("A retention period must be a positive number of days")
		}
		if action == "" {
			action = model.DataRetentionActionDelete
		}
		if !allowedDataRetentionAction(spec, action) {
			return common.NewMessage("Invalid parameters")
		}
		row := model.AdtyDataRetentionPolicy{
			Category:      category,
			RetentionDays: input.RetentionDays,
			Action:        action,
			UpdatedBy:     actorID,
			UpdatedAt:     common.GetTimestamp(),
		}
		if err := model.UpsertAdtyDataRetentionPolicy(row); err != nil {
			return err
		}
		prev := "unconfigured"
		if previous.RetentionDays != nil {
			prev = fmt.Sprintf("%s/%d", previous.Action, *previous.RetentionDays)
		}
		changes = append(changes, fmt.Sprintf("%s:%s->%s/%d", category, prev, action, *input.RetentionDays))
	}
	if len(changes) > 0 {
		entry := model.AuditLog{
			UserId:    actorID,
			Username:  actorName,
			ActorRole: common.RoleRootUser,
			Category:  model.AuditCategoryOperation,
			Action:    "data_retention.update",
			Success:   true,
			Content:   "data retention policy updated: " + strings.Join(changes, "; "),
			Other: model.AuditOther{
				Op: &model.AuditOperation{
					Action: "data_retention.update",
					Params: model.AuditFields{"changes": strings.Join(changes, "; ")},
				},
			},
		}
		model.RecordAuditLog(c, entry)
	}
	return nil
}

func allowedDataRetentionAction(spec dataRetentionCategorySpec, action string) bool {
	for _, allowed := range spec.AllowedActions {
		if allowed == action {
			return true
		}
	}
	return false
}

// ConfiguredDataRetentionDays returns the Root-configured period when one is set.
func ConfiguredDataRetentionDays(category string) (int, bool) {
	row, ok, err := model.GetAdtyDataRetentionPolicy(category)
	if err != nil || !ok || row.RetentionDays == nil || *row.RetentionDays <= 0 {
		return 0, false
	}
	return *row.RetentionDays, true
}

func usageLogCleanupFloor(now time.Time) time.Duration {
	floor := LogCleanupRetention
	if days, ok := ConfiguredDataRetentionDays(model.DataRetentionCategoryAPIUsageLogs); ok {
		configured := time.Duration(days) * 24 * time.Hour
		if configured > floor {
			return configured
		}
	}
	return floor
}

func ListDataRetentionRuns(limit int) ([]DataRetentionRunView, error) {
	runs, items, err := model.ListAdtyDataRetentionRuns(limit)
	if err != nil {
		return nil, err
	}
	views := make([]DataRetentionRunView, 0, len(runs))
	for _, run := range runs {
		views = append(views, DataRetentionRunView{
			Id:           run.Id,
			StartedAt:    run.StartedAt,
			FinishedAt:   run.FinishedAt,
			Trigger:      run.Trigger,
			Scope:        run.Scope,
			SystemTaskID: run.SystemTaskID,
			Items:        items[run.Id],
		})
	}
	return views, nil
}

// RunDataRetentionCleanup applies configured category rules and always records
// notification-history exclusion. Changing a policy never calls this function.
func RunDataRetentionCleanup(ctx context.Context, trigger, systemTaskID string, now time.Time) (*model.AdtyDataRetentionRun, []model.AdtyDataRetentionRunItem, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if now.IsZero() {
		now = time.Now()
	}
	if trigger == "" {
		trigger = model.DataRetentionTriggerScheduled
	}
	started := now.Unix()
	policies, err := model.GetAdtyDataRetentionPolicies()
	if err != nil {
		return nil, nil, err
	}
	items := make([]model.AdtyDataRetentionRunItem, 0, len(dataRetentionCatalog()))
	for _, spec := range dataRetentionCatalog() {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		item := applyDataRetentionCategory(ctx, spec, policies[spec.Category], now)
		items = append(items, item)
	}
	finished := time.Now().Unix()
	run := &model.AdtyDataRetentionRun{
		StartedAt:    started,
		FinishedAt:   finished,
		Trigger:      trigger,
		Scope:        model.DataRetentionScopePlatform,
		SystemTaskID: systemTaskID,
	}
	if err := model.CreateAdtyDataRetentionRun(run, items); err != nil {
		return nil, nil, err
	}
	return run, items, nil
}

func applyDataRetentionCategory(ctx context.Context, spec dataRetentionCategorySpec, policy model.AdtyDataRetentionPolicy, now time.Time) model.AdtyDataRetentionRunItem {
	item := model.AdtyDataRetentionRunItem{
		Category:   spec.Category,
		Scope:      model.DataRetentionScopePlatform,
		ExecutedAt: now.Unix(),
	}
	if spec.Category == model.DataRetentionCategoryNotificationHistory || !spec.AutoCleanable {
		item.Rule = "exclude; permanent retention"
		item.SkipReason = dataRetentionSkipNotification
		return item
	}
	configured := policy.RetentionDays != nil && *policy.RetentionDays > 0
	if !configured {
		item.Rule = "unconfigured"
		item.SkipReason = dataRetentionSkipUnconfigured
		return item
	}
	action := policy.Action
	if action == "" {
		action = model.DataRetentionActionDelete
	}
	days := *policy.RetentionDays
	item.Rule = fmt.Sprintf("%s after %d days", action, days)
	if spec.MinDays > 0 && days < spec.MinDays {
		item.Rule = fmt.Sprintf("%s after %d days (safety floor %d days)", action, days, spec.MinDays)
	}
	if spec.SkipConfigured != "" {
		item.SkipReason = spec.SkipConfigured
		return item
	}
	if action == model.DataRetentionActionDeidentify {
		item.SkipReason = dataRetentionSkipDeidentify
		return item
	}
	deleted, failed, err := executeDataRetentionDelete(ctx, spec.Category, effectiveRetentionDays(days, spec.MinDays), now)
	if err != nil {
		item.FailureCount = failed
		if item.FailureCount == 0 {
			item.FailureCount = 1
		}
		item.SkipReason = err.Error()
		return item
	}
	item.DeleteCount = deleted
	item.FailureCount = failed
	return item
}

func effectiveRetentionDays(days, minDays int) int {
	if minDays > 0 && days < minDays {
		return minDays
	}
	return days
}

func executeDataRetentionDelete(ctx context.Context, category string, days int, now time.Time) (deleted, failed int64, err error) {
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour).Unix()
	switch category {
	case model.DataRetentionCategoryAPIUsageLogs:
		count, err := drainRetentionBatches(ctx, func() (int64, error) {
			return model.DeleteOldLogBatch(ctx, cutoff, dataRetentionCleanupBatchSize)
		})
		if err != nil {
			return count, 1, err
		}
		quota, err := drainRetentionBatches(ctx, func() (int64, error) {
			return model.DeleteOldQuotaDataBatch(ctx, cutoff, dataRetentionCleanupBatchSize)
		})
		return count + quota, 0, err
	case model.DataRetentionCategoryLoginSecurity:
		logs, err := drainRetentionBatches(ctx, func() (int64, error) {
			return model.DeleteOldLogsOfTypesBatch(ctx, []int{model.LogTypeLogin}, cutoff, dataRetentionCleanupBatchSize)
		})
		if err != nil {
			return logs, 1, err
		}
		audits, err := drainRetentionBatches(ctx, func() (int64, error) {
			return model.DeleteOldAuditLogsByCategoriesBatch(ctx, []string{model.AuditCategoryLogin, model.AuditCategorySecurity}, cutoff, dataRetentionCleanupBatchSize)
		})
		if err != nil {
			return logs + audits, 1, err
		}
		sessions, err := model.DeleteInactiveUserSessionsForRetention(now.Unix(), days)
		if err != nil {
			return logs + audits + sessions, 1, err
		}
		tokens, err := model.DeleteExpiredUserAccessTokensForRetention(now.Unix(), days)
		return logs + audits + sessions + tokens, 0, err
	case model.DataRetentionCategoryOperationAudit:
		count, err := drainRetentionBatches(ctx, func() (int64, error) {
			return model.DeleteOldAuditLogsByCategoriesBatch(ctx, []string{model.AuditCategoryOperation, model.AuditCategoryAccessToken}, cutoff, dataRetentionCleanupBatchSize)
		})
		if err != nil {
			return count, 1, err
		}
		return count, 0, nil
	default:
		return 0, 0, nil
	}
}

func drainRetentionBatches(ctx context.Context, fn func() (int64, error)) (int64, error) {
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		n, err := fn()
		if err != nil {
			return total, err
		}
		total += n
		if n == 0 {
			return total, nil
		}
	}
}

type dataRetentionHandler struct{}

func (dataRetentionHandler) Type() string { return model.SystemTaskTypeDataRetention }

func (dataRetentionHandler) Enabled() bool { return true }

func (dataRetentionHandler) Interval() time.Duration { return dataRetentionSchedulerInterval }

func (dataRetentionHandler) NewPayload() any {
	return DataRetentionPayload{Trigger: model.DataRetentionTriggerScheduled}
}

func (dataRetentionHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) {
	payload := DataRetentionPayload{Trigger: model.DataRetentionTriggerScheduled}
	_ = task.DecodePayload(&payload)
	if payload.Trigger == "" {
		payload.Trigger = model.DataRetentionTriggerScheduled
	}
	run, _, err := RunDataRetentionCleanup(ctx, payload.Trigger, task.TaskID, time.Now())
	if err != nil {
		failSystemTask(task, runnerID, err)
		return
	}
	result := DataRetentionResult{}
	if run != nil {
		result.RunID = run.Id
	}
	if err := model.FinishSystemTask(task.TaskID, runnerID, model.SystemTaskStatusSucceeded, result, ""); err != nil {
		logSystemTaskLockError(ctx, task, err)
	}
}

func init() {
	RegisterSystemTaskHandler(dataRetentionHandler{})
}
