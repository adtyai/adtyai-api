package adty_setting

import "github.com/QuantumNous/new-api/setting/config"

// LegalSetting 是多语言条款（任务 8.6），键名 adty_legal_setting.<json 标签>。
// 每个字段都是「内容语言码（en、zh-CN、zh-TW、fr、ja、ko、ru、vi，约定 E）→ Markdown」。
// 上游 legal.user_agreement、legal.privacy_policy 保留作默认文本，/api/status 的两个开关只看上游。
type LegalSetting struct {
	UserAgreement map[string]string `json:"user_agreement"`
	PrivacyPolicy map[string]string `json:"privacy_policy"`
	RefundPolicy  map[string]string `json:"refund_policy"`
	// 开站协议和分站客户条款（P11）。版本号改了内容就加 1，用户下次开站或登录时要重新勾选；0 表示还没写。
	SiteAgreement            map[string]string `json:"site_agreement"`
	SiteCustomerTerms        map[string]string `json:"site_customer_terms"`
	SiteAgreementVersion     int               `json:"site_agreement_version"`
	SiteCustomerTermsVersion int               `json:"site_customer_terms_version"`
}

var legalSetting = LegalSetting{
	UserAgreement: map[string]string{},
	PrivacyPolicy: map[string]string{},
	RefundPolicy:  map[string]string{},

	SiteAgreement:     map[string]string{},
	SiteCustomerTerms: map[string]string{},
}

func init() {
	config.GlobalConfig.Register("adty_legal_setting", &legalSetting)
}

// GetLegalSetting 返回当前的多语言条款。
func GetLegalSetting() *LegalSetting {
	return &legalSetting
}
