package model

// 推广返佣的 12 张表（任务 6.1）。表结构沿用旧项目，金额列沿用 float64 + decimal(20,8)（执行层决定 19），
// 结算币种改成 USD；新加合作伙伴状态、绑定的风险标记、提现的人民币汇率和金额。

type ReferralAffiliate struct {
	Id                  int      `json:"id"`
	UserId              int      `json:"user_id" gorm:"uniqueIndex"`
	InviteCode          string   `json:"invite_code" gorm:"type:varchar(32);uniqueIndex"`
	Status              string   `json:"status" gorm:"type:varchar(32);index"`
	SourceType          string   `json:"source_type" gorm:"type:varchar(32);default:'register'"`
	PartnerStatus       string   `json:"partner_status" gorm:"type:varchar(16);index"`
	ApplicantNote       string   `json:"applicant_note" gorm:"type:text"`
	RateOverride        *float64 `json:"rate_override" gorm:"type:decimal(10,4)"`
	AcquisitionEnabled  bool     `json:"acquisition_enabled"`
	SettlementEnabled   bool     `json:"settlement_enabled"`
	WithdrawalEnabled   bool     `json:"withdrawal_enabled"`
	RiskReason          string   `json:"risk_reason" gorm:"type:text"`
	RiskNote            string   `json:"risk_note" gorm:"type:text"`
	ApprovedBy          int      `json:"approved_by" gorm:"index"`
	ApprovedAt          int64    `json:"approved_at" gorm:"default:0"`
	DisabledBy          int      `json:"disabled_by" gorm:"index"`
	DisabledAt          int64    `json:"disabled_at" gorm:"default:0"`
	PendingReviewCursor int64    `json:"pending_review_cursor" gorm:"default:0;index"`
	// SiteOwnerSince 是开分站的时间（11.1 开站时写入，关站时不清）。大于 0 时邀请返佣停止，
	// 由管理员在推广管理里「恢复邀请返佣」清零（执行层决定 27）。
	SiteOwnerSince int64 `json:"site_owner_since" gorm:"default:0"`
	CreatedAt      int64 `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      int64 `json:"updated_at" gorm:"autoUpdateTime"`
}

type ReferralBinding struct {
	Id            int    `json:"id"`
	InviteeUserId int    `json:"invitee_user_id" gorm:"uniqueIndex"`
	InviterUserId int    `json:"inviter_user_id" gorm:"index"`
	AffiliateId   int    `json:"affiliate_id" gorm:"index"`
	BindSource    string `json:"bind_source" gorm:"type:varchar(32);default:'cookie'"`
	BindCode      string `json:"bind_code" gorm:"type:varchar(32)"`
	BindIpHash    string `json:"-" gorm:"type:varchar(64);index"`
	RiskFlag      string `json:"risk_flag" gorm:"type:varchar(32);index"`
	BoundAt       int64  `json:"bound_at" gorm:"default:0;index"`
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type ReferralClick struct {
	Id            int    `json:"id"`
	AffiliateId   int    `json:"affiliate_id" gorm:"index"`
	InviteCode    string `json:"invite_code" gorm:"type:varchar(32);index"`
	Referer       string `json:"referer" gorm:"type:text"`
	LandingPath   string `json:"landing_path" gorm:"type:text"`
	IpHash        string `json:"ip_hash" gorm:"type:varchar(64);index"`
	UserAgentHash string `json:"ua_hash" gorm:"type:varchar(64);index"`
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

type ReferralCommissionAccount struct {
	Id                 int     `json:"id"`
	AffiliateId        int     `json:"affiliate_id" gorm:"uniqueIndex"`
	UserId             int     `json:"user_id" gorm:"uniqueIndex"`
	SettlementCurrency string  `json:"settlement_currency" gorm:"type:varchar(16);default:'USD'"`
	PendingAmount      float64 `json:"pending_amount" gorm:"type:decimal(20,8);not null;default:0"`
	AvailableAmount    float64 `json:"available_amount" gorm:"type:decimal(20,8);not null;default:0"`
	FrozenAmount       float64 `json:"frozen_amount" gorm:"type:decimal(20,8);not null;default:0"`
	WithdrawnAmount    float64 `json:"withdrawn_amount" gorm:"type:decimal(20,8);not null;default:0"`
	CreatedAt          int64   `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt          int64   `json:"updated_at" gorm:"autoUpdateTime"`
}

