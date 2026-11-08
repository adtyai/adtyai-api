package model

import (
	"crypto/rand"
	"errors"
	"math/big"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

const (
	adtyGroupIDMaxLen    = 32
	adtyGroupIDAlphabet  = "abcdefghijklmnopqrstuvwxyz0123456789"
	adtyGroupIDSuffixLen = 4
	// AdtyDefaultGroupID 是 users、channels 的列默认值，不能删除。
	AdtyDefaultGroupID = "default"
)

// adtyGroupConfigMu 让删除分组时的整份回写和单键保存排队。
// 只在内存快照上改完再写回，会盖掉这中间已经保存的倍率。
var adtyGroupConfigMu sync.Mutex

func adtyGroupConfigOption(key string) bool {
	switch key {
	case "GroupRatio", "UserUsableGroups", "TopupGroupRatio", "GroupGroupRatio", "AutoGroups",
		"group_ratio_setting.group_special_usable_group", "ModelRequestRateLimitGroup":
		return true
	default:
		return false
	}
}

func lockAdtyGroupConfig(keys ...string) func() {
	for _, key := range keys {
		if adtyGroupConfigOption(key) {
			adtyGroupConfigMu.Lock()
			return adtyGroupConfigMu.Unlock
		}
	}
	return func() {}
}

// AdtyContentLangs 是分组显示名和说明要写全的语言。
var AdtyContentLangs = []string{"zh-CN", "zh-TW", "en", "fr", "ja", "ko", "ru", "vi"}

var (
	ErrAdtyGroupNotFound     = errors.New("adty group not found")
	ErrAdtyGroupTextRequired = errors.New("adty group text required")
	errAdtyGroupIDExhausted  = errors.New("adty group id exhausted")
	adtyGroupCreateAttempts  = 5
	adtySkippedSeedGroupIDs  = map[string]struct{}{"auto": {}, "edit_this": {}}
	// adtyReservedGroupIDs 永远不会被生成：auto 是自动分组的关键字，default 是列默认值。
	adtyReservedGroupIDs = map[string]struct{}{"auto": {}, AdtyDefaultGroupID: {}}
	// adtyGroupIDTaken 由设置里出现过的分组键组成；生成 ID 时一起避开。测试可以替换。
	adtyGroupIDTaken = adtyGroupSeedSources
)

// AdtyLangText 按语言存文本。Value 返回字符串，PostgreSQL 简单协议才能写入 json 列。
type AdtyLangText map[string]string

func (text AdtyLangText) Value() (driverValue, error) {
	return adtyLangTextValue(text)
}

func (text *AdtyLangText) Scan(value any) error {
	return adtyLangTextScan(text, value)
}

// AdtyGroup 是分组表。ID 创建后不改；软删除的行仍占用 ID，历史日志靠它显示名字。
type AdtyGroup struct {
	ID        string         `json:"id" gorm:"column:id;type:varchar(32);primaryKey"`
	Names     AdtyLangText   `json:"names" gorm:"column:names;type:json;not null"`
	Descs     AdtyLangText   `json:"descs" gorm:"column:descs;type:json;not null"`
	CreatedAt int64          `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt int64          `json:"updated_at" gorm:"autoUpdateTime"`
	DeletedAt gorm.DeletedAt `json:"-" gorm:"index"`
}

func (AdtyGroup) TableName() string {
	return "adty_groups"
}

// Deleted 表示这一行已软删除。
func (group AdtyGroup) Deleted() bool {
	return group.DeletedAt.Valid
}

// AdtyGroupRefs 是分组被引用的数量。一个密钥即使分组和自动分组都命中，也只算一次。
type AdtyGroupRefs struct {
	Users    int64 `json:"users"`
	Tokens   int64 `json:"tokens"`
	Channels int64 `json:"channels"`
}

// Total 是三类引用之和。
func (refs AdtyGroupRefs) Total() int64 {
	return refs.Users + refs.Tokens + refs.Channels
}

// NewAdtyGroupID 生成 "g" + 4 位 [a-z0-9]，避开保留字、表里已有的 ID（含软删除）和设置里出现过的分组键。
func NewAdtyGroupID(tx *gorm.DB) (string, error) {
	var ids []string
	if err := tx.Unscoped().Model(&AdtyGroup{}).Pluck("id", &ids).Error; err != nil {
		return "", err
	}
	used := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		used[id] = struct{}{}
	}
	taken, err := adtyGroupIDTaken()
	if err != nil {
		return "", err
	}
	for id := range taken {
		used[id] = struct{}{}
	}
	for range 100 {
		id, err := randomAdtyGroupID()
		if err != nil {
			return "", err
		}
		if _, reserved := adtyReservedGroupIDs[id]; reserved {
			continue
		}
		if _, exists := used[id]; exists {
			continue
		}
		return id, nil
	}
	return "", errAdtyGroupIDExhausted
}

func randomAdtyGroupID() (string, error) {
	var b strings.Builder
	b.WriteByte('g')
	limit := big.NewInt(int64(len(adtyGroupIDAlphabet)))
	for range adtyGroupIDSuffixLen {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		b.WriteByte(adtyGroupIDAlphabet[n.Int64()])
	}
	return b.String(), nil
}

// CreateAdtyGroup 新建分组。名字必填，说明可空；两者都写进全部语言，之后可按语言改。
func CreateAdtyGroup(name, desc string) (*AdtyGroup, error) {
	name = strings.TrimSpace(name)
	desc = strings.TrimSpace(desc)
	if name == "" {
		return nil, ErrAdtyGroupTextRequired
	}
	var created *AdtyGroup
	var lastErr error
	for range adtyGroupCreateAttempts {
		err := DB.Transaction(func(tx *gorm.DB) error {
			id, err := NewAdtyGroupID(tx)
			if err != nil {
				return err
			}
			row := &AdtyGroup{
				ID:    id,
				Names: adtyFillLangs(name),
				Descs: adtyFillLangs(desc),
			}
			if err := tx.Create(row).Error; err != nil {
				return err
			}
			created = row
			return nil
		})
		if err == nil {
			return created, nil
		}
		if !isAdtyUniqueViolation(err) {
			return nil, err
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errAdtyGroupIDExhausted
	}
	return nil, lastErr
}

// UpdateAdtyGroupTexts 只改显示名和说明，ID 不变。英文名必填，其余语言可空（显示时回退英文）。
func UpdateAdtyGroupTexts(id string, names, descs AdtyLangText) (*AdtyGroup, error) {
	names, descs, err := normalizeAdtyGroupTexts(names, descs)
	if err != nil {
		return nil, err
	}
	group, err := GetAdtyGroup(id)
	if err != nil {
		return nil, err
	}
	group.Names = names
	group.Descs = descs
	if err := DB.Save(group).Error; err != nil {
		return nil, err
	}
	return group, nil
}

// ListAdtyGroups 按 ID 排序返回分组；includeDeleted 为真时带上软删除的行。
func ListAdtyGroups(includeDeleted bool) ([]AdtyGroup, error) {
	var groups []AdtyGroup
	query := DB
	if includeDeleted {
		query = query.Unscoped()
	}
	err := query.Order("id").Find(&groups).Error
	return groups, err
}

// GetAdtyGroup 取一个未删除的分组。
func GetAdtyGroup(id string) (*AdtyGroup, error) {
	var groups []AdtyGroup
	if err := DB.Where("id = ?", id).Limit(1).Find(&groups).Error; err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return nil, ErrAdtyGroupNotFound
	}
	return &groups[0], nil
}

// ActiveAdtyGroupIDs 返回未删除分组的 ID 集合，保存设置时按它校验。
func ActiveAdtyGroupIDs() (map[string]struct{}, error) {
	var ids []string
	if err := DB.Model(&AdtyGroup{}).Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out, nil
}

// CountAdtyGroupRefs 统计还在使用这个分组的用户、密钥和渠道。
func CountAdtyGroupRefs(id string) (AdtyGroupRefs, error) {
	return countAdtyGroupRefs(DB, id)
}

func countAdtyGroupRefs(db *gorm.DB, id string) (AdtyGroupRefs, error) {
	var refs AdtyGroupRefs
	var err error
	refs.Users, err = countAdtyUserRefs(db, id)
	if err != nil {
		return refs, err
	}
	refs.Tokens, err = countAdtyTokenRefs(db, id)
	if err != nil {
		return refs, err
	}
	refs.Channels, err = countAdtyChannelRefs(db, id)
	return refs, err
}

func countAdtyUserRefs(db *gorm.DB, id string) (int64, error) {
	var count int64
	err := db.Model(&User{}).Where(commonGroupCol+" = ?", id).Count(&count).Error
	return count, err
}

func countAdtyTokenRefs(db *gorm.DB, id string) (int64, error) {
	var count int64
	// 空字符串不能转 jsonb。分组列和自动分组同时命中时，这一行只计一次。
	err := db.Model(&Token{}).Where(
		`(`+commonGroupCol+` = ? OR (auto_groups <> '' AND jsonb_exists(auto_groups::jsonb, ?)))`,
		id, id,
	).Count(&count).Error
	return count, err
}

func countAdtyChannelRefs(db *gorm.DB, id string) (int64, error) {
	var count int64
	// 和 Channel.GetGroups 一样：先去掉首尾逗号，再按逗号拆开后精确匹配。group_2 不能命中 group_20。
	query := `EXISTS (
		SELECT 1
		FROM unnest(string_to_array(trim(both ',' from channels.` + commonGroupCol + `), ',')) AS part
		WHERE btrim(part) = ?
	)`
	err := db.Model(&Channel{}).Where(query, id).Count(&count).Error
	return count, err
}

// lockLiveAdtyGroupsTx 按 ID 顺序锁住还在的分组行，避免删除和迁移互相死锁。
func lockLiveAdtyGroupsTx(tx *gorm.DB, ids ...string) error {
	sorted := append([]string(nil), ids...)
	slices.Sort(sorted)
	seen := map[string]struct{}{}
	for _, id := range sorted {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		var rows []AdtyGroup
		if err := lockForUpdate(tx).Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return ErrAdtyGroupNotFound
		}
	}
	return nil
}

// lockAssignableAdtyGroupsTx 在把分组写到用户、密钥或渠道上时调用。
// 没有对应分组行的旧名字放行。已经软删除的分组拒绝，避免删掉以后又被挂上。
func lockAssignableAdtyGroupsTx(tx *gorm.DB, ids []string) error {
	// 还没建分组表的库没有可锁定的行，旧名字照旧放行。
	if tx == nil || !tx.Migrator().HasTable(&AdtyGroup{}) {
		return nil
	}
	sorted := append([]string(nil), ids...)
	slices.Sort(sorted)
	seen := map[string]struct{}{}
	for _, id := range sorted {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		var rows []AdtyGroup
		if err := lockForUpdate(tx.Unscoped()).Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			continue
		}
		if rows[0].DeletedAt.Valid {
			return ErrAdtyGroupNotFound
		}
	}
	return nil
}

func adtyFillLangs(text string) AdtyLangText {
	out := make(AdtyLangText, len(AdtyContentLangs))
	for _, lang := range AdtyContentLangs {
		out[lang] = text
	}
	return out
}

// ValidateAdtyGroupTexts 在读库前检查英文名不为空。
func ValidateAdtyGroupTexts(names, descs AdtyLangText) error {
	_, _, err := normalizeAdtyGroupTexts(names, descs)
	return err
}

// normalizeAdtyGroupTexts 去掉首尾空白，只保留 8 种内容语言；英文名必须有。
func normalizeAdtyGroupTexts(names, descs AdtyLangText) (AdtyLangText, AdtyLangText, error) {
	outNames := make(AdtyLangText, len(AdtyContentLangs))
	outDescs := make(AdtyLangText, len(AdtyContentLangs))
	for _, lang := range AdtyContentLangs {
		outNames[lang] = strings.TrimSpace(names[lang])
		outDescs[lang] = strings.TrimSpace(descs[lang])
	}
	if outNames["en"] == "" {
		return nil, nil, ErrAdtyGroupTextRequired
	}
	return outNames, outDescs, nil
}

func adtySeedableGroupID(id string) bool {
	id = strings.TrimSpace(id)
	if id == "" || utf8.RuneCountInString(id) > adtyGroupIDMaxLen {
		return false
	}
	_, skip := adtySkippedSeedGroupIDs[id]
	return !skip
}

func adtyStripGroupPrefix(key string) string {
	key = strings.TrimSpace(key)
	if after, ok := strings.CutPrefix(key, "+:"); ok {
		return strings.TrimSpace(after)
	}
	if after, ok := strings.CutPrefix(key, "-:"); ok {
		return strings.TrimSpace(after)
	}
	return key
}

// IsAdtyUniqueViolation reports a unique-index conflict from PostgreSQL or SQLite.
func IsAdtyUniqueViolation(err error) bool {
	return isAdtyUniqueViolation(err)
}

func isAdtyUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return true
	}
	text := err.Error()
	return strings.Contains(text, "UNIQUE constraint failed") || strings.Contains(text, "duplicate key")
}

var (
	ErrAdtyGroupSameTarget  = errors.New("adty group migrate target equals source")
	ErrAdtyGroupInUse       = errors.New("adty group still in use")
	ErrAdtyGroupUndeletable = errors.New("adty default group cannot be deleted")
)

// MigrateAdtyGroup 在一个事务里把用户、密钥（分组和自动分组）和渠道从 fromID 改到 toID，
// 渠道的 abilities 跟着重建。软删除的用户和密钥也一起改，免得恢复后指向不存在的分组。
// 提交后刷新受影响用户的分组缓存和密钥缓存，再重建本机的渠道缓存。
func MigrateAdtyGroup(fromID, toID string) (AdtyGroupRefs, error) {
	var moved AdtyGroupRefs
	if fromID == toID {
		return moved, ErrAdtyGroupSameTarget
	}
	if _, err := GetAdtyGroup(fromID); err != nil {
		return moved, err
	}
	if _, err := GetAdtyGroup(toID); err != nil {
		return moved, err
	}
	affectedUsers := map[int]struct{}{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockLiveAdtyGroupsTx(tx, fromID, toID); err != nil {
			return err
		}
		var users []User
		if err := lockForUpdate(tx.Unscoped()).Select("id").Where(commonGroupCol+" = ?", fromID).Order("id").Find(&users).Error; err != nil {
			return err
		}
		if len(users) > 0 {
			ids := make([]int, 0, len(users))
			for _, user := range users {
				ids = append(ids, user.Id)
				affectedUsers[user.Id] = struct{}{}
			}
			if err := tx.Unscoped().Model(&User{}).Where("id IN ?", ids).Update(commonGroupCol, toID).Error; err != nil {
				return err
			}
			moved.Users = int64(len(ids))
		}

		var tokens []Token
		if err := lockForUpdate(tx.Unscoped()).Select("id", "user_id", commonKeyCol, commonGroupCol, "auto_groups").
			Where(`(`+commonGroupCol+` = ? OR (auto_groups <> '' AND jsonb_exists(auto_groups::jsonb, ?)))`, fromID, fromID).
			Order("id").Find(&tokens).Error; err != nil {
			return err
		}
		if err := invalidateTokensCache(tokens); err != nil {
			return err
		}
		for _, token := range tokens {
			updates := map[string]any{}
			if token.Group == fromID {
				updates[commonGroupCol] = toID
			}
			groups, err := token.GetAutoGroups()
			if err != nil {
				return err
			}
			if slices.Contains(groups, fromID) {
				next := make([]string, 0, len(groups))
				for _, group := range groups {
					if group == fromID {
						group = toID
					}
					if !slices.Contains(next, group) {
						next = append(next, group)
					}
				}
				if err := token.SetAutoGroups(next); err != nil {
					return err
				}
				updates["auto_groups"] = token.AutoGroups
			}
			if len(updates) == 0 {
				continue
			}
			if err := tx.Unscoped().Model(&Token{}).Where("id = ?", token.Id).Updates(updates).Error; err != nil {
				return err
			}
			affectedUsers[token.UserId] = struct{}{}
			moved.Tokens++
		}

		var channels []Channel
		if err := lockForUpdate(tx).Select("id", "models", "status", commonGroupCol, "priority", "weight", "tag").Order("id").Find(&channels).Error; err != nil {
			return err
		}
		for _, channel := range channels {
			groups := channel.GetGroups()
			if !slices.Contains(groups, fromID) {
				continue
			}
			next := make([]string, 0, len(groups))
			for _, group := range groups {
				if group == fromID {
					group = toID
				}
				if !slices.Contains(next, group) {
					next = append(next, group)
				}
			}
			channel.Group = strings.Join(next, ",")
			if err := tx.Model(&Channel{}).Where("id = ?", channel.Id).Update(commonGroupCol, channel.Group).Error; err != nil {
				return err
			}
			if err := channel.UpdateAbilities(tx); err != nil {
				return err
			}
			moved.Channels++
		}
		return nil
	})
	if err != nil {
		return AdtyGroupRefs{}, err
	}
	for userID := range affectedUsers {
		if err := RefreshUserGroupCache(userID); err != nil {
			common.SysError(common.LogText("refresh user group cache after group migrate: %s", err.Error()))
		}
		if err := InvalidateUserTokensCache(userID); err != nil {
			common.SysError(common.LogText("invalidate token cache after group migrate: %s", err.Error()))
		}
	}
	if moved.Channels > 0 {
		InitChannelCache()
	}
	return moved, nil
}

// DeleteAdtyGroup 软删除分组：default 不能删，还有用户、密钥或渠道在用时拒绝。
// 同一个事务里从各分组配置里去掉这个 ID，提交后再更新内存里的配置。
func DeleteAdtyGroup(id string) error {
	if id == AdtyDefaultGroupID {
		return ErrAdtyGroupUndeletable
	}
	if _, err := GetAdtyGroup(id); err != nil {
		return err
	}
	adtyGroupConfigMu.Lock()
	defer adtyGroupConfigMu.Unlock()
	var values map[string]string
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := lockLiveAdtyGroupsTx(tx, id); err != nil {
			return err
		}
		refs, err := countAdtyGroupRefs(tx, id)
		if err != nil {
			return err
		}
		if refs.Total() > 0 {
			return ErrAdtyGroupInUse
		}
		values, err = adtyGroupOptionsWithout(id, func(key string) (string, error) {
			return loadAdtyGroupOptionTx(tx, key)
		})
		if err != nil {
			return err
		}
		for key, value := range values {
			option := Option{Key: key}
			if err := tx.FirstOrCreate(&option, Option{Key: key}).Error; err != nil {
				return err
			}
			option.Value = value
			if err := tx.Save(&option).Error; err != nil {
				return err
			}
		}
		return tx.Where("id = ?", id).Delete(&AdtyGroup{}).Error
	})
	if err != nil {
		return err
	}
	for key, value := range values {
		if err := updateOptionMap(key, value); err != nil {
			return err
		}
	}
	return nil
}

// loadAdtyGroupOptionTx 锁住这一行配置再读。没有这一行时返回空串，调用方改用当前内存。
func loadAdtyGroupOptionTx(tx *gorm.DB, key string) (string, error) {
	var rows []Option
	if err := lockForUpdate(tx).Where(commonKeyCol+" = ?", key).Limit(1).Find(&rows).Error; err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	return rows[0].Value, nil
}

func adtyGroupOptionSource(load func(string) (string, error), key, memory string) (string, error) {
	if load == nil {
		return memory, nil
	}
	raw, err := load(key)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(raw) == "" {
		return memory, nil
	}
	return raw, nil
}

// adtyGroupOptionsWithout 返回去掉这个分组以后、内容有变化的分组配置（键 → 新的 JSON）。
// load 返回数据库里锁住的当前值。有行就以它为准，避免用删除开始前的内存快照盖掉后写入的倍率。
func adtyGroupOptionsWithout(id string, load func(string) (string, error)) (map[string]string, error) {
	values := map[string]string{}
	put := func(key string, changed bool, value any) error {
		if !changed {
			return nil
		}
		raw, err := common.Marshal(value)
		if err != nil {
			return err
		}
		values[key] = string(raw)
		return nil
	}

	ratioRaw, err := adtyGroupOptionSource(load, "GroupRatio", ratio_setting.GroupRatio2JSONString())
	if err != nil {
		return nil, err
	}
	ratios := map[string]float64{}
	if err := adtyUnmarshalObject(ratioRaw, &ratios); err != nil {
		return nil, err
	}
	_, had := ratios[id]
	delete(ratios, id)
	if err := put("GroupRatio", had, ratios); err != nil {
		return nil, err
	}

	usableRaw, err := adtyGroupOptionSource(load, "UserUsableGroups", setting.UserUsableGroups2JSONString())
	if err != nil {
		return nil, err
	}
	usable := map[string]string{}
	if err := adtyUnmarshalObject(usableRaw, &usable); err != nil {
		return nil, err
	}
	_, had = usable[id]
	delete(usable, id)
	if err := put("UserUsableGroups", had, usable); err != nil {
		return nil, err
	}

	topupRaw, err := adtyGroupOptionSource(load, "TopupGroupRatio", common.TopupGroupRatio2JSONString())
	if err != nil {
		return nil, err
	}
	topup := map[string]float64{}
	if err := adtyUnmarshalObject(topupRaw, &topup); err != nil {
		return nil, err
	}
	_, had = topup[id]
	delete(topup, id)
	if err := put("TopupGroupRatio", had, topup); err != nil {
		return nil, err
	}

	groupRaw, err := adtyGroupOptionSource(load, "GroupGroupRatio", ratio_setting.GroupGroupRatio2JSONString())
	if err != nil {
		return nil, err
	}
	nested := map[string]map[string]float64{}
	if err := adtyUnmarshalObject(groupRaw, &nested); err != nil {
		return nil, err
	}
	_, had = nested[id]
	delete(nested, id)
	for _, inner := range nested {
		if _, ok := inner[id]; ok {
			delete(inner, id)
			had = true
		}
	}
	if err := put("GroupGroupRatio", had, nested); err != nil {
		return nil, err
	}

	autoRaw, err := adtyGroupOptionSource(load, "AutoGroups", setting.AutoGroups2JsonString())
	if err != nil {
		return nil, err
	}
	auto := []string{}
	if err := adtyUnmarshalObject(autoRaw, &auto); err != nil {
		return nil, err
	}
	if err := put("AutoGroups", slices.Contains(auto, id), slices.DeleteFunc(slices.Clone(auto), func(group string) bool { return group == id })); err != nil {
		return nil, err
	}

	specialMemory := ""
	if spec := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup; spec != nil {
		specialMemory = spec.MarshalJSONString()
	}
	specialRaw, err := adtyGroupOptionSource(load, "group_ratio_setting.group_special_usable_group", specialMemory)
	if err != nil {
		return nil, err
	}
	special := map[string]map[string]string{}
	if err := adtyUnmarshalObject(specialRaw, &special); err != nil {
		return nil, err
	}
	_, had = special[id]
	delete(special, id)
	for owner, inner := range special {
		next := map[string]string{}
		for key, desc := range inner {
			if adtyStripGroupPrefix(key) == id {
				had = true
				continue
			}
			next[key] = desc
		}
		special[owner] = next
	}
	if err := put("group_ratio_setting.group_special_usable_group", had, special); err != nil {
		return nil, err
	}

	limitRaw, err := adtyGroupOptionSource(load, "ModelRequestRateLimitGroup", setting.ModelRequestRateLimitGroup2JSONString())
	if err != nil {
		return nil, err
	}
	limits := map[string][2]int{}
	if err := adtyUnmarshalObject(limitRaw, &limits); err != nil {
		return nil, err
	}
	_, had = limits[id]
	delete(limits, id)
	if err := put("ModelRequestRateLimitGroup", had, limits); err != nil {
		return nil, err
	}
	return values, nil
}
