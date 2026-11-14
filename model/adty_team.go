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

// AdtyBeforeUserDeleteHook closes a team when its leader is deleted.
var AdtyBeforeUserDeleteHook func(tx *gorm.DB, userId int) error
