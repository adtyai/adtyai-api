package i18n

// 二开的后端提示键（执行层决定 13）：中转接口报错、邮件和通知用。
// 译文在 locales/adty.<语言>.yaml；上游的 keys.go 和三个 yaml 不动。
const (
	MsgAdtyEpayMaintenanceSubject = "adty.epay.maintenance_subject"
	MsgAdtyEpayMaintenanceBody    = "adty.epay.maintenance_body"

	MsgAdtyReferralWithdrawPaidSubject     = "adty.referral.withdraw_paid_subject"
	MsgAdtyReferralWithdrawPaidBody        = "adty.referral.withdraw_paid_body"
	MsgAdtyReferralWithdrawRejectedSubject = "adty.referral.withdraw_rejected_subject"
	MsgAdtyReferralWithdrawRejectedBody    = "adty.referral.withdraw_rejected_body"

	MsgAdtyEmailRegisteredSubject = "adty.email.registered_subject"
	MsgAdtyEmailRegisteredBody    = "adty.email.registered_body"

	MsgAdtyLimitRPM         = "adty.limit.rpm"
	MsgAdtyLimitTPM         = "adty.limit.tpm"
	MsgAdtyLimitConcurrency = "adty.limit.concurrency"

	MsgAdtyMarketingUnsubscribe        = "adty.marketing.unsubscribe"
	MsgAdtyMarketingUnsubscribePrompt  = "adty.marketing.unsubscribe_prompt"
	MsgAdtyMarketingUnsubscribeDone    = "adty.marketing.unsubscribe_done"
	MsgAdtyMarketingUnsubscribeInvalid = "adty.marketing.unsubscribe_invalid"
)
