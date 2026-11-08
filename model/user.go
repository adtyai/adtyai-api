package model

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relaykit/dto"

	"github.com/bytedance/gopkg/util/gopool"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const UserNameMaxLength = 20

var userSortColumns = map[string]string{
	"id":            "id",
	"username":      "username",
	"quota":         "quota",
	"group":         "group",
	"created_at":    "created_at",
	"last_login_at": "last_login_at",
}

type UserSortOptions struct {
	SortBy    string
	SortOrder string
}

func NewUserSortOptions(sortBy string, sortOrder string) UserSortOptions {
	normalizedSortBy := strings.ToLower(strings.TrimSpace(sortBy))
	normalizedSortOrder := strings.ToLower(strings.TrimSpace(sortOrder))
	if _, ok := userSortColumns[normalizedSortBy]; !ok {
		normalizedSortBy = "id"
		normalizedSortOrder = "desc"
	} else if normalizedSortOrder != "asc" {
		normalizedSortOrder = "desc"
	}

	return UserSortOptions{
		SortBy:    normalizedSortBy,
		SortOrder: normalizedSortOrder,
	}
}

func (options UserSortOptions) Apply(query *gorm.DB) *gorm.DB {
	columnName, ok := userSortColumns[options.SortBy]
	if !ok {
		columnName = "id"
	}
	column := clause.Column{Name: columnName}
	if columnName == "quota" {
		column = clause.Column{Name: "quota + quota_pending", Raw: true}
	}
	q := query.Order(clause.OrderByColumn{
		Column: column,
		Desc:   options.SortOrder != "asc",
	})
	if columnName != "id" {
		q = q.Order(clause.OrderByColumn{
			Column: clause.Column{Name: "id"},
			Desc:   true,
		})
	}
	return q
}

func resolveUserSortOptions(sortOptions []UserSortOptions) UserSortOptions {
	if len(sortOptions) == 0 {
		return NewUserSortOptions("", "")
	}
	return sortOptions[0]
}

// User if you add sensitive fields, don't forget to clean them in setupLogin function.
// Otherwise, the sensitive information will be saved on local storage in plain text!
type User struct {
	Id                   int     `json:"id"`
	Username             string  `json:"username" gorm:"unique;index" validate:"max=20,excludes=@"`
	Password             string  `json:"password" gorm:"not null;" validate:"min=8,max=128"`
	HasPassword          bool    `json:"-" gorm:"-:all"`
	OriginalPassword     string  `json:"original_password" gorm:"-:all"` // this field is only for Password change verification, don't save it to database!
	DisplayName          string  `json:"display_name" gorm:"index" validate:"max=20"`
	Role                 int     `json:"role" gorm:"type:int;default:1"`   // admin, common
	Status               int     `json:"status" gorm:"type:int;default:1"` // enabled, disabled
	Email                string  `json:"email" gorm:"index" validate:"max=50"`
	GitHubId             string  `json:"github_id" gorm:"column:github_id;index"`
	DiscordId            string  `json:"discord_id" gorm:"column:discord_id;index"`
	OidcId               string  `json:"oidc_id" gorm:"column:oidc_id;index"`
	WeChatId             string  `json:"wechat_id" gorm:"column:wechat_id;index"`
	TelegramId           string  `json:"telegram_id" gorm:"column:telegram_id;index"`
	VerificationCode     string  `json:"verification_code" gorm:"-:all"`                         // this field is only for Email verification, don't save it to database!
	AccessToken          *string `json:"-" gorm:"type:char(32);column:access_token;uniqueIndex"` // Deprecated: 旧版面板访问令牌，仅在升级后的过渡期内使用；删除 users.access_token 列时一并移除。
	AccessTokenCreatedAt *int64  `json:"-" gorm:"type:bigint;column:access_token_created_at"`    // Deprecated: 旧版面板访问令牌，仅在升级后的过渡期内使用；删除 users.access_token 列时一并移除。
	Quota                int     `json:"quota" gorm:"type:int;default:0"`
	QuotaPending         int     `json:"-" gorm:"type:bigint;not null;default:0;column:quota_pending"`
	UsedQuota            int     `json:"used_quota" gorm:"type:int;default:0;column:used_quota"` // used quota
	// CheckinQuota 是还在钱包里的签到赠送。花掉的钱先算本金，所以只有余额掉到它下面才减少。退款不能把它当成本金。
	CheckinQuota     int                        `json:"-" gorm:"type:int;default:0;column:checkin_quota"`
	RequestCount     int                        `json:"request_count" gorm:"type:int;default:0;"` // request number
	Group            string                     `json:"group" gorm:"type:varchar(64);default:'default'"`
	DeletedAt        gorm.DeletedAt             `gorm:"index"`
	LinuxDOId        string                     `json:"linux_do_id" gorm:"column:linux_do_id;index"`
	Setting          string                     `json:"setting" gorm:"type:text;column:setting"`
	Remark           string                     `json:"remark,omitempty" gorm:"type:varchar(255)" validate:"max=255"`
	StripeCustomer   string                     `json:"stripe_customer" gorm:"type:varchar(64);column:stripe_customer;index"`
	CreatedAt        int64                      `json:"created_at" gorm:"autoCreateTime;column:created_at"`
	LastLoginAt      int64                      `json:"last_login_at" gorm:"default:0;column:last_login_at"`
	AuthVersion      int64                      `json:"-" gorm:"type:bigint;not null;default:1;column:auth_version"`
	SiteId           int                        `json:"site_id" gorm:"index;default:0;column:site_id"` // 所属分站（任务 11.1），0 是主站；只由分站服务写
	AdminPermissions map[string]map[string]bool `json:"admin_permissions,omitempty" gorm:"-:all"`
}

func (user *User) ToBaseUser() *UserBase {
	quota, err := userWalletBalance(user.Quota, user.QuotaPending)
	if err != nil {
		common.SysError(common.LogText("invalid durable wallet balance: user=%d", user.Id))
		quota = 0
	}
	cache := &UserBase{
		Id:          user.Id,
		Group:       user.Group,
		Quota:       quota,
		Status:      user.Status,
		Role:        user.Role,
		Username:    user.Username,
		Setting:     user.Setting,
		Email:       user.Email,
		AuthVersion: user.AuthVersion,
		SiteId:      user.SiteId, // 二开：P11
		CacheSchema: userCacheSchemaVersion,
	}
	return cache
}

// Deprecated: 旧版面板访问令牌，仅在升级后的过渡期内使用；删除 users.access_token 列时一并移除。
func (user *User) GetAccessToken() string {
	if user.AccessToken == nil {
		return ""
	}
	return *user.AccessToken
}

// Deprecated: 旧版面板访问令牌，仅在升级后的过渡期内使用；删除 users.access_token 列时一并移除。
func (user *User) SetAccessToken(token string) {
	user.AccessToken = &token
}

