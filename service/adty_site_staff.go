package service

import (
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	ErrAdtyStaffDenied   = common.NewMessage("Staff access is not permitted")
	ErrAdtyStaffInput    = common.NewMessage("Invalid staff role, permissions or account details")
	ErrAdtyStaffConflict = common.NewMessage("The staff account changed. Refresh and try again.")
)

var adtyStaffPermissions = []string{"customers.read", "customers.balance", "customers.topups", "customers.usage"}

type AdtyStaffInput struct {
	Username        string    `json:"username"`
	DisplayName     string    `json:"display_name"`
	Password        string    `json:"password"`
	Role            string    `json:"role"`
	Permissions     *[]string `json:"permissions"`
	Status          string    `json:"status"`
	ExpectedVersion int64     `json:"expected_version"`
}

type AdtyStaffView struct {
	model.AdtySiteStaff
	Permissions []string `json:"permissions"`
}

func AdtyStaffViewOf(staff model.AdtySiteStaff) AdtyStaffView {
	permissions := []string{}
	_ = common.UnmarshalJsonStr(staff.Permissions, &permissions)
	return AdtyStaffView{AdtySiteStaff: staff, Permissions: permissions}
}

// Roles never grant write authority. An explicit empty array grants nothing.
func adtyStaffGrants(role string, custom *[]string) ([]string, error) {
	var grants []string
	switch role {
	case "support":
		grants = []string{"customers.read"}
	case "finance", "manager":
		grants = slices.Clone(adtyStaffPermissions)
	case "operations":
		grants = []string{"customers.read", "customers.usage"}
	default:
		return nil, ErrAdtyStaffInput
	}
	if custom != nil {
		grants = slices.Clone(*custom)
	}
	if grants == nil {
		grants = []string{}
	}
	for _, grant := range grants {
		if !slices.Contains(adtyStaffPermissions, grant) {
			return nil, ErrAdtyStaffInput
		}
	}
	slices.Sort(grants)
	return slices.Compact(grants), nil
}

// Recheck ownership inside the mutation transaction, including account status,
// the current agreement and site state. A cached HTTP precheck is insufficient.
func adtyStaffOwnerTx(tx *gorm.DB, ownerID, siteID int) error {
	var owner model.User
	if err := model.AdtyLockForUpdate(tx).First(&owner, ownerID).Error; err != nil {
		return err
	}
	if owner.Status != common.UserStatusEnabled || owner.StaffSiteId != 0 {
		return ErrAdtyStaffDenied
	}
	var site model.AdtySite
	if err := model.AdtyLockForUpdate(tx).First(&site, siteID).Error; err != nil {
		return err
	}
	if site.OwnerUserId != ownerID || site.AgreementVer != AdtyCurrentSiteAgreementVersion() || (site.Status != model.AdtySiteStatusActive && site.Status != model.AdtySiteStatusPaused && site.Status != model.AdtySiteStatusClosing) {
		return ErrAdtyStaffDenied
	}
	return nil
}

