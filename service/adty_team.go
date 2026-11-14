package service

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// adtyTeamTableReady caches a positive schema check per connection pool.
// A missing team table means no binding can exist, so personal billing continues.
var adtyTeamTableReady sync.Map

func adtyTeamTableReadyFor(tx *gorm.DB, modelValue any) bool {
	if tx == nil {
		return false
	}
	sqlDB, err := tx.DB()
	if err != nil {
		return false
	}
	key := struct {
		db    any
		model reflect.Type
	}{sqlDB, reflect.TypeOf(modelValue)}
	if ready, ok := adtyTeamTableReady.Load(key); ok {
		return ready.(bool)
	}
	ready := tx.Migrator().HasTable(modelValue)
	if ready {
		adtyTeamTableReady.Store(key, true)
	}
	return ready
}

func init() {
	model.AdtyTeamReserveHook = reserveAdtyTeamQuotaHook
	model.AdtyBeforeUserDeleteHook = closeAdtyTeamForDeletedLeader
	model.AdtyCreditTeamTopUpTx = creditAdtyTeamTopUpTx
}

func creditAdtyTeamTopUpTx(tx *gorm.DB, topUp *model.TopUp, ext *model.AdtyTopupOrder, principal int64) error {
	if principal <= 0 || ext == nil || ext.TeamId <= 0 {
		return ErrAdtyTeamInvalid
	}
	var team model.AdtyTeam
	if err := model.AdtyLockForUpdate(tx).First(&team, ext.TeamId).Error; err != nil {
		return err
	}
	if team.Status != model.AdtyTeamStatusActive || team.LeaderUserId != topUp.UserId {
		return ErrAdtyTeamForbidden
	}
	return applyAdtyTeamQuotaTx(tx, &team, principal, "topup", ext.TradeNo, topUp.UserId)
}

// SaveAdtyMainTeamAgreementPrice sets a main-site team discount. Subsite teams are rejected.
// There is no supply-price floor. The discount must be greater than zero and at most 1.
func SaveAdtyMainTeamAgreementPrice(teamID int64, raw string) error {
	value, err := decimal.NewFromString(strings.TrimSpace(raw))
	if teamID <= 0 || err != nil || !value.IsPositive() || value.GreaterThan(decimal.NewFromInt(1)) {
		return ErrAdtyTeamInvalid
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var team model.AdtyTeam
		if err := model.AdtyLockForUpdate(tx).First(&team, teamID).Error; err != nil {
			return err
		}
		if team.SiteId != 0 || team.Status != model.AdtyTeamStatusActive {
			return ErrAdtyTeamForbidden
		}
		return tx.Model(&team).Update("discount", value).Error
	})
}

var (
	ErrAdtyTeamInvalid     = common.NewMessage("Invalid parameters")
	ErrAdtyTeamForbidden   = common.NewMessage("No permission to perform this action")
	ErrAdtyTeamBusy        = common.NewMessage("This account already belongs to a team")
	ErrAdtyTeamFunds       = common.NewMessage("Insufficient available funds")
	ErrAdtyTeamClosed      = common.NewMessage("This team is closed")
	ErrAdtyTeamLeaderFixed = common.NewMessage("The team leader cannot be replaced")
)

type AdtyTeamCreated struct {
	Team   model.AdtyTeam
	Member model.AdtyTeamMember
}