// RevokeUserAccessToken returns the generation actually revoked under the row lock.
//
// Deprecated: 旧版面板访问令牌，仅在升级后的过渡期内使用；删除 users.access_token 列时一并移除。
func RevokeUserAccessToken(id int) (string, error) {
	var tokenRef string
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).Select("id", "access_token").First(&user, id).Error; err != nil {
			return err
		}
		tokenRef = AccessTokenFingerprint(user.GetAccessToken())
		if tokenRef == "" {
			return nil
		}
		return tx.Model(&User{}).Where("id = ?", id).Updates(map[string]any{"access_token": nil, "access_token_created_at": nil}).Error
	})
	return tokenRef, err
}

func (user *User) GetSetting() dto.UserSetting {
	setting := dto.UserSetting{}
	if user.Setting != "" {
		err := common.Unmarshal([]byte(user.Setting), &setting)
		if err != nil {
			common.SysLog(common.LogText("failed to unmarshal setting: %s", err.Error()))
		}
	}
	return setting
}

func (user *User) SetSetting(setting dto.UserSetting) {
	settingBytes, err := common.Marshal(setting)
	if err != nil {
		common.SysLog(common.LogText("failed to marshal setting: %s", err.Error()))
		return
	}
	user.Setting = string(settingBytes)
}

func UpdateUserSetting(userId int, setting dto.UserSetting) error {
	if userId == 0 {
		return common.NewMessage("ID is empty!")
	}
	settingBytes, err := common.Marshal(setting)
	if err != nil {
		return err
	}
	settingValue := string(settingBytes)
	if err = DB.Model(&User{}).Where("id = ?", userId).Update("setting", settingValue).Error; err != nil {
		return err
	}
	return updateUserSettingCache(userId, settingValue)
}

// userBindColumns 允许通过 UpdateUserBindColumn 更新的第三方账号绑定列白名单。
// 列名只可能来自代码内部的 provider 实现，白名单是防御纵深，不依赖调用方自律。
var userBindColumns = map[string]bool{
	"github_id":   true,
	"discord_id":  true,
	"oidc_id":     true,
	"linux_do_id": true,
	"wechat_id":   true,
}

// UpdateUserBindColumn 第三方账号绑定字段的专用更新。
// 绑定操作必须只写绑定列：若改为“读取完整用户 → 改一个字段 → 整体更新”，
// 读快照期间并发发生的封禁、降权或分组变更会被旧快照覆盖恢复。
// 角色、状态、分组只允许通过各自带锁/CAS 的专用方法修改。
func UpdateUserBindColumn(userId int, column string, value string) error {
	if userId <= 0 {
		return common.NewMessage("ID is empty!")
	}
	if !userBindColumns[column] {
		return fmt.Errorf("invalid user bind column: %s", column)
	}
	return DB.Model(&User{}).Where("id = ?", userId).Update(column, value).Error
}

// 根据用户角色生成默认的边栏配置
func generateDefaultSidebarConfigForRole(userRole int) string {
	defaultConfig := map[string]any{}

	// 聊天区域 - 所有用户都可以访问
	defaultConfig["chat"] = map[string]any{
		"enabled":    true,
		"playground": true,
		"chat":       true,
	}

	// 控制台区域 - 所有用户都可以访问
	defaultConfig["console"] = map[string]any{
		"enabled":    true,
		"detail":     true,
		"token":      true,
		"log":        true,
		"midjourney": true,
		"task":       true,
	}

	// 个人中心区域 - 所有用户都可以访问
	defaultConfig["personal"] = map[string]any{
		"enabled":  true,
		"topup":    true,
		"personal": true,
	}

	// 管理员区域 - 根据角色决定
	if userRole == common.RoleAdminUser {
		// 管理员可以访问管理员区域，但不能访问系统设置
		defaultConfig["admin"] = map[string]any{
			"enabled": true,
			"channel": true,
			"models":  true,
			"user":    true,
			"setting": false, // 管理员不能访问系统设置
		}
	} else if userRole == common.RoleRootUser {
		// 超级管理员可以访问所有功能
		defaultConfig["admin"] = map[string]any{
			"enabled": true,
			"channel": true,
			"models":  true,
			"user":    true,
			"setting": true,
		}
	}
	// 普通用户不包含admin区域

	// 转换为JSON字符串
	configBytes, err := common.Marshal(defaultConfig)
	if err != nil {
		common.SysLog(common.LogText("failed to generate the default sidebar config: %s", err.Error()))
		return ""
	}

	return string(configBytes)
}

// CheckUserExistOrDeleted check if user exist or deleted, if not exist, return false, nil, if deleted or exist, return true, nil
func CheckUserExistOrDeleted(username string, email string) (bool, error) {
	var user User

	// err := DB.Unscoped().First(&user, "username = ? or email = ?", username, email).Error
	// check email if empty
	var err error
	email = NormalizeEmail(email)
	if email == "" {
		err = DB.Unscoped().First(&user, "username = ?", username).Error
	} else {
		err = DB.Unscoped().First(&user, "username = ? or LOWER(email) = ?", username, email).Error
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// not exist, return false, nil
			return false, nil
		}
		// other error, return false, err
		return false, err
	}
	// exist, return true, nil
	return true, nil
}

func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func emailQuery(tx *gorm.DB, email string) *gorm.DB {
	if tx == nil {
		tx = DB
	}
	return tx.Unscoped().Model(&User{}).Where("LOWER(email) = ?", NormalizeEmail(email))
}

func CountUsersByEmail(email string) (int64, error) {
	email = NormalizeEmail(email)
	if email == "" {
		return 0, nil
	}
	var count int64
	err := emailQuery(DB, email).Count(&count).Error
	return count, err
}