type ReferralCommission struct {
	Id                   int     `json:"id"`
	AffiliateId          int     `json:"affiliate_id" gorm:"index"`
	AffiliateUserId      int     `json:"affiliate_user_id" gorm:"index"`
	InviteeUserId        int     `json:"invitee_user_id" gorm:"index"`
	SourceType           string  `json:"source_type" gorm:"type:varchar(32);uniqueIndex:idx_referral_commission_source,priority:1;index"`
	SourceOrderId        int     `json:"source_order_id" gorm:"index"`
	SourceTradeNo        string  `json:"source_trade_no" gorm:"type:varchar(255);uniqueIndex:idx_referral_commission_source,priority:2;index"`
	OrderType            string  `json:"order_type" gorm:"type:varchar(32);index"`
	BaseAmount           float64 `json:"base_amount" gorm:"type:decimal(20,8);not null;default:0"`
	PaidAmount           float64 `json:"paid_amount" gorm:"type:decimal(20,8);not null;default:0"`
	PaidCurrency         string  `json:"paid_currency" gorm:"type:varchar(16);default:''"`
	SettlementCurrency   string  `json:"settlement_currency" gorm:"type:varchar(16);default:'USD';index"`
	SettlementFxRate     float64 `json:"settlement_fx_rate" gorm:"type:decimal(20,8);not null;default:1"`
	SettlementBaseAmount float64 `json:"settlement_base_amount" gorm:"type:decimal(20,8);not null;default:0"`
	Rate                 float64 `json:"rate" gorm:"type:decimal(10,4);not null;default:0"`
	CommissionAmount     float64 `json:"commission_amount" gorm:"type:decimal(20,8);not null;default:0"`
	Status               string  `json:"status" gorm:"type:varchar(32);index"`
	SettleAt             int64   `json:"settle_at" gorm:"default:0;index"`
	AvailableAt          int64   `json:"available_at" gorm:"default:0"`
	FrozenAt             int64   `json:"frozen_at" gorm:"default:0"`
	WithdrawalId         int     `json:"withdrawal_id" gorm:"index"`
	CreatedAt            int64   `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt            int64   `json:"updated_at" gorm:"autoUpdateTime"`
}

type ReferralCommissionLedger struct {
	Id                 int     `json:"id"`
	AffiliateId        int     `json:"affiliate_id" gorm:"index"`
	UserId             int     `json:"user_id" gorm:"index"`
	CommissionId       int     `json:"commission_id" gorm:"index"`
	WithdrawalId       int     `json:"withdrawal_id" gorm:"index"`
	Type               string  `json:"type" gorm:"type:varchar(32);index"`
	RefType            string  `json:"ref_type" gorm:"type:varchar(32)"`
	RefId              string  `json:"ref_id" gorm:"type:varchar(128)"`
	ExternalRefId      string  `json:"external_ref_id" gorm:"type:varchar(160);uniqueIndex"`
	SettlementCurrency string  `json:"settlement_currency" gorm:"type:varchar(16);default:'USD'"`
	DeltaPending       float64 `json:"delta_pending" gorm:"type:decimal(20,8);not null;default:0"`
	DeltaAvailable     float64 `json:"delta_available" gorm:"type:decimal(20,8);not null;default:0"`
	DeltaFrozen        float64 `json:"delta_frozen" gorm:"type:decimal(20,8);not null;default:0"`
	DeltaWithdrawn     float64 `json:"delta_withdrawn" gorm:"type:decimal(20,8);not null;default:0"`
	Remark             string  `json:"remark" gorm:"type:text"`
	Operator           string  `json:"operator" gorm:"type:varchar(64);default:'system'"`
	CreatedAt          int64   `json:"created_at" gorm:"autoCreateTime;index"`
}

type ReferralAsset struct {
	Id          int    `json:"id"`
	OwnerUserId int    `json:"owner_user_id" gorm:"index"`
	Purpose     string `json:"purpose" gorm:"type:varchar(32);index"`
	StoragePath string `json:"storage_path" gorm:"type:varchar(255);uniqueIndex"`
	ContentType string `json:"content_type" gorm:"type:varchar(128)"`
	Size        int64  `json:"size" gorm:"default:0"`
	CreatedBy   string `json:"created_by" gorm:"type:varchar(32);index"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

type ReferralWithdrawal struct {
	Id                 int     `json:"id"`
	AffiliateId        int     `json:"affiliate_id" gorm:"index"`
	UserId             int     `json:"user_id" gorm:"index"`
	SettlementCurrency string  `json:"settlement_currency" gorm:"type:varchar(16);default:'USD'"`
	Amount             float64 `json:"amount" gorm:"type:decimal(20,8);not null;default:0"`
	FeeAmount          float64 `json:"fee_amount" gorm:"type:decimal(20,8);not null;default:0"`
	NetAmount          float64 `json:"net_amount" gorm:"type:decimal(20,8);not null;default:0"`
	AccountType        string  `json:"account_type" gorm:"type:varchar(32)"`
	AccountName        string  `json:"account_name" gorm:"type:varchar(128)"`
	AccountNo          string  `json:"account_no" gorm:"type:text"`
	AccountNetwork     string  `json:"account_network" gorm:"type:varchar(32)"`
	QRImageURL         string  `json:"qr_image_url" gorm:"column:qr_image_url;type:text"`
	CNYRate            string  `json:"cny_rate" gorm:"column:cny_rate;type:varchar(32)"`
	CNYAmount          string  `json:"cny_amount" gorm:"column:cny_amount;type:varchar(32)"`
	ApplicantNote      string  `json:"applicant_note" gorm:"type:text"`
	AdminNote          string  `json:"admin_note" gorm:"type:text"`
	PaymentProofURL    string  `json:"payment_proof_url" gorm:"type:text"`
	PaymentTxnNo       string  `json:"payment_txn_no" gorm:"type:varchar(128)"`
	RejectProofURL     string  `json:"reject_proof_url" gorm:"type:text"`
	Status             string  `json:"status" gorm:"type:varchar(32);index"`
	IdempotencyKey     *string `json:"-" gorm:"type:varchar(128);uniqueIndex:idx_referral_withdrawal_idempotency"`
	SubmittedAt        int64   `json:"submitted_at" gorm:"default:0;index"`
	ApprovedAt         int64   `json:"approved_at" gorm:"default:0"`
	PayoutDeadlineAt   int64   `json:"payout_deadline_at" gorm:"default:0"`
	PaidAt             int64   `json:"paid_at" gorm:"default:0"`
	RejectedAt         int64   `json:"rejected_at" gorm:"default:0"`
	CanceledAt         int64   `json:"canceled_at" gorm:"default:0"`
	ReviewedBy         int     `json:"reviewed_by" gorm:"index"`
	RejectedBy         int     `json:"rejected_by" gorm:"index"`
	CanceledBy         int     `json:"canceled_by" gorm:"index"`
	RejectReason       string  `json:"reject_reason" gorm:"type:text"`
	CreatedAt          int64   `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt          int64   `json:"updated_at" gorm:"autoUpdateTime"`
}

type ReferralWithdrawalItem struct {
	Id              int     `json:"id"`
	WithdrawalId    int     `json:"withdrawal_id" gorm:"index"`
	CommissionId    int     `json:"commission_id" gorm:"index"`
	AllocatedAmount float64 `json:"allocated_amount" gorm:"type:decimal(20,8);not null;default:0"`
	Status          string  `json:"status" gorm:"type:varchar(32);index"`
	CreatedAt       int64   `json:"created_at" gorm:"autoCreateTime"`
}

type ReferralSettlementBatch struct {
	Id           int    `json:"id"`
	BatchNo      string `json:"batch_no" gorm:"type:varchar(64);uniqueIndex"`
	Status       string `json:"status" gorm:"type:varchar(32);index"`
	StartedAt    int64  `json:"started_at" gorm:"default:0"`
	FinishedAt   int64  `json:"finished_at" gorm:"default:0"`
	ScannedCount int    `json:"scanned_count" gorm:"default:0"`
	SettledCount int    `json:"settled_count" gorm:"default:0"`
	SkippedCount int    `json:"skipped_count" gorm:"default:0"`
	FailedCount  int    `json:"failed_count" gorm:"default:0"`
	ErrorSummary string `json:"error_summary" gorm:"type:text"`
}

type ReferralCommissionJob struct {
	Id            int    `json:"id"`
	SourceType    string `json:"source_type" gorm:"type:varchar(32);uniqueIndex:idx_referral_job_source,priority:1;index"`
	SourceTradeNo string `json:"source_trade_no" gorm:"type:varchar(255);uniqueIndex:idx_referral_job_source,priority:2;index"`
	AffiliateId   int    `json:"affiliate_id" gorm:"index"`
	Status        string `json:"status" gorm:"type:varchar(32);index"`
	AttemptCount  int    `json:"attempt_count" gorm:"default:0"`
	LastError     string `json:"last_error" gorm:"type:text"`
	LockedAt      int64  `json:"locked_at" gorm:"default:0"`
	SucceededAt   int64  `json:"succeeded_at" gorm:"default:0"`
	FailedAt      int64  `json:"failed_at" gorm:"default:0"`
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type ReferralAdminAuditLog struct {
	Id           int    `json:"id"`
	Action       string `json:"action" gorm:"type:varchar(64);index"`
	TargetUserId int    `json:"target_user_id" gorm:"index"`
	AffiliateId  int    `json:"affiliate_id" gorm:"index"`
	AdminUserId  int    `json:"admin_user_id" gorm:"index"`
	Reason       string `json:"reason" gorm:"type:text"`
	Ip           string `json:"ip" gorm:"type:varchar(64)"`
	UserAgent    string `json:"user_agent" gorm:"type:text"`
	OldValue     string `json:"old_value" gorm:"type:text"`
	NewValue     string `json:"new_value" gorm:"type:text"`
	CreatedAt    int64  `json:"created_at" gorm:"autoCreateTime;index"`
}

const (
	ReferralAffiliateStatusApproved = "approved"
	ReferralAffiliateStatusDisabled = "disabled"

	ReferralPartnerStatusPending  = "pending"
	ReferralPartnerStatusApproved = "approved"
	ReferralPartnerStatusRejected = "rejected"

	ReferralCommissionStatusPending    = "pending"
	ReferralCommissionStatusAvailable  = "available"
	ReferralCommissionStatusFrozen     = "frozen"
	ReferralCommissionStatusPaid       = "paid"
	ReferralCommissionStatusVoid       = "void"
	ReferralCommissionStatusClawedBack = "clawed_back"

	ReferralWithdrawalStatusPending  = "pending"
	ReferralWithdrawalStatusApproved = "approved"
	ReferralWithdrawalStatusPaid     = "paid"
	ReferralWithdrawalStatusRejected = "rejected"
	ReferralWithdrawalStatusCanceled = "canceled"

	ReferralWithdrawalItemStatusFrozen    = "frozen"
	ReferralWithdrawalItemStatusReleased  = "released"
	ReferralWithdrawalItemStatusWithdrawn = "withdrawn"

	ReferralCommissionJobStatusPending    = "pending"
	ReferralCommissionJobStatusProcessing = "processing"
	ReferralCommissionJobStatusSkipped    = "skipped"
	ReferralCommissionJobStatusSucceeded  = "succeeded"
	ReferralCommissionJobStatusFailed     = "failed"

	ReferralAssetPurposeWithdrawalQR = "withdrawal_qr"
	ReferralAssetPurposePaymentProof = "payment_proof"

	ReferralRiskSelfInvite    = "self_invite"
	ReferralRiskBatchRegister = "batch_register"

	ReferralSourceTopup = "topup"

	ReferralWithdrawToBalance = "balance"
	ReferralWithdrawToUSDT    = "usdt"
	ReferralWithdrawToAlipay  = "alipay"
	ReferralWithdrawToWechat  = "wechat"
)

// ReferralTables 是要建的 12 张表，迁移和测试共用。
func ReferralTables() []any {
	return []any{
		&ReferralAffiliate{}, &ReferralBinding{}, &ReferralClick{}, &ReferralCommissionAccount{},
		&ReferralCommission{}, &ReferralCommissionLedger{}, &ReferralAsset{}, &ReferralWithdrawal{},
		&ReferralWithdrawalItem{}, &ReferralSettlementBatch{}, &ReferralCommissionJob{}, &ReferralAdminAuditLog{},
	}
}
