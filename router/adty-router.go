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
	registerAdtyMarketingRoutes(apiRouter)
	registerAdtyResellerRoutes(apiRouter)

	// 余额流水（任务 7.2）
	users := apiRouter.Group("/adty/admin/users", middleware.AdminAuth())
	handlePermissionRoute(users, http.MethodGet, "/:id/ledger", authz.AdtyUserRead, controller.GetAdtyUserLedger)
	// 按用户的请求限制（任务 7.4）
	handlePermissionRoute(users, http.MethodGet, "/:id/limits", authz.AdtyUserRead, controller.GetAdtyUserLimits)
	handlePermissionRoute(users, http.MethodPut, "/:id/limits", authz.AdtyUserWrite, controller.PutAdtyUserLimits)
}

// registerAdtyTopUpRoutes 挂钱包的充值接口（任务 3.3），令牌规则照上游钱包接口。
func registerAdtyTopUpRoutes(apiRouter *gin.RouterGroup) {
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/topup/info", "scope", "wallet:read")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/topup/quote", "scope", "wallet:read")
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/topup/orders/:trade_no", "scope", "wallet:read")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/topup/orders/:trade_no/cancel", "scope", "wallet:write")
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/topup/pending", "scope", "wallet:read")
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/topup/self", "scope", "wallet:read")

	topup := apiRouter.Group("/adty/topup", middleware.UserAuth(), middleware.AdtyRequireCurrentSiteTerms())
	topup.GET("/info", controller.GetAdtyTopUpInfo)
	topup.POST("/quote", middleware.PromoCodeUserRateLimit("CT:promo-quote"), controller.PostAdtyTopUpQuote)
	topup.GET("/orders/:trade_no", controller.GetAdtyTopUpOrder)
	topup.POST("/orders/:trade_no/cancel", middleware.CriticalRateLimit(), controller.CancelAdtyTopUpOrder)
	topup.GET("/pending", controller.GetAdtyTopUpPending)
	topup.GET("/self", controller.GetAdtyTopUpSelf)
}