func IsEmailAvailable(email string, excludeUserID int) (bool, error) {
	email = NormalizeEmail(email)
	if email == "" {
		return true, nil
	}
	query := emailQuery(DB, email)
	if excludeUserID > 0 {
		query = query.Where("id <> ?", excludeUserID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count == 0, nil
}

func EnsureEmailAvailable(email string, excludeUserID int) error {
	available, err := IsEmailAvailable(email, excludeUserID)
	if err != nil {
		return err
	}
	if !available {
		return ErrEmailAlreadyTaken
	}
	return nil
}

// withNormalizedEmailLock serializes concurrent writers that target the same
// normalized email inside tx, so a "check then write" sequence cannot be raced
// by two transactions. It must be called inside an active transaction; the lock
// is scoped to that transaction and released on commit/rollback.
//
//   - PostgreSQL: transaction-level advisory lock keyed by the normalized email.
//   - MySQL (default REPEATABLE READ): a locking read that takes a next-key/gap
//     lock on the email index, blocking concurrent inserts of the same value.
//   - SQLite: no explicit lock; the single-writer model already serializes the
//     write, so a racing second write fails instead of duplicating.
//
// An empty email is allowed to repeat and needs no serialization.
func withNormalizedEmailLock(tx *gorm.DB, email string, fn func(tx *gorm.DB) error) error {
	if err := lockNormalizedEmail(tx, email); err != nil {
		return err
	}
	return fn(tx)
}

func lockNormalizedEmail(tx *gorm.DB, email string) error {
	email = NormalizeEmail(email)
	if email == "" {
		return nil
	}
	switch {
	case common.UsingMainDatabase(common.DatabaseTypePostgreSQL):
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", email).Error; err != nil {
			return err
		}
	case common.UsingMainDatabase(common.DatabaseTypeMySQL):
		var ids []int
		if err := tx.Raw("SELECT id FROM users WHERE email = ? FOR UPDATE", email).Scan(&ids).Error; err != nil {
			return err
		}
	}
	return nil
}

func GetMaxUserId() int {
	var user User
	DB.Unscoped().Last(&user)
	return user.Id
}

func GetAllUsers(pageInfo *common.PageInfo, belowRole int, sortOptions ...UserSortOptions) (users []*User, total int64, err error) {
	// Start transaction
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Get total count within transaction
	err = limitVisibleRoles(tx.Unscoped().Model(&User{}), belowRole).Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Get paginated users within same transaction
	order := resolveUserSortOptions(sortOptions)
	err = order.Apply(limitVisibleRoles(tx.Unscoped(), belowRole)).Limit(pageInfo.GetPageSize()).Offset(pageInfo.GetStartIdx()).Omit("password", "access_token").Find(&users).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// Commit transaction
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func limitVisibleRoles(tx *gorm.DB, belowRole int) *gorm.DB {
	if belowRole > 0 {
		return tx.Where("role < ?", belowRole)
	}
	return tx
}

func SearchUsers(keyword string, group string, role *int, status *int, startIdx int, num int, belowRole int, sortOptions ...UserSortOptions) ([]*User, int64, error) {
	var users []*User
	var total int64
	var err error

	// 开始事务
	tx := DB.Begin()
	if tx.Error != nil {
		return nil, 0, tx.Error
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 构建基础查询
	query := limitVisibleRoles(tx.Unscoped().Model(&User{}), belowRole)

	// 构建搜索条件
	likeCondition := "username LIKE ? OR email LIKE ? OR display_name LIKE ?"
	likeArgs := []any{"%" + keyword + "%", "%" + keyword + "%", "%" + keyword + "%"}

	// 尝试将关键字转换为整数ID
	keywordInt, err := strconv.Atoi(keyword)
	if err == nil {
		// 如果是数字，同时搜索ID和其他字段
		likeCondition = "id = ? OR " + likeCondition
		likeArgs = append([]any{keywordInt}, likeArgs...)
	}

	query = query.Where("("+likeCondition+")", likeArgs...)
	if group != "" {
		query = query.Where(commonGroupCol+" = ?", group)
	}
	if role != nil {
		query = query.Where("role = ?", *role)
	}
	if status != nil {
		if *status == -1 {
			query = query.Where("deleted_at IS NOT NULL")
		} else {
			query = query.Where("deleted_at IS NULL").Where("status = ?", *status)
		}
	}

	// 获取总数
	err = query.Count(&total).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 获取分页数据
	order := resolveUserSortOptions(sortOptions)
	err = order.Apply(query.Omit("password", "access_token")).Limit(num).Offset(startIdx).Find(&users).Error
	if err != nil {
		tx.Rollback()
		return nil, 0, err
	}

	// 提交事务
	if err = tx.Commit().Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func GetUserById(id int, selectAll bool) (*User, error) {
	return getUserById(DB, id, selectAll)
}

// GetUserByIdIncludingDeleted finds a soft-deleted account so an administrator
// can hard-delete it and release the username, email, and provider subjects
// that the deleted row still holds.
func GetUserByIdIncludingDeleted(id int, selectAll bool) (*User, error) {
	return getUserById(DB.Unscoped(), id, selectAll)
}

func getUserById(query *gorm.DB, id int, selectAll bool) (*User, error) {
	if id == 0 {
		return nil, common.NewMessage("ID is empty!")
	}
	user := User{Id: id}
	var err error
	if selectAll {
		err = query.First(&user, "id = ?", id).Error
	} else {
		err = query.Omit("password", "access_token").First(&user, "id = ?", id).Error
	}
	return &user, err
}

// GetSelfUserById reads dashboard profile data and password existence in one
// query. The password hash and management access token are never selected.
func GetSelfUserById(id int) (*User, error) {
	if id == 0 {
		return nil, common.NewMessage("ID is empty!")
	}
	var profile struct {
		User
		HasPassword bool `gorm:"column:has_password"`
	}
	err := DB.Model(&User{}).Select([]string{
		"id", "username", "display_name", "role", "status", "email",
		"github_id", "discord_id", "oidc_id", "wechat_id", "telegram_id",
		"group", "quota", "quota_pending", "used_quota", "request_count", "linux_do_id", "setting",
		"stripe_customer", "auth_version",
		"CASE WHEN password <> '' THEN 1 ELSE 0 END AS has_password",
	}).First(&profile, "id = ?", id).Error
	profile.User.HasPassword = profile.HasPassword
	return &profile.User, err
}

// DeleteUserById soft-deletes a user and returns how many scoped access tokens
// were deleted with it.
func DeleteUserById(id int) (int64, error) {
	if id == 0 {
		return 0, common.NewMessage("ID is empty!")
	}
	user := User{Id: id}
	return user.Delete()
}

// HardDeleteUserById permanently deletes a user and returns how many scoped
// access tokens were deleted with it.
func HardDeleteUserById(id int) (int64, error) {
	if id == 0 {
		return 0, common.NewMessage("ID is empty!")
	}
	user := User{Id: id}
	return user.HardDelete()
}

func (user *User) prepareForInsert(tx *gorm.DB) error {
	user.Email = NormalizeEmail(user.Email)
	if err := ensureEmailAvailableWithTx(tx, user.Email, 0); err != nil {
		return err
	}
	if user.Password == "" {
		return nil
	}
	var err error
	user.Password, err = common.HashAccountPassword(user.Password)
	return err
}

// BindEmailToUser atomically checks email availability and assigns it to the
// user, serializing concurrent binds of the same email so two accounts cannot
// end up sharing one address. The email is normalized before check and store.
func BindEmailToUser(user *User, email string) error {
	email = NormalizeEmail(email)
	if err := DB.Transaction(func(tx *gorm.DB) error {
		return withNormalizedEmailLock(tx, email, func(tx *gorm.DB) error {
			if err := ensureEmailAvailableWithTx(tx, email, user.Id); err != nil {
				return err
			}
			user.Email = email
			return user.UpdateWithTx(tx, false)
		})
	}); err != nil {
		return err
	}
	return updateUserCache(*user)
}

func ensureEmailAvailableWithTx(tx *gorm.DB, email string, excludeUserID int) error {
	email = NormalizeEmail(email)
	if email == "" {
		return nil
	}
	query := emailQuery(tx, email)
	if excludeUserID > 0 {
		query = query.Where("id <> ?", excludeUserID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return ErrEmailAlreadyTaken
	}
	return nil
}

func (user *User) Insert(_ int) error {
	if err := DB.Transaction(func(tx *gorm.DB) error {
		return withNormalizedEmailLock(tx, user.Email, func(tx *gorm.DB) error {
			if err := user.prepareForInsert(tx); err != nil {
				return err
			}
			user.Quota = common.QuotaForNewUser

			// 初始化用户设置，包括默认的边栏配置
			if user.Setting == "" {
				defaultSetting := dto.UserSetting{}
				// 这里暂时不设置SidebarModules，因为需要在用户创建后根据角色设置
				user.SetSetting(defaultSetting)
			}

			return tx.Create(user).Error
		})
	}); err != nil {
		return err
	}

	user.finishInsert()
	return nil
}

func (user *User) finishInsert() {
	// 用户创建成功后，根据角色初始化边栏配置
	// 需要重新获取用户以确保有正确的ID和Role
	var createdUser User
	if err := DB.Where("username = ?", user.Username).First(&createdUser).Error; err == nil {
		// 生成基于角色的默认边栏配置
		defaultSidebarConfig := generateDefaultSidebarConfigForRole(createdUser.Role)
		if defaultSidebarConfig != "" {
			currentSetting := createdUser.GetSetting()
			currentSetting.SidebarModules = defaultSidebarConfig
			createdUser.SetSetting(currentSetting)
			createdUser.Update(false)
			common.SysLog(common.LogText("initialized the sidebar config for new user %s (role: %d)", createdUser.Username, createdUser.Role))
		}
	}

	if common.QuotaForNewUser > 0 {
		RecordLog(user.Id, LogTypeSystem, common.NewMessage("New user sign-up bonus {{quota}}", map[string]any{"quota": logger.FormatQuota(common.QuotaForNewUser)}))
	}
}

func (user *User) FinishInsert(_ int) {
	user.finishInsert()
}

// InsertWithTx inserts a new user within an existing transaction.
// This is used for OAuth registration where user creation and binding need to be atomic.
// Post-creation tasks (sidebar config, logs) are handled after the transaction commits.
func (user *User) InsertWithTx(tx *gorm.DB, _ int) error {
	return withNormalizedEmailLock(tx, user.Email, func(tx *gorm.DB) error {
		if err := user.prepareForInsert(tx); err != nil {
			return err
		}
		user.Quota = common.QuotaForNewUser

		// 初始化用户设置
		if user.Setting == "" {
			defaultSetting := dto.UserSetting{}
			user.SetSetting(defaultSetting)
		}

		return tx.Create(user).Error
	})
}

// FinalizeOAuthUserCreation performs post-transaction tasks for OAuth user creation.
// This should be called after the transaction commits successfully.
func (user *User) FinalizeOAuthUserCreation(_ int) {
	// 用户创建成功后，根据角色初始化边栏配置
	var createdUser User
	if err := DB.Where("id = ?", user.Id).First(&createdUser).Error; err == nil {
		defaultSidebarConfig := generateDefaultSidebarConfigForRole(createdUser.Role)
		if defaultSidebarConfig != "" {
			currentSetting := createdUser.GetSetting()
			currentSetting.SidebarModules = defaultSidebarConfig
			createdUser.SetSetting(currentSetting)
			createdUser.Update(false)
			common.SysLog(common.LogText("initialized the sidebar config for new user %s (role: %d)", createdUser.Username, createdUser.Role))
		}
	}

	if common.QuotaForNewUser > 0 {
		RecordLog(user.Id, LogTypeSystem, common.NewMessage("New user sign-up bonus {{quota}}", map[string]any{"quota": logger.FormatQuota(common.QuotaForNewUser)}))
	}
}

func (user *User) Update(updatePassword bool) error {
	var previousAuthVersion int64
	if err := DB.Model(&User{}).Where("id = ?", user.Id).Select("auth_version").Find(&previousAuthVersion).Error; err != nil {
		return err
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		return user.UpdateWithTx(tx, updatePassword)
	}); err != nil {
		return err
	}
	if err := updateUserCache(*user); err != nil {
		return err
	}
	if user.AuthVersion > previousAuthVersion {
		_, err := RevokeAllUserSessions(user.Id, "user_security_changed")
		return err
	}
	return nil
}

func (user *User) UpdateWithTx(tx *gorm.DB, updatePassword bool) error {
	var err error
	if updatePassword {
		user.Password, err = common.HashAccountPassword(user.Password)
		if err != nil {
			return err
		}
	}
	newUser := *user
	current := User{}
	if err = tx.First(&current, user.Id).Error; err != nil {
		return err
	}
	// Updates(struct) ignores zero values. Match that behavior when deciding
	// whether this request actually changes authentication-sensitive state;
	// partial self-profile updates intentionally leave role/status/group empty.
	authChanged := (updatePassword && current.Password != newUser.Password) ||
		(newUser.Role != 0 && current.Role != newUser.Role) ||
		(newUser.Status != 0 && current.Status != newUser.Status) ||
		(newUser.Group != "" && current.Group != newUser.Group)
	if authChanged {
		newUser.AuthVersion, err = IncrementUserAuthVersionWithTx(tx, user.Id)
		if err != nil {
			return err
		}
	}
	if err = tx.Model(&current).Omit(
		"access_token",
		"quota",
		"quota_pending",
		"checkin_quota",
		"used_quota",
		"request_count",
		"auth_version",
		"site_id",
	).Updates(newUser).Error; err != nil {
		return err
	}
	return tx.First(user, user.Id).Error
}

// ApplyManagedUserAccessTx persists only a role or status change for an
// administrator action. The user value already carries the requested role and
// status. Password, email, group, setting, and every other column stay as they
// are in the database, so a profile write that landed after the caller read
// the row is not restored from that old snapshot. changed is false when the
// locked row already has the requested role and status.
func ApplyManagedUserAccessTx(tx *gorm.DB, user *User) (bool, error) {
	if tx == nil || user == nil || user.Id <= 0 {
		return false, fmt.Errorf("invalid managed user update")
	}
	var current User
	if err := lockForUpdate(tx).Select("id", "role", "status").Where("id = ?", user.Id).First(&current).Error; err != nil {
		return false, err
	}
	updates := map[string]any{}
	if user.Role != 0 && user.Role != current.Role {
		updates["role"] = user.Role
	}
	if user.Status != 0 && user.Status != current.Status {
		updates["status"] = user.Status
	}
	if len(updates) == 0 {
		return false, nil
	}
	next, err := IncrementUserAuthVersionWithTx(tx, user.Id)
	if err != nil {
		return false, err
	}
	if err := tx.Model(&User{}).Where("id = ?", user.Id).Updates(updates).Error; err != nil {
		return false, err
	}
	user.AuthVersion = next
	return true, nil
}

func (user *User) Edit(updatePassword bool) error {
	var previousAuthVersion int64
	if err := DB.Model(&User{}).Where("id = ?", user.Id).Select("auth_version").Find(&previousAuthVersion).Error; err != nil {
		return err
	}
	if err := DB.Transaction(func(tx *gorm.DB) error {
		return user.EditWithTx(tx, updatePassword)
	}); err != nil {
		return err
	}
	if err := updateUserCache(*user); err != nil {
		return err
	}
	if user.AuthVersion > previousAuthVersion {
		_, err := RevokeAllUserSessions(user.Id, "user_security_changed")
		return err
	}
	return nil
}

func (user *User) EditWithTx(tx *gorm.DB, updatePassword bool) error {
	var err error
	if updatePassword {
		user.Password, err = common.HashAccountPassword(user.Password)
		if err != nil {
			return err
		}
	}

	newUser := *user
	updates := map[string]any{
		"username":     newUser.Username,
		"display_name": newUser.DisplayName,
		"group":        newUser.Group,
		"remark":       newUser.Remark,
	}
	if updatePassword {
		updates["password"] = newUser.Password
	}

	current := User{}
	if err = tx.First(&current, user.Id).Error; err != nil {
		return err
	}
	if newUser.Group != current.Group {
		if err = lockAssignableAdtyGroupsTx(tx, []string{newUser.Group}); err != nil {
			return err
		}
	}
	authChanged := (updatePassword && current.Password != newUser.Password) || current.Group != newUser.Group
	if authChanged {
		newUser.AuthVersion, err = IncrementUserAuthVersionWithTx(tx, user.Id)
		if err != nil {
			return err
		}
	}
	if err = tx.Model(&current).Updates(updates).Error; err != nil {
		return err
	}
	return tx.First(user, user.Id).Error
}

func (user *User) ClearBinding(bindingType string) error {
	if user.Id == 0 {
		return errors.New("user id is empty")
	}

	bindingColumnMap := map[string]string{
		"email":    "email",
		"github":   "github_id",
		"discord":  "discord_id",
		"oidc":     "oidc_id",
		"wechat":   "wechat_id",
		"telegram": "telegram_id",
		"linuxdo":  "linux_do_id",
	}

	column, ok := bindingColumnMap[bindingType]
	if !ok {
		return errors.New("invalid binding type")
	}

	if err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&User{}).Where("id = ?", user.Id).Update(column, "").Error; err != nil {
			return err
		}
		if bindingType == ExternalIdentityProviderTelegram {
			return ReleaseExternalIdentityWithTx(tx, ExternalIdentityProviderTelegram, user.Id)
		}
		return nil
	}); err != nil {
		return err
	}

	if err := DB.Where("id = ?", user.Id).First(user).Error; err != nil {
		return err
	}

	return updateUserCache(*user)
}

func (user *User) Delete() (int64, error) {
	return user.delete(nil)
}

func DeleteUserForSession(identity AuthSessionIdentity) (int64, error) {
	user := User{Id: identity.UserID}
	return user.delete(&identity)
}

// deletedUserUsername frees the original username. The tombstone stays unique
// among soft-deleted rows, which the username unique index still covers.
func deletedUserUsername(tx *gorm.DB, userId int) (string, error) {
	base := fmt.Sprintf("del%d", userId)
	if len(base) > UserNameMaxLength {
		base = base[:UserNameMaxLength]
	}
	for n := 0; n < 100; n++ {
		candidate := base
		if n > 0 {
			suffix := fmt.Sprintf("x%d", n)
			if len(base)+len(suffix) > UserNameMaxLength {
				candidate = base[:UserNameMaxLength-len(suffix)] + suffix
			} else {
				candidate += suffix
			}
		}
		var count int64
		if err := tx.Unscoped().Model(&User{}).Where("username = ? AND id <> ?", candidate, userId).Count(&count).Error; err != nil {
			return "", err
		}
		if count == 0 {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("no free username tombstone for user %d", userId)
}

// releaseSoftDeletedIdentity drops the login names and provider subjects before
// a soft delete. Those columns are still unique, or looked up with Unscoped,
// so leaving them would block the same person from registering again.
func releaseSoftDeletedIdentity(tx *gorm.DB, userId int) error {
	if err := releaseAllExternalIdentitiesWithTx(tx, userId); err != nil {
		return err
	}
	username, err := deletedUserUsername(tx, userId)
	if err != nil {
		return err
	}
	return tx.Model(&User{}).Where("id = ?", userId).Updates(map[string]any{
		"username":    username,
		"email":       "",
		"github_id":   "",
		"discord_id":  "",
		"oidc_id":     "",
		"wechat_id":   "",
		"telegram_id": "",
		"linux_do_id": "",
	}).Error
}

// ErrSiteOwnerCannotDelete means the account still owns a site that serves
// customers. Deleting it would keep settling earnings into an account that
// can no longer sign in.
var ErrSiteOwnerCannotDelete = errors.New("close the site before deleting this account")

func userOwnsOpenSite(tx *gorm.DB, userId int) (bool, error) {
	if tx == nil || userId <= 0 || !tx.Migrator().HasTable(&AdtySite{}) {
		return false, nil
	}
	var count int64
	err := tx.Model(&AdtySite{}).Where("owner_user_id = ? AND status IN ?", userId, []string{
		AdtySiteStatusActive, AdtySiteStatusPaused, AdtySiteStatusClosing,
	}).Count(&count).Error
	return count > 0, err
}

func (user *User) delete(identity *AuthSessionIdentity) (int64, error) {
	if user.Id == 0 {
		return 0, common.NewMessage("ID is empty!")
	}
	var nextAuthVersion int64
	var revokedAccessTokens int64
	if err := DB.Transaction(func(tx *gorm.DB) error {
		if identity != nil {
			if err := ValidateAuthSessionWithTx(tx, *identity); err != nil {
				return err
			}
			var role int
			if err := tx.Model(&User{}).Where("id = ?", user.Id).Select("role").Scan(&role).Error; err != nil {
				return err
			}
			if role == common.RoleRootUser {
				return ErrCannotDeleteRootUser
			}
		}
		ownsSite, err := userOwnsOpenSite(tx, user.Id)
		if err != nil {
			return err
		}
		if ownsSite {
			return ErrSiteOwnerCannotDelete
		}
		nextAuthVersion, err = IncrementUserAuthVersionWithTx(tx, user.Id)
		if err != nil {
			return err
		}
		revokedAccessTokens, err = DeleteUserAccessTokensWithTx(tx, user.Id)
		if err != nil {
			return err
		}
		if err := releaseSoftDeletedIdentity(tx, user.Id); err != nil {
			return err
		}
		return tx.Delete(user).Error
	}); err != nil {
		return 0, err
	}
	if err := publishCommittedUserAuthVersion(user.Id, nextAuthVersion); err != nil {
		return revokedAccessTokens, err
	}
	if _, err := RevokeAllUserSessions(user.Id, "user_deleted"); err != nil {
		return revokedAccessTokens, err
	}
	return revokedAccessTokens, invalidateUserCache(user.Id)
}

func (user *User) HardDelete() (int64, error) {
	if user.Id == 0 {
		return 0, common.NewMessage("ID is empty!")
	}
	var tokens []Token
	var deletedAuthVersion int64
	var revokedAccessTokens int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		ownsSite, err := userOwnsOpenSite(tx, user.Id)
		if err != nil {
			return err
		}
		if ownsSite {
			return ErrSiteOwnerCannotDelete
		}
		deletedAuthVersion, err = IncrementUserAuthVersionWithTx(tx, user.Id)
		if err != nil {
			return err
		}
		revokedAccessTokens, err = DeleteUserAccessTokensWithTx(tx, user.Id)
		if err != nil {
			return err
		}
		if common.RedisEnabled {
			if err := tx.Unscoped().Select("id", commonKeyCol).Where("user_id = ?", user.Id).Find(&tokens).Error; err != nil {
				return err
			}
		}
		if err := deleteUserAuthenticationData(tx, user.Id); err != nil {
			return err
		}
		return tx.Unscoped().Delete(user).Error
	})
	if err != nil {
		return 0, err
	}
	if err := publishCommittedUserAuthVersion(user.Id, deletedAuthVersion); err != nil {
		common.SysError(common.LogText("failed to publish auth tombstone after hard deleting user %d: %v", user.Id, err))
	}
	if err := invalidateTokensCache(tokens); err != nil {
		common.SysError(common.LogText("failed to invalidate token cache after hard deleting user %d: %v", user.Id, err))
	}
	if err := invalidateUserCache(user.Id); err != nil {
		common.SysError(common.LogText("failed to invalidate user cache after hard deleting user %d: %v", user.Id, err))
	}
	return revokedAccessTokens, nil
}

func deleteUserAuthenticationData(tx *gorm.DB, userId int) error {
	if err := releaseAllExternalIdentitiesWithTx(tx, userId); err != nil {
		return err
	}
	for _, authenticationData := range []any{
		&TwoFABackupCode{},
		&TwoFA{},
		&UserSession{},
		&AuthFlow{},
		&PasskeyCredential{},
		&Token{},
	} {
		if err := tx.Unscoped().Where("user_id = ?", userId).Delete(authenticationData).Error; err != nil {
			return err
		}
	}
	return deleteUserOAuthBindingsByUserId(tx, userId)
}

// ValidateAndFill check password & user status
func (user *User) ValidateAndFill() (err error) {
	// When querying with struct, GORM will only query with non-zero fields,
	// that means if your field's value is 0, '', false or other zero values,
	// it won't be used to build query conditions
	password := user.Password
	username := strings.TrimSpace(user.Username)
	if username == "" || password == "" {
		return ErrUserEmptyCredentials
	}
	// Usernames stay case-sensitive. Email is stored lowercase, and PostgreSQL
	// equality is case-sensitive, so an address typed with different case is
	// looked up through the same normalized match as registration.
	err = DB.Where("username = ?", username).First(user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		matched, emailErr := GetUniqueUserByEmail(username)
		switch {
		case emailErr == nil:
			*user = *matched
		case errors.Is(emailErr, ErrEmailNotFound), errors.Is(emailErr, ErrEmailAmbiguous):
			return ErrInvalidCredentials
		default:
			return fmt.Errorf("%w: %v", ErrDatabase, emailErr)
		}
	} else if err != nil {
		return fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	if user.Password == "" {
		return ErrInvalidCredentials
	}
	okay := common.ValidatePasswordAndHash(password, user.Password)
	if !okay || user.Status != common.UserStatusEnabled {
		return ErrInvalidCredentials
	}
	return nil
}

func (user *User) FillUserById() error {
	if user.Id == 0 {
		return common.NewMessage("ID is empty!")
	}
	DB.Where(User{Id: user.Id}).First(user)
	return nil
}

func (user *User) FillUserByEmail() error {
	if user.Email == "" {
		return common.NewMessage("Email is empty!")
	}
	DB.Where(User{Email: user.Email}).First(user)
	return nil
}

func (user *User) FillUserByGitHubId() error {
	if user.GitHubId == "" {
		return common.NewMessage("GitHub ID is empty!")
	}
	DB.Where(User{GitHubId: user.GitHubId}).First(user)
	return nil
}

func (user *User) FillUserByDiscordId() error {
	if user.DiscordId == "" {
		return common.NewMessage("Discord ID is empty!")
	}
	DB.Where(User{DiscordId: user.DiscordId}).First(user)
	return nil
}

func (user *User) FillUserByOidcId() error {
	if user.OidcId == "" {
		return common.NewMessage("OIDC ID is empty!")
	}
	DB.Where(User{OidcId: user.OidcId}).First(user)
	return nil
}

func (user *User) FillUserByWeChatId() error {
	if user.WeChatId == "" {
		return common.NewMessage("WeChat ID is empty!")
	}
	DB.Where(User{WeChatId: user.WeChatId}).First(user)
	return nil
}

func (user *User) FillUserByTelegramId() error {
	if user.TelegramId == "" {
		return common.NewMessage("Telegram ID is empty!")
	}
	err := DB.Where(User{TelegramId: user.TelegramId}).First(user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return common.NewMessage("This Telegram account is not linked. Sign in using another method and link it first.")
	}
	return nil
}

func IsEmailAlreadyTaken(email string) bool {
	count, err := CountUsersByEmail(email)
	return err == nil && count > 0
}

func GetUniqueUserByEmail(email string) (*User, error) {
	email = NormalizeEmail(email)
	if email == "" {
		return nil, ErrEmailNotFound
	}
	var users []User
	if err := DB.Where("LOWER(email) = ?", email).Limit(2).Find(&users).Error; err != nil {
		return nil, err
	}
	switch len(users) {
	case 0:
		return nil, ErrEmailNotFound
	case 1:
		return &users[0], nil
	default:
		return nil, ErrEmailAmbiguous
	}
}

// AssignBuiltinProviderIDsWithTx writes the built-in provider ids for a user
// that was just inserted. A partial unique index makes a repeated subject fail
// inside this statement instead of landing on two users.
func AssignBuiltinProviderIDsWithTx(tx *gorm.DB, user *User) error {
	if tx == nil || user == nil || user.Id <= 0 {
		return ErrAccountBindingChanged
	}
	err := tx.Model(&User{}).Where("id = ?", user.Id).Updates(map[string]any{
		"github_id":   user.GitHubId,
		"discord_id":  user.DiscordId,
		"oidc_id":     user.OidcId,
		"linux_do_id": user.LinuxDOId,
		"wechat_id":   user.WeChatId,
		"telegram_id": user.TelegramId,
	}).Error
	if isAdtyUniqueViolation(err) {
		return ErrExternalIdentityAlreadyClaimed
	}
	return err
}

// EnsureBuiltinOAuthSubjectIndexes adds partial unique indexes for the built-in
// OAuth columns. Empty values stay unrestricted. Existing duplicates stop the
// migration instead of being kept.
func EnsureBuiltinOAuthSubjectIndexes(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("builtin oauth subject indexes: database is nil")
	}
	if !db.Migrator().HasTable(&User{}) {
		return nil
	}
	for _, column := range []string{"github_id", "discord_id", "oidc_id", "linux_do_id", "wechat_id"} {
		var duplicates int64
		query := fmt.Sprintf(
			"SELECT COUNT(*) FROM (SELECT %[1]s FROM users WHERE %[1]s <> '' GROUP BY %[1]s HAVING COUNT(*) > 1) AS duplicated",
			column,
		)
		if err := db.Raw(query).Scan(&duplicates).Error; err != nil {
			return fmt.Errorf("count duplicate %s bindings: %w", column, err)
		}
		if duplicates > 0 {
			return fmt.Errorf("duplicate %s bindings must be resolved before the unique index", column)
		}
		statement := fmt.Sprintf(
			"CREATE UNIQUE INDEX IF NOT EXISTS ux_users_%[1]s_subject ON users (%[1]s) WHERE %[1]s <> ''",
			column,
		)
		if err := db.Exec(statement).Error; err != nil {
			return fmt.Errorf("create builtin oauth subject index %s: %w", column, err)
		}
	}
	return nil
}

func IsWeChatIdAlreadyTaken(wechatId string) bool {
	return DB.Unscoped().Where("wechat_id = ?", wechatId).Find(&User{}).RowsAffected == 1
}

func IsGitHubIdAlreadyTaken(githubId string) bool {
	return DB.Unscoped().Where("github_id = ?", githubId).Find(&User{}).RowsAffected == 1
}

func IsDiscordIdAlreadyTaken(discordId string) bool {
	return DB.Unscoped().Where("discord_id = ?", discordId).Find(&User{}).RowsAffected == 1
}

func IsOidcIdAlreadyTaken(oidcId string) bool {
	return DB.Where("oidc_id = ?", oidcId).Find(&User{}).RowsAffected == 1
}

func IsTelegramIdAlreadyTaken(telegramId string) bool {
	return DB.Unscoped().Where("telegram_id = ?", telegramId).Find(&User{}).RowsAffected == 1
}

type PasswordResetCommitState uint8

const (
	PasswordResetNotCommitted PasswordResetCommitState = iota
	PasswordResetCommitted
	PasswordResetCommitUnknown
)

// PasswordResetOutcome contains only trusted context for audit/notification.
// A failed COMMIT acknowledgement must not be treated as a confirmed rollback.
type PasswordResetOutcome struct {
	State    PasswordResetCommitState
	UserID   int
	Role     int
	Email    string
	Language string
}

func ResetUserPasswordByEmail(email string, password string) error {
	_, err := ResetUserPasswordByEmailWithOutcome(email, password)
	return err
}

func ResetUserPasswordByEmailWithOutcome(email string, password string) (PasswordResetOutcome, error) {
	outcome := PasswordResetOutcome{}
	if email == "" || password == "" {
		return outcome, common.NewMessage("Email or password is empty!")
	}
	user, err := GetUniqueUserByEmail(email)
	if err != nil {
		return outcome, err
	}
	outcome.UserID, outcome.Role = user.Id, user.Role
	outcome.Email, outcome.Language = user.Email, user.GetSetting().Language
	hashedPassword, err := common.HashAccountPassword(password)
	if err != nil {
		return outcome, err
	}
	readyToCommit := false
	err = DB.Transaction(func(tx *gorm.DB) error {
		var current User
		if err := lockForUpdate(tx).First(&current, user.Id).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrEmailNotFound
			}
			return err
		}
		if NormalizeEmail(current.Email) != NormalizeEmail(email) {
			return ErrEmailNotFound
		}
		outcome.UserID, outcome.Role = current.Id, current.Role
		outcome.Email, outcome.Language = current.Email, current.GetSetting().Language
		if _, err := IncrementUserAuthVersionWithTx(tx, user.Id); err != nil {
			return err
		}
		if _, err := DeleteUserAccessTokensWithTx(tx, user.Id); err != nil {
			return err
		}
		updated := tx.Model(&User{}).Where("id = ?", user.Id).Update("password", hashedPassword)
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return ErrEmailNotFound
		}
		readyToCommit = true
		return nil
	})
	if err != nil {
		if readyToCommit {
			outcome.State = PasswordResetCommitUnknown
		}
		return outcome, err
	}
	outcome.State = PasswordResetCommitted
	if err := PublishUserAuthCache(user.Id); err != nil {
		return outcome, err
	}
	_, err = RevokeAllUserSessions(user.Id, "password_reset")
	return outcome, err
}

