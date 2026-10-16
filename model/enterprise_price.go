package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

var (
	ErrEnterprisePriceNotFound       = errors.New("enterprise price rule not found")
	ErrEnterprisePriceConfirmRequired = errors.New("confirm=true required")
	ErrEnterprisePriceBadDiscount    = errors.New("discount must be between 0.1 and 1")
	ErrEnterprisePriceUserRequired   = errors.New("user_id required")
	ErrEnterprisePriceConflict       = errors.New("enterprise price rule conflict")
	ErrEnterprisePriceUserMissing    = errors.New("target user not found")
)

// EnterprisePriceRule A8 企业协议价。
// group_code / model_name 为 NULL 表示通配（全部）；唯一键 (user_id, COALESCE(group,''), COALESCE(model,''))。
type EnterprisePriceRule struct {
	Id        int64      `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId    int        `json:"user_id" gorm:"not null;index"`
	GroupCode *string    `json:"group_code" gorm:"type:varchar(64)"`
	ModelName *string    `json:"model_name" gorm:"type:varchar(128)"`
	Discount  float64    `json:"discount" gorm:"type:numeric(4,3);not null"`
	ExpiresAt *time.Time `json:"expires_at"`
	CreatedBy int        `json:"created_by" gorm:"not null;default:0"`
	CreatedAt time.Time  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt time.Time  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (EnterprisePriceRule) TableName() string { return "enterprise_price_rules" }

func normalizeEnterpriseScope(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}

func enterpriseScopeKey(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// EnsureEnterprisePriceSchema 幂等：表 + COALESCE 唯一索引 + discount CHECK。
func EnsureEnterprisePriceSchema(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	if err := db.AutoMigrate(&EnterprisePriceRule{}); err != nil {
		return fmt.Errorf("enterprise_price_rules migrate: %w", err)
	}
	if !common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		return nil
	}
	stmts := []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_enterprise_price_rules_user_scope
		 ON enterprise_price_rules (user_id, COALESCE(group_code, ''), COALESCE(model_name, ''))`,
		`DO $$ BEGIN
		  IF NOT EXISTS (
		    SELECT 1 FROM pg_constraint WHERE conname = 'chk_enterprise_price_discount'
		  ) THEN
		    ALTER TABLE enterprise_price_rules
		      ADD CONSTRAINT chk_enterprise_price_discount
		      CHECK (discount >= 0.1 AND discount <= 1);
		  END IF;
		END $$`,
	}
	for _, sql := range stmts {
		if err := db.Exec(sql).Error; err != nil {
			return fmt.Errorf("enterprise_price schema: %w", err)
		}
	}
	return nil
}

// enterprisePriceMatchFrozen was a release freeze for A8 runtime matching.
// false = MatchEnterprisePriceRule applies protocol discounts again
// (relay/helper/price.go attachEnterprisePrice, service/quota.go).
const enterprisePriceMatchFrozen = false

// MatchEnterprisePriceRule 按优先级取一条未过期规则：分组+模型 > 仅模型 > 仅分组 > 全部。
// 无匹配返回 discount=1、ruleId=0。
func MatchEnterprisePriceRule(userId int, groupCode, modelName string) (ruleId int64, discount float64) {
	discount = 1.0
	if enterprisePriceMatchFrozen {
		return 0, 1.0
	}
	if userId <= 0 || DB == nil {
		return 0, 1.0
	}
	groupCode = strings.TrimSpace(groupCode)
	modelName = strings.TrimSpace(modelName)
	now := time.Now()
	var rules []EnterprisePriceRule
	if err := DB.Where("user_id = ? AND (expires_at IS NULL OR expires_at > ?)", userId, now).
		Find(&rules).Error; err != nil || len(rules) == 0 {
		return 0, 1.0
	}
	bestPri := 99
	var best *EnterprisePriceRule
	for i := range rules {
		r := &rules[i]
		gc := enterpriseScopeKey(r.GroupCode)
		mn := enterpriseScopeKey(r.ModelName)
		pri := 0
		switch {
		case gc != "" && mn != "":
			if gc == groupCode && mn == modelName {
				pri = 1
			} else {
				continue
			}
		case gc == "" && mn != "":
			if mn == modelName {
				pri = 2
			} else {
				continue
			}
		case gc != "" && mn == "":
			if gc == groupCode {
				pri = 3
			} else {
				continue
			}
		default:
			pri = 4
		}
		if pri < bestPri {
			bestPri = pri
			best = r
		}
	}
	if best == nil {
		return 0, 1.0
	}
	if best.Discount < 0.1 || best.Discount > 1 {
		return 0, 1.0
	}
	return best.Id, best.Discount
}

