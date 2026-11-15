package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

var (
	ErrAdtyTeamRegistrationExpired    = common.NewMessage("This team registration link has expired")
	ErrAdtyTeamRegistrationRevoked    = common.NewMessage("This team registration link has been revoked")
	ErrAdtyTeamRegistrationUsed       = common.NewMessage("This team registration link has already been used")
	ErrAdtyTeamRegistrationUnverified = common.NewMessage("This account could not be verified")
)

// AdtyTeamRegistrationIssue is returned once when a create-team link is issued.
// The plaintext code is not stored.
type AdtyTeamRegistrationIssue struct {
	Id        int64  `json:"id"`
	Code      string `json:"code"`
	SiteId    int    `json:"site_id"`
	ExpiresAt int64  `json:"expires_at"`
	CreatedBy int    `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
	Status    string `json:"status"`
}

// AdtyTeamRegistrationView is a secret-free list row for issued create-team links.
type AdtyTeamRegistrationView struct {
	Id        int64  `json:"id"`
	SiteId    int    `json:"site_id"`
	CreatedBy int    `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
	UsedBy    int    `json:"used_by"`
	UsedAt    int64  `json:"used_at"`
	TeamId    int64  `json:"team_id"`
	RevokedBy int    `json:"revoked_by"`
	RevokedAt int64  `json:"revoked_at"`
	Status    string `json:"status"`
}

func hashAdtyTeamRegistrationCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func newAdtyTeamRegistrationCode() (code, hash string, err error) {
	raw := make([]byte, 16)
	if _, err = rand.Read(raw); err != nil {
		return "", "", err
	}
	code = hex.EncodeToString(raw)
	return code, hashAdtyTeamRegistrationCode(code), nil
}

func adtyTeamRegistrationTTLValid(ttl time.Duration) bool {
	return ttl > 0 && ttl <= adtyTeamInviteMaxTTL
}

// CreateAdtyMainTeamRegistration issues a main-site create-team link. Only Root may call this.
func CreateAdtyMainTeamRegistration(actorId int, ttl time.Duration) (*AdtyTeamRegistrationIssue, error) {
	if !adtyTeamRegistrationTTLValid(ttl) {
		return nil, ErrAdtyTeamInvalid
	}
	var actor model.User
	if err := model.DB.Select("id", "role").First(&actor, actorId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAdtyTeamForbidden
		}
		return nil, err
	}
	if actor.Role != common.RoleRootUser {
		return nil, ErrAdtyTeamForbidden
	}
	return issueAdtyTeamRegistration(actorId, 0, ttl)
}

// CreateAdtyOwnerSiteTeamRegistration issues a create-team link bound to the
// owner's own site. The site comes from the trusted owner context.
func CreateAdtyOwnerSiteTeamRegistration(ownerId, siteId int, ttl time.Duration) (*AdtyTeamRegistrationIssue, error) {
	if !adtyTeamRegistrationTTLValid(ttl) {
		return nil, ErrAdtyTeamInvalid
	}
	if siteId <= 0 {
		return nil, ErrAdtyTeamForbidden
	}
	site, err := lockOwnerSite(siteId, ownerId)
	if err != nil {
		return nil, err
	}
	if site.Status != model.AdtySiteStatusActive {
		return nil, ErrAdtyTeamForbidden
	}
	return issueAdtyTeamRegistration(ownerId, site.Id, ttl)
}

func issueAdtyTeamRegistration(actorId, siteId int, ttl time.Duration) (*AdtyTeamRegistrationIssue, error) {
	code, hash, err := newAdtyTeamRegistrationCode()
	if err != nil {
		return nil, err
	}
	link := model.AdtyTeamRegistrationLink{
		SiteId: siteId, CodeHash: hash, CreatedBy: actorId, ExpiresAt: time.Now().Add(ttl).Unix(),
	}
	if err := model.DB.Create(&link).Error; err != nil {
		return nil, err
	}
	issued := &AdtyTeamRegistrationIssue{
		Id: link.Id, Code: code, SiteId: link.SiteId, ExpiresAt: link.ExpiresAt,
		CreatedBy: link.CreatedBy, CreatedAt: link.CreatedAt,
		Status: model.AdtyTeamRegistrationStatusOutstanding,
	}
	writeAdtyTeamRegistrationAudit(actorId, siteId, link.Id, "adty.team.registration.create", nil, map[string]any{
		"link_id": link.Id, "site_id": siteId, "created_by": actorId,
		"expires_at": link.ExpiresAt, "result": model.AuditResultSuccess,
	}, model.AuditResultSuccess)
	return issued, nil
}