func IsAdmin(userId int) bool {
	if userId == 0 {
		return false
	}
	var user User
	err := DB.Where("id = ?", userId).Select("role").Find(&user).Error
	if err != nil {
		common.SysLog(common.LogText("no such user %s", err.Error()))
		return false
	}
	return user.Role >= common.RoleAdminUser
}

// ValidateAccessToken resolves a legacy plaintext access token. After the
// transition deadline it rejects every value without querying the database.
//
// Deprecated: 旧版面板访问令牌，仅在升级后的过渡期内使用；删除 users.access_token 列时一并移除。
func ValidateAccessToken(token string) (*User, error) {
	if token == "" {
		return nil, nil
	}
	if LegacyAccessTokensRetired(common.GetTimestamp()) {
		return nil, ErrLegacyAccessTokenRetired
	}
	token = strings.Replace(token, "Bearer ", "", 1)
	user := &User{}
	err := DB.Where("access_token = ?", token).First(user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: %v", ErrDatabase, err)
	}
	return user, nil
}

// GetUserQuota returns the raw row for fromDB callers and the durable logical
// balance otherwise. Redis metadata is never an authority for wallet amounts.
func GetUserQuota(id int, fromDB bool) (quota int, err error) {
	if !fromDB {
		var user User
		if err := DB.Select("quota", "quota_pending").First(&user, id).Error; err != nil {
			return 0, err
		}
		return userWalletBalance(user.Quota, user.QuotaPending)
	}
	err = DB.Model(&User{}).Where("id = ?", id).Select("quota").Find(&quota).Error
	if err != nil {
		return 0, err
	}

	return quota, nil
}