func CreateAdtyStaff(ownerID, siteID int, input AdtyStaffInput) (*AdtyStaffView, error) {
	input.Username, input.DisplayName = strings.TrimSpace(input.Username), strings.TrimSpace(input.DisplayName)
	if input.Username == "" || input.DisplayName == "" || utf8.RuneCountInString(input.DisplayName) > 20 || utf8.RuneCountInString(input.Password) < 15 {
		return nil, ErrAdtyStaffInput
	}
	if err := common.Validate.StructPartial(&model.User{Username: input.Username}, "Username"); err != nil {
		return nil, ErrAdtyStaffInput
	}
	grants, err := adtyStaffGrants(input.Role, input.Permissions)
	if err != nil {
		return nil, err
	}
	hash, err := common.HashAccountPassword(input.Password)
	if err != nil {
		return nil, err
	}
	var staff model.AdtySiteStaff
	err = model.DB.Transaction(func(tx *gorm.DB) error {
		if err := adtyStaffOwnerTx(tx, ownerID, siteID); err != nil {
			return err
		}
		// Global login names cannot be reassigned, including deleted accounts.
		var count int64
		if err := tx.Unscoped().Model(&model.User{}).Where("username = ?", input.Username).Count(&count).Error; err != nil {
			return err
		}
		if count != 0 {
			return ErrAdtyStaffInput
		}
		user := model.User{Username: input.Username, DisplayName: input.DisplayName, Password: hash, Role: common.RoleCommonUser,
			Status: common.UserStatusEnabled, Group: "default", SiteId: siteID, StaffSiteId: siteID, AuthVersion: 1}
		if err := tx.Session(&gorm.Session{Logger: logger.Discard}).Create(&user).Error; err != nil {
			return ErrAdtyStaffInput
		}
		staff = model.AdtySiteStaff{SiteId: siteID, OwnerUserId: ownerID, UserId: user.Id, Username: user.Username,
			DisplayName: input.DisplayName, Role: input.Role, Permissions: common.GetJsonString(grants), Status: "enabled", Version: 1}
		if err := tx.Create(&staff).Error; err != nil {
			return err
		}
		return recordSiteAuditTx(tx, siteID, "staff.create", ownerID, nil, AdtyStaffViewOf(staff), "")
	})
	if err != nil {
		return nil, err
	}
	view := AdtyStaffViewOf(staff)
	return &view, nil
}

// Mutation and version advancement are atomic with the audit record. Disabled
// and deleted accounts keep their dedicated identity; enabling never revives JWTs.
func MutateAdtyStaff(ownerID, siteID, staffID int, action string, input AdtyStaffInput) (*AdtyStaffView, error) {
	var hash string
	if action == AdtySiteOwnerActionStaffPassword {
		if utf8.RuneCountInString(input.Password) < 15 {
			return nil, ErrAdtyStaffInput
		}
		var err error
		hash, err = common.HashAccountPassword(input.Password)
		if err != nil {
			return nil, err
		}
	}
	var staff model.AdtySiteStaff
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		if err := adtyStaffOwnerTx(tx, ownerID, siteID); err != nil {
			return err
		}
		if err := model.AdtyLockForUpdate(tx).Where("id = ? AND site_id = ? AND owner_user_id = ? AND status <> ?", staffID, siteID, ownerID, "deleted").First(&staff).Error; err != nil {
			return err
		}
		if input.ExpectedVersion <= 0 || staff.Version != input.ExpectedVersion {
			return ErrAdtyStaffConflict
		}
		before := AdtyStaffViewOf(staff)
		userUpdates := map[string]any{}
		switch action {
		case AdtySiteOwnerActionStaffUpdate:
			input.DisplayName = strings.TrimSpace(input.DisplayName)
			if input.DisplayName == "" || utf8.RuneCountInString(input.DisplayName) > 20 || (input.Status != "enabled" && input.Status != "disabled") {
				return ErrAdtyStaffInput
			}
			if input.Permissions == nil && input.Role == staff.Role {
				input.Permissions = &before.Permissions
			}
			grants, err := adtyStaffGrants(input.Role, input.Permissions)
			if err != nil {
				return err
			}
			staff.DisplayName, staff.Role, staff.Permissions, staff.Status = input.DisplayName, input.Role, common.GetJsonString(grants), input.Status
			userUpdates["display_name"] = staff.DisplayName
			userUpdates["status"] = common.UserStatusDisabled
			if staff.Status == "enabled" {
				userUpdates["status"] = common.UserStatusEnabled
			}
		case AdtySiteOwnerActionStaffPassword:
			userUpdates["password"] = hash
		case AdtySiteOwnerActionStaffDelete:
			staff.Status = "deleted"
			userUpdates["status"] = common.UserStatusDisabled
		default:
			return ErrAdtyStaffInput
		}
		if _, err := model.IncrementUserAuthVersionWithTx(tx, staff.UserId); err != nil {
			return err
		}
		if err := tx.Session(&gorm.Session{Logger: logger.Discard}).Model(&model.User{}).Where("id = ? AND staff_site_id = ?", staff.UserId, siteID).Updates(userUpdates).Error; err != nil {
			return ErrAdtyStaffInput
		}
		staff.Version++
		result := tx.Model(&model.AdtySiteStaff{}).Where("id = ? AND version = ?", staffID, input.ExpectedVersion).Updates(map[string]any{
			"display_name": staff.DisplayName, "role": staff.Role, "permissions": staff.Permissions, "status": staff.Status, "version": staff.Version})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrAdtyStaffConflict
		}
		return recordSiteAuditTx(tx, siteID, action, ownerID, before, AdtyStaffViewOf(staff), "")
	})
	if err != nil {
		return nil, err
	}
	if err := model.PublishUserAuthCache(staff.UserId); err != nil {
		common.SysError("Failed to publish staff auth cache")
	}
	view := AdtyStaffViewOf(staff)
	return &view, nil
}