// registerAdtyFinanceRoutes 挂充值对账、补单和退款（任务 3.5），按后台权限注册。
func registerAdtyFinanceRoutes(apiRouter *gin.RouterGroup) {
	finance := apiRouter.Group("/user/admin/finance", middleware.AdminAuth())
	handlePermissionRoute(finance, http.MethodGet, "/recharge-audit", authz.AdtyFinanceRead, controller.GetRechargeAudit)
	handlePermissionRoute(finance, http.MethodGet, "/recharge-audit/summary", authz.AdtyFinanceRead, controller.GetRechargeAuditSummary)
	handlePermissionRoute(finance, http.MethodGet, "/payment-orphans", authz.AdtyFinanceRead, controller.GetPaymentOrphans)
	handlePermissionRoute(finance, http.MethodPost, "/payment-orphans/:id/credit", authz.AdtyFinanceManualCredit, controller.CreditPaymentOrphan)
	handlePermissionRoute(finance, http.MethodPost, "/payment-orphans/:id/resolve", authz.AdtyFinanceManualCredit, controller.ResolvePaymentOrphan)

	topup := apiRouter.Group("/adty/admin/topup", middleware.AdminAuth())
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
		{http.MethodGet, "/settings", authz.AdtyReferralSettings, controller.GetReferralSettings},
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

// registerAdtyMarketingRoutes 挂营销（P12）。webhook 是匿名回调，登记 session；后台按营销权限注册。
func registerAdtyMarketingRoutes(apiRouter *gin.RouterGroup) {
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/marketing/consent-default", "any", "")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/marketing/resend-webhook", "session", "")
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/marketing/unsubscribe", "session", "")
	middleware.DeclareAdtyAccessTokenRoute("POST", "/api/adty/marketing/unsubscribe", "session", "")
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/marketing/self", "scope", "profile:read")
	middleware.DeclareAdtyAccessTokenRoute("PUT", "/api/adty/marketing/self", "scope", "profile:write")
	apiRouter.GET("/adty/marketing/consent-default", controller.GetAdtyMarketingConsentDefault)
	apiRouter.POST("/adty/marketing/resend-webhook", controller.PostAdtyResendWebhook)
	// 自动邮件的退订链接（任务 12.3）：GET 显示确认页，POST 退订（含 RFC 8058 一键退订）
	apiRouter.GET("/adty/marketing/unsubscribe", middleware.CriticalRateLimit(), controller.GetAdtyMarketingUnsubscribe)
	apiRouter.POST("/adty/marketing/unsubscribe", middleware.CriticalRateLimit(), middleware.AnonymousRequestBodyLimit(), controller.PostAdtyMarketingUnsubscribe)
	self := apiRouter.Group("/adty/marketing/self", middleware.UserAuth())
	self.GET("", controller.GetAdtyMarketingSelf)
	self.PUT("", controller.PutAdtyMarketingSelf)

	admin := apiRouter.Group("/adty/admin/marketing", middleware.AdminAuth())
	handlePermissionRoute(admin, http.MethodGet, "/settings", authz.AdtyMarketingRead, controller.GetAdtyMarketingSettings)
	handlePermissionRoute(admin, http.MethodPut, "/settings", authz.AdtyMarketingWrite, controller.PutAdtyMarketingSettings)
	middleware.DeclareAdtyAccessTokenRoute("PUT", "/api/adty/admin/marketing/secrets", "scope", "option:write")
	apiRouter.PUT("/adty/admin/marketing/secrets", middleware.AdminAuth(), middleware.RootAuth(), controller.PutAdtyMarketingSecrets)
	// 群发（任务 12.2）
	handlePermissionRoute(admin, http.MethodGet, "/campaigns", authz.AdtyMarketingRead, controller.GetAdtyCampaigns)
	handlePermissionRoute(admin, http.MethodGet, "/campaigns/:id", authz.AdtyMarketingRead, controller.GetAdtyCampaign)
	handlePermissionRoute(admin, http.MethodGet, "/campaigns/:id/plan", authz.AdtyMarketingRead, controller.GetAdtyCampaignPlan)
	handlePermissionRoute(admin, http.MethodPost, "/audience/estimate", authz.AdtyMarketingRead, controller.PostAdtyAudienceEstimate)
	handlePermissionRoute(admin, http.MethodPost, "/campaigns", authz.AdtyMarketingWrite, controller.PostAdtyCampaign)
	handlePermissionRoute(admin, http.MethodPut, "/campaigns/:id", authz.AdtyMarketingWrite, controller.PutAdtyCampaign)
	handlePermissionRoute(admin, http.MethodDelete, "/campaigns/:id", authz.AdtyMarketingWrite, controller.DeleteAdtyCampaign)
	handlePermissionRoute(admin, http.MethodPost, "/campaigns/:id/test", authz.AdtyMarketingSend, middleware.CriticalRateLimit(), controller.PostAdtyCampaignTest)
	handlePermissionRoute(admin, http.MethodPost, "/campaigns/:id/send", authz.AdtyMarketingSend, controller.PostAdtySendCampaign)
	handlePermissionRoute(admin, http.MethodPost, "/campaigns/:id/cancel", authz.AdtyMarketingSend, controller.PostAdtyCancelCampaign)
	// 自动触发邮件（任务 12.3）
	handlePermissionRoute(admin, http.MethodGet, "/triggers", authz.AdtyMarketingRead, controller.GetAdtyMarketingTriggers)
	handlePermissionRoute(admin, http.MethodPut, "/triggers", authz.AdtyMarketingWrite, controller.PutAdtyMarketingTriggers)
	// 注册来源统计（任务 12.5）
	handlePermissionRoute(admin, http.MethodGet, "/sources", authz.AdtyMarketingRead, controller.GetAdtySignupSourceStats)
	// 优惠码（任务 12.4），平台出资
	handlePermissionRoute(admin, http.MethodGet, "/promo-codes", authz.AdtyMarketingRead, controller.GetAdtyPromoCodes)
	handlePermissionRoute(admin, http.MethodPost, "/promo-codes", authz.AdtyMarketingWrite, controller.PostAdtyPromoCode)
	handlePermissionRoute(admin, http.MethodPut, "/promo-codes/:id", authz.AdtyMarketingWrite, controller.PutAdtyPromoCode)
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
		{http.MethodPost, "/sites", authz.AdtySiteAdjust, controller.CreateAdtyAdminSite},
		{http.MethodPost, "/sites/:id/review", authz.AdtySiteReview, controller.ReviewAdtyAdminSite},
		{http.MethodPost, "/sites/customers/:id/review", authz.AdtySiteReview, controller.ReviewAdtySiteCustomer},
		{http.MethodPut, "/sites/:id", authz.AdtySiteAdjust, controller.AdjustAdtyAdminSite},
		{http.MethodPut, "/sites/:id/status", authz.AdtySiteAdjust, controller.SetAdtyAdminSiteStatus},
		{http.MethodPost, "/sites/:id/settlement/resume", authz.AdtySiteAdjust, controller.ResumeAdtySiteSettlement},
		{http.MethodDelete, "/sites/:id/sender-domain", authz.AdtySiteAdjust, controller.DisableAdtyAdminSiteSenderDomain},
		{http.MethodDelete, "/sites/domains/:id", authz.AdtySiteAdjust, controller.DisableAdtyAdminSiteDomain},
		{http.MethodDelete, "/sites/:id/notice", authz.AdtySiteAdjust, controller.ClearAdtyAdminSiteNotice},
		{http.MethodGet, "/reseller/usage", authz.AdtySiteRead, controller.GetAdtyResellerUsage},
		{http.MethodGet, "/reseller/invite-codes", authz.AdtySiteSettings, controller.GetAdtySiteInviteCodes},
		{http.MethodPost, "/reseller/invite-codes", authz.AdtySiteSettings, controller.CreateAdtySiteInviteCode},
		{http.MethodPut, "/reseller/invite-codes/:id", authz.AdtySiteSettings, controller.UpdateAdtySiteInviteCode},
		{http.MethodDelete, "/reseller/invite-codes/:id", authz.AdtySiteSettings, controller.DeleteAdtySiteInviteCode},
	} {
		handlePermissionRoute(admin, route.method, route.path, route.permission, route.handler)
	}
	// 分站资金参数和 Cloudflare 令牌只给超级管理员（任务 11.9）。
	middleware.DeclareAdtyAccessTokenRoute("GET", "/api/adty/admin/reseller/settings", "scope", "option:read")
	middleware.DeclareAdtyAccessTokenRoute("PUT", "/api/adty/admin/reseller/settings", "scope", "option:write")
	root := apiRouter.Group("/adty/admin/reseller", middleware.AdminAuth(), middleware.RootAuth())
	root.GET("/settings", controller.GetAdtyResellerSettings)
	root.PUT("/settings", controller.UpdateAdtyResellerSettings)
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