func GetUserUsedQuota(id int) (quota int, err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Select("used_quota").Find(&quota).Error
	return quota, err
}

func GetUserEmail(id int) (email string, err error) {
	err = DB.Model(&User{}).Where("id = ?", id).Select("email").Find(&email).Error
	return email, err
}

// GetUserGroup gets group from Redis first, falls back to DB if needed
func GetUserGroup(id int, fromDB bool) (group string, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := RefreshUserGroupCache(id); err != nil {
					common.SysLog(common.LogText("failed to update user group cache: %s", err.Error()))
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		group, err := getUserGroupCache(id)
		if err == nil {
			return group, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select(commonGroupCol).Find(&group).Error
	if err != nil {
		return "", err
	}

	return group, nil
}

// GetUserSetting gets setting from Redis first, falls back to DB if needed
func GetUserSetting(id int, fromDB bool) (settingMap dto.UserSetting, err error) {
	var setting string
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserSettingCache(id, setting); err != nil {
					common.SysLog(common.LogText("failed to update user setting cache: %s", err.Error()))
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		setting, err := getUserSettingCache(id)
		if err == nil {
			return setting, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	// can be nil setting
	var safeSetting sql.NullString
	err = DB.Model(&User{}).Where("id = ?", id).Select("setting").Find(&safeSetting).Error
	if err != nil {
		return settingMap, err
	}
	if safeSetting.Valid {
		setting = safeSetting.String
	} else {
		setting = ""
	}
	userBase := &UserBase{
		Setting: setting,
	}
	return userBase.GetSetting(), nil
}

func IncreaseUserQuota(id int, quota int, db bool) (err error) {
	if quota < 0 {
		return errors.New("quota cannot be negative")
	}
	if err := common.ValidateWalletQuota(quota); err != nil {
		return err
	}
	if !db && common.BatchUpdateEnabled {
		return persistUserQuotaDelta(id, quota)
	}
	if err := increaseUserQuota(id, quota); err != nil {
		return err
	}
	gopool.Go(func() {
		if err := invalidateUserCache(id); err != nil {
			common.SysLog(common.LogText("failed to increase user quota: %s", err.Error()))
		}
	})
	return nil
}

func increaseUserQuota(id int, quota int) (err error) {
	return DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).First(&user, id).Error; err != nil {
			return err
		}
		return applyUserQuotaDeltaTx(tx, &user, quota, false)
	})
}

func DecreaseUserQuota(id int, quota int, db bool) (err error) {
	if quota < 0 {
		return errors.New("quota cannot be negative")
	}
	if !db && common.BatchUpdateEnabled {
		return persistUserQuotaDelta(id, -quota)
	}
	if err := decreaseUserQuota(id, quota); err != nil {
		return err
	}
	if err := invalidateUserCache(id); err != nil {
		common.SysLog(common.LogText("failed to decrease user quota: %s", err.Error()))
	}
	return nil
}

func decreaseUserQuota(id int, quota int) (err error) {
	return DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx).First(&user, id).Error; err != nil {
			return err
		}
		return applyUserQuotaDeltaTx(tx, &user, -quota, false)
	})
}

