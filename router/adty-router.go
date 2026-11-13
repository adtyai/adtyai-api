package router

import (
	"net/http"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/gin-gonic/gin"
)

// SetAdtyRouter 挂分组路由。令牌规则在注册处理函数前写入，覆盖测试才能看到。
func SetAdtyRouter(apiRouter *gin.RouterGroup) {
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/groups/display", "any", "")
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/admin/groups", "scope", "option:read")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/admin/groups", "scope", "option:write")
	middleware.DeclareAdtyAccessTokenRoute("PUT", "/api/adty/admin/groups/:id", "scope", "option:write")
	middleware.DeclareAdtyAccessTokenRoute("DELETE", "/api/adty/admin/groups/:id", "scope", "option:write")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/admin/groups/:id/migrate", "scope", "option:write")

	apiRouter.GET("/adty/groups/display", middleware.TryUserAuth(), controller.GetAdtyGroupDisplay)
	apiRouter.GET("/adty/admin/groups", middleware.RootAuth(), controller.GetAdtyGroups)
	apiRouter.POST("/adty/admin/groups", middleware.RootAuth(), controller.PostAdtyGroup)
	apiRouter.PUT("/adty/admin/groups/:id", middleware.RootAuth(), controller.UpdateAdtyGroup)
	apiRouter.DELETE("/adty/admin/groups/:id", middleware.RootAuth(), controller.DeleteAdtyGroup)
	apiRouter.POST("/adty/admin/groups/:id/migrate", middleware.RootAuth(), controller.MigrateAdtyGroup)

	registerAdtyTopUpRoutes(apiRouter)
	registerAdtyFinanceRoutes(apiRouter)
	registerAdtyBEpusdtRoutes(apiRouter)
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/internal/flush-batch", "session", "")
	apiRouter.POST("/adty/internal/flush-batch", controller.FlushAdtyBatchUpdates)
	registerAdtyEpayRoutes(apiRouter)
	registerAdtyReferralRoutes(apiRouter)
	registerAdtyOfficialPriceRoutes(apiRouter)
	registerAdtySiteRoutes(apiRouter)
	registerAdtyLegalRoutes(apiRouter)
	registerAdtyPromoRoutes(apiRouter)
	registerAdtyResellerRoutes(apiRouter)
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/notifications", "scope", "profile:read")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/notifications/read", "scope", "profile:write")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/notifications/hide", "scope", "profile:write")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/notifications/todo/complete", "scope", "profile:write")
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/notifications/announcements/:key", "scope", "profile:read")
	notifications := apiRouter.Group("/adty/notifications", middleware.UserAuth())
	notifications.GET("/announcements/:key", controller.GetAdtyAnnouncement)
	notifications.GET("", controller.GetAdtyNotifications)
	notifications.POST("/read", controller.ReadAdtyNotifications)
	notifications.POST("/hide", controller.PostAdtyNotificationHide)
	notifications.POST("/todo/complete", controller.PostAdtyNotificationTodoComplete)
	for _, route := range []struct{ method, path, scope string }{
		{http.MethodGet, "", "read"}, {http.MethodPost, "", "write"},
		{http.MethodPut, "/:id", "write"}, {http.MethodDelete, "/:id", "write"},
	} {
		rootPath, ownerPath := "/adty/admin/notifications"+route.path, "/adty/site/notifications"+route.path
		middleware.DeclareAdtyAccessTokenRoute(route.method, "/api"+rootPath, "scope", "option:"+route.scope)
		middleware.DeclareAdtyAccessTokenRoute(route.method, "/api"+ownerPath, "scope", "wallet:"+route.scope)
		handler := controller.WriteAdtyManagedNotification
		if route.method == http.MethodGet {
			handler = controller.GetAdtyManagedNotifications
		}
		apiRouter.Handle(route.method, rootPath, middleware.RootAuth(), func(c *gin.Context) { c.Set("notification_root", true) }, handler)
		apiRouter.Handle(route.method, ownerPath, middleware.UserAuth(), handler)
	}

	// 余额流水（任务 7.2）
	users := apiRouter.Group("/adty/admin/users", middleware.AdminAuth())
	handlePermissionRoute(users, http.MethodGet, "/:id/ledger", authz.AdtyUserRead, controller.GetAdtyUserLedger)
	// 按用户的请求限制（任务 7.4）
	handlePermissionRoute(users, http.MethodGet, "/:id/limits", authz.AdtyUserRead, controller.GetAdtyUserLimits)
	handlePermissionRoute(users, http.MethodPut, "/:id/limits", authz.AdtyUserWrite, controller.PutAdtyUserLimits)
	registerAdtyWorkspaceRoutes(apiRouter)
}

