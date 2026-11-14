package service

import (
	"strings"

	"github.com/QuantumNous/new-api/model"
)

// AdtyOfficialGlossaryPair is one A.20.2 term in the official languages.
// English is the official counterpart already used in the repo or the plan.
type AdtyOfficialGlossaryPair struct {
	ZH string
	EN string
}

// AdtyOfficialGlossary is the protected A.20.2 terminology for zh-CN and en.
// These pairs are seeded into AdtyGlossaryTerm so dynamic translation cannot
// rewrite the business terms.
var AdtyOfficialGlossary = []AdtyOfficialGlossaryPair{
	{ZH: "本金", EN: "principal"},
	{ZH: "永久赠金", EN: "permanent bonus"},
	{ZH: "冻结金额", EN: "frozen amount"},
	{ZH: "可用余额", EN: "available balance"},
	{ZH: "收益", EN: "earnings"},
	{ZH: "保证金", EN: "deposit"},
	{ZH: "人工退款", EN: "manual refund"},
	{ZH: "佣金释放", EN: "commission release"},
	{ZH: "冲回", EN: "clawback"},
	{ZH: "追偿", EN: "recovery"},
}

const (
	AdtyOfficialGlossaryLocaleEN   = "en"
	AdtyOfficialGlossaryLocaleZhCN = "zh-CN"
)

// SeedAdtyOfficialGlossary upserts the official A.20.2 pairs for zh-CN and en.
// Existing rows for the same source and locale are updated to the official target.
func SeedAdtyOfficialGlossary() error {
	for _, term := range AdtyOfficialGlossary {
		if err := SaveAdtyGlossaryTerm(term.ZH, AdtyOfficialGlossaryLocaleEN, term.EN); err != nil {
			return err
		}
		if err := SaveAdtyGlossaryTerm(term.EN, AdtyOfficialGlossaryLocaleZhCN, term.ZH); err != nil {
			return err
		}
	}
	return nil
}

func adtyTranslationKeepsGlossary(source, english string) bool {
	var terms []model.AdtyGlossaryTerm
	if err := model.DB.Where("locale = ?", AdtyOfficialGlossaryLocaleEN).Find(&terms).Error; err != nil {
		return true
	}
	lower := strings.ToLower(english)
	for _, term := range terms {
		if term.Source == "" || term.Target == "" {
			continue
		}
		if strings.Contains(source, term.Source) && !strings.Contains(lower, strings.ToLower(term.Target)) {
			return false
		}
	}
	return true
}