type AdtyTeamMemberView struct {
	UserId   int    `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Status   string `json:"status"`
}

type AdtyTeamKeyView struct {
	TokenId int    `json:"token_id"`
	Name    string `json:"name"`
	UserId  int    `json:"user_id"`
	Status  string `json:"status"`
}

type AdtyTeamMine struct {
	TeamId       int64                `json:"team_id"`
	Name         string               `json:"name"`
	SiteId       int                  `json:"site_id"`
	SourceSiteId int                  `json:"source_site_id"`
	Role         string               `json:"role"`
	Discount     decimal.Decimal      `json:"discount"`
	SharedQuota  int64                `json:"shared_quota"`
	Status       string               `json:"status"`
	Members      []AdtyTeamMemberView `json:"members"`
	Keys         []AdtyTeamKeyView    `json:"keys"`
}

// AdtyTeamInviteDefaultTTL is the existing controller default when the caller
// does not set a validity period. It is not a newly signed official number.
const AdtyTeamInviteDefaultTTL = 24 * time.Hour

// adtyTeamInviteMaxTTL is the existing CreateAdtyTeamInvite upper bound.
const adtyTeamInviteMaxTTL = 7 * 24 * time.Hour

// AdtyTeamInviteIssue is returned once when an invite is created. The plaintext
// code is not stored and is omitted from later list views.
type AdtyTeamInviteIssue struct {
	Id        int64  `json:"id"`
	Code      string `json:"code,omitempty"`
	ExpiresAt int64  `json:"expires_at"`
	CreatedBy int    `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
	Status    string `json:"status"`
}