func registerAdtyWorkspaceRoutes(apiRouter *gin.RouterGroup) {
	for _, route := range []struct{ method, path, scope string }{
		{http.MethodPost, "/api/adty/teams/:id/close", "wallet:write"},
		{http.MethodPost, "/api/adty/teams/:id/profile", "wallet:write"},
		{http.MethodPost, "/api/adty/teams/:id/leave", "wallet:write"},
		{http.MethodPost, "/api/adty/teams/:id/members/:userId/disable", "wallet:write"},
		{http.MethodPost, "/api/adty/teams/:id/keys", "wallet:write"},
		{http.MethodPost, "/api/adty/teams/:id/keys/:tokenId/revoke", "wallet:write"},
		{http.MethodPost, "/api/adty/teams/:id/transfer", "wallet:write"},
		{http.MethodGet, "/api/adty/telegram/me", "profile:read"},
		{http.MethodPost, "/api/adty/telegram/bind/start", "profile:write"},
		{http.MethodPost, "/api/adty/telegram/bind/confirm", "profile:write"},
		{http.MethodGet, "/api/adty/site/telegram", "wallet:read"},
		{http.MethodPost, "/api/adty/site/telegram/bind/start", "wallet:write"},
		{http.MethodPost, "/api/adty/site/telegram/bind/confirm", "wallet:write"},
		{http.MethodGet, "/api/adty/admin/telegram/bots", "option:read"},
		{http.MethodPut, "/api/adty/admin/telegram/bots/:purpose", "option:write"},
		{http.MethodPost, "/api/adty/admin/telegram/bots/:purpose/test", "option:write"},
		{http.MethodPost, "/api/adty/telegram/webhook/:purpose/:secret", "session"},
		{http.MethodGet, "/api/adty/tickets", "profile:read"},
		{http.MethodPost, "/api/adty/tickets", "profile:write"},
		{http.MethodGet, "/api/adty/tickets/:id", "profile:read"},
		{http.MethodPost, "/api/adty/tickets/:id/replies", "profile:write"},
		{http.MethodPost, "/api/adty/tickets/:id/notes", "profile:write"},
		{http.MethodPost, "/api/adty/tickets/:id/status", "profile:write"},
		{http.MethodPost, "/api/adty/tickets/:id/images", "profile:write"},
		{http.MethodGet, "/api/adty/tickets/:id/images/:imageId", "profile:read"},
		{http.MethodGet, "/api/adty/admin/translation", "option:read"},
		{http.MethodPut, "/api/adty/admin/translation", "option:write"},
		{http.MethodPost, "/api/adty/admin/translation/test", "option:write"},
		{http.MethodPost, "/api/adty/admin/translation/run", "option:write"},
		{http.MethodGet, "/api/adty/admin/translation/usage", "option:read"},
		{http.MethodGet, "/api/adty/groups/health", "api_key:read"},
		{http.MethodPut, "/api/adty/admin/group-health", "option:write"},
		{http.MethodPost, "/api/adty/admin/group-health/probe", "option:write"},
	} {
		kind := "scope"
		scope := route.scope
		if route.scope == "session" {
			kind, scope = "session", ""
		}
		middleware.DeclareAdtyAccessTokenRoute(route.method, route.path, kind, scope)
	}
	apiRouter.POST("/adty/teams/:id/close", middleware.UserAuth(), controller.PostAdtyTeamClose)
	apiRouter.POST("/adty/teams/:id/profile", middleware.UserAuth(), controller.PostAdtyTeamProfile)
	apiRouter.POST("/adty/teams/:id/leave", middleware.UserAuth(), controller.PostAdtyTeamLeave)
	apiRouter.POST("/adty/teams/:id/members/:userId/disable", middleware.UserAuth(), controller.PostAdtyTeamDisableMember)
	apiRouter.POST("/adty/teams/:id/keys", middleware.UserAuth(), controller.PostAdtyTeamBindKey)
	apiRouter.POST("/adty/teams/:id/keys/:tokenId/revoke", middleware.UserAuth(), controller.PostAdtyTeamRevokeKey)
	apiRouter.POST("/adty/teams/:id/transfer", middleware.UserAuth(), controller.PostAdtyTeamTransfer)
	apiRouter.GET("/adty/telegram/me", middleware.UserAuth(), controller.GetAdtyTelegramMe)
	apiRouter.POST("/adty/telegram/bind/start", middleware.UserAuth(), controller.PostAdtyTelegramBindStart)
	apiRouter.POST("/adty/telegram/bind/confirm", middleware.UserAuth(), controller.PostAdtyTelegramBindConfirm)
	apiRouter.GET("/adty/site/telegram", middleware.UserAuth(), controller.GetAdtyTelegramMe)
	apiRouter.POST("/adty/site/telegram/bind/start", middleware.UserAuth(), controller.PostAdtyTelegramBindStart)
	apiRouter.POST("/adty/site/telegram/bind/confirm", middleware.UserAuth(), controller.PostAdtyTelegramBindConfirm)
	apiRouter.POST("/adty/telegram/webhook/:purpose/:secret", middleware.CriticalRateLimit(), controller.PostAdtyTelegramWebhook)
	root := apiRouter.Group("/adty/admin", middleware.RootAuth())
	root.GET("/telegram/bots", controller.GetAdtyAdminTelegram)
	root.PUT("/telegram/bots/:purpose", controller.PutAdtyAdminTelegram)
	root.POST("/telegram/bots/:purpose/test", controller.PostAdtyAdminTelegramTest)
	root.GET("/translation", controller.GetAdtyTranslation)
	root.PUT("/translation", controller.PutAdtyTranslation)
	root.POST("/translation/test", controller.PostAdtyTranslationTest)
	root.POST("/translation/run", controller.PostAdtyTranslationRun)
	root.GET("/translation/usage", controller.GetAdtyTranslationUsage)
	root.PUT("/group-health", controller.PutAdtyGroupHealth)
	root.POST("/group-health/probe", controller.PostAdtyGroupProbe)
	apiRouter.GET("/adty/tickets", middleware.UserAuth(), controller.GetAdtyTickets)
	apiRouter.POST("/adty/tickets", middleware.UserAuth(), controller.PostAdtyTicket)
	apiRouter.GET("/adty/tickets/:id", middleware.UserAuth(), controller.GetAdtyTicket)
	apiRouter.POST("/adty/tickets/:id/replies", middleware.UserAuth(), controller.PostAdtyTicketReply)
	apiRouter.POST("/adty/tickets/:id/notes", middleware.UserAuth(), controller.PostAdtyTicketNote)
	apiRouter.POST("/adty/tickets/:id/status", middleware.UserAuth(), controller.PostAdtyTicketStatus)
	apiRouter.POST("/adty/tickets/:id/images", middleware.UserAuth(), controller.PostAdtyTicketImage)
	apiRouter.GET("/adty/tickets/:id/images/:imageId", middleware.UserAuth(), controller.GetAdtyTicketImage)
	apiRouter.GET("/adty/groups/health", middleware.UserAuth(), controller.GetAdtyGroupHealth)
}

