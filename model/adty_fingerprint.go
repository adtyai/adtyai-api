package model

// AdtyUserFingerprint 记下用户注册、登录、点击推广链接时的 IP 和设备（都是 HMAC 哈希），
// 用来判断自己邀请自己和批量注册（任务 6.5）。
type AdtyUserFingerprint struct {
	Id         int64  `json:"id"`
	UserId     int    `json:"user_id" gorm:"index"`
	IpHash     string `json:"ip_hash" gorm:"type:varchar(64);index"`
	DeviceHash string `json:"device_hash" gorm:"type:varchar(64);index"`
	Kind       string `json:"kind" gorm:"type:varchar(16)"` // register | login | click
	CreatedAt  int64  `json:"created_at" gorm:"index"`
}

func (AdtyUserFingerprint) TableName() string {
	return "adty_user_fingerprints"
}
