package service

import "regexp"

// adtyRedactPattern 是「脱敏」用的判据：命中就替换成 [redacted]。
//
// 它的目标只有一个——把用户粘进工单、公告里的凭据原文抹掉。所以它可以宽：
// 多打一段码的代价只是少了一点可读性，而漏掉一段 key 就是明文落库、跟着工单
// 一起被导出归档（ASVS 5.0.0 14.2.6）。
//
// JWT 那一段必须是三段式。eyJ 只是 base64 的 `{"`，单独出现于
// `data:application/json;base64,eyJ…` 和讲 JWT 的文档里都是正常文本；
// 写成 eyJ[A-Za-z0-9_-]{8,} 会把它们一起打码。
var adtyRedactPattern = regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_\-]{8,}|bot\d{6,}:[A-Za-z0-9_\-]{10,}|\d{6,12}:[A-Za-z0-9_\-]{20,}|api[_-]?key\s*[:=]\s*\S+|password\s*[:=]\s*\S+|验证码\s*[:=]\s*\S+|Bearer\s+[A-Za-z0-9._\-]{16,}|eyJ[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{10,}\.[A-Za-z0-9_\-]{8,}|AKIA[0-9A-Z]{16}|ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|xox[baprs]-[A-Za-z0-9\-]{10,})`)

// adtyRejectPattern 是「拒单」用的判据：命中就整个操作被拒。
//
// 它的代价和脱敏完全不同，所以它必须窄——只保留几乎不可能出现在正常文本里的
// 形态。api_key:、password:、验证码: 和裸 <id>:<token> 这四类在讲配置的文档、
// 用户贴的报错原文、待翻译的正文里都是正常内容，落在这里会让正常操作被拒，
// 而翻译那条拒得还是静默的（err=nil + Status=failed）。
var adtyRejectPattern = regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_\-]{8,}|bot\d{6,}:[A-Za-z0-9_\-]{10,}|AKIA[0-9A-Z]{16}|ghp_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|xox[baprs]-[A-Za-z0-9\-]{10,})`)

func redactAdtySecrets(text string) string {
	return adtyRedactPattern.ReplaceAllString(text, "[redacted]")
}

func adtyTextHasSecret(text string) bool {
	return adtyRejectPattern.MatchString(text)
}