// clampCheckinQuotaDB 把签到赠送收到不超过当前余额。余额先扣本金，扣进赠送后签到额度跟着降。
func clampCheckinQuotaDB(db *gorm.DB, userID int) error {
	if db == nil || userID <= 0 {
		return nil
	}
	return db.Model(&User{}).Where("id = ?", userID).Update("checkin_quota", gorm.Expr(`CASE
		WHEN checkin_quota < 0 OR quota + quota_pending < 0 THEN 0
		WHEN checkin_quota > quota + quota_pending THEN quota + quota_pending
		ELSE checkin_quota END`)).Error
}

func DeltaUpdateUserQuota(id int, delta int) (err error) {
	if delta == 0 {
		return nil
	}
	if delta > 0 {
		return IncreaseUserQuota(id, delta, false)
	} else {
		return DecreaseUserQuota(id, -delta, false)
	}
}

//func GetRootUserEmail() (email string) {
//	DB.Model(&User{}).Where("role = ?", common.RoleRootUser).Select("email").Find(&email)
//	return email
//}

func UpdateUserLastLoginAt(id int) {
	if err := DB.Model(&User{}).Where("id = ?", id).Update("last_login_at", common.GetTimestamp()).Error; err != nil {
		common.SysLog(common.LogText("failed to update user last_login_at: %s", err.Error()))
	}
}

