package model

import "gorm.io/gorm"

// ReadOptionFromDB 直接查 options 表，不读 OptionMap（任务 6.4）：多实例时别的节点刚改的值，
// 本机的 OptionMap 可能还没同步到。行不存在时返回 false。
func ReadOptionFromDB(key string) (string, bool, error) {
	return readOption(DB, key, false)
}

// ReadOptionForUpdate 在调用方事务里锁住这一行再读。行不存在时不加锁，返回 false。
func ReadOptionForUpdate(tx *gorm.DB, key string) (string, bool, error) {
	return readOption(tx, key, true)
}

func readOption(db *gorm.DB, key string, lock bool) (string, bool, error) {
	if isAdtySecretOption(key) {
		var value string
		var exists bool
		err := db.Transaction(func(tx *gorm.DB) error {
			var err error
			value, exists, err = readAdtyOptionSecret(tx, key)
			return err
		})
		return value, exists, err
	}
	var options []Option
	query := db
	if lock {
		query = lockForUpdate(db)
	}
	if err := query.Where(commonKeyCol+" = ?", key).Limit(1).Find(&options).Error; err != nil {
		return "", false, err
	}
	if len(options) == 0 {
		return "", false, nil
	}
	return options[0].Value, true, nil
}
