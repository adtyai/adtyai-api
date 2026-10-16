package model

import (
	"strings"

	"github.com/QuantumNous/new-api/common"
)

func sanitizeBrandOptionValue(key, value string) string {
	switch key {
	case "SystemName":
		v := common.SanitizeSystemName(value)
		if strings.TrimSpace(v) == "" {
			return common.DefaultSystemName
		}
		return v
	case "Footer":
		return common.SanitizeFooterHTML(value)
	case "About":
		return common.SanitizeAbout(value)
	default:
		return value
	}
}

// persistSanitizedBrandOptions writes cleaned SystemName/Footer/About back to DB
// when the stored value still carries upstream New API brand residue.
func persistSanitizedBrandOptions() {
	for _, key := range []string{"SystemName", "Footer", "About"} {
		var row Option
		err := DB.Where(Option{Key: key}).First(&row).Error
		if err != nil {
			// missing row is fine — in-memory default already ADTY AI / empty
			continue
		}
		plain, _, decErr := decodeOptionValueFromDB(key, row.Value)
		if decErr != nil {
			continue
		}
		cleaned := sanitizeBrandOptionValue(key, plain)
		if cleaned == plain {
			continue
		}
		dbValue, encErr := encodeOptionValueForDB(key, cleaned)
		if encErr != nil {
			common.SysLog("brand sanitize encode " + key + ": " + encErr.Error())
			continue
		}
		row.Value = dbValue
		if err := DB.Save(&row).Error; err != nil {
			common.SysLog("brand sanitize save " + key + ": " + err.Error())
			continue
		}
		_ = updateOptionMap(key, cleaned)
		common.SysLog("brand sanitize: cleared New API residue in option " + key)
	}
}