// registerAdtyTopUpRoutes 挂钱包的充值接口（任务 3.3），令牌规则照上游钱包接口。
func registerAdtyTopUpRoutes(apiRouter *gin.RouterGroup) {
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/admin/payment-scenes", "scope", "option:read")
	middleware.DeclareAdtyAccessTokenRoute("PUT", "/api/adty/admin/payment-scenes", "scope", "option:write")
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/admin/fund-anomalies", "scope", "option:read")
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/admin/fund-anomalies/dispositions", "scope", "option:read")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/admin/fund-anomalies/dispositions/:id/review", "scope", "option:write")
	apiRouter.GET("/adty/admin/fund-anomalies/dispositions", middleware.RootAuth(), controller.GetAdtyAnomalyDispositions)
	apiRouter.POST("/adty/admin/fund-anomalies/dispositions/:id/review", middleware.RootAuth(), controller.PostAdtyAnomalyDispositionReview)
	apiRouter.GET("/adty/admin/fund-anomalies", middleware.RootAuth(), controller.GetAdtyFundAnomalies)
	middleware.DeclareAdtyAccessTokenRoute("PUT", "/api/adty/admin/site-withdrawals/rules", "scope", "option:write")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/admin/site-withdrawals/:id/paying", "scope", "option:write")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/admin/site-withdrawals/:id/reject", "scope", "option:write")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/admin/site-withdrawals/:id/paid", "scope", "option:write")
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/admin/site-referral/cap", "scope", "option:read")
	middleware.DeclareAdtyAccessTokenRoute("PUT", "/api/adty/admin/site-referral/cap", "scope", "option:write")
	middleware.DeclareAdtyAccessTokenRoute("PUT", "/api/adty/admin/teams/:id/agreement-price", "scope", "option:write")
	apiRouter.PUT("/adty/admin/teams/:id/agreement-price", middleware.RootAuth(), controller.PutAdtyMainTeamAgreementPrice)
	apiRouter.GET("/adty/admin/site-referral/cap", middleware.RootAuth(), controller.GetAdtySiteReferralCap)
	apiRouter.PUT("/adty/admin/site-referral/cap", middleware.RootAuth(), controller.PutAdtySiteReferralCap)
	apiRouter.PUT("/adty/admin/site-withdrawals/rules", middleware.RootAuth(), controller.PutAdtySiteWithdrawRules)
	apiRouter.POST("/adty/admin/site-withdrawals/:id/paying", middleware.RootAuth(), controller.PostAdtySiteWithdrawalPaying)
	apiRouter.POST("/adty/admin/site-withdrawals/:id/reject", middleware.RootAuth(), controller.PostAdtySiteWithdrawalReject)
	apiRouter.POST("/adty/admin/site-withdrawals/:id/paid", middleware.RootAuth(), controller.PostAdtySiteWithdrawalPaid)
	apiRouter.GET("/adty/admin/payment-scenes", middleware.RootAuth(), controller.GetAdtyPaymentScenes)
	apiRouter.PUT("/adty/admin/payment-scenes", middleware.RootAuth(), controller.PutAdtyPaymentScenes)
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/topup/info", "scope", "wallet:read")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/topup/quote", "scope", "wallet:read")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/topup/hosted/pay", "scope", "wallet:write")
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/topup/orders/:trade_no", "scope", "wallet:read")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/topup/orders/:trade_no/cancel", "scope", "wallet:write")
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/topup/pending", "scope", "wallet:read")
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/topup/self", "scope", "wallet:read")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/vouchers/redeem", "scope", "wallet:write")
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/teams/mine", "scope", "wallet:read")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/teams", "scope", "wallet:write")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/teams/:id/invites", "scope", "wallet:write")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/teams/accept", "scope", "wallet:write")

	topup := apiRouter.Group("/adty/topup", middleware.UserAuth(), middleware.AdtyRequireCurrentSiteTerms())
	topup.GET("/info", controller.GetAdtyTopUpInfo)
	topup.POST("/quote", middleware.PromoCodeUserRateLimit("CT:promo-quote"), controller.PostAdtyTopUpQuote)
	topup.POST("/hosted/pay", middleware.UserCriticalRateLimit("CT:pay-hosted"), controller.PostAdtyHostedPayment)
	topup.GET("/orders/:trade_no", controller.GetAdtyTopUpOrder)
	topup.POST("/orders/:trade_no/cancel", middleware.CriticalRateLimit(), controller.CancelAdtyTopUpOrder)
	topup.GET("/pending", controller.GetAdtyTopUpPending)
	topup.GET("/self", controller.GetAdtyTopUpSelf)
	apiRouter.POST("/adty/vouchers/redeem", middleware.UserAuth(), middleware.UserCriticalRateLimit("CT:voucher-redeem"), controller.PostAdtyVoucherRedeem)
	apiRouter.GET("/adty/teams/mine", middleware.UserAuth(), controller.GetAdtyTeamMine)
	apiRouter.POST("/adty/teams", middleware.UserAuth(), controller.PostAdtyTeam)
	apiRouter.POST("/adty/teams/:id/invites", middleware.UserAuth(), controller.PostAdtyTeamInvite)
	apiRouter.POST("/adty/teams/accept", middleware.UserAuth(), controller.PostAdtyTeamAccept)
}