// CompleteAdtyTeamRegistration creates the team and leader from a one-time link.
// requestedSiteID, when provided, must match the site frozen at issuance.
func CompleteAdtyTeamRegistration(code, username, password, teamName string, requestedSiteID *int) (*AdtyTeamCreated, error) {
	code = strings.TrimSpace(code)
	username = strings.TrimSpace(username)
	teamName = strings.TrimSpace(teamName)
	if len(code) < 16 || username == "" || strings.Contains(username, "@") || len(username) > 20 || teamName == "" || password == "" {
		return nil, ErrAdtyTeamInvalid
	}
	hash := hashAdtyTeamRegistrationCode(code)
	var created *AdtyTeamCreated
	var link model.AdtyTeamRegistrationLink
	var newUser *model.User
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		lookup := model.AdtyLockForUpdate(tx).Where("code_hash = ?", hash).Limit(1).Find(&link)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected == 0 {
			return ErrAdtyTeamInvalid
		}
		now := time.Now().Unix()
		switch model.AdtyTeamRegistrationStatus(link, now) {
		case model.AdtyTeamRegistrationStatusUsed:
			return ErrAdtyTeamRegistrationUsed
		case model.AdtyTeamRegistrationStatusRevoked:
			return ErrAdtyTeamRegistrationRevoked
		case model.AdtyTeamRegistrationStatusExpired:
			return ErrAdtyTeamRegistrationExpired
		}
		if requestedSiteID != nil && *requestedSiteID != link.SiteId {
			return ErrAdtyTeamForbidden
		}
		leaderID, createdUser, err := resolveAdtyTeamRegistrationLeader(tx, link.SiteId, username, password)
		if err != nil {
			return err
		}
		row, err := createAdtyTeamTx(tx, leaderID, link.SiteId, teamName, adtyTeamDefaultPayRatio)
		if err != nil {
			return err
		}
		if err := tx.Model(&link).Updates(map[string]any{
			"used_by": leaderID, "used_at": now, "team_id": row.Team.Id,
		}).Error; err != nil {
			return err
		}
		link.UsedBy, link.UsedAt, link.TeamId = leaderID, now, row.Team.Id
		created = row
		newUser = createdUser
		return nil
	})
	if err != nil {
		return nil, err
	}
	if newUser != nil {
		newUser.FinishInsert(0)
	}
	writeAdtyTeamRegistrationAudit(link.UsedBy, link.SiteId, link.Id, "adty.team.registration.complete", nil, map[string]any{
		"link_id": link.Id, "site_id": link.SiteId, "created_by": link.CreatedBy,
		"used_by": link.UsedBy, "team_id": link.TeamId, "result": model.AuditResultSuccess,
	}, model.AuditResultSuccess)
	return created, nil
}

func resolveAdtyTeamRegistrationLeader(tx *gorm.DB, siteId int, username, password string) (int, *model.User, error) {
	var existing model.User
	lookup := tx.Unscoped().Where("LOWER(username) = ?", model.NormalizeUsername(username)).Limit(1).Find(&existing)
	if lookup.Error != nil {
		return 0, nil, lookup.Error
	}
	if lookup.RowsAffected == 0 {
		if err := common.ValidateNewAccountPassword(password); err != nil {
			return 0, nil, err
		}
		user := &model.User{
			Username:    username,
			Password:    password,
			DisplayName: username,
			Role:        common.RoleCommonUser,
			Status:      common.UserStatusEnabled,
			Group:       "default",
			AuthVersion: 1,
			SiteId:      siteId,
		}
		if err := user.InsertWithTx(tx, 0); err != nil {
			if errors.Is(err, model.ErrUsernameAlreadyTaken) || errors.Is(err, model.ErrEmailAlreadyTaken) {
				return 0, nil, ErrAdtyTeamInvalid
			}
			return 0, nil, err
		}
		return user.Id, user, nil
	}
	if existing.DeletedAt.Valid {
		return 0, nil, ErrAdtyTeamInvalid
	}
	if existing.SiteId != siteId || existing.StaffSiteId != 0 || existing.Status != common.UserStatusEnabled {
		return 0, nil, ErrAdtyTeamForbidden
	}
	if existing.Password == "" {
		return 0, nil, ErrAdtyTeamRegistrationUnverified
	}
	// 这个入口是匿名的（router 只挂了 IP 级限流），口令校验必须和登录共用同一套
	// 失败计数与锁定：否则一枚注册链接就是全平台用户名的密码预言机，猜错也不计数
	// （ISS-R4-0105）。锁上之后连密码正确也返回同一句话，不额外暴露状态。
	if common.LoginLocked(existing.Username) {
		return 0, nil, ErrAdtyTeamRegistrationUnverified
	}
	if !common.ValidatePasswordAndHash(password, existing.Password) {
		if err := common.RecordLoginFailure(existing.Username); err != nil {
			common.SysLog(common.LogText("team registration failure lock unavailable: %s", err.Error()))
		}
		return 0, nil, ErrAdtyTeamRegistrationUnverified
	}
	common.ClearLoginFailures(existing.Username)
	return existing.Id, nil, nil
}

func ListAdtyMainTeamRegistrations(actorId int) ([]AdtyTeamRegistrationView, error) {
	var actor model.User
	if err := model.DB.Select("id", "role").First(&actor, actorId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAdtyTeamForbidden
		}
		return nil, err
	}
	if actor.Role != common.RoleRootUser {
		return nil, ErrAdtyTeamForbidden
	}
	return listAdtyTeamRegistrations(0)
}

