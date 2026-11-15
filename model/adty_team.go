package model

import (
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

const (
	AdtyTeamStatusActive = "active"
	AdtyTeamStatusClosed = "closed"
	AdtyTeamRoleLeader   = "leader"
	AdtyTeamRoleMember   = "member"
	AdtyTeamMemberActive = "active"
	AdtyTeamMemberLeft   = "left"
)

// AdtyTeam is one shared-balance team on the main site or a subsite.
type AdtyTeam struct {
	Id           int64           `json:"id"`
	SiteId       int             `json:"site_id" gorm:"index"`
	SourceSiteId int             `json:"source_site_id" gorm:"index;not null;default:0;column:source_site_id"` // 关站迁入主站时记下原分站；0 表示不是迁入团队或历史未回填。历史主站团队仅在来源已独立记录时由 BackfillAdtyTeamSourceSiteID 补写
	LeaderUserId int             `json:"leader_user_id" gorm:"index"`
	Name         string          `json:"name" gorm:"type:varchar(64);not null"`
	Discount     decimal.Decimal `json:"discount" gorm:"type:decimal(10,4);not null"`
	Status       string          `json:"status" gorm:"type:varchar(16);index;not null"`
	SharedQuota  int64           `json:"shared_quota"`
	CreatedAt    int64           `json:"created_at" gorm:"autoCreateTime"`
	ClosedAt     int64           `json:"closed_at"`
}

func (AdtyTeam) TableName() string { return "adty_teams" }

// AdtyTeamMember binds one user to one team. A second active membership is rejected.
// idx_adty_team_member uniquely binds (team_id, user_id). A partial unique index
// ux_adty_team_members_one_active_user additionally guarantees at most one active
// row per user across teams. Left history is outside that index.
type AdtyTeamMember struct {
	Id        int64  `json:"id"`
	TeamId    int64  `json:"team_id" gorm:"uniqueIndex:idx_adty_team_member,priority:1"`
	UserId    int    `json:"user_id" gorm:"uniqueIndex:idx_adty_team_member,priority:2;index"`
	Role      string `json:"role" gorm:"type:varchar(16);not null"`
	Status    string `json:"status" gorm:"type:varchar(16);index;not null"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtyTeamMember) TableName() string { return "adty_team_members" }

// AdtyTeamLedger records shared-balance credits and spends.
type AdtyTeamLedger struct {
	Id             int64  `json:"id"`
	TeamId         int64  `json:"team_id" gorm:"index"`
	DeltaQuota     int64  `json:"delta_quota"`
	BalanceBefore  int64  `json:"balance_before"`
	BalanceAfter   int64  `json:"balance_after"`
	Type           string `json:"type" gorm:"type:varchar(32)"`
	RefId          string `json:"ref_id" gorm:"type:varchar(64)"`
	ActorUserId    int    `json:"actor_user_id"`
	IdempotencyKey string `json:"-" gorm:"uniqueIndex;type:varchar(128)"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtyTeamLedger) TableName() string { return "adty_team_ledgers" }

const (
	AdtyTeamInviteStatusOutstanding = "outstanding"
	AdtyTeamInviteStatusExpired     = "expired"
	AdtyTeamInviteStatusUsed        = "used"
	AdtyTeamInviteStatusRevoked     = "revoked"
)

// AdtyTeamInvite is a one-time, expiring join credential. The plaintext is not stored.
type AdtyTeamInvite struct {
	Id        int64  `json:"id"`
	TeamId    int64  `json:"team_id" gorm:"index"`
	CodeHash  string `json:"-" gorm:"uniqueIndex;type:varchar(64);not null"`
	CreatedBy int    `json:"created_by"`
	ExpiresAt int64  `json:"expires_at" gorm:"index"`
	UsedBy    int    `json:"used_by"`
	UsedAt    int64  `json:"used_at"`
	RevokedBy int    `json:"revoked_by"`
	RevokedAt int64  `json:"revoked_at"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
}

// AdtyTeamInviteStatus returns outstanding, expired, used, or revoked.
// Revoked is preferred over expiry; a used invite stays used even if later marked revoked.
func AdtyTeamInviteStatus(invite AdtyTeamInvite, now int64) string {
	if invite.UsedAt != 0 {
		return AdtyTeamInviteStatusUsed
	}
	if invite.RevokedAt != 0 {
		return AdtyTeamInviteStatusRevoked
	}
	if invite.ExpiresAt < now {
		return AdtyTeamInviteStatusExpired
	}
	return AdtyTeamInviteStatusOutstanding
}

func (AdtyTeamInvite) TableName() string { return "adty_team_invites" }

const (
	AdtyTeamRegistrationStatusOutstanding = "outstanding"
	AdtyTeamRegistrationStatusExpired     = "expired"
	AdtyTeamRegistrationStatusUsed        = "used"
	AdtyTeamRegistrationStatusRevoked     = "revoked"
)

// AdtyTeamRegistrationLink is a one-time, expiring credential that creates a
// team and its unique leader. SiteId is fixed at issuance. The plaintext code
// is not stored.
type AdtyTeamRegistrationLink struct {
	Id        int64  `json:"id"`
	SiteId    int    `json:"site_id" gorm:"index;not null"`
	CodeHash  string `json:"-" gorm:"uniqueIndex;type:varchar(64);not null"`
	CreatedBy int    `json:"created_by"`
	ExpiresAt int64  `json:"expires_at" gorm:"index"`
	UsedBy    int    `json:"used_by"`
	UsedAt    int64  `json:"used_at"`
	TeamId    int64  `json:"team_id"`
	RevokedBy int    `json:"revoked_by"`
	RevokedAt int64  `json:"revoked_at"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
}

// AdtyTeamRegistrationStatus returns outstanding, expired, used, or revoked.
func AdtyTeamRegistrationStatus(link AdtyTeamRegistrationLink, now int64) string {
	if link.UsedAt != 0 {
		return AdtyTeamRegistrationStatusUsed
	}
	if link.RevokedAt != 0 {
		return AdtyTeamRegistrationStatusRevoked
	}
	if link.ExpiresAt < now {
		return AdtyTeamRegistrationStatusExpired
	}
	return AdtyTeamRegistrationStatusOutstanding
}

func (AdtyTeamRegistrationLink) TableName() string { return "adty_team_registration_links" }

// AdtyTeamKey binds an API token to the member and team that the server stored.
type AdtyTeamKey struct {
	Id        int64  `json:"id"`
	TokenId   int    `json:"token_id" gorm:"uniqueIndex"`
	TeamId    int64  `json:"team_id" gorm:"index"`
	UserId    int    `json:"user_id" gorm:"index"`
	Status    string `json:"status" gorm:"type:varchar(16);index"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (AdtyTeamKey) TableName() string { return "adty_team_keys" }

// AdtyTeamReserveHook spends a team key from the shared balance inside the billing transaction.
// handled is false when the token is not a team key.
var AdtyTeamReserveHook func(tx *gorm.DB, tokenId, userId, delta int, ref string) (handled bool, err error)

// AdtyTeamSettleHook 是 AdtyTeamReserveHook 的结算侧对称项（`ISS-R4-0017`）：
// 团队 Key 的结算差额记到团队共享余额，成员个人钱包不再参与。
//
//   - delta > 0：从团队余额补扣，applied 为团队真正付得起的那部分（余额见底时
//     小于 delta，差额由调用方记 Shortfall，绝不回落成员钱包）；
//   - delta < 0：把未用的预扣退回团队，applied = delta；
//   - delta == 0：不动账，也不写流水；
//   - handled 为 false 表示这个 token 不是团队 Key，调用方按个人钱包结算。
//
// 判定和扣减必须在结算事务里完成，并且要在动 users.quota 之前。
var AdtyTeamSettleHook func(tx *gorm.DB, tokenId, userId, delta int, ref string) (handled bool, applied int, err error)

// AdtyTokenIsTeamKeyTx reports whether this token is bound to a team. A team key
// spends the shared team balance, so the member's personal wallet — and their
// principal lots — must not be charged for it.
//
// 判断只看绑定关系，不看团队余额：reserveAdtyTeamQuotaHook 在团队余额不足时
// 直接报错，而不是回落到个人钱包，所以「有绑定」等价于「这笔是团队付的」。
func AdtyTokenIsTeamKeyTx(tx *gorm.DB, tokenId int) (bool, error) {
	if tx == nil || tokenId <= 0 {
		return false, nil
	}
	if !tx.Migrator().HasTable(&AdtyTeamKey{}) {
		// 没有绑表就等于没有团队 Key，个人计费照旧。
		return false, nil
	}
	var count int64
	if err := tx.Model(&AdtyTeamKey{}).Where("token_id = ?", tokenId).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// AdtyBeforeUserDeleteHook closes a team when its leader is deleted.
var AdtyBeforeUserDeleteHook func(tx *gorm.DB, userId int) error

// BackfillAdtyTeamSourceSiteID 为已迁入主站（site_id=0）且 source_site_id 仍为空的历史团队补写来源站（A.16.4 / G.7.2）。
// 只使用已核销的团队注册链接上独立记下的非零 SiteId 和 TeamId；同一团队若出现多个不同来源则视为不明确，不回填。
// 不从负责人和成员的 inherited_from_site_id、用户迁移映射或订单/工单上的客户站点推断来源，以免把原生主站团队误标成迁入团队。
// 已有非零 source_site_id 不覆盖；仍在分站（site_id!=0）或没有已知来源的主站团队保持 0。可重复执行。
func BackfillAdtyTeamSourceSiteID(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	if db == nil {
		return nil
	}
	if !db.Migrator().HasTable(&AdtyTeam{}) || !db.Migrator().HasColumn(&AdtyTeam{}, "source_site_id") {
		return nil
	}
	if !db.Migrator().HasTable(&AdtyTeamRegistrationLink{}) {
		return nil
	}

	var rows []struct {
		TeamId int64 `gorm:"column:team_id"`
		SiteId int   `gorm:"column:site_id"`
	}
	if err := db.Model(&AdtyTeamRegistrationLink{}).
		Select("team_id, MAX(site_id) AS site_id").
		Where("team_id > ? AND site_id > ? AND used_at <> ?", 0, 0, 0).
		Group("team_id").
		Having("COUNT(DISTINCT site_id) = ?", 1).
		Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, row := range rows {
			if row.TeamId <= 0 || row.SiteId <= 0 {
				continue
			}
			if err := tx.Model(&AdtyTeam{}).
				Where("id = ? AND site_id = ? AND source_site_id = ?", row.TeamId, 0, 0).
				Update("source_site_id", row.SiteId).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