// registerAdtyFinanceRoutes 挂充值对账、补单和退款（任务 3.5），按后台权限注册。
func registerAdtyFinanceRoutes(apiRouter *gin.RouterGroup) {
	finance := apiRouter.Group("/user/admin/finance", middleware.AdminAuth())
	handlePermissionRoute(finance, http.MethodGet, "/recharge-audit", authz.AdtyFinanceRead, controller.GetRechargeAudit)
	handlePermissionRoute(finance, http.MethodGet, "/recharge-audit/summary", authz.AdtyFinanceRead, controller.GetRechargeAuditSummary)
	handlePermissionRoute(finance, http.MethodGet, "/recharge-audit/:trade_no", authz.AdtyFinanceRead, controller.GetRechargeAuditOrder)
	handlePermissionRoute(finance, http.MethodGet, "/payment-orphans", authz.AdtyFinanceRead, controller.GetPaymentOrphans)
	handlePermissionRoute(finance, http.MethodPost, "/payment-orphans/:id/credit", authz.AdtyFinanceManualCredit, controller.CreditPaymentOrphan)
	handlePermissionRoute(finance, http.MethodPost, "/payment-orphans/:id/resolve", authz.AdtyFinanceManualCredit, controller.ResolvePaymentOrphan)
	handlePermissionRoute(finance, http.MethodGet, "/site-earning-reviews", authz.AdtyFinanceRead, controller.GetAdtySiteEarningReviews)
	handlePermissionRoute(finance, http.MethodPost, "/site-earning-reviews/resolve", authz.AdtyFinanceManualCredit, controller.ReviewAdtySiteEarning)

	topup := apiRouter.Group("/adty/admin/topup", middleware.AdminAuth())
	handlePermissionRoute(topup, http.MethodPost, "/:trade_no/query", authz.AdtyFinanceManualCredit, controller.QueryAdtyTopUp)
	handlePermissionRoute(topup, http.MethodPost, "/:trade_no/complete", authz.AdtyFinanceManualCredit, controller.ManualCompleteAdtyTopUp)
	handlePermissionRoute(topup, http.MethodGet, "/:trade_no/refund-preview", authz.AdtyFinanceRefund, controller.PreviewAdtyTopUpRefund)
	handlePermissionRoute(topup, http.MethodPost, "/:trade_no/refund", authz.AdtyFinanceRefund, controller.RefundAdtyTopUp)
}

// registerAdtyBEpusdtRoutes 挂 USDT 下单和回调（任务 4.2）。BEpusdt 只发 POST JSON 回调，不注册 GET。
func registerAdtyBEpusdtRoutes(apiRouter *gin.RouterGroup) {
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/user/bepusdt/pay", "scope", "wallet:write")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/user/bepusdt/notify", "session", "")
	apiRouter.POST("/user/bepusdt/pay", middleware.UserAuth(), middleware.AdtyRequireCurrentSiteTerms(), middleware.UserCriticalRateLimit("CT:pay-bepusdt"), controller.RequestBEpusdtPay)
	apiRouter.POST("/user/bepusdt/notify", middleware.AnonymousRequestBodyLimit(), controller.BEpusdtTopUpNotify)
}

// registerAdtyEpayRoutes 挂易支付多通道（任务 5.2、5.4）。静态的 /api/user/epay/notify 是上游的，和这里不冲突。
func registerAdtyEpayRoutes(apiRouter *gin.RouterGroup) {
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/epay/pay", "scope", "wallet:write")
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/user/epay/notify/:channelId", "session", "")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/user/epay/notify/:channelId", "session", "")
	apiRouter.POST("/adty/epay/pay", middleware.UserAuth(), middleware.AdtyRequireCurrentSiteTerms(), middleware.UserCriticalRateLimit("CT:pay-epay"), controller.RequestAdtyEpayPay)
	apiRouter.GET("/user/epay/notify/:channelId", controller.AdtyEpayNotify)
	apiRouter.POST("/user/epay/notify/:channelId", middleware.AnonymousRequestBodyLimit(), controller.AdtyEpayNotify)

	admin := apiRouter.Group("/adty/admin/epay", middleware.AdminAuth())
	handlePermissionRoute(admin, http.MethodGet, "/channels", authz.AdtyPaymentRead, controller.GetAdtyEpayChannels)
	handlePermissionRoute(admin, http.MethodPost, "/channels/:id/status", authz.AdtyPaymentOperate, controller.SetAdtyEpayChannelStatus)
	handlePermissionRoute(admin, http.MethodPost, "/channels/:id/test", authz.AdtyPaymentOperate, controller.TestAdtyEpayChannel)

	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/admin/epay/channels", "scope", "option:write")
	middleware.DeclareAdtyAccessTokenRoute("PUT", "/api/adty/admin/epay/channels/:id", "scope", "option:write")
	middleware.DeclareAdtyAccessTokenRoute("DELETE", "/api/adty/admin/epay/channels/:id", "scope", "option:write")
	root := apiRouter.Group("/adty/admin/epay", middleware.RootAuth())
	root.POST("/channels", controller.CreateAdtyEpayChannel)
	root.PUT("/channels/:id", controller.UpdateAdtyEpayChannel)
	root.DELETE("/channels/:id", controller.DeleteAdtyEpayChannel)
}

