package model

import (
	"errors"
	"math"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrGroupBillingExtraCode = errors.New("group table cannot add a billing group")
	ErrGroupBillingKeySet    = errors.New("group table cannot change the billing group set")
	ErrGroupBillingRatio     = errors.New("group ratio must be a number")
	ErrGroupBillingUsable    = errors.New("group table cannot publish an unknown group")
)

// SaveGroupBillingTable writes GroupRatio, UserUsableGroups, and display names
// in one transaction. It refuses to add or remove billing codes. Memory maps
// change only after the transaction commits.
func SaveGroupBillingTable(ratio map[string]float64, usable map[string]string, names map[string]string) error {
	if ratio == nil {
		return ErrGroupBillingKeySet
	}
	currentRatio := ratio_setting.GetGroupRatioCopy()
	if len(ratio) != len(currentRatio) {
		return ErrGroupBillingKeySet
	}
	for code, value := range ratio {
		if _, ok := currentRatio[code]; !ok {
			return ErrGroupBillingExtraCode
		}
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return ErrGroupBillingRatio
		}
	}
	if usable == nil {
		usable = map[string]string{}
	}
	currentUsable := setting.GetUserUsableGroupsCopy()
	cleanedUsable := make(map[string]string, len(usable))
	for code, desc := range usable {
		if _, ok := currentRatio[code]; ok {
			cleanedUsable[code] = desc
			continue
		}
		if _, ok := currentUsable[code]; !ok {
			return ErrGroupBillingUsable
		}
	}
	usable = cleanedUsable
	for code, name := range names {
		if _, ok := currentRatio[code]; !ok {
			return ErrGroupBillingExtraCode
		}
		if strings.TrimSpace(name) == "" {
			return ErrGroupMetaDisplayName
		}
		if err := ValidateGroupMetaCode(code); err != nil {
			return err
		}
	}

	ratioBytes, err := common.Marshal(ratio)
	if err != nil {
		return err
	}
	usableBytes, err := common.Marshal(usable)
	if err != nil {
		return err
	}
	values := map[string]string{
		"GroupRatio":       string(ratioBytes),
		"UserUsableGroups": string(usableBytes),
	}
	if err = ratio_setting.CheckGroupRatio(values["GroupRatio"]); err != nil {
		return err
	}
	dbValues, err := encodeOptionValuesForDB(values)
	if err != nil {
		return err
	}
	for attempt := 0; attempt < groupRatioWriteAttempts; attempt++ {
		err = DB.Transaction(func(tx *gorm.DB) error {
			snap, readErr := readGroupRatioTx(tx)
			if readErr != nil {
				return readErr
			}
			if snap.exists && !sameGroupRatioKeys(snap.ratios, ratio) {
				return &groupRatioStaleError{plain: snap.plain}
			}
			usableOption := Option{Key: "UserUsableGroups"}
			if err := tx.FirstOrCreate(&usableOption, Option{Key: "UserUsableGroups"}).Error; err != nil {
				return err
			}
			usableOption.Value = dbValues["UserUsableGroups"]
			if err := tx.Save(&usableOption).Error; err != nil {
				return err
			}
			if err := writeGroupRatioTx(tx, snap, values["GroupRatio"]); err != nil {
				return err
			}
			for code, name := range names {
				trimmed := strings.TrimSpace(name)
				if _, _, err := UpdateGroupMetaDisplay(tx, code, GroupMetaPutInput{DisplayName: &trimmed}); err != nil {
					return err
				}
			}
			return nil
		})
		if err == nil || !isGroupRatioConflict(err) {
			break
		}
	}
	var stale *groupRatioStaleError
	if errors.As(err, &stale) {
		if stale != nil && stale.plain != "" {
			_ = publishCommittedOption("GroupRatio", stale.plain)
		}
		return ErrGroupBillingKeySet
	}
	if err != nil {
		return err
	}
	if err = publishCommittedOption("GroupRatio", values["GroupRatio"]); err != nil {
		return err
	}
	return publishCommittedOption("UserUsableGroups", values["UserUsableGroups"])
}

const groupRatioWriteAttempts = 5

var errGroupRatioConflict = errors.New("group ratio changed")

type groupRatioSnapshot struct {
	ratios  map[string]float64
	plain   string
	encoded string
	exists  bool
}

type groupRatioStaleError struct {
	plain string
}

func (e *groupRatioStaleError) Error() string { return ErrGroupBillingKeySet.Error() }
func (e *groupRatioStaleError) Unwrap() error { return ErrGroupBillingKeySet }

func copyGroupRatio(source map[string]float64) map[string]float64 {
	next := make(map[string]float64, len(source))
	for key, value := range source {
		next[key] = value
	}
	return next
}

func sameGroupRatioKeys(left, right map[string]float64) bool {
	if len(left) != len(right) {
		return false
	}
	for code := range left {
		if _, ok := right[code]; !ok {
			return false
		}
	}
	return true
}

func isGroupRatioConflict(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errGroupRatioConflict) {
		return true
	}
	text := strings.ToLower(err.Error())
	return isUniqueViolation(err) || strings.Contains(text, "database is locked") || strings.Contains(text, "sqlite_busy") || strings.Contains(text, "deadlock")
}

func readGroupRatioTx(tx *gorm.DB) (groupRatioSnapshot, error) {
	query := tx
	if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var option Option
	err := query.Where(Option{Key: "GroupRatio"}).First(&option).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return groupRatioSnapshot{ratios: ratio_setting.GetGroupRatioCopy()}, nil
	}
	if err != nil {
		return groupRatioSnapshot{}, err
	}
	plain, _, decodeErr := decodeOptionValueFromDB("GroupRatio", option.Value)
	if decodeErr != nil {
		return groupRatioSnapshot{}, decodeErr
	}
	parsed := map[string]float64{}
	if strings.TrimSpace(plain) != "" {
		if err := common.Unmarshal([]byte(plain), &parsed); err != nil {
			return groupRatioSnapshot{}, err
		}
	}
	if parsed == nil {
		parsed = map[string]float64{}
	}
	return groupRatioSnapshot{ratios: parsed, plain: plain, encoded: option.Value, exists: true}, nil
}

func writeGroupRatioTx(tx *gorm.DB, snap groupRatioSnapshot, plain string) error {
	encoded, err := encodeOptionValueForDB("GroupRatio", plain)
	if err != nil {
		return err
	}
	if !snap.exists {
		return tx.Create(&Option{Key: "GroupRatio", Value: encoded}).Error
	}
	if snap.encoded == encoded {
		return nil
	}
	result := tx.Model(&Option{}).
		Where(clause.Eq{Column: clause.Column{Name: "key"}, Value: "GroupRatio"}).
		Where(clause.Eq{Column: clause.Column{Name: "value"}, Value: snap.encoded}).
		Update("value", encoded)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errGroupRatioConflict
	}
	return nil
}
