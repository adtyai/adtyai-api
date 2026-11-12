package service

import (
	"database/sql"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

var ErrAdtySiteOwnershipConflict = common.NewMessage("Site ownership changed; reload the site before trying again")
var ErrAdtySiteOwnershipTarget = common.NewMessage("Choose an enabled main-site user who does not own another open site")
var ErrAdtySiteOwnerImmutable = common.NewMessage("Site owners cannot be replaced")

type AdtySiteOwnerHandoverInput struct {
	SiteId                   int    `json:"-"`
	OperatorId               int    `json:"-"`
	ExpectedOwnerUserId      int    `json:"expected_owner_user_id"`
	ExpectedOwnershipVersion *int64 `json:"expected_ownership_version"`
	NewOwnerUserId           int    `json:"new_owner_user_id"`
	Reason                   string `json:"reason"`
}

// HandoverAdtySiteOwner refuses owner replacement. Historical ownership rows
// remain readable; this entry must not create a new one or move authority.
func HandoverAdtySiteOwner(input AdtySiteOwnerHandoverInput) (*model.AdtySite, error) {
	input.Reason = strings.TrimSpace(input.Reason)
	if input.SiteId <= 0 || input.OperatorId <= 0 || input.ExpectedOwnerUserId <= 0 || input.NewOwnerUserId <= 0 || input.ExpectedOwnerUserId == input.NewOwnerUserId ||
		input.ExpectedOwnershipVersion == nil || *input.ExpectedOwnershipVersion < 0 || input.Reason == "" || utf8.RuneCountInString(input.Reason) > 500 {
		return nil, common.NewMessage("Invalid parameters")
	}
	var site model.AdtySite
	var options []*sql.TxOptions
	if common.UsingMainDatabase(common.DatabaseTypeMySQL) {
		options = append(options, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	}
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		now := time.Now().Unix()
		ownerIDs := []int{input.ExpectedOwnerUserId, input.NewOwnerUserId}
		slices.Sort(ownerIDs)
		// Match opening/referral transactions: affiliates before user wallets.
		for _, id := range ownerIDs {
			if err := markAffiliateSiteOwnerTx(tx, id, now); err != nil {
				return err
			}
		}
		userIDs := append(slices.Clone(ownerIDs), input.OperatorId)
		slices.Sort(userIDs)
		userIDs = slices.Compact(userIDs)
		var users []model.User
		if err := model.AdtyLockForUpdate(tx).Where("id IN ?", userIDs).Order("id").Find(&users).Error; err != nil {
			return err
		}
		var actor, target *model.User
		for i := range users {
			if users[i].Id == input.OperatorId {
				actor = &users[i]
			}
			if users[i].Id == input.NewOwnerUserId {
				target = &users[i]
			}
		}
		if actor == nil || actor.Role != common.RoleRootUser || actor.Status != common.UserStatusEnabled || actor.SiteId != 0 || actor.StaffSiteId != 0 {
			return common.NewMessage("No permission to perform this action")
		}
		if target == nil || target.Status != common.UserStatusEnabled || target.SiteId != 0 || target.StaffSiteId != 0 {
			return ErrAdtySiteOwnershipTarget
		}
		if err := model.AdtyLockForUpdate(tx).First(&site, input.SiteId).Error; err != nil {
			return err
		}
		if site.OwnerUserId != input.ExpectedOwnerUserId || site.OwnershipVersion != *input.ExpectedOwnershipVersion {
			return ErrAdtySiteOwnershipConflict
		}
		if site.Status != model.AdtySiteStatusActive && site.Status != model.AdtySiteStatusPaused {
			return ErrAdtySiteStatus
		}
		var owned int64
		if err := tx.Model(&model.AdtySite{}).Where("owner_user_id = ? AND status <> ?", target.Id, model.AdtySiteStatusClosed).Count(&owned).Error; err != nil {
			return err
		}
		if owned > 0 {
			return ErrAdtySiteAlreadyOwner
		}
		return ErrAdtySiteOwnerImmutable
	}, options...)
	if err == nil {
		err = ErrAdtySiteOwnerImmutable
	}
	return nil, err
}