// AdtyTeamInviteView is a secret-free list row for a team's invites.
type AdtyTeamInviteView struct {
	Id        int64  `json:"id"`
	TeamId    int64  `json:"team_id"`
	CreatedBy int    `json:"created_by"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt int64  `json:"expires_at"`
	UsedBy    int    `json:"used_by"`
	UsedAt    int64  `json:"used_at"`
	RevokedBy int    `json:"revoked_by"`
	RevokedAt int64  `json:"revoked_at"`
	Status    string `json:"status"`
}

func GetAdtyTeamMine(userId int) (*AdtyTeamMine, error) {
	var membership model.AdtyTeamMember
	lookup := model.DB.Where("user_id = ? AND status = ?", userId, model.AdtyTeamMemberActive).Limit(1).Find(&membership)
	if lookup.Error != nil {
		return nil, lookup.Error
	}
	if lookup.RowsAffected == 0 {
		return nil, nil
	}
	var team model.AdtyTeam
	if err := model.DB.First(&team, membership.TeamId).Error; err != nil {
		return nil, err
	}
	page := &AdtyTeamMine{
		TeamId: team.Id, Name: team.Name, SiteId: team.SiteId, SourceSiteId: team.SourceSiteId, Role: membership.Role,
		Discount: team.Discount, SharedQuota: team.SharedQuota, Status: team.Status,
		Members: []AdtyTeamMemberView{}, Keys: []AdtyTeamKeyView{},
	}
	if err := fillAdtyTeamPage(page, membership); err != nil {
		return nil, err
	}
	return page, nil
}

func fillAdtyTeamPage(page *AdtyTeamMine, membership model.AdtyTeamMember) error {
	memberQuery := model.DB.Where("team_id = ?", page.TeamId)
	keyQuery := model.DB.Where("team_id = ?", page.TeamId)
	if membership.Role != model.AdtyTeamRoleLeader {
		memberQuery = memberQuery.Where("user_id = ?", membership.UserId)
		keyQuery = keyQuery.Where("user_id = ?", membership.UserId)
	}
	var members []model.AdtyTeamMember
	if err := memberQuery.Order("id ASC").Find(&members).Error; err != nil {
		return err
	}
	ids := make([]int, 0, len(members))
	for _, member := range members {
		ids = append(ids, member.UserId)
	}
	names := map[int]string{}
	if len(ids) > 0 {
		var users []model.User
		if err := model.DB.Select("id", "username").Where("id IN ?", ids).Find(&users).Error; err != nil {
			return err
		}
		for _, user := range users {
			names[user.Id] = user.Username
		}
	}
	for _, member := range members {
		page.Members = append(page.Members, AdtyTeamMemberView{
			UserId: member.UserId, Username: names[member.UserId], Role: member.Role, Status: member.Status,
		})
	}
	var keys []model.AdtyTeamKey
	if err := keyQuery.Order("id ASC").Find(&keys).Error; err != nil {
		return err
	}
	tokenIds := make([]int, 0, len(keys))
	for _, key := range keys {
		tokenIds = append(tokenIds, key.TokenId)
	}
	tokenNames := map[int]string{}
	if len(tokenIds) > 0 {
		var tokens []model.Token
		if err := model.DB.Select("id", "name").Where("id IN ?", tokenIds).Find(&tokens).Error; err != nil {
			return err
		}
		for _, token := range tokens {
			tokenNames[token.Id] = token.Name
		}
	}
	for _, key := range keys {
		page.Keys = append(page.Keys, AdtyTeamKeyView{
			TokenId: key.TokenId, Name: tokenNames[key.TokenId], UserId: key.UserId, Status: key.Status,
		})
	}
	return nil
}

// TransferAdtyTeamLeader is refused. Closing a team does not appoint a new leader.
func TransferAdtyTeamLeader(int, int64, int) error {
	return ErrAdtyTeamLeaderFixed
}

func DisableAdtyTeamMember(leaderId int, teamId int64, memberUserId int) error {
	if memberUserId <= 0 || memberUserId == leaderId {
		return ErrAdtyTeamLeaderFixed
	}
	return model.DB.Transaction(func(tx *gorm.DB) error {
		team, err := lockActiveTeam(tx, teamId)
		if err != nil {
			return err
		}
		if team.LeaderUserId != leaderId {
			return ErrAdtyTeamForbidden
		}
		var member model.AdtyTeamMember
		lookup := tx.Where("team_id = ? AND user_id = ? AND status = ?", team.Id, memberUserId, model.AdtyTeamMemberActive).Limit(1).Find(&member)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected == 0 || member.Role == model.AdtyTeamRoleLeader {
			return ErrAdtyTeamForbidden
		}
		if err := tx.Model(&member).Update("status", model.AdtyTeamMemberLeft).Error; err != nil {
			return err
		}
		return disableAdtyTeamMemberKeysTx(tx, team.Id, memberUserId)
	})
}

func LeaveAdtyTeam(userId int, teamId int64) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var team model.AdtyTeam
		if err := model.AdtyLockForUpdate(tx).First(&team, teamId).Error; err != nil {
			return err
		}
		if team.LeaderUserId == userId {
			return ErrAdtyTeamLeaderFixed
		}
		var member model.AdtyTeamMember
		lookup := tx.Where("team_id = ? AND user_id = ? AND status = ?", team.Id, userId, model.AdtyTeamMemberActive).Limit(1).Find(&member)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected == 0 {
			return ErrAdtyTeamForbidden
		}
		if err := tx.Model(&member).Update("status", model.AdtyTeamMemberLeft).Error; err != nil {
			return err
		}
		return disableAdtyTeamMemberKeysTx(tx, team.Id, userId)
	})
}

func RevokeAdtyTeamKey(actorId int, teamId int64, tokenId int) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var key model.AdtyTeamKey
		lookup := model.AdtyLockForUpdate(tx).Where("team_id = ? AND token_id = ?", teamId, tokenId).Limit(1).Find(&key)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected == 0 {
			return ErrAdtyTeamForbidden
		}
		var team model.AdtyTeam
		if err := tx.First(&team, teamId).Error; err != nil {
			return err
		}
		if actorId != key.UserId && actorId != team.LeaderUserId {
			return ErrAdtyTeamForbidden
		}
		if err := tx.Model(&model.Token{}).Where("id = ?", key.TokenId).Update("status", common.TokenStatusDisabled).Error; err != nil {
			return err
		}
		return tx.Model(&key).Update("status", model.AdtyTeamMemberLeft).Error
	})
}

// CreateAdtyTeamForCurrentUser is the former self-serve create path.
// Ordinary users must not create a team or submit a protocol discount.
func CreateAdtyTeamForCurrentUser(_ int, _ string, _ decimal.Decimal) (*AdtyTeamCreated, error) {
	return nil, ErrAdtyTeamForbidden
}

// adtyTeamDefaultPayRatio is "no discount": the team pays the ordinary price.
var adtyTeamDefaultPayRatio = decimal.NewFromInt(1)

func adtyTeamDiscountOrDefault(raw string) (decimal.Decimal, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return adtyTeamDefaultPayRatio, nil
	}
	value, err := decimal.NewFromString(raw)
	if err != nil {
		return decimal.Zero, ErrAdtyTeamInvalid
	}
	return value, nil
}

func createAdtyTeamHidingForeignLeader(leaderId, siteId int, name string, discount decimal.Decimal) (*AdtyTeamCreated, error) {
	created, err := CreateAdtyTeam(leaderId, siteId, name, discount)
	if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAdtyTeamForbidden
	}
	return created, err
}

// CreateAdtyMainTeamByRoot creates a main-site team and designates an existing
// main-site user as the unique leader. Only Root may call this.
func CreateAdtyMainTeamByRoot(actorId, leaderId int, name, rawDiscount string) (*AdtyTeamCreated, error) {
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
	discount, err := adtyTeamDiscountOrDefault(rawDiscount)
	if err != nil {
		return nil, err
	}
	return createAdtyTeamHidingForeignLeader(leaderId, 0, name, discount)
}

// CreateAdtyOwnerSiteTeam creates a team on the owner's own site and designates
// an existing user of that site as the unique leader. The site comes from the
// trusted owner context, not users.site_id.
func CreateAdtyOwnerSiteTeam(ownerId, siteId, leaderId int, name, rawDiscount string) (*AdtyTeamCreated, error) {
	if siteId <= 0 {
		return nil, ErrAdtyTeamForbidden
	}
	site, err := lockOwnerSite(siteId, ownerId)
	if err != nil {
		return nil, err
	}
	discount, err := adtyTeamDiscountOrDefault(rawDiscount)
	if err != nil {
		return nil, err
	}
	return createAdtyTeamHidingForeignLeader(leaderId, site.Id, name, discount)
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
			return mapAdtyTeamMemberWriteError(err)
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
			return mapAdtyTeamMemberWriteError(err)
		}
		member = &row
		return nil
	})
	return member, err
}

// CreateAdtyTeamTopupOrder records a leader top-up with no payment-method gift.
// It does not credit the team until payment completion.
func CreateAdtyTeamTopupOrder(leaderID int, teamID int64, amountUSD decimal.Decimal, key string) (*model.AdtyTopupOrder, error) {
	key = strings.TrimSpace(key)
	if leaderID <= 0 || teamID <= 0 || !amountUSD.IsPositive() || key == "" {
		return nil, ErrAdtyTeamInvalid
	}
	quota, err := common.WalletQuotaFromDecimalStrict(amountUSD.Mul(common.QuotaPerUnitDecimal()).Floor())
	if err != nil || quota <= 0 {
		return nil, ErrAdtyTeamInvalid
	}
	tradeNo := "TTEAM" + common.Sha1([]byte(key))[:20]
	var existing model.AdtyTopupOrder
	lookup := model.DB.Where("trade_no = ?", tradeNo).Limit(1).Find(&existing)
	if lookup.Error != nil {
		return nil, lookup.Error
	}
	if lookup.RowsAffected == 1 {
		if existing.TeamId != teamID || !existing.FaceUSD.Equal(amountUSD) || existing.UserId != leaderID {
			return nil, ErrAdtyTeamInvalid
		}
		return &existing, nil
	}
	return createAdtyTeamTopupOrder(leaderID, teamID, amountUSD, int64(quota), tradeNo)
}

func createAdtyTeamTopupOrder(leaderID int, teamID int64, amountUSD decimal.Decimal, quota int64, tradeNo string) (*model.AdtyTopupOrder, error) {
	var created *model.AdtyTopupOrder
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		var team model.AdtyTeam
		if err := model.AdtyLockForUpdate(tx).First(&team, teamID).Error; err != nil {
			return err
		}
		if team.Status != model.AdtyTeamStatusActive || team.LeaderUserId != leaderID {
			return ErrAdtyTeamForbidden
		}
		topUp := &model.TopUp{
			UserId: leaderID, Amount: amountUSD.IntPart(), Money: 0, TradeNo: tradeNo,
			PaymentMethod: "team", PaymentProvider: model.PaymentProviderAdtyEpay,
			CreateTime: common.GetTimestamp(), Status: common.TopUpStatusPending,
		}
		if err := tx.Create(topUp).Error; err != nil {
			return err
		}
		ext := &model.AdtyTopupOrder{
			TradeNo: tradeNo, UserId: leaderID, TeamId: teamID, Provider: model.PaymentProviderAdtyEpay,
			Method: "team", FaceUSD: amountUSD, BonusRate: decimal.Zero, BonusUSD: decimal.Zero,
			PromoBonusUSD: decimal.Zero, CreditQuota: quota, PayAmount: amountUSD, PayCurrency: "USD",
			TopupGroupRatio: decimal.NewFromInt(1),
		}
		if err := tx.Create(ext).Error; err != nil {
			return err
		}
		created = ext
		return nil
	})
	return created, err
}

func ListAdtyTeamTopups(leaderID int, teamID int64) ([]model.AdtyTopupOrder, error) {
	var team model.AdtyTeam
	if err := model.DB.First(&team, teamID).Error; err != nil {
		return nil, err
	}
	if team.LeaderUserId != leaderID {
		return nil, ErrAdtyTeamForbidden
	}
	var rows []model.AdtyTopupOrder
	err := model.DB.Where("team_id = ?", teamID).Order("id desc").Limit(20).Find(&rows).Error
	return rows, err
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
		if err := disableAdtyTeamKeysTx(tx, team.Id); err != nil {
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

func mapAdtyTeamMemberWriteError(err error) error {
	if model.IsActiveTeamMembershipUniqueViolation(err) {
		return ErrAdtyTeamBusy
	}
	return err
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

func canManageAdtyTeamInvites(team *model.AdtyTeam, actorId int) bool {
	return team != nil && team.LeaderUserId == actorId
}

func CreateAdtyTeamInvite(actorId int, teamId int64, ttl time.Duration) (*AdtyTeamInviteIssue, error) {
	if ttl <= 0 || ttl > adtyTeamInviteMaxTTL {
		return nil, ErrAdtyTeamInvalid
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	code := hex.EncodeToString(raw)
	sum := sha256.Sum256([]byte(code))
	hash := hex.EncodeToString(sum[:])
	var invite model.AdtyTeamInvite
	var siteID int
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		team, err := lockActiveTeam(tx, teamId)
		if err != nil {
			return err
		}
		if !canManageAdtyTeamInvites(team, actorId) {
			return ErrAdtyTeamForbidden
		}
		invite = model.AdtyTeamInvite{
			TeamId: team.Id, CodeHash: hash, CreatedBy: actorId, ExpiresAt: time.Now().Add(ttl).Unix(),
		}
		if err := tx.Create(&invite).Error; err != nil {
			return err
		}
		siteID = team.SiteId
		return nil
	})
	if err != nil {
		return nil, err
	}
	issued := &AdtyTeamInviteIssue{
		Id: invite.Id, Code: code, ExpiresAt: invite.ExpiresAt,
		CreatedBy: invite.CreatedBy, CreatedAt: invite.CreatedAt,
		Status: model.AdtyTeamInviteStatusOutstanding,
	}
	writeAdtyTeamInviteAudit(actorId, siteID, invite.Id, "adty.team.invite.create", nil, map[string]any{
		"invite_id": invite.Id, "team_id": invite.TeamId, "created_by": actorId,
		"expires_at": invite.ExpiresAt, "result": model.AuditResultSuccess,
	}, model.AuditResultSuccess)
	return issued, nil
}

func AcceptAdtyTeamInvite(userId int, code string) (*model.AdtyTeamMember, error) {
	code = strings.TrimSpace(code)
	if len(code) < 16 {
		return nil, ErrAdtyTeamInvalid
	}
	sum := sha256.Sum256([]byte(code))
	hash := hex.EncodeToString(sum[:])
	var member *model.AdtyTeamMember
	var invite model.AdtyTeamInvite
	var siteID int
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		lookup := model.AdtyLockForUpdate(tx).Where("code_hash = ?", hash).Limit(1).Find(&invite)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected == 0 || invite.UsedAt != 0 || invite.RevokedAt != 0 || invite.ExpiresAt < time.Now().Unix() {
			return ErrAdtyTeamInvalid
		}
		var team model.AdtyTeam
		if err := tx.First(&team, invite.TeamId).Error; err != nil {
			return err
		}
		siteID = team.SiteId
		joined, err := joinAdtyTeamTx(tx, userId, invite.TeamId)
		if err != nil {
			return err
		}
		if err := tx.Model(&invite).Updates(map[string]any{"used_by": userId, "used_at": time.Now().Unix()}).Error; err != nil {
			return err
		}
		invite.UsedBy, invite.UsedAt = userId, time.Now().Unix()
		member = joined
		return nil
	})
	if err != nil {
		return nil, err
	}
	writeAdtyTeamInviteAudit(userId, siteID, invite.Id, "adty.team.invite.accept", nil, map[string]any{
		"invite_id": invite.Id, "team_id": invite.TeamId, "created_by": invite.CreatedBy,
		"used_by": userId, "result": model.AuditResultSuccess,
	}, model.AuditResultSuccess)
	return member, nil
}

func ListAdtyTeamInvites(actorId int, teamId int64) ([]AdtyTeamInviteView, error) {
	var team model.AdtyTeam
	if err := model.DB.First(&team, teamId).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAdtyTeamForbidden
		}
		return nil, err
	}
	if !canManageAdtyTeamInvites(&team, actorId) {
		return nil, ErrAdtyTeamForbidden
	}
	var rows []model.AdtyTeamInvite
	if err := model.DB.Where("team_id = ?", team.Id).Order("id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	out := make([]AdtyTeamInviteView, 0, len(rows))
	for _, row := range rows {
		out = append(out, adtyTeamInviteView(row, now))
	}
	return out, nil
}

func RevokeAdtyTeamInvite(actorId int, teamId, inviteId int64) (*AdtyTeamInviteView, error) {
	if inviteId <= 0 {
		return nil, ErrAdtyTeamInvalid
	}
	var view AdtyTeamInviteView
	var siteID int
	var createdBy int
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		team, err := lockActiveTeam(tx, teamId)
		if err != nil {
			return err
		}
		if !canManageAdtyTeamInvites(team, actorId) {
			return ErrAdtyTeamForbidden
		}
		var invite model.AdtyTeamInvite
		lookup := model.AdtyLockForUpdate(tx).Where("id = ? AND team_id = ?", inviteId, team.Id).Limit(1).Find(&invite)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected == 0 {
			return ErrAdtyTeamForbidden
		}
		if invite.UsedAt != 0 || invite.RevokedAt != 0 {
			return ErrAdtyTeamInvalid
		}
		now := time.Now().Unix()
		if err := tx.Model(&invite).Updates(map[string]any{"revoked_by": actorId, "revoked_at": now}).Error; err != nil {
			return err
		}
		invite.RevokedBy, invite.RevokedAt = actorId, now
		siteID = team.SiteId
		createdBy = invite.CreatedBy
		view = adtyTeamInviteView(invite, now)
		return nil
	})
	if err != nil {
		return nil, err
	}
	writeAdtyTeamInviteAudit(actorId, siteID, view.Id, "adty.team.invite.revoke", map[string]any{
		"invite_id": view.Id, "team_id": teamId, "status": model.AdtyTeamInviteStatusOutstanding,
	}, map[string]any{
		"invite_id": view.Id, "team_id": teamId, "created_by": createdBy,
		"revoked_by": actorId, "revoked_at": view.RevokedAt, "result": model.AuditResultSuccess,
		"status": model.AdtyTeamInviteStatusRevoked,
	}, model.AuditResultSuccess)
	return &view, nil
}

func adtyTeamInviteView(invite model.AdtyTeamInvite, now int64) AdtyTeamInviteView {
	return AdtyTeamInviteView{
		Id: invite.Id, TeamId: invite.TeamId, CreatedBy: invite.CreatedBy, CreatedAt: invite.CreatedAt,
		ExpiresAt: invite.ExpiresAt, UsedBy: invite.UsedBy, UsedAt: invite.UsedAt,
		RevokedBy: invite.RevokedBy, RevokedAt: invite.RevokedAt,
		Status: model.AdtyTeamInviteStatus(invite, now),
	}
}

func writeAdtyTeamInviteAudit(actorId, siteId int, inviteId int64, action string, before, after any, result string) {
	rec := model.OfficialAuditWrite{
		UserId: actorId, SiteID: model.OfficialAuditSiteIDPointer(siteId),
		ObjectType: "team_invite", ObjectID: strconv.FormatInt(inviteId, 10),
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
		common.SysLog(common.LogText("official team invite audit write failed: %s", err.Error()))
	}
}

func BindAdtyTeamKey(userId, tokenId int) error {
	return model.DB.Transaction(func(tx *gorm.DB) error {
		var token model.Token
		if err := model.AdtyLockForUpdate(tx).First(&token, tokenId).Error; err != nil {
			return err
		}
		if token.UserId != userId || token.Status != common.TokenStatusEnabled {
			return ErrAdtyTeamForbidden
		}
		var membership model.AdtyTeamMember
		lookup := tx.Where("user_id = ? AND status = ?", userId, model.AdtyTeamMemberActive).Limit(1).Find(&membership)
		if lookup.Error != nil {
			return lookup.Error
		}
		if lookup.RowsAffected == 0 {
			return ErrAdtyTeamForbidden
		}
		row := model.AdtyTeamKey{TokenId: tokenId, TeamId: membership.TeamId, UserId: userId, Status: model.AdtyTeamMemberActive}
		return tx.Create(&row).Error
	})
}

// InheritAdtyTeamsOnCloseTx 把关闭分站上仍活跃的团队迁入主站。
// team_id、负责人和成员关系保持不变；当前 site_id 改为主站 0，并在 source_site_id 仍为空时记下原分站。
// 已记录的 source_site_id 不会被重试改成 0 或另一个猜测值。已在主站且来源为空的历史团队不会被回填。
// 关站迁入不会关闭团队；原团队 API Key 仍按迁移安全规则停用。
func InheritAdtyTeamsOnCloseTx(tx *gorm.DB, siteId int) error {
	if !adtyTeamTableReadyFor(tx, &model.AdtyTeam{}) {
		return nil
	}
	if siteId == 0 {
		return nil
	}
	var teams []model.AdtyTeam
	if err := tx.Where("site_id = ? AND status = ?", siteId, model.AdtyTeamStatusActive).Find(&teams).Error; err != nil {
		return err
	}
	for _, team := range teams {
		if err := disableAdtyTeamKeysTx(tx, team.Id); err != nil {
			return err
		}
		updates := map[string]any{"site_id": 0}
		// 只在来源尚未记录、且迁移前 site_id 就是正在关闭的分站时写入；已在主站（0）的不猜测来源。
		if team.SourceSiteId == 0 && team.SiteId == siteId {
			updates["source_site_id"] = team.SiteId
		}
		if err := tx.Model(&model.AdtyTeam{}).Where("id = ?", team.Id).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

func reserveAdtyTeamQuotaHook(tx *gorm.DB, tokenId, userId, delta int, ref string) (bool, error) {
	if tokenId <= 0 || delta <= 0 {
		return false, nil
	}
	if !adtyTeamTableReadyFor(tx, &model.AdtyTeamKey{}) {
		return false, nil
	}
	var key model.AdtyTeamKey
	lookup := tx.Where("token_id = ? AND status = ?", tokenId, model.AdtyTeamMemberActive).Limit(1).Find(&key)
	if lookup.Error != nil {
		return false, lookup.Error
	}
	if lookup.RowsAffected == 0 {
		return false, nil
	}
	if key.UserId != userId {
		return false, ErrAdtyTeamForbidden
	}
	var team model.AdtyTeam
	if err := model.AdtyLockForUpdate(tx).First(&team, key.TeamId).Error; err != nil {
		return false, err
	}
	if team.Status != model.AdtyTeamStatusActive || team.SharedQuota < int64(delta) {
		return false, model.ErrBillingWalletInsufficient
	}
	if err := applyAdtyTeamQuotaTx(tx, &team, -int64(delta), "reserve", ref, userId); err != nil {
		return false, err
	}
	return true, nil
}

func closeAdtyTeamForDeletedLeader(tx *gorm.DB, userId int) error {
	if !adtyTeamTableReadyFor(tx, &model.AdtyTeam{}) {
		return nil
	}
	var team model.AdtyTeam
	lookup := tx.Where("leader_user_id = ? AND status = ?", userId, model.AdtyTeamStatusActive).Limit(1).Find(&team)
	if lookup.Error != nil {
		return lookup.Error
	}
	if lookup.RowsAffected == 0 {
		return nil
	}
	if err := tx.Model(&model.AdtyTeamMember{}).Where("team_id = ? AND status = ?", team.Id, model.AdtyTeamMemberActive).
		Update("status", model.AdtyTeamMemberLeft).Error; err != nil {
		return err
	}
	if err := disableAdtyTeamKeysTx(tx, team.Id); err != nil {
		return err
	}
	return tx.Model(&team).Updates(map[string]any{"status": model.AdtyTeamStatusClosed, "closed_at": time.Now().Unix()}).Error
}

func disableAdtyTeamMemberKeysTx(tx *gorm.DB, teamId int64, userId int) error {
	var keys []model.AdtyTeamKey
	if err := tx.Where("team_id = ? AND user_id = ? AND status = ?", teamId, userId, model.AdtyTeamMemberActive).Find(&keys).Error; err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	ids := make([]int, 0, len(keys))
	for _, key := range keys {
		ids = append(ids, key.TokenId)
	}
	if err := tx.Model(&model.Token{}).Where("id IN ?", ids).Update("status", common.TokenStatusDisabled).Error; err != nil {
		return err
	}
	return tx.Model(&model.AdtyTeamKey{}).Where("team_id = ? AND user_id = ?", teamId, userId).Update("status", model.AdtyTeamMemberLeft).Error
}

func disableAdtyTeamKeysTx(tx *gorm.DB, teamId int64) error {
	var keys []model.AdtyTeamKey
	if err := tx.Where("team_id = ? AND status = ?", teamId, model.AdtyTeamMemberActive).Find(&keys).Error; err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	ids := make([]int, 0, len(keys))
	for _, key := range keys {
		ids = append(ids, key.TokenId)
	}
	if err := tx.Model(&model.Token{}).Where("id IN ?", ids).Update("status", common.TokenStatusDisabled).Error; err != nil {
		return err
	}
	return tx.Model(&model.AdtyTeamKey{}).Where("team_id = ?", teamId).Update("status", model.AdtyTeamMemberLeft).Error
}

func joinAdtyTeamTx(tx *gorm.DB, userId int, teamId int64) (*model.AdtyTeamMember, error) {
	user, err := lockAdtyTeamUser(tx, userId)
	if err != nil {
		return nil, err
	}
	var team model.AdtyTeam
	if err := model.AdtyLockForUpdate(tx).First(&team, teamId).Error; err != nil {
		return nil, err
	}
	if team.Status != model.AdtyTeamStatusActive {
		return nil, ErrAdtyTeamClosed
	}
	if user.SiteId != team.SiteId || user.StaffSiteId != 0 {
		return nil, ErrAdtyTeamForbidden
	}
	if err := rejectActiveTeamMembership(tx, user.Id); err != nil {
		return nil, err
	}
	row := model.AdtyTeamMember{TeamId: team.Id, UserId: user.Id, Role: model.AdtyTeamRoleMember, Status: model.AdtyTeamMemberActive}
	if err := tx.Create(&row).Error; err != nil {
		return nil, mapAdtyTeamMemberWriteError(err)
	}
	return &row, nil
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