func UpdateUserUsedQuotaAndRequestCount(id int, quota int) {
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUsedQuota, id, quota)
		addNewRecord(BatchUpdateTypeRequestCount, id, 1)
		return
	}
	updateUserUsedQuotaAndRequestCount(id, quota, 1)
}

// UpdateUserUsedQuota adjusts accumulated usage without changing request count.
func UpdateUserUsedQuota(id int, quota int) {
	if common.BatchUpdateEnabled {
		addNewRecord(BatchUpdateTypeUsedQuota, id, quota)
		return
	}
	if err := DB.Model(&User{}).Where("id = ?", id).Update("used_quota", gorm.Expr("used_quota + ?", quota)).Error; err != nil {
		common.SysLog(common.LogText("failed to update user used quota: %s", err.Error()))
	}
}

func updateUserUsedQuotaAndRequestCount(id int, quota int, count int) {
	err := DB.Model(&User{}).Where("id = ?", id).Updates(
		map[string]any{
			"used_quota":    gorm.Expr("used_quota + ?", quota),
			"request_count": gorm.Expr("request_count + ?", count),
		},
	).Error
	if err != nil {
		common.SysLog(common.LogText("failed to update user used quota and request count: %s", err.Error()))
		return
	}

	//// 更新缓存
	//if err := invalidateUserCache(id); err != nil {
	//	common.SysError("failed to invalidate user cache: " + err.Error())
	//}
}

