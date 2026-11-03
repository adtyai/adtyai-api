package model

import (
	"errors"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
)

// TwoFA 用户2FA设置表
type TwoFA struct {
	Id             int            `json:"id" gorm:"primaryKey"`
	UserId         int            `json:"user_id" gorm:"unique;not null;index"`
	Secret         string         `json:"-" gorm:"type:varchar(255);not null"` // TOTP密钥，不返回给前端
	IsEnabled      bool           `json:"is_enabled"`
	FailedAttempts int            `json:"failed_attempts" gorm:"default:0"`
	LockedUntil    *time.Time     `json:"locked_until,omitempty"`
	LastUsedAt     *time.Time     `json:"last_used_at,omitempty"`
	LastTOTPStep   int64          `json:"-" gorm:"type:bigint;not null;default:0"` // 已接受的 TOTP 时间步，同一枚码不能再开会话
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `json:"-" gorm:"index"`
}

// TwoFABackupCode 备用码使用记录表
type TwoFABackupCode struct {
	Id        int            `json:"id" gorm:"primaryKey"`
	UserId    int            `json:"user_id" gorm:"not null;index"`
	CodeHash  string         `json:"-" gorm:"type:varchar(255);not null"` // 备用码哈希
	IsUsed    bool           `json:"is_used"`
	UsedAt    *time.Time     `json:"used_at,omitempty"`
	CreatedAt time.Time      `json:"created_at"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

// GetTwoFAByUserId 根据用户ID获取2FA设置
func GetTwoFAByUserId(userId int) (*TwoFA, error) {
	if userId == 0 {
		return nil, errors.New("user ID is empty")
	}

	var twoFA TwoFA
	err := DB.Where("user_id = ?", userId).First(&twoFA).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // 返回nil表示未设置2FA
		}
		return nil, err
	}

	return &twoFA, nil
}

// IsTwoFAEnabled 检查用户是否启用了2FA
func IsTwoFAEnabled(userId int) (bool, error) {
	twoFA, err := GetTwoFAByUserId(userId)
	if err != nil {
		return false, err
	}
	return twoFA != nil && twoFA.IsEnabled, nil
}

func (t *TwoFA) updateUsageState() error {
	if t.Id == 0 {
		return errors.New("2FA record ID is empty")
	}
	return DB.Model(&TwoFA{}).Where("id = ?", t.Id).Updates(map[string]any{
		"failed_attempts": t.FailedAttempts,
		"locked_until":    t.LockedUntil,
		"last_used_at":    t.LastUsedAt,
	}).Error
}

// ResetFailedAttempts 重置失败尝试次数
func (t *TwoFA) ResetFailedAttempts() error {
	t.FailedAttempts = 0
	t.LockedUntil = nil
	return t.updateUsageState()
}

// IncrementFailedAttempts 增加失败尝试次数
func (t *TwoFA) IncrementFailedAttempts() error {
	if t.Id == 0 {
		return errors.New("2FA record ID is empty")
	}

	const maxUpdateRetries = 5
	for range maxUpdateRetries {
		var current TwoFA
		if err := DB.Select("id", "failed_attempts", "locked_until").First(&current, t.Id).Error; err != nil {
			return err
		}

		now := time.Now()
		if current.LockedUntil != nil && now.Before(*current.LockedUntil) {
			t.FailedAttempts = current.FailedAttempts
			t.LockedUntil = current.LockedUntil
			return nil
		}

		nextFailedAttempts := current.FailedAttempts + 1
		nextLockedUntil := current.LockedUntil
		if nextFailedAttempts >= common.MaxFailAttempts {
			lockUntil := now.Add(time.Duration(common.LockoutDuration) * time.Second)
			nextLockedUntil = &lockUntil
		}

		result := DB.Model(&TwoFA{}).
			Where("id = ? AND failed_attempts = ? AND (locked_until IS NULL OR locked_until <= ?)", current.Id, current.FailedAttempts, now).
			Updates(map[string]any{
				"failed_attempts": nextFailedAttempts,
				"locked_until":    nextLockedUntil,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}

		t.FailedAttempts = nextFailedAttempts
		t.LockedUntil = nextLockedUntil
		return nil
	}

	return errors.New("conflicting updates to the 2FA failure count, please retry")
}

// IsLocked 检查账户是否被锁定
func (t *TwoFA) IsLocked() bool {
	if t.LockedUntil == nil {
		return false
	}
	return time.Now().Before(*t.LockedUntil)
}

func replaceBackupCodesWithTx(tx *gorm.DB, userId int, codes []string) error {
	if err := tx.Where("user_id = ?", userId).Delete(&TwoFABackupCode{}).Error; err != nil {
		return err
	}
	for _, code := range codes {
		hashedCode, err := common.HashBackupCode(code)
		if err != nil {
			return err
		}
		if err := tx.Create(&TwoFABackupCode{UserId: userId, CodeHash: hashedCode, IsUsed: false}).Error; err != nil {
			return err
		}
	}
	return nil
}

// ReplaceBackupCodesWithAuthVersion atomically replaces the factor's recovery
// credentials and advances the user's authentication version.
func ReplaceBackupCodesWithAuthVersion(userId int, codes []string) error {
	return replaceBackupCodesWithAuthVersion(userId, codes, nil)
}

func ReplaceBackupCodesForSession(identity AuthSessionIdentity, codes []string) error {
	return replaceBackupCodesWithAuthVersion(identity.UserID, codes, &identity)
}

func replaceBackupCodesWithAuthVersion(userId int, codes []string, identity *AuthSessionIdentity) error {
	if err := DB.Transaction(func(tx *gorm.DB) error {
		if identity != nil {
			if err := ValidateAuthSessionWithTx(tx, *identity); err != nil {
				return err
			}
		} else if err := lockForUpdate(tx).Select("id").First(&User{}, userId).Error; err != nil {
			return err
		}
		var enabled TwoFA
		if err := lockForUpdate(tx).Where("user_id = ? AND is_enabled = ?", userId, true).First(&enabled).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTwoFANotEnabled
			}
			return err
		}
		if _, err := IncrementUserAuthVersionWithTx(tx, userId); err != nil {
			return err
		}
		return replaceBackupCodesWithTx(tx, userId, codes)
	}); err != nil {
		return err
	}
	// 新码已经落库。缓存发布失败不能再把这次操作说成失败，否则明文不会交给用户。
	if err := PublishUserAuthCache(userId); err != nil {
		common.SysError(common.LogText("failed to publish auth cache after backup codes replaced user=%d: %s", userId, err.Error()))
	}
	return nil
}

// ValidateBackupCode 验证并使用备用码。登录链路不要走这里：核销必须和会话提交在同一个事务里。
func ValidateBackupCode(userId int, code string) (bool, error) {
	if !common.ValidateBackupCode(code) {
		return false, errors.New("the verification code or backup code is incorrect")
	}
	var id int
	err := DB.Transaction(func(tx *gorm.DB) error {
		consumed, err := ConsumeUnusedBackupCodeWithTx(tx, userId, code)
		id = consumed
		return err
	})
	if err != nil {
		return false, err
	}
	return id > 0, nil
}

// MatchUnusedBackupCode 只判断备用码是否还能用，不核销。
func MatchUnusedBackupCode(userId int, code string) (bool, error) {
	if userId <= 0 || !common.ValidateBackupCode(code) {
		return false, nil
	}
	id, err := findUnusedBackupCodeID(DB, userId, code)
	return id > 0, err
}

// ConsumeUnusedBackupCodeWithTx 在调用方事务里核销一枚未使用的备用码。
// 返回核销行的 id；没有匹配或已经被别的事务核销时返回 0。
func ConsumeUnusedBackupCodeWithTx(tx *gorm.DB, userId int, code string) (int, error) {
	if tx == nil || userId <= 0 || !common.ValidateBackupCode(code) {
		return 0, nil
	}
	normalizedCode := common.NormalizeBackupCode(code)
	var backupCodes []TwoFABackupCode
	if err := lockForUpdate(tx).Where("user_id = ? AND is_used = ?", userId, false).Find(&backupCodes).Error; err != nil {
		return 0, err
	}
	for _, bc := range backupCodes {
		if !common.ValidatePasswordAndHash(normalizedCode, bc.CodeHash) {
			continue
		}
		now := time.Now()
		result := tx.Model(&TwoFABackupCode{}).
			Where("id = ? AND is_used = ?", bc.Id, false).
			Updates(map[string]any{
				"is_used": true,
				"used_at": now,
			})
		if result.Error != nil {
			return 0, result.Error
		}
		if result.RowsAffected != 1 {
			return 0, nil
		}
		return bc.Id, nil
	}
	return 0, nil
}

// ReleaseConsumedBackupCodeWithTx 把已核销的备用码改回未使用。只在对应会话没有留下时调用。
func ReleaseConsumedBackupCodeWithTx(tx *gorm.DB, id int) error {
	if tx == nil || id <= 0 {
		return nil
	}
	return tx.Exec(
		"UPDATE two_fa_backup_codes SET is_used = ?, used_at = NULL WHERE id = ? AND is_used = ?",
		false, id, true,
	).Error
}

// ResetTwoFAUsageWithTx 在核销备用码的同一事务里清掉失败次数。
func ResetTwoFAUsageWithTx(tx *gorm.DB, userId int) error {
	if tx == nil || userId <= 0 {
		return ErrTwoFANotEnabled
	}
	now := time.Now()
	result := tx.Model(&TwoFA{}).Where("user_id = ? AND is_enabled = ?", userId, true).Updates(map[string]any{
		"failed_attempts": 0,
		"locked_until":    nil,
		"last_used_at":    now,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTwoFANotEnabled
	}
	return nil
}

func findUnusedBackupCodeID(tx *gorm.DB, userId int, code string) (int, error) {
	normalizedCode := common.NormalizeBackupCode(code)
	var backupCodes []TwoFABackupCode
	if err := tx.Where("user_id = ? AND is_used = ?", userId, false).Find(&backupCodes).Error; err != nil {
		return 0, err
	}
	for _, bc := range backupCodes {
		if common.ValidatePasswordAndHash(normalizedCode, bc.CodeHash) {
			return bc.Id, nil
		}
	}
	return 0, nil
}

// GetUnusedBackupCodeCount 获取未使用的备用码数量
func GetUnusedBackupCodeCount(userId int) (int, error) {
	var count int64
	err := DB.Model(&TwoFABackupCode{}).Where("user_id = ? AND is_used = false", userId).Count(&count).Error
	return int(count), err
}

// DisableTwoFAWithAuthVersion atomically removes the factor and invalidates
// every access token issued against the previous security configuration.
func DisableTwoFAWithAuthVersion(userId int) error {
	return disableTwoFAWithAuthVersion(userId, nil)
}

func DisableTwoFAForSession(identity AuthSessionIdentity) error {
	return disableTwoFAWithAuthVersion(identity.UserID, &identity)
}

func disableTwoFAWithAuthVersion(userId int, identity *AuthSessionIdentity) error {
	if err := DB.Transaction(func(tx *gorm.DB) error {
		if identity != nil {
			if err := ValidateAuthSessionWithTx(tx, *identity); err != nil {
				return err
			}
		} else if err := lockForUpdate(tx).Select("id").First(&User{}, userId).Error; err != nil {
			return err
		}
		var twoFA TwoFA
		if err := lockForUpdate(tx).Where("user_id = ? AND is_enabled = ?", userId, true).First(&twoFA).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTwoFANotEnabled
			}
			return err
		}
		if _, err := IncrementUserAuthVersionWithTx(tx, userId); err != nil {
			return err
		}
		if err := tx.Unscoped().Where("user_id = ?", userId).Delete(&TwoFABackupCode{}).Error; err != nil {
			return err
		}
		return tx.Unscoped().Delete(&twoFA).Error
	}); err != nil {
		return err
	}
	return PublishUserAuthCache(userId)
}

// ErrTOTPReplay 表示这枚验证码对应的时间步已经用过。
var ErrTOTPReplay = errors.New("this verification code was already used")

// ClaimTOTPStepWithTx 在调用方事务里记下已接受的时间步。时间步不比库里的新就返回 ErrTOTPReplay。
func ClaimTOTPStepWithTx(tx *gorm.DB, userId int, step int64) error {
	if tx == nil || userId <= 0 || step <= 0 {
		return ErrTOTPReplay
	}
	now := time.Now()
	result := tx.Model(&TwoFA{}).
		Where("user_id = ? AND is_enabled = ? AND COALESCE(last_totp_step, 0) < ?", userId, true, step).
		Updates(map[string]any{
			"last_totp_step":  step,
			"failed_attempts": 0,
			"locked_until":    nil,
			"last_used_at":    now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrTOTPReplay
	}
	return nil
}

// ReleaseTOTPStepWithTx 在会话没有留下时，把这次记下的时间步清掉。
func ReleaseTOTPStepWithTx(tx *gorm.DB, userId int, step int64) error {
	if tx == nil || userId <= 0 || step <= 0 {
		return nil
	}
	return tx.Exec(
		"UPDATE two_fas SET last_totp_step = 0 WHERE user_id = ? AND last_totp_step = ?",
		userId, step,
	).Error
}

// ValidateTOTPAndUpdateUsage 验证TOTP并更新使用记录。同一时间步只能成功一次。
func (t *TwoFA) ValidateTOTPAndUpdateUsage(code string) (bool, error) {
	// 检查是否被锁定
	if t.IsLocked() {
		return false, fmt.Errorf("the account is locked, retry after %v", t.LockedUntil.Format("2006-01-02 15:04:05"))
	}

	step, ok := common.MatchTOTPStep(t.Secret, code, time.Now())
	if !ok {
		if err := t.IncrementFailedAttempts(); err != nil {
			return false, err
		}
		return false, nil
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		return ClaimTOTPStepWithTx(tx, t.UserId, step)
	})
	if errors.Is(err, ErrTOTPReplay) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	now := time.Now()
	t.FailedAttempts = 0
	t.LockedUntil = nil
	t.LastUsedAt = &now
	t.LastTOTPStep = step
	return true, nil
}

// ValidateBackupCodeAndUpdateUsage 验证备用码并更新使用记录
func (t *TwoFA) ValidateBackupCodeAndUpdateUsage(code string) (bool, error) {
	// 检查是否被锁定
	if t.IsLocked() {
		return false, fmt.Errorf("the account is locked, retry after %v", t.LockedUntil.Format("2006-01-02 15:04:05"))
	}

	// 验证备用码
	valid, err := ValidateBackupCode(t.UserId, code)
	if err != nil {
		return false, err
	}

	if !valid {
		// 增加失败次数
		if err := t.IncrementFailedAttempts(); err != nil {
			return false, err
		}
		return false, nil
	}

	// 验证成功，重置失败次数并更新最后使用时间
	now := time.Now()
	t.FailedAttempts = 0
	t.LockedUntil = nil
	t.LastUsedAt = &now

	if err := t.updateUsageState(); err != nil {
		return false, err
	}

	return true, nil
}

// GetTwoFAStats 获取2FA统计信息（管理员使用）
func GetTwoFAStats() (map[string]any, error) {
	var totalUsers, enabledUsers int64

	// 总用户数
	if err := DB.Model(&User{}).Count(&totalUsers).Error; err != nil {
		return nil, err
	}

	// 启用2FA的用户数
	if err := DB.Model(&TwoFA{}).Where("is_enabled = true").Count(&enabledUsers).Error; err != nil {
		return nil, err
	}

	enabledRate := float64(0)
	if totalUsers > 0 {
		enabledRate = float64(enabledUsers) / float64(totalUsers) * 100
	}

	return map[string]any{
		"total_users":   totalUsers,
		"enabled_users": enabledUsers,
		"enabled_rate":  fmt.Sprintf("%.1f%%", enabledRate),
	}, nil
}
