package model

import "github.com/shopspring/decimal"

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
