package model

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// A08 / A2 group_meta: fixed English code + mutable display_name.
// Upstream users/tokens/channels/GroupRatio keep storing the code string unchanged.

var (
	ErrGroupMetaInvalidCode   = errors.New("invalid group code")
	ErrGroupMetaCodeImmutable = errors.New("group code is immutable")
	ErrGroupMetaNotFound      = errors.New("group meta not found")
	ErrGroupMetaExists        = errors.New("group meta already exists")
	ErrGroupMetaDisplayName   = errors.New("display_name required")
)

var groupMetaCodeRe = regexp.MustCompile(`^[a-z0-9_-]{2,32}$`)

// GroupMeta is the display-name registry for fixed group codes.
type GroupMeta struct {
	Code        string    `json:"code" gorm:"primaryKey;type:text"`
	DisplayName string    `json:"display_name" gorm:"type:text;not null"`
	Sort        int       `json:"sort" gorm:"not null;default:0"`
	CreatedAt   time.Time `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt   time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (GroupMeta) TableName() string { return "group_meta" }

// GroupMetaView is one row in GET /api/admin/groups (meta may be virtual).
type GroupMetaView struct {
	Code        string `json:"code"`
	DisplayName string `json:"display_name"`
	Sort        int    `json:"sort"`
	HasMeta     bool   `json:"has_meta"`
}

// ValidateGroupMetaCode checks A2 code format.
func ValidateGroupMetaCode(code string) error {
	code = strings.TrimSpace(code)
	if !groupMetaCodeRe.MatchString(code) {
		return ErrGroupMetaInvalidCode
	}
	return nil
}

// EnsureGroupMetaSchema migrates group_meta and seeds known codes (display_name=code).
func EnsureGroupMetaSchema(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	if db == nil {
		return fmt.Errorf("group_meta: db nil")
	}
	if err := db.AutoMigrate(&GroupMeta{}); err != nil {
		return fmt.Errorf("group_meta migrate: %w", err)
	}
	return SeedGroupMetaKnownCodes(db)
}

func addGroupCode(out map[string]struct{}, c string) {
	c = strings.TrimSpace(c)
	if c == "" || c == "auto" {
		return
	}
	if ValidateGroupMetaCode(c) != nil {
		return
	}
	out[c] = struct{}{}
}

// CollectKnownGroupCodes gathers codes from GroupRatio / TopupGroupRatio / DB columns.
func CollectKnownGroupCodes(db *gorm.DB) map[string]struct{} {
	out := map[string]struct{}{}
	for c := range ratio_setting.GetGroupRatioCopy() {
		addGroupCode(out, c)
	}
	if raw := common.TopupGroupRatio2JSONString(); raw != "" {
		var m map[string]float64
		if err := common.Unmarshal([]byte(raw), &m); err == nil {
			for c := range m {
				addGroupCode(out, c)
			}
		}
	}
	if db == nil {
		return out
	}
	pluckDistinctGroup := func(table string) {
		var cols []string
		// "group" is reserved in PostgreSQL; quote via GORM column name on model fields.
		_ = db.Table(table).Distinct("\"group\"").Pluck("group", &cols)
		for _, c := range cols {
			for _, part := range strings.Split(c, ",") {
				addGroupCode(out, part)
			}
		}
	}
	pluckDistinctGroup("users")
	pluckDistinctGroup("tokens")
	pluckDistinctGroup("channels")
	return out
}

// SeedGroupMetaKnownCodes inserts missing rows with display_name=code (idempotent).
func SeedGroupMetaKnownCodes(db *gorm.DB) error {
	if db == nil {
		db = DB
	}
	codes := CollectKnownGroupCodes(db)
	if len(codes) == 0 {
		return nil
	}
	now := time.Now()
	rows := make([]GroupMeta, 0, len(codes))
	for c := range codes {
		rows = append(rows, GroupMeta{
			Code:        c,
			DisplayName: c,
			Sort:        0,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}
	// Do nothing on conflict — never overwrite an admin-edited display_name.
	return db.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(rows, 100).Error
}

// ListGroupMetaViews returns meta rows plus in-use codes without meta (display_name=code).
func ListGroupMetaViews(db *gorm.DB) ([]GroupMetaView, error) {
	if db == nil {
		db = DB
	}
	var rows []GroupMeta
	if err := db.Order("sort ASC, code ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	persisted := make(map[string]GroupMeta, len(rows))
	for _, r := range rows {
		persisted[r.Code] = r
	}
	known := CollectKnownGroupCodes(db)
	for c := range known {
		if _, ok := persisted[c]; !ok {
			persisted[c] = GroupMeta{Code: c, DisplayName: c, Sort: 0}
		}
	}
	// Mark which were actually in DB
	inDB := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		inDB[r.Code] = struct{}{}
	}
	out := make([]GroupMetaView, 0, len(persisted))
	for c, r := range persisted {
		dn := strings.TrimSpace(r.DisplayName)
		if dn == "" {
			dn = c
		}
		_, has := inDB[c]
		out = append(out, GroupMetaView{
			Code:        c,
			DisplayName: dn,
			Sort:        r.Sort,
			HasMeta:     has,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sort != out[j].Sort {
			return out[i].Sort < out[j].Sort
		}
		return out[i].Code < out[j].Code
	})
	return out, nil
}

// BuildGroupDisplayNameMap returns code → display_name for status / frontend.
// Missing meta falls back to code. Safe when DB is nil (empty map).
func BuildGroupDisplayNameMap() map[string]string {
	out := map[string]string{}
	if DB == nil {
		return out
	}
	views, err := ListGroupMetaViews(DB)
	if err != nil {
		return out
	}
	for _, v := range views {
		dn := v.DisplayName
		if dn == "" {
			dn = v.Code
		}
		out[v.Code] = dn
	}
	return out
}

// GetGroupDisplayName resolves one code (fallback to code itself).
func GetGroupDisplayName(code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}
	if DB == nil {
		return code
	}
	var row GroupMeta
	err := DB.Where("code = ?", code).First(&row).Error
	if err != nil || strings.TrimSpace(row.DisplayName) == "" {
		return code
	}
	return row.DisplayName
}

// GroupMetaPutInput is PUT body. Code in body must be absent or equal path code.
type GroupMetaPutInput struct {
	Code        *string `json:"code"`
	DisplayName *string `json:"display_name"`
	Sort        *int    `json:"sort"`
}

// GroupMetaCreateInput is POST body.
type GroupMetaCreateInput struct {
	Code        string `json:"code"`
	DisplayName string `json:"display_name"`
	Sort        *int   `json:"sort"`
}

// UpdateGroupMetaDisplay updates display_name (and optional sort) for pathCode.
// Rejects any attempt to change code via body.
func UpdateGroupMetaDisplay(db *gorm.DB, pathCode string, in GroupMetaPutInput) (oldSnap, newSnap GroupMetaView, err error) {
	if db == nil {
		db = DB
	}
	pathCode = strings.TrimSpace(pathCode)
	if err = ValidateGroupMetaCode(pathCode); err != nil {
		return
	}
	if in.Code != nil {
		bodyCode := strings.TrimSpace(*in.Code)
		if bodyCode != "" && bodyCode != pathCode {
			err = ErrGroupMetaCodeImmutable
			return
		}
	}
	if in.DisplayName == nil && in.Sort == nil {
		err = fmt.Errorf("display_name or sort required")
		return
	}
	var row GroupMeta
	findErr := db.Where("code = ?", pathCode).First(&row).Error
	if findErr != nil {
		if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			err = findErr
			return
		}
		dn := pathCode
		if in.DisplayName != nil {
			dn = strings.TrimSpace(*in.DisplayName)
			if dn == "" {
				err = ErrGroupMetaDisplayName
				return
			}
		}
		sortV := 0
		if in.Sort != nil {
			sortV = *in.Sort
		}
		oldSnap = GroupMetaView{Code: pathCode, DisplayName: pathCode, Sort: 0, HasMeta: false}
		row = GroupMeta{Code: pathCode, DisplayName: dn, Sort: sortV}
		if err = db.Create(&row).Error; err != nil {
			return
		}
		newSnap = GroupMetaView{Code: row.Code, DisplayName: row.DisplayName, Sort: row.Sort, HasMeta: true}
		return
	}
	oldSnap = GroupMetaView{Code: row.Code, DisplayName: row.DisplayName, Sort: row.Sort, HasMeta: true}
	updates := map[string]any{}
	if in.DisplayName != nil {
		dn := strings.TrimSpace(*in.DisplayName)
		if dn == "" {
			err = ErrGroupMetaDisplayName
			return
		}
		updates["display_name"] = dn
	}
	if in.Sort != nil {
		updates["sort"] = *in.Sort
	}
	if err = db.Model(&GroupMeta{}).Where("code = ?", pathCode).Updates(updates).Error; err != nil {
		return
	}
	_ = db.Where("code = ?", pathCode).First(&row).Error
	newSnap = GroupMetaView{Code: row.Code, DisplayName: row.DisplayName, Sort: row.Sort, HasMeta: true}
	return
}

// CreateGroupMeta inserts a new meta row.
func CreateGroupMeta(db *gorm.DB, in GroupMetaCreateInput) (GroupMetaView, error) {
	if db == nil {
		db = DB
	}
	code := strings.TrimSpace(in.Code)
	if err := ValidateGroupMetaCode(code); err != nil {
		return GroupMetaView{}, err
	}
	dn := strings.TrimSpace(in.DisplayName)
	if dn == "" {
		dn = code
	}
	sortV := 0
	if in.Sort != nil {
		sortV = *in.Sort
	}
	var n int64
	if err := db.Model(&GroupMeta{}).Where("code = ?", code).Count(&n).Error; err != nil {
		return GroupMetaView{}, err
	}
	if n > 0 {
		return GroupMetaView{}, ErrGroupMetaExists
	}
	row := GroupMeta{Code: code, DisplayName: dn, Sort: sortV}
	if err := db.Create(&row).Error; err != nil {
		return GroupMetaView{}, err
	}
	return GroupMetaView{Code: row.Code, DisplayName: row.DisplayName, Sort: row.Sort, HasMeta: true}, nil
}

// CreateGroupWithRatio writes metadata and a default hidden billing ratio in one commit.
// auto is rejected. Existing metadata without a ratio is repaired instead of reported as exists.
func CreateGroupWithRatio(db *gorm.DB, in GroupMetaCreateInput) (GroupMetaView, error) {
	if db == nil {
		db = DB
	}
	code := strings.TrimSpace(in.Code)
	if code == "auto" {
		return GroupMetaView{}, ErrGroupMetaInvalidCode
	}
	if err := ValidateGroupMetaCode(code); err != nil {
		return GroupMetaView{}, err
	}
	dn := strings.TrimSpace(in.DisplayName)
	if dn == "" {
		dn = code
	}
	sortV := 0
	if in.Sort != nil {
		sortV = *in.Sort
	}
	var view GroupMetaView
	var saved string
	var seenPlain string
	var txErr error
	for attempt := 0; attempt < groupRatioWriteAttempts; attempt++ {
		saved = ""
		seenPlain = ""
		txErr = db.Transaction(func(tx *gorm.DB) error {
			snap, readErr := readGroupRatioTx(tx)
			if readErr != nil {
				return readErr
			}
			seenPlain = snap.plain
			_, hasRatio := snap.ratios[code]
			var existing GroupMeta
			findErr := tx.Where("code = ?", code).First(&existing).Error
			metaExists := findErr == nil
			if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
				return findErr
			}
			if metaExists && hasRatio {
				return ErrGroupMetaExists
			}
			if !metaExists {
				row := GroupMeta{Code: code, DisplayName: dn, Sort: sortV}
				if createErr := tx.Create(&row).Error; createErr != nil {
					return createErr
				}
				view = GroupMetaView{Code: row.Code, DisplayName: row.DisplayName, Sort: row.Sort, HasMeta: true}
			} else {
				view = GroupMetaView{Code: existing.Code, DisplayName: existing.DisplayName, Sort: existing.Sort, HasMeta: true}
			}
			if hasRatio {
				return nil
			}
			next := copyGroupRatio(snap.ratios)
			next[code] = 1
			raw, marshalErr := common.Marshal(next)
			if marshalErr != nil {
				return marshalErr
			}
			saved = string(raw)
			if checkErr := ratio_setting.CheckGroupRatio(saved); checkErr != nil {
				return checkErr
			}
			return writeGroupRatioTx(tx, snap, saved)
		})
		if txErr == nil || errors.Is(txErr, ErrGroupMetaExists) || !isGroupRatioConflict(txErr) {
			break
		}
	}
	if txErr != nil {
		return GroupMetaView{}, txErr
	}
	refresh := saved
	if refresh == "" {
		refresh = seenPlain
	}
	if refresh != "" {
		if mapErr := publishCommittedOption("GroupRatio", refresh); mapErr != nil {
			return GroupMetaView{}, mapErr
		}
	}
	return view, nil
}