func ListAdtyOwnerSiteTeamRegistrations(ownerId, siteId int) ([]AdtyTeamRegistrationView, error) {
	if siteId <= 0 {
		return nil, ErrAdtyTeamForbidden
	}
	if _, err := lockOwnerSite(siteId, ownerId); err != nil {
		return nil, err
	}
	return listAdtyTeamRegistrations(siteId)
}

func listAdtyTeamRegistrations(siteId int) ([]AdtyTeamRegistrationView, error) {
	var rows []model.AdtyTeamRegistrationLink
	if err := model.DB.Where("site_id = ?", siteId).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	out := make([]AdtyTeamRegistrationView, 0, len(rows))
	for _, row := range rows {
		out = append(out, adtyTeamRegistrationView(row, now))
	}
	return out, nil
}

func RevokeAdtyMainTeamRegistration(actorId int, linkId int64) (*AdtyTeamRegistrationView, error) {
	var actor model.User
	if err := model.DB.Select("id", "role").First(&actor, actorId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAdtyTeamForbidden
		}
		return nil, err
	}
	if actor.Role != common.RoleRootUser {
		return nil, ErrAdtyTeamForbidden
	}
	return revokeAdtyTeamRegistration(actorId, 0, linkId)
}

func RevokeAdtyOwnerSiteTeamRegistration(ownerId, siteId int, linkId int64) (*AdtyTeamRegistrationView, error) {
	if siteId <= 0 {
		return nil, ErrAdtyTeamForbidden
	}
	if _, err := lockOwnerSite(siteId, ownerId); err != nil {
		return nil, err
	}
	return revokeAdtyTeamRegistration(ownerId, siteId, linkId)
}

func revokeAdtyTeamRegistration(actorId, siteId int, linkId int64) (*AdtyTeamRegistrationView, error) {
	if linkId <= 0 {
		return nil, ErrAdtyTeamInvalid
	}
	var view AdtyTeamRegistrationView
	var createdBy int
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var link model.AdtyTeamRegistrationLink
		lookup := model.AdtyLockForUpdate(tx).Where("id = ? AND site_id = ?", linkId, siteId).Limit(1).Find(&link)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected == 0 {
			return ErrAdtyTeamForbidden
		}
		if link.UsedAt != 0 || link.RevokedAt != 0 {
			return ErrAdtyTeamInvalid
		}
		now := time.Now().Unix()
		if err := tx.Model(&link).Updates(map[string]any{"revoked_by": actorId, "revoked_at": now}).Error; err != nil {
			return err
		}
		link.RevokedBy, link.RevokedAt = actorId, now
		createdBy = link.CreatedBy
		view = adtyTeamRegistrationView(link, now)
		return nil
	})
	if err != nil {
		return nil, err
	}
	writeAdtyTeamRegistrationAudit(actorId, siteId, view.Id, "adty.team.registration.revoke", map[string]any{
		"link_id": view.Id, "site_id": siteId, "status": model.AdtyTeamRegistrationStatusOutstanding,
	}, map[string]any{
		"link_id": view.Id, "site_id": siteId, "created_by": createdBy,
		"revoked_by": actorId, "revoked_at": view.RevokedAt, "result": model.AuditResultSuccess,
		"status": model.AdtyTeamRegistrationStatusRevoked,
	}, model.AuditResultSuccess)
	return &view, nil
}

func adtyTeamRegistrationView(link model.AdtyTeamRegistrationLink, now int64) AdtyTeamRegistrationView {
	return AdtyTeamRegistrationView{
		Id: link.Id, SiteId: link.SiteId, CreatedBy: link.CreatedBy, CreatedAt: link.CreatedAt,
		ExpiresAt: link.ExpiresAt, UsedBy: link.UsedBy, UsedAt: link.UsedAt, TeamId: link.TeamId,
		RevokedBy: link.RevokedBy, RevokedAt: link.RevokedAt,
		Status: model.AdtyTeamRegistrationStatus(link, now),
	}
}

func writeAdtyTeamRegistrationAudit(actorId, siteId int, linkId int64, action string, before, after any, result string) {
	rec := model.OfficialAuditWrite{
		UserId: actorId, SiteID: model.OfficialAuditSiteIDPointer(siteId),
		ObjectType: "team_registration", ObjectID: strconv.FormatInt(linkId, 10),
		Action: action, Category: model.AuditCategoryOperation, Content: action,
		Before: before, After: after, Result: result, Success: result == model.AuditResultSuccess,
	}
	if actorId > 0 {
		if user, err := model.GetUserById(actorId, false); err == nil && user != nil {
			rec.Username = user.Username
			rec.ActorRole = user.Role
		}
	}
	if err := model.WriteOfficialAuditLog(nil, rec); err != nil {
		common.SysLog(common.LogText("official team registration audit write failed: %s", err.Error()))
	}
}