func updateUserQuotaUsedQuotaAndRequestCount(id int, usedQuota int, requestCount int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := lockForUpdate(tx.Unscoped()).First(&user, id).Error; err != nil {
			return err
		}
		quota, err := userWalletBalance(user.Quota, user.QuotaPending)
		if err != nil {
			return err
		}
		if user.QuotaPending == 0 && usedQuota == 0 && requestCount == 0 {
			return nil
		}
		return tx.Unscoped().Model(&User{}).Where("id = ?", id).Updates(map[string]any{
			"quota": quota, "quota_pending": 0,
			"checkin_quota": max(0, min(user.CheckinQuota, quota)),
			"used_quota":    gorm.Expr("used_quota + ?", usedQuota),
			"request_count": gorm.Expr("request_count + ?", requestCount),
		}).Error
	})
}

// GetUsernameById gets username from Redis first, falls back to DB if needed
func GetUsernameById(id int, fromDB bool) (username string, err error) {
	defer func() {
		// Update Redis cache asynchronously on successful DB read
		if shouldUpdateRedis(fromDB, err) {
			gopool.Go(func() {
				if err := updateUserNameCache(id, username); err != nil {
					common.SysLog(common.LogText("failed to update user name cache: %s", err.Error()))
				}
			})
		}
	}()
	if !fromDB && common.RedisEnabled {
		username, err := getUserNameCache(id)
		if err == nil {
			return username, nil
		}
		// Don't return error - fall through to DB
	}
	fromDB = true
	err = DB.Model(&User{}).Where("id = ?", id).Select("username").Find(&username).Error
	if err != nil {
		return "", err
	}

	return username, nil
}

func IsLinuxDOIdAlreadyTaken(linuxDOId string) bool {
	var user User
	err := DB.Unscoped().Where("linux_do_id = ?", linuxDOId).First(&user).Error
	return !errors.Is(err, gorm.ErrRecordNotFound)
}

func (user *User) FillUserByLinuxDOId() error {
	if user.LinuxDOId == "" {
		return errors.New("linux do id is empty")
	}
	err := DB.Where("linux_do_id = ?", user.LinuxDOId).First(user).Error
	return err
}

func RootUserExists() bool {
	var user User
	err := DB.Where("role = ?", common.RoleRootUser).First(&user).Error
	if err != nil {
		return false
	}
	return true
}

// EnsureOneRootUserIndex 让未删除的根账号最多一个。普通唯一索引拦不住多条空 deleted_at。
func EnsureOneRootUserIndex(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&User{}) {
		return nil
	}
	var roots int64
	if err := db.Model(&User{}).Where("role = ?", common.RoleRootUser).Count(&roots).Error; err != nil {
		return err
	}
	if roots > 1 {
		return nil
	}
	// SQLite 不允许部分索引的 WHERE 里出现参数，角色值是常量，直接写进语句。
	return db.Exec(fmt.Sprintf(
		"CREATE UNIQUE INDEX IF NOT EXISTS ux_users_one_live_root ON users (role) WHERE role = %d AND deleted_at IS NULL",
		common.RoleRootUser,
	)).Error
}

// CreateRootUserIfAbsent 在初始化锁里创建根账号。并发初始化只会留下一个。
func CreateRootUserIfAbsent(user *User) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
			if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext('adty-setup-root'))").Error; err != nil {
				return err
			}
		}
		var count int64
		if err := tx.Model(&User{}).Where("role = ?", common.RoleRootUser).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return nil
		}
		if err := tx.Create(user).Error; err != nil && !isAdtyUniqueViolation(err) {
			return err
		}
		return nil
	})
}
