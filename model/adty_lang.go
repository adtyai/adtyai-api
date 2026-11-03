package model

import (
	"database/sql/driver"
	"fmt"

	"github.com/QuantumNous/new-api/common"
)

// driverValue 把 database/sql 的返回类型留在这个文件，避免分组文件再引一层。
type driverValue = driver.Value

func adtyLangTextValue(text AdtyLangText) (driver.Value, error) {
	if text == nil {
		return "{}", nil
	}
	raw, err := common.Marshal(text)
	if err != nil {
		return nil, err
	}
	return string(raw), nil
}

func adtyLangTextScan(text *AdtyLangText, value any) error {
	if text == nil {
		return fmt.Errorf("AdtyLangText scan on nil pointer")
	}
	switch typed := value.(type) {
	case nil:
		*text = AdtyLangText{}
		return nil
	case []byte:
		if len(typed) == 0 {
			*text = AdtyLangText{}
			return nil
		}
		return common.Unmarshal(typed, text)
	case string:
		if typed == "" {
			*text = AdtyLangText{}
			return nil
		}
		return common.Unmarshal([]byte(typed), text)
	default:
		raw, err := common.Marshal(typed)
		if err != nil {
			return err
		}
		return common.Unmarshal(raw, text)
	}
}
