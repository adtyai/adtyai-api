package service

import (
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var (
	ErrAdtyTeamInvalid   = common.NewMessage("Invalid parameters")
	ErrAdtyTeamForbidden = common.NewMessage("No permission to perform this action")
	ErrAdtyTeamBusy      = common.NewMessage("This account already belongs to a team")
	ErrAdtyTeamFunds     = common.NewMessage("Insufficient available funds")
	ErrAdtyTeamClosed    = common.NewMessage("This team is closed")
)

type AdtyTeamCreated struct {
	Team   model.AdtyTeam
	Member model.AdtyTeamMember
}

func CreateAdtyTeam(leaderId, siteId int, name string, discount decimal.Decimal) (*AdtyTeamCreated, error) {
	name = strings.TrimSpace(name)
	if leaderId <= 0 || siteId < 0 || name == "" || len(name) > 64 || !discount.IsPositive() || discount.GreaterThan(decimal.NewFromInt(1)) {
		return nil, ErrAdtyTeamInvalid
	}
	var created *AdtyTeamCreated
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		leader, err := lockAdtyTeamUser(tx, leaderId)
		if err != nil {
			return err
		}
		if leader.SiteId != siteId || leader.StaffSiteId != 0 {
			return ErrAdtyTeamForbidden
		}
		if siteId > 0 {
			var site model.AdtySite
			if err := tx.First(&site, siteId).Error; err != nil {
				return err
			}
			if site.Status != model.AdtySiteStatusActive {
				return ErrAdtyTeamForbidden
			}
			if site.CostFactor != nil && discount.LessThan(*site.CostFactor) {
				return ErrAdtyTeamInvalid
			}
		}
		if err := rejectActiveTeamMembership(tx, leader.Id); err != nil {
			return err
		}
		team := model.AdtyTeam{SiteId: siteId, LeaderUserId: leader.Id, Name: name, Discount: discount, Status: model.AdtyTeamStatusActive}
		if err := tx.Create(&team).Error; err != nil {
			return err
		}
		member := model.AdtyTeamMember{TeamId: team.Id, UserId: leader.Id, Role: model.AdtyTeamRoleLeader, Status: model.AdtyTeamMemberActive}
		if err := tx.Create(&member).Error; err != nil {
			return err
		}
		created = &AdtyTeamCreated{Team: team, Member: member}
		return nil
	})
	return created, err
}

func JoinAdtyTeam(userId int, teamId int64) (*model.AdtyTeamMember, error) {
	var member *model.AdtyTeamMember
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		user, err := lockAdtyTeamUser(tx, userId)
		if err != nil {
			return err
		}
		var team model.AdtyTeam
		if err := model.AdtyLockForUpdate(tx).First(&team, teamId).Error; err != nil {
			return err
		}
		if team.Status != model.AdtyTeamStatusActive {
			return ErrAdtyTeamClosed
		}
		if user.SiteId != team.SiteId || user.StaffSiteId != 0 {
			return ErrAdtyTeamForbidden
		}
		if err := rejectActiveTeamMembership(tx, user.Id); err != nil {
			return err
		}
		row := model.AdtyTeamMember{TeamId: team.Id, UserId: user.Id, Role: model.AdtyTeamRoleMember, Status: model.AdtyTeamMemberActive}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		member = &row
		return nil
	})
	return member, err
}

func CreditAdtyTeam(leaderId, teamId int64, quota int64, ref string) error {
	if quota <= 0 || strings.TrimSpace(ref) == "" {
		return ErrAdtyTeamInvalid
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		team, err := lockActiveTeam(tx, teamId)
		if err != nil {
			return err
		}
		if int64(team.LeaderUserId) != leaderId {
			return ErrAdtyTeamForbidden
		}
		return applyAdtyTeamQuotaTx(tx, team, quota, "credit", ref, int(leaderId))
	})
}

func SpendAdtyTeam(userId int, quota int64, ref string) error {
	if quota <= 0 || strings.TrimSpace(ref) == "" {
		return ErrAdtyTeamInvalid
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var membership model.AdtyTeamMember
		lookup := tx.Where("user_id = ?", userId).Order("id DESC").Limit(1).Find(&membership)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected == 0 {
			return ErrAdtyTeamForbidden
		}
		var team model.AdtyTeam
		if err := model.AdtyLockForUpdate(tx).First(&team, membership.TeamId).Error; err != nil {
			return err
		}
		if team.Status != model.AdtyTeamStatusActive {
			return ErrAdtyTeamClosed
		}
		if membership.Status != model.AdtyTeamMemberActive {
			return ErrAdtyTeamForbidden
		}
		if team.SharedQuota < quota {
			return ErrAdtyTeamFunds
		}
		return applyAdtyTeamQuotaTx(tx, &team, -quota, "spend", ref, userId)
	})
}

func CloseAdtyTeam(leaderId int, teamId int64) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		team, err := lockActiveTeam(tx, teamId)
		if err != nil {
			return err
		}
		if team.LeaderUserId != leaderId {
			return ErrAdtyTeamForbidden
		}
		if err := tx.Model(&model.AdtyTeamMember{}).Where("team_id = ? AND status = ?", team.Id, model.AdtyTeamMemberActive).
			Update("status", model.AdtyTeamMemberLeft).Error; err != nil {
			return err
		}
		return tx.Model(team).Updates(map[string]any{"status": model.AdtyTeamStatusClosed, "closed_at": time.Now().Unix()}).Error
	})
}

func lockAdtyTeamUser(tx *gorm.DB, id int) (*model.User, error) {
	var user model.User
	if err := model.AdtyLockForUpdate(tx).First(&user, id).Error; err != nil {
		return nil, err
	}
	if user.Status != common.UserStatusEnabled {
		return nil, ErrAdtyTeamForbidden
	}
	return &user, nil
}

func rejectActiveTeamMembership(tx *gorm.DB, userId int) error {
	var count int64
	if err := tx.Model(&model.AdtyTeamMember{}).Where("user_id = ? AND status = ?", userId, model.AdtyTeamMemberActive).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrAdtyTeamBusy
	}
	return nil
}

func lockActiveTeam(tx *gorm.DB, teamId int64) (*model.AdtyTeam, error) {
	var team model.AdtyTeam
	if err := model.AdtyLockForUpdate(tx).First(&team, teamId).Error; err != nil {
		return nil, err
	}
	if team.Status != model.AdtyTeamStatusActive {
		return nil, ErrAdtyTeamClosed
	}
	return &team, nil
}

func applyAdtyTeamQuotaTx(tx *gorm.DB, team *model.AdtyTeam, delta int64, kind, ref string, actor int) error {
	before := team.SharedQuota
	after := before + delta
	if after < 0 {
		return ErrAdtyTeamFunds
	}
	if err := tx.Model(team).Update("shared_quota", after).Error; err != nil {
		return err
	}
	team.SharedQuota = after
	return tx.Create(&model.AdtyTeamLedger{
		TeamId: team.Id, DeltaQuota: delta, BalanceBefore: before, BalanceAfter: after,
		Type: kind, RefId: ref, ActorUserId: actor, IdempotencyKey: kind + ":" + ref,
	}).Error
}