// EffectiveGroupRatio 分组倍率之后再乘协议价折扣（无规则时 discount=1）。
func EffectiveGroupRatio(groupRatio, enterpriseDiscount float64) float64 {
	if enterpriseDiscount <= 0 {
		return groupRatio
	}
	return groupRatio * enterpriseDiscount
}

type EnterprisePriceCreateInput struct {
	UserId    int
	GroupCode *string
	ModelName *string
	Discount  float64
	ExpiresAt *time.Time
	CreatedBy int
	Confirm   bool
}

type EnterprisePriceUpdateInput struct {
	GroupCode *string
	ModelName *string
	Discount  *float64
	ExpiresAt *time.Time
	ClearExpires bool // 显式清空过期时间
	Confirm   bool
}

func validateDiscount(d float64) error {
	if d < 0.1 || d > 1 {
		return ErrEnterprisePriceBadDiscount
	}
	return nil
}

func CreateEnterprisePriceRule(in EnterprisePriceCreateInput) (*EnterprisePriceRule, error) {
	if in.UserId <= 0 {
		return nil, ErrEnterprisePriceUserRequired
	}
	if err := validateDiscount(in.Discount); err != nil {
		return nil, err
	}
	var exists int64
	if err := DB.Model(&User{}).Where("id = ?", in.UserId).Count(&exists).Error; err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, ErrEnterprisePriceUserMissing
	}
	row := &EnterprisePriceRule{
		UserId:    in.UserId,
		GroupCode: normalizeEnterpriseScope(in.GroupCode),
		ModelName: normalizeEnterpriseScope(in.ModelName),
		Discount:  in.Discount,
		ExpiresAt: in.ExpiresAt,
		CreatedBy: in.CreatedBy,
	}
	if err := DB.Create(row).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, ErrEnterprisePriceConflict
		}
		return nil, err
	}
	return row, nil
}

func UpdateEnterprisePriceRule(id int64, in EnterprisePriceUpdateInput) (*EnterprisePriceRule, error) {
	if id <= 0 {
		return nil, ErrEnterprisePriceNotFound
	}
	var row EnterprisePriceRule
	if err := DB.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEnterprisePriceNotFound
		}
		return nil, err
	}
	updates := map[string]any{}
	if in.GroupCode != nil {
		updates["group_code"] = normalizeEnterpriseScope(in.GroupCode)
	}
	if in.ModelName != nil {
		updates["model_name"] = normalizeEnterpriseScope(in.ModelName)
	}
	if in.Discount != nil {
		if err := validateDiscount(*in.Discount); err != nil {
			return nil, err
		}
		updates["discount"] = *in.Discount
	}
	if in.ClearExpires {
		updates["expires_at"] = nil
	} else if in.ExpiresAt != nil {
		updates["expires_at"] = in.ExpiresAt
	}
	if len(updates) == 0 {
		return &row, nil
	}
	if err := DB.Model(&row).Updates(updates).Error; err != nil {
		if isUniqueViolation(err) {
			return nil, ErrEnterprisePriceConflict
		}
		return nil, err
	}
	if err := DB.First(&row, id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func DeleteEnterprisePriceRule(id int64, confirm bool) error {
	_ = confirm // 已废弃，保留参数兼容调用方
	if id <= 0 {
		return ErrEnterprisePriceNotFound
	}
	res := DB.Delete(&EnterprisePriceRule{}, id)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrEnterprisePriceNotFound
	}
	return nil
}

func GetEnterprisePriceRule(id int64) (*EnterprisePriceRule, error) {
	var row EnterprisePriceRule
	if err := DB.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEnterprisePriceNotFound
		}
		return nil, err
	}
	return &row, nil
}

func ListEnterprisePriceRules(userId int, offset, limit int) ([]EnterprisePriceRule, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	if offset < 0 {
		offset = 0
	}
	q := DB.Model(&EnterprisePriceRule{})
	if userId > 0 {
		q = q.Where("user_id = ?", userId)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []EnterprisePriceRule
	if err := q.Order("id DESC").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