// Always read the current membership, user status/version and owner from SQL.
// Authorization cannot be inherited by transferring a site or replaying a JWT.
func AdtyStaffAccess(userID int, authVersion int64, permission string) (*model.AdtySiteStaff, *model.AdtySite, error) {
	var staff model.AdtySiteStaff
	if err := model.DB.Where("user_id = ? AND status = ?", userID, "enabled").First(&staff).Error; err != nil {
		return nil, nil, ErrAdtyStaffDenied
	}
	var user model.User
	if err := model.DB.Select("id", "staff_site_id", "site_id", "status", "auth_version", "role").First(&user, userID).Error; err != nil {
		return nil, nil, err
	}
	if user.Status != common.UserStatusEnabled || user.AuthVersion != authVersion || user.StaffSiteId != staff.SiteId || user.SiteId != staff.SiteId || user.Role != common.RoleCommonUser {
		return nil, nil, ErrAdtyStaffDenied
	}
	var site model.AdtySite
	if err := model.DB.First(&site, staff.SiteId).Error; err != nil {
		return nil, nil, err
	}
	if site.OwnerUserId != staff.OwnerUserId || site.AgreementVer != AdtyCurrentSiteAgreementVersion() || (site.Status != model.AdtySiteStatusActive && site.Status != model.AdtySiteStatusPaused && site.Status != model.AdtySiteStatusClosing) {
		return nil, nil, ErrAdtyStaffDenied
	}
	grants := AdtyStaffViewOf(staff).Permissions
	if permission != "" && (!slices.Contains(adtyStaffPermissions, permission) || !slices.Contains(grants, permission)) {
		return nil, nil, ErrAdtyStaffDenied
	}
	return &staff, &site, nil
}

// Staff visibility is the intersection of platform visibility and explicit
// grants. Emails are always masked and customer API keys have no staff endpoint.
func AdtyStaffCustomerSite(site *model.AdtySite, staff *model.AdtySiteStaff) *model.AdtySite {
	copy := *site
	visibility := AdtySiteVisibility(site)
	visibility["customer_email"] = "masked"
	visibility["customer_tokens"] = "off"
	for grant, key := range map[string]string{"customers.balance": "customer_balance", "customers.usage": "customer_usage", "customers.topups": "customer_topups"} {
		if !slices.Contains(AdtyStaffViewOf(*staff).Permissions, grant) {
			visibility[key] = "off"
		}
	}
	copy.Visibility = common.GetJsonString(visibility)
	return &copy
}

func AdtyStaffErrorStatus(err error) int {
	switch {
	case errors.Is(err, ErrAdtyStaffDenied):
		return 403
	case errors.Is(err, gorm.ErrRecordNotFound):
		return 404
	case errors.Is(err, ErrAdtyStaffConflict):
		return 409
	case errors.Is(err, ErrAdtyStaffInput), errors.Is(err, common.ErrAccountPasswordLength), errors.Is(err, common.ErrPasswordLegacyLimit):
		return 400
	default:
		return 500
	}
}