// registerAdtyReferralRoutes 挂推广返佣（P6）。用户侧接口读 wallet:read、写 wallet:write；后台按权限注册。
func registerAdtyReferralRoutes(apiRouter *gin.RouterGroup) {
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/referral/click", "session", "")
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/referral/public-settings", "any", "")
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/referral-assets/*path", "session", "")
	apiRouter.POST("/adty/referral/click", middleware.CriticalRateLimit(), controller.PostAdtyReferralClick)
	apiRouter.GET("/adty/referral/public-settings", controller.GetAdtyReferralPublicSettings)
	apiRouter.GET("/referral-assets/*path", controller.GetReferralAsset)

	user := apiRouter.Group("/user/referral", middleware.UserAuth())
	for _, route := range []struct {
		method, path, scope string
		handler             gin.HandlerFunc
	}{
		{http.MethodGet, "/profile", "wallet:read", controller.GetReferralProfile},
		{http.MethodPost, "/apply", "wallet:write", controller.ApplyReferralPartner},
		{http.MethodGet, "/commissions", "wallet:read", controller.GetReferralCommissions},
		{http.MethodGet, "/withdrawals", "wallet:read", controller.GetReferralWithdrawals},
		{http.MethodPost, "/withdrawals", "wallet:write", controller.CreateReferralWithdrawal},
		{http.MethodPost, "/withdrawals/:id/cancel", "wallet:write", controller.CancelReferralWithdrawal},
		{http.MethodPost, "/upload", "wallet:write", controller.UploadReferralAsset},
	} {
		middleware.DeclareAdtyAccessTokenRoute(route.method, "/api/user/referral"+route.path, "scope", route.scope)
		user.Handle(route.method, route.path, route.handler)
	}

	admin := apiRouter.Group("/user/admin/referral", middleware.AdminAuth())
	for _, route := range []struct {
		method, path string
		permission   authz.Permission
		handler      gin.HandlerFunc
	}{
		{http.MethodGet, "/overview", authz.AdtyReferralRead, controller.GetReferralOverview},
		{http.MethodGet, "/badges", authz.AdtyReferralRead, controller.GetReferralAdminBadges},
		{http.MethodGet, "/affiliates", authz.AdtyReferralRead, controller.GetReferralAffiliates},
		{http.MethodGet, "/affiliates/:user_id/bindings", authz.AdtyReferralRead, controller.GetReferralBindings},
		{http.MethodGet, "/commissions", authz.AdtyReferralRead, controller.GetAdminReferralCommissions},
		{http.MethodGet, "/commission-jobs", authz.AdtyReferralRead, controller.GetReferralCommissionJobs},
		{http.MethodGet, "/ledgers", authz.AdtyReferralRead, controller.GetReferralLedgers},
		{http.MethodGet, "/audit-logs", authz.AdtyReferralRead, controller.GetReferralAdminAuditLogs},
		{http.MethodGet, "/withdrawals", authz.AdtyReferralRead, controller.GetAdminReferralWithdrawals},
		{http.MethodPost, "/affiliates/:user_id/partner", authz.AdtyReferralReview, controller.ReviewReferralPartner},
		{http.MethodPost, "/withdrawals/:id/review", authz.AdtyReferralReview, controller.ReviewReferralWithdrawal},
		{http.MethodPost, "/withdrawals/:id/pay", authz.AdtyReferralPayout, controller.PayReferralWithdrawal},
		{http.MethodPost, "/upload", authz.AdtyReferralPayout, controller.UploadReferralAsset},
		{http.MethodPost, "/affiliates/:user_id/update", authz.AdtyReferralAdjust, controller.UpdateReferralAffiliate},
		{http.MethodPost, "/commission-jobs/retry", authz.AdtyReferralAdjust, controller.RetryReferralCommissionJob},
		{http.MethodPost, "/settlements/run", authz.AdtyReferralAdjust, controller.RunReferralSettlement},
		{http.MethodGet, "/settings", authz.AdtyReferralSettingsRead, controller.GetReferralSettings},
		{http.MethodPut, "/settings", authz.AdtyReferralSettings, controller.UpdateReferralSettings},
	} {
		handlePermissionRoute(admin, route.method, route.path, route.permission, route.handler)
	}
}

// registerAdtyOfficialPriceRoutes 挂官方价（任务 8.4）。公开接口和 /api/pricing 同一个开关、同样登记 any；
// 后台三个接口只有超级管理员。*model 让带斜杠的模型名也能改。
func registerAdtyOfficialPriceRoutes(apiRouter *gin.RouterGroup) {
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/pricing/official", "any", "")
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/admin/official-prices", "scope", "option:read")
	middleware.DeclareAdtyAccessTokenRoute("PUT", "/api/adty/admin/official-prices/*model", "scope", "option:write")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/admin/official-prices/sync", "scope", "option:write")
	apiRouter.GET("/adty/pricing/official", middleware.HeaderNavModuleAuth("pricing"), controller.GetAdtyOfficialPrices)

	root := apiRouter.Group("/adty/admin/official-prices", middleware.RootAuth())
	root.GET("", controller.GetAdtyAdminOfficialPrices)
	root.PUT("/*model", controller.PutAdtyOfficialPrice)
	root.POST("/sync", controller.SyncAdtyOfficialPrices)
}

// registerAdtySiteRoutes 挂站点公开配置（任务 8.3）。
func registerAdtySiteRoutes(apiRouter *gin.RouterGroup) {
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/site", "any", "")
	apiRouter.GET("/adty/site", controller.GetAdtySite)
}

// registerAdtyLegalRoutes 挂多语言条款（任务 8.6），和上游 /api/user-agreement 一样公开。
func registerAdtyLegalRoutes(apiRouter *gin.RouterGroup) {
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/legal/:doc", "any", "")
	apiRouter.GET("/adty/legal/:doc", controller.GetAdtyLegal)
}

// registerAdtyPromoRoutes keeps paid top-up promo codes separate from retired marketing.
func registerAdtyPromoRoutes(apiRouter *gin.RouterGroup) {
	admin := apiRouter.Group("/adty/admin/promo-codes", middleware.AdminAuth())
	handlePermissionRoute(admin, http.MethodGet, "", authz.AdtyPaymentRead, controller.GetAdtyPromoCodes)
	for _, route := range []struct {
		method, path string
		handler      gin.HandlerFunc
	}{
		{http.MethodPost, "", controller.PostAdtyPromoCode},
		{http.MethodPut, "/:id", controller.PutAdtyPromoCode},
	} {
		middleware.DeclareAdtyAccessTokenRoute(route.method, "/api/adty/admin/promo-codes"+route.path, "scope", "option:write")
		admin.Handle(route.method, route.path, middleware.RootAuth(), route.handler)
	}
}

// registerAdtyResellerRoutes 挂站长分站（P11）。后台接口按「分站」权限注册（任务 11.9）；
// 站长侧接口读 wallet:read、写 wallet:write（任务 11.1、11.6）。
func registerAdtyResellerRoutes(apiRouter *gin.RouterGroup) {
	owner := apiRouter.Group("/adty/site", middleware.UserAuth())
	for _, route := range []struct {
		method, path, scope string
		handlers            []gin.HandlerFunc
	}{
		{http.MethodPost, "/apply", "wallet:write", []gin.HandlerFunc{middleware.UserCriticalRateLimit("CT:site-apply"), controller.ApplyAdtySite}},
		{http.MethodPost, "/close", "wallet:write", []gin.HandlerFunc{controller.RequestAdtySiteClose}},
		// 改价、改加赠、送试用、删域名要二次验证（任务 11.6）
		{http.MethodPut, "/pricing", "wallet:write", []gin.HandlerFunc{controller.PutAdtySitePricingWithProof}},
		{http.MethodPut, "/customers/:id/pricing", "wallet:write", []gin.HandlerFunc{controller.PutAdtySiteCustomerPricingWithProof}},
		// 自定义域名（任务 11.8）
		{http.MethodGet, "/domains", "wallet:read", []gin.HandlerFunc{controller.GetAdtySiteDomains}},
		{http.MethodPost, "/domains", "wallet:write", []gin.HandlerFunc{middleware.UserCriticalRateLimit("CT:site-domain"), controller.AddAdtySiteDomain}},
		{http.MethodDelete, "/domains/:id", "wallet:write", []gin.HandlerFunc{controller.RemoveAdtySiteDomainWithProof}},
		// 站长出资的优惠码（任务 12.4）
		{http.MethodGet, "/promo-codes", "wallet:read", []gin.HandlerFunc{controller.GetAdtySitePromoCodes}},
		{http.MethodPost, "/promo-codes", "wallet:write", []gin.HandlerFunc{controller.PostAdtySitePromoCode}},
		{http.MethodPut, "/promo-codes/:id", "wallet:write", []gin.HandlerFunc{controller.PutAdtySitePromoCode}},
		{http.MethodDelete, "/promo-codes/:id", "wallet:write", []gin.HandlerFunc{controller.DeleteAdtySitePromoCode}},
		// 站长中心（任务 11.6、11.7）。接受协议不走站长中心的协议门。
		{http.MethodPost, "/terms/accept", "profile:write", []gin.HandlerFunc{controller.AcceptAdtySiteTerms}},
		{http.MethodGet, "/self", "wallet:read", []gin.HandlerFunc{controller.GetAdtyOwnerSite}},
		{http.MethodPut, "/self", "wallet:write", []gin.HandlerFunc{controller.PutAdtyOwnerSite}},
		{http.MethodGet, "/stats", "wallet:read", []gin.HandlerFunc{controller.GetAdtyOwnerStats}},
		{http.MethodGet, "/earnings", "wallet:read", []gin.HandlerFunc{controller.GetAdtyOwnerEarnings}},
		{http.MethodPost, "/vouchers", "wallet:write", []gin.HandlerFunc{controller.PostAdtySiteVoucher}},
		{http.MethodPost, "/vouchers/:id/cancel", "wallet:write", []gin.HandlerFunc{controller.PostAdtyVoucherCancel}},
		{http.MethodPost, "/vouchers/:id/freeze", "wallet:write", []gin.HandlerFunc{controller.PostAdtyVoucherFreeze}},
		{http.MethodPut, "/bonus-methods", "wallet:write", []gin.HandlerFunc{controller.PutAdtySiteMethodBonus}},
		{http.MethodGet, "/wallets", "wallet:read", []gin.HandlerFunc{controller.GetAdtyOwnerWallets}},
		{http.MethodPost, "/wallets/earning-to-margin", "wallet:write", []gin.HandlerFunc{controller.PostAdtyEarningToMargin}},
		{http.MethodGet, "/withdrawals/rules", "wallet:read", []gin.HandlerFunc{controller.GetAdtySiteWithdrawRules}},
		{http.MethodGet, "/withdrawals", "wallet:read", []gin.HandlerFunc{controller.GetAdtySiteWithdrawals}},
		{http.MethodPost, "/withdrawals", "wallet:write", []gin.HandlerFunc{controller.PostAdtySiteWithdrawal}},
		{http.MethodPost, "/withdrawals/:id/cancel", "wallet:write", []gin.HandlerFunc{controller.PostAdtySiteWithdrawalCancel}},
		{http.MethodGet, "/wallet-ledgers", "wallet:read", []gin.HandlerFunc{controller.GetAdtyOwnerWalletLedger}},
		{http.MethodGet, "/orders", "wallet:read", []gin.HandlerFunc{controller.GetAdtyOwnerOrders}},
		{http.MethodGet, "/orders/summary", "wallet:read", []gin.HandlerFunc{controller.GetAdtyOwnerOrderSummary}},
		{http.MethodGet, "/orders/:trade_no", "wallet:read", []gin.HandlerFunc{controller.GetAdtyOwnerOrder}},
		{http.MethodGet, "/teams", "wallet:read", []gin.HandlerFunc{controller.GetAdtyOwnerSiteTeams}},
		{http.MethodPut, "/teams/:id/discount", "wallet:write", []gin.HandlerFunc{controller.PutAdtyOwnerSiteTeamDiscount}},
		{http.MethodGet, "/referral", "wallet:read", []gin.HandlerFunc{controller.GetAdtyOwnerSiteReferral}},
		{http.MethodPut, "/referral", "wallet:write", []gin.HandlerFunc{controller.PutAdtyOwnerSiteReferral}},
		{http.MethodGet, "/referral/invites", "wallet:read", []gin.HandlerFunc{controller.GetAdtyOwnerSiteInvites}},
		{http.MethodPost, "/referral/invites", "wallet:write", []gin.HandlerFunc{controller.PostAdtyOwnerSiteInvite}},
		{http.MethodGet, "/customers", "wallet:read", []gin.HandlerFunc{controller.GetAdtyOwnerCustomers}},
		{http.MethodGet, "/customers/lookup", "wallet:read", []gin.HandlerFunc{controller.LookupAdtyOwnerCustomer}},
		{http.MethodGet, "/customers/:id", "wallet:read", []gin.HandlerFunc{controller.GetAdtyOwnerCustomer}},
		{http.MethodGet, "/customers/:id/logs", "wallet:read", []gin.HandlerFunc{controller.GetAdtyOwnerCustomerLogs}},
		{http.MethodGet, "/customers/:id/topups", "wallet:read", []gin.HandlerFunc{controller.GetAdtyOwnerCustomerTopUps}},
		{http.MethodPost, "/customers/:id/trial", "wallet:write", []gin.HandlerFunc{middleware.UserCriticalRateLimit("CT:site-trial"), controller.PostAdtyOwnerTrial}},
		{http.MethodPost, "/report", "wallet:write", []gin.HandlerFunc{middleware.UserCriticalRateLimit("CT:site-report"), controller.PostAdtyOwnerReport}},
		{http.MethodGet, "/export", "wallet:read", []gin.HandlerFunc{controller.GetAdtyOwnerExport}},
		{http.MethodPost, "/logo", "wallet:write", []gin.HandlerFunc{middleware.UserCriticalRateLimit("CT:site-logo"), controller.UploadAdtySiteLogo}},
		// 站长发件域名（任务 11.3）
		{http.MethodGet, "/sender-domain", "wallet:read", []gin.HandlerFunc{controller.GetAdtySiteSenderDomain}},
		{http.MethodPost, "/sender-domain", "wallet:write", []gin.HandlerFunc{middleware.UserCriticalRateLimit("CT:site-sender"), controller.AddAdtySiteSenderDomain}},
		{http.MethodDelete, "/sender-domain", "wallet:write", []gin.HandlerFunc{controller.RemoveAdtySiteSenderDomainWithProof}},
	} {
		middleware.DeclareAdtyAccessTokenRoute(route.method, "/api/adty/site"+route.path, "scope", route.scope)
		owner.Handle(route.method, route.path, route.handlers...)
	}

	// Employee administration and workspace require live browser sessions.
	for _, route := range []struct {
		method, path string
		handler      gin.HandlerFunc
	}{
		{http.MethodGet, "/adty/site/staff", controller.GetAdtyOwnerStaff},
		{http.MethodGet, "/adty/site/staff/audits", controller.GetAdtyOwnerStaffAudits},
		{http.MethodPost, "/adty/site/staff", controller.PostAdtyOwnerStaff},
		{http.MethodPut, "/adty/site/staff/:id", controller.PutAdtyOwnerStaff},
		{http.MethodPut, "/adty/site/staff/:id/password", controller.PutAdtyOwnerStaffPassword},
		{http.MethodDelete, "/adty/site/staff/:id", controller.DeleteAdtyOwnerStaff},
		{http.MethodGet, "/adty/staff/self", controller.GetAdtyStaffSelf},
		{http.MethodGet, "/adty/staff/customers", controller.GetAdtyStaffCustomers},
		{http.MethodGet, "/adty/staff/customers/:id", controller.GetAdtyStaffCustomer},
		{http.MethodGet, "/adty/staff/customers/:id/topups", controller.GetAdtyStaffCustomerTopUps},
		{http.MethodGet, "/adty/staff/customers/:id/logs", controller.GetAdtyStaffCustomerLogs},
		{http.MethodPost, "/adty/staff/customers/:id/password-reset", controller.PostAdtyStaffCustomerPasswordReset},
		{http.MethodPost, "/adty/staff/customers/:id/disable", controller.PostAdtyStaffCustomerDisable},
		{http.MethodGet, "/adty/staff/balance-requests", controller.GetAdtyStaffBalanceRequests},
		{http.MethodPost, "/adty/staff/balance-requests", controller.PostAdtyStaffBalanceRequest},
		{http.MethodGet, "/adty/site/balance-requests", controller.GetAdtyOwnerBalanceRequests},
		{http.MethodPost, "/adty/site/balance-requests/:id/review", controller.PostAdtyOwnerBalanceReview},
		{http.MethodGet, "/adty/admin/balance-requests", controller.GetAdtyAdminBalanceRequests},
		{http.MethodPost, "/adty/admin/balance-requests/:id/review", controller.PostAdtyAdminBalanceReview},
	} {
		middleware.DeclareAdtyAccessTokenRoute(route.method, "/api"+route.path, "session", "")
		handlers := []gin.HandlerFunc{middleware.UserAuth(), middleware.DisableCache()}
		if route.method == http.MethodPost && (route.path == "/adty/staff/customers/:id/password-reset" || route.path == "/adty/staff/customers/:id/disable") {
			handlers = append(handlers, middleware.UserCriticalRateLimit("CT:staff-customer-security"))
		}
		apiRouter.Handle(route.method, route.path, append(handlers, route.handler)...)
	}

	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/admin/reseller/tiers", "scope", "option:read")
	middleware.DeclareAdtyAccessTokenRoute("PUT", "/api/adty/admin/reseller/tiers", "scope", "option:write")
	root := apiRouter.Group("/adty/admin/reseller", middleware.RootAuth())
	root.GET("/tiers", controller.GetAdtySiteTiers)
	root.PUT("/tiers", controller.PutAdtySiteTiers)
	middleware.DeclareAdtyAccessTokenRoute("PUT", "/api/adty/admin/bonus-methods", "scope", "option:write")
	apiRouter.PUT("/adty/admin/bonus-methods", middleware.RootAuth(), controller.PutAdtyMainMethodBonus)
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/admin/sites/:id/wallets", "scope", "option:read")
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/admin/sites/:id/wallet-ledgers", "scope", "option:read")
	rootWallets := apiRouter.Group("/adty/admin/sites", middleware.RootAuth())
	middleware.DeclareAdtyAccessTokenRoute("PUT", "/api/adty/admin/sites/:id/bonus-methods", "scope", "option:write")
	rootWallets.PUT("/:id/bonus-methods", controller.PutAdtySiteBonusMethodAuth)
	rootWallets.GET("/:id/wallets", controller.GetAdtyAdminSiteWallets)
	rootWallets.GET("/:id/wallet-ledgers", controller.GetAdtyAdminSiteWalletLedger)
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/admin/sites", "scope", "option:write")
	rootWallets.POST("", controller.CreateAdtyAdminSite)
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/admin/sites/:id/review", "scope", "option:write")
	rootWallets.POST("/:id/review", controller.ReviewAdtyAdminSite)
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/admin/sites/:id/owner", "scope", "option:write")
	rootWallets.POST("/:id/owner", controller.HandoverAdtyAdminSiteOwner)
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/admin/vouchers", "scope", "option:write")
	middleware.DeclareAdtyAccessTokenRoute("PUT", "/api/adty/admin/vouchers/limit", "scope", "option:write")
	middleware.DeclareAdtyAccessTokenRoute("PUT", "/api/adty/admin/vouchers/sites/:id/margin", "scope", "option:write")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/admin/vouchers/:id/freeze", "scope", "option:write")
	rootVouchers := apiRouter.Group("/adty/admin/vouchers", middleware.RootAuth())
	rootVouchers.POST("", controller.PostAdtyAdminVoucher)
	rootVouchers.PUT("/limit", controller.PutAdtyVoucherIssuanceLimit)
	rootVouchers.PUT("/sites/:id/margin", controller.PutAdtyVoucherMarginAuthorization)
	rootVouchers.POST("/:id/freeze", controller.PostAdtyVoucherFreeze)

	admin := apiRouter.Group("/adty/admin", middleware.AdminAuth())
	for _, route := range []struct {
		method, path string
		permission   authz.Permission
		handler      gin.HandlerFunc
	}{
		{http.MethodGet, "/sites", authz.AdtySiteRead, controller.GetAdtyAdminSites},
		// 静态路径写在 /sites/:id 前面，gin 静态段优先。
		{http.MethodGet, "/sites/overview", authz.AdtySiteRead, controller.GetAdtyAdminSitesOverview},
		{http.MethodGet, "/sites/flagged-customers", authz.AdtySiteRead, controller.GetAdtyAdminFlaggedCustomers},
		{http.MethodGet, "/sites/reports", authz.AdtySiteRead, controller.GetAdtyAdminSiteReports},
		{http.MethodPost, "/sites/reports/:id/handle", authz.AdtySiteReview, controller.HandleAdtyAdminSiteReport},
		{http.MethodGet, "/sites/:id/stats", authz.AdtySiteRead, controller.GetAdtyAdminSiteStats},
		{http.MethodGet, "/sites/:id", authz.AdtySiteRead, controller.GetAdtyAdminSite},
		{http.MethodGet, "/sites/:id/customers", authz.AdtySiteRead, controller.GetAdtyAdminSiteCustomers},
		{http.MethodPost, "/sites/customers/:id/review", authz.AdtySiteReview, controller.ReviewAdtySiteCustomer},
		{http.MethodPut, "/sites/:id", authz.AdtySiteAdjust, controller.AdjustAdtyAdminSite},
		{http.MethodPut, "/sites/:id/status", authz.AdtySiteAdjust, controller.SetAdtyAdminSiteStatus},
		{http.MethodPost, "/sites/:id/settlement/resume", authz.AdtySiteAdjust, controller.ResumeAdtySiteSettlement},
		{http.MethodDelete, "/sites/:id/sender-domain", authz.AdtySiteDelete, controller.DisableAdtyAdminSiteSenderDomain},
		{http.MethodDelete, "/sites/domains/:id", authz.AdtySiteDelete, controller.DisableAdtyAdminSiteDomain},
		{http.MethodDelete, "/sites/:id/notice", authz.AdtySiteDelete, controller.ClearAdtyAdminSiteNotice},
		{http.MethodGet, "/reseller/usage", authz.AdtySiteRead, controller.GetAdtyResellerUsage},
		{http.MethodGet, "/reseller/settings", authz.AdtySiteSettingsRead, controller.GetAdtyResellerSettings},
		{http.MethodPut, "/reseller/settings", authz.AdtySiteSettings, controller.UpdateAdtyResellerSettings},
		{http.MethodGet, "/reseller/invite-codes", authz.AdtySiteSettingsRead, controller.GetAdtySiteInviteCodes},
		{http.MethodPost, "/reseller/invite-codes", authz.AdtySiteSettingsCreate, controller.CreateAdtySiteInviteCode},
		{http.MethodPut, "/reseller/invite-codes/:id", authz.AdtySiteSettings, controller.UpdateAdtySiteInviteCode},
		{http.MethodDelete, "/reseller/invite-codes/:id", authz.AdtySiteSettingsDelete, controller.DeleteAdtySiteInviteCode},
	} {
		handlePermissionRoute(admin, route.method, route.path, route.permission, route.handler)
	}
}

// SetAdtyWebRoutes 挂 /api 以外的二开页面（任务 11.3）：易支付付完跳回的 /pay/result/:token、按站点返回的 /robots.txt
// 和站长 Logo。
// 在 SetWebRouter 的 NoRoute 之前注册，所以它们不经过前端静态文件。
func SetAdtyWebRoutes(router *gin.Engine) {
	router.GET("/pay/result", controller.AdtyPayResult) // 不带签名的一律跳主站钱包
	router.GET("/pay/result/:token", controller.AdtyPayResult)
	router.GET("/robots.txt", AdtyRobotsTxt())
	router.GET("/sitemap.xml", AdtySitemap())
	// 站长上传的 Logo（任务 11.3）：公开读取，文件名是随机的。
	router.GET("/uploads/site-assets/:name", controller.GetAdtySiteAsset)
}
