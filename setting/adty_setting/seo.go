package adty_setting

import "github.com/QuantumNous/new-api/setting/config"

// SEOSetting 是首页 8 个语言网址的标题、描述和分享图（任务 9.3），键名 adty_seo_setting.<json 标签>。
// HomeTitle、HomeDescription 是「内容语言码（en、zh-CN、zh-TW、fr、ja、ko、ru、vi，约定 E）→ 文本」。
// 某种语言没填就用 en 的；en 也没填时，标题用站名，描述不写。
type SEOSetting struct {
	HomeTitle       map[string]string `json:"home_title"`
	HomeDescription map[string]string `json:"home_description"`
	OGImage         string            `json:"og_image"` // 以 / 开头时拼在服务器地址后面
}

var seoSetting = SEOSetting{
	HomeTitle:       map[string]string{},
	HomeDescription: map[string]string{},
	OGImage:         "/adty-og.png",
}

func init() {
	config.GlobalConfig.Register("adty_seo_setting", &seoSetting)
}

// GetSEOSetting 返回当前的首页 SEO 配置。
func GetSEOSetting() *SEOSetting {
	return &seoSetting
}
