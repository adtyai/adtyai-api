package model

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// 站长分站（P11）的站点、开站邀请码和站点审计。只由 service/adty_site*.go 写。

// 站点状态和开站方式。
const (
	AdtySiteStatusPending = "pending"
	AdtySiteStatusActive  = "active"
	AdtySiteStatusPaused  = "paused"
	AdtySiteStatusClosing = "closing"
	AdtySiteStatusClosed  = "closed"

	AdtySiteOpenedByApply  = "apply"
	AdtySiteOpenedByAdmin  = "admin"
	AdtySiteOpenedByInvite = "invite"
)

// AdtySite 是一个分站。一个用户同时最多一个没关闭的站：开站事务里先锁用户行再查，不靠唯一索引（关闭的站保留存档）。
// 子域名全局唯一，关站后保留，用来把老链接跳到客户的新归属。
type AdtySite struct {
	Id             int              `json:"id"`
	OwnerUserId    int              `json:"owner_user_id" gorm:"index"`
	Subdomain      string           `json:"subdomain" gorm:"uniqueIndex;type:varchar(63)"`
	RootDomain     string           `json:"root_domain" gorm:"type:varchar(253)"`
	Status         string           `json:"status" gorm:"type:varchar(16);index"`
	CostFactor     *decimal.Decimal `json:"cost_factor" gorm:"type:decimal(10,6)"` // 单站 d；空就用档位或全局
	MaxMarkup      *decimal.Decimal `json:"max_markup" gorm:"type:decimal(10,4)"`  // 单站 k 上限；空就用全局
	TierCostFactor *decimal.Decimal `json:"tier_cost_factor" gorm:"type:decimal(10,6)"`
	TierMonth      string           `json:"tier_month" gorm:"type:varchar(7)"`
	Visibility     string           `json:"visibility" gorm:"type:text"`
	Brand          string           `json:"brand" gorm:"type:text"`
	BonusRates     string           `json:"bonus_rates" gorm:"type:text"`
	SeoIndex       bool             `json:"seo_index"`
	AgreementVer   string           `json:"agreement_ver" gorm:"type:varchar(32)"`
	AgreementAt    int64            `json:"agreement_at"`
	ContactInfo    string           `json:"contact_info" gorm:"type:text"`
	OpenedBy       string           `json:"opened_by" gorm:"type:varchar(16)"`
	ReviewNote     string           `json:"review_note" gorm:"type:text"`
	// 关站：站长申请时写 CloseRequestedAt；管理员定下客户去向后状态改 closing，到 CloseExecuteAt 由主节点执行。
	CloseRequestedAt int64  `json:"close_requested_at"`
	CloseExecuteAt   int64  `json:"close_execute_at" gorm:"index"`
	CloseTransferTo  int    `json:"close_transfer_to"` // 0 转回主站，否则转给这个站
	CloseClawBack    bool   `json:"close_claw_back"`
	CloseReason      string `json:"close_reason" gorm:"type:text"`
	ClosedAt         int64  `json:"closed_at"`
	// 日结算对不上消费日志时暂停正收益，等管理员核对后恢复。负收益照常结算。
	SettleHoldReason string `json:"settle_hold_reason" gorm:"type:text"`
	// 收益钩子写库失败的次数。只记数，不重试；日结算对账会拦住漏记的正收益。
	EarningHookFailures int64 `json:"earning_hook_failures"`
	CreatedAt           int64 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt           int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

func (AdtySite) TableName() string { return "adty_sites" }

// AdtySiteInviteCode 是开站邀请码：有效就直接开成 active，并用码上预设的 d 和 k 上限。
type AdtySiteInviteCode struct {
	Id         int              `json:"id"`
	Code       string           `json:"code" gorm:"uniqueIndex;type:varchar(32)"`
	MaxUses    int              `json:"max_uses"` // 0 不限
	UsedCount  int              `json:"used_count"`
	ExpireAt   int64            `json:"expire_at"` // 0 不过期
	CostFactor *decimal.Decimal `json:"cost_factor" gorm:"type:decimal(10,6)"`
	MaxMarkup  *decimal.Decimal `json:"max_markup" gorm:"type:decimal(10,4)"`
	Note       string           `json:"note" gorm:"type:text"`
	CreatedBy  int              `json:"created_by"`
	CreatedAt  int64            `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtySiteInviteCode) TableName() string { return "adty_site_invite_codes" }

// AdtySiteAudit 是分站的操作记录：开站、审核、改 d 和 k、暂停、关站、客户转移。
type AdtySiteAudit struct {
	Id         int64  `json:"id"`
	SiteId     int    `json:"site_id" gorm:"index"`
	Action     string `json:"action" gorm:"type:varchar(32);index"`
	OperatorId int    `json:"operator_id" gorm:"index"`
	OldValue   string `json:"old_value" gorm:"type:text"`
	NewValue   string `json:"new_value" gorm:"type:text"`
	Reason     string `json:"reason" gorm:"type:text"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

func (AdtySiteAudit) TableName() string { return "adty_site_audits" }

// AdtySiteCustomer 是分站客户，和 users.site_id 同步写（11.2 注册、11.1 关站转移）。
// 风控标记和复核在 11.5，条款勾选在 11.7。
type AdtySiteCustomer struct {
	UserId       int    `json:"user_id" gorm:"primaryKey;autoIncrement:false"`
	SiteId       int    `json:"site_id" gorm:"index"`
	RiskFlag     string `json:"risk_flag" gorm:"type:varchar(16)"` // 空 | self_use | migration
	RiskReason   string `json:"-" gorm:"type:text"`                // 只给后台看
	Reviewed     bool   `json:"reviewed"`
	TermsVersion string `json:"terms_version" gorm:"type:varchar(32)"`
	TermsAt      int64  `json:"terms_at"`
	JoinedAt     int64  `json:"joined_at"`
}

func (AdtySiteCustomer) TableName() string { return "adty_site_customers" }

// AdtySitePrice 是站长定价（11.4）：同一站内按「客户 + 分组」取最具体的一条。
type AdtySitePrice struct {
	Id            int             `json:"id"`
	SiteId        int             `json:"site_id" gorm:"uniqueIndex:idx_site_price"`
	UserId        int             `json:"user_id" gorm:"uniqueIndex:idx_site_price"`                // 0 = 全站默认
	Group         string          `json:"group" gorm:"uniqueIndex:idx_site_price;type:varchar(64)"` // 空 = 所有分组
	Markup        decimal.Decimal `json:"markup" gorm:"type:decimal(10,4);not null;default:1"`
	PendingMarkup decimal.Decimal `json:"pending_markup" gorm:"type:decimal(10,4);not null;default:0"` // 0 = 没有待生效的涨价
	PendingAt     int64           `json:"pending_at"`
	Version       int64           `json:"version"`
	UpdatedAt     int64           `json:"updated_at"`
}

func (AdtySitePrice) TableName() string { return "adty_site_prices" }

// 站长收益日汇总的类型和状态（11.5）。
const (
	AdtySiteEarningUsage         = "usage"
	AdtySiteEarningRefund        = "refund"
	AdtySiteEarningBonus         = "bonus"
	AdtySiteEarningBonusReversal = "bonus_reversal"
	AdtySiteEarningTrial         = "trial"
	AdtySiteEarningSkipped       = "skipped"

	AdtySiteEarningPending   = "pending"
	AdtySiteEarningAvailable = "available"
	AdtySiteEarningVoid      = "void"

	AdtySiteRiskSelfUse   = "self_use"
	AdtySiteRiskMigration = "migration"
)

// AdtySiteEarningDaily 按「站、客户、日期、类型」汇总站长收益，一天一行（11.5）。
// 只用 OnConflict DoUpdate 累加，状态在结算时改成 available。结算后同一天、同一类型再有消费，
// 记进 Day 加 "+n" 的新行（见 service 的记账），不改已经结算的行。
type AdtySiteEarningDaily struct {
	Id        int64           `json:"id"`
	SiteId    int             `json:"site_id" gorm:"uniqueIndex:idx_site_earning"`
	UserId    int             `json:"user_id" gorm:"uniqueIndex:idx_site_earning"`
	Day       string          `json:"day" gorm:"uniqueIndex:idx_site_earning;type:varchar(16)"`
	Kind      string          `json:"kind" gorm:"uniqueIndex:idx_site_earning;type:varchar(16)"`
	Quota     int64           `json:"quota"`
	Revenue   decimal.Decimal `json:"revenue" gorm:"type:decimal(20,8);not null;default:0"`
	Platform  decimal.Decimal `json:"platform" gorm:"type:decimal(20,8);not null;default:0"`
	Earning   decimal.Decimal `json:"earning" gorm:"type:decimal(20,8);not null;default:0"`
	Reason    string          `json:"reason" gorm:"type:varchar(16)"` // skipped 行的原因类别：self_use | migration
	Status    string          `json:"status" gorm:"type:varchar(16);index"`
	SettledAt int64           `json:"settled_at"`
	UpdatedAt int64           `json:"updated_at"`
}

func (AdtySiteEarningDaily) TableName() string { return "adty_site_earning_daily" }

// AdtySiteTrialGrant 是站长送给客户的试用额度，按幂等键去重，也用来算每天的总额（11.5）。
type AdtySiteTrialGrant struct {
	Id             int64           `json:"id"`
	SiteId         int             `json:"site_id" gorm:"index"`
	UserId         int             `json:"user_id" gorm:"index"`
	AmountUSD      decimal.Decimal `json:"amount_usd" gorm:"type:decimal(20,8);not null;default:0"`
	Quota          int64           `json:"quota"`
	Day            string          `json:"day" gorm:"type:varchar(10);index"`
	IdempotencyKey string          `json:"-" gorm:"uniqueIndex;type:varchar(128)"`
	CreatedAt      int64           `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtySiteTrialGrant) TableName() string { return "adty_site_trial_grants" }

// 举报状态（11.6 站长举报客户，11.9 后台处理）。
const (
	AdtySiteReportPending   = "pending"
	AdtySiteReportResolved  = "resolved"
	AdtySiteReportDismissed = "dismissed"
)

// AdtySiteReport 是站长对自己站上客户的举报，进入主站分站管理的待处理。
type AdtySiteReport struct {
	Id         int    `json:"id"`
	SiteId     int    `json:"site_id" gorm:"index"`
	UserId     int    `json:"user_id" gorm:"index"`
	ReporterId int    `json:"reporter_id"`
	Reason     string `json:"reason" gorm:"type:text"`
	Status     string `json:"status" gorm:"type:varchar(16);index"`
	HandledBy  int    `json:"handled_by"`
	HandleNote string `json:"handle_note" gorm:"type:text"`
	HandledAt  int64  `json:"handled_at"`
	CreatedAt  int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

func (AdtySiteReport) TableName() string { return "adty_site_reports" }

// AdtySiteLookup 记站长每次精确查找客户（11.7）：限流按小时数这张表，同时就是审计。
type AdtySiteLookup struct {
	Id        int64  `json:"id"`
	SiteId    int    `json:"site_id" gorm:"index:idx_site_lookup_time,priority:1"`
	OwnerId   int    `json:"owner_id"`
	Query     string `json:"-" gorm:"type:varchar(255)"` // 只给后台审计看
	Found     bool   `json:"found"`
	CreatedAt int64  `json:"created_at" gorm:"index:idx_site_lookup_time,priority:2"`
}

func (AdtySiteLookup) TableName() string { return "adty_site_lookups" }

// AdtySiteTables 是分站的表，迁移和测试共用。
func AdtySiteTables() []any {
	return []any{&AdtySite{}, &AdtySiteInviteCode{}, &AdtySiteAudit{}, &AdtySiteCustomer{}, &AdtySitePrice{}, &AdtySiteDomain{},
		&AdtySiteEarningDaily{}, &AdtySiteTrialGrant{}, &AdtySiteReport{}, &AdtySiteLookup{}, &AdtySiteSenderDomain{}}
}

// AdtySetUsersSiteTx 把 fromSite 的客户全部改到 toSite（0 是主站），返回改了哪些用户。
// users.site_id 只由分站服务通过这里和 AdtySetUserSiteTx 改（UpdateWithTx 不会改它）。
func AdtySetUsersSiteTx(tx *gorm.DB, fromSite, toSite int) ([]int, error) {
	var ids []int
	if err := tx.Model(&User{}).Where("site_id = ?", fromSite).Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	if err := tx.Model(&User{}).Where("site_id = ?", fromSite).Update("site_id", toSite).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// AdtySetUserSiteTx 设置一个用户的所属分站（注册时用）。
func AdtySetUserSiteTx(tx *gorm.DB, userId, siteId int) error {
	return tx.Model(&User{}).Where("id = ?", userId).Update("site_id", siteId).Error
}

// AdtyInvalidateUserCaches 在改了 site_id 的事务提交后清这些用户的缓存，下次读取从库里重建。
func AdtyInvalidateUserCaches(ids []int) {
	for _, id := range ids {
		if err := invalidateUserCache(id); err != nil {
			common.SysError(common.LogText("invalidate user cache failed user=%d: %s", id, err.Error()))
		}
	}
}

// AdtyOnConsumeLog 由分站注册（任务 11.4、11.5），RecordConsumeLog 在消费日志开关之前调用：
// 把这次请求的分站倍率写进 params.Other 的 admin_info，并给站长记收益。
var AdtyOnConsumeLog func(c *gin.Context, userId int, params *RecordConsumeLogParams)

// AdtyOnTaskBillingLog 由分站注册（任务 11.5），RecordTaskBillingLog 在消费日志开关之前调用。
var AdtyOnTaskBillingLog func(params *RecordTaskBillingLogParams)

// AdtyGroupColumn 是带引号的 group 列名（保留字），service 包写原生 Select 时用。
func AdtyGroupColumn() string {
	return commonGroupCol
}
