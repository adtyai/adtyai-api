package router

import (
	"net/http"

	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/service/authz"

	// Import oauth package to register providers via init()
	_ "github.com/QuantumNous/new-api/oauth"

	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func SetApiRouter(router *gin.Engine) {
	apiRouter := router.Group("/api")
	apiRouter.Use(middleware.RouteTag("api"))
	apiRouter.Use(gzip.Gzip(gzip.DefaultCompression))
	apiRouter.Use(middleware.AccessTokenAudit())
	apiRouter.Use(middleware.BodyStorageCleanup()) // 清理请求体存储
	apiRouter.Use(middleware.GlobalAPIRateLimit())
	anonymousRequestBodyLimit := middleware.AnonymousRequestBodyLimit()
	{
		apiRouter.GET("/setup", controller.GetSetup)
		apiRouter.POST("/setup", anonymousRequestBodyLimit, controller.PostSetup)
		apiRouter.GET("/status", controller.GetStatus)
		apiRouter.GET("/uptime/status", controller.GetUptimeKumaStatus)
		apiRouter.GET("/models", middleware.UserAuth(), controller.DashboardListModels)
		apiRouter.GET("/status/test", middleware.AdminAuth(), controller.TestStatus)
		apiRouter.GET("/notice", controller.GetNotice)
		apiRouter.GET("/user-agreement", controller.GetUserAgreement)
		apiRouter.GET("/privacy-policy", controller.GetPrivacyPolicy)
		apiRouter.GET("/about", controller.GetAbout)
		//apiRouter.GET("/midjourney", controller.GetMidjourney)
		apiRouter.GET("/home_page_content", controller.GetHomePageContent)
		apiRouter.GET("/pricing", middleware.HeaderNavModuleAuth("pricing"), controller.GetPricing)
		perfMetricsRoute := apiRouter.Group("/perf-metrics")
		perfMetricsRoute.Use(middleware.HeaderNavModulePublicOrUserAuth("pricing"))
		{
			perfMetricsRoute.GET("/summary", controller.GetPerfMetricsSummary)
			perfMetricsRoute.GET("", controller.GetPerfMetrics)
		}
		apiRouter.GET("/rankings", middleware.HeaderNavModuleAuth("rankings"), controller.GetRankings)
		apiRouter.GET("/verification", middleware.EmailVerificationRateLimit(), middleware.TurnstileCheck(), controller.SendEmailVerification)
		apiRouter.GET("/reset_password", middleware.CriticalRateLimit(), middleware.TurnstileCheck(), controller.SendPasswordResetEmail)
		apiRouter.POST("/user/reset", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, controller.ResetPassword)
		// OAuth routes - specific routes must come before :provider wildcard
		apiRouter.POST("/oauth/state", middleware.CriticalRateLimit(), middleware.DisableCache(), middleware.TryUserAuth(), anonymousRequestBodyLimit, controller.GenerateOAuthCode)
		apiRouter.POST("/oauth/email/bind/start", middleware.UserAuth(), middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("account-security"), middleware.EmailVerificationRateLimit(), middleware.DisableCache(), controller.EmailBindStart)
		apiRouter.POST("/oauth/email/bind/resend", middleware.UserAuth(), middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("account-security"), middleware.EmailVerificationRateLimit(), middleware.DisableCache(), controller.EmailBindResend)
		apiRouter.POST("/oauth/email/bind", middleware.UserAuth(), middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("account-security"), middleware.DisableCache(), controller.EmailBind)
		// WeChat uses its existing authorization-code service.
		apiRouter.GET("/oauth/wechat", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.WeChatAuth)
		apiRouter.POST("/oauth/wechat/bind", middleware.UserAuth(), middleware.CriticalRateLimit(), controller.WeChatBind)
		apiRouter.GET("/oauth/telegram/login", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.TelegramLegacyAuth)
		apiRouter.POST("/oauth/telegram/bind/start", middleware.UserAuth(), middleware.CriticalRateLimit(), middleware.DisableCache(), controller.TelegramLegacyAuth)
		apiRouter.GET("/oauth/telegram/bind/:flow_token", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.TelegramLegacyAuth)
		// Standard OAuth providers (GitHub, Discord, OIDC, LinuxDO, Telegram) - unified route
		apiRouter.GET("/oauth/:provider", middleware.CriticalRateLimit(), middleware.DisableCache(), middleware.TryUserAuth(), controller.HandleOAuth)
		apiRouter.GET("/ratio_config", middleware.CriticalRateLimit(), controller.GetRatioConfig)

		apiRouter.POST("/stripe/webhook", anonymousRequestBodyLimit, controller.StripeWebhook)
		apiRouter.POST("/creem/webhook", anonymousRequestBodyLimit, controller.CreemWebhook)
		apiRouter.POST("/waffo/webhook", anonymousRequestBodyLimit, controller.WaffoWebhook)
		// :env separates test vs prod URLs so the operator can register each
		// in Pancake's matching webhook slot; handler enforces env match.
		apiRouter.POST("/waffo-pancake/webhook/:env", anonymousRequestBodyLimit, controller.WaffoPancakeWebhook)

		// adtyai 二开：充值档位 / 首充 / 下单（最小闭环；真支付回调后续接同一 CompleteTopupOrder）
		topupRoute := apiRouter.Group("/topup")
		{
			topupRoute.GET("/tiers", controller.GetTopupTiers)
			topupAuth := topupRoute.Group("")
			topupAuth.Use(middleware.UserAuth())
			{
				topupAuth.GET("/first-recharge", controller.GetFirstRechargeInfo)
				topupAuth.POST("/order", middleware.TopupOrderRateLimit(), controller.CreateTopupOrder)
				topupAuth.GET("/order/:no", controller.GetTopupOrder)
			}
		}
		registerAdtyPaymentAdminRoutes(apiRouter, middleware.AdminAuth(), middleware.RootAuth())
		adminRisk := apiRouter.Group("/admin/risk-reviews")
		adminRisk.Use(middleware.AdminAuth())
		{
			adminRisk.GET("", controller.AdminListRiskReviews)
			adminRisk.GET("/:id", controller.AdminGetRiskReview)
			adminRisk.POST("/:id/approve", controller.AdminApproveRiskReview)
			adminRisk.POST("/:id/reject", controller.AdminRejectRiskReview)
		}
		// adtyai 易支付回调：不依赖登录态，按 instance_id 取密钥验签
		apiRouter.GET("/pay/epay/notify/:instance_id", controller.AdtyEpayNotify)
		apiRouter.POST("/pay/epay/notify/:instance_id", anonymousRequestBodyLimit, controller.AdtyEpayNotify)

		// adtyai USDT / BEpusdt：新路径只验签；老路径默认关闭
		apiRouter.GET("/pay/usdt/notify", controller.AdtyUsdtNotify)
		apiRouter.POST("/pay/usdt/notify", anonymousRequestBodyLimit, controller.AdtyUsdtNotify)
		apiRouter.GET("/user/bepusdt/notify", controller.AdtyBepusdtLegacyNotify)
		apiRouter.POST("/user/bepusdt/notify", anonymousRequestBodyLimit, controller.AdtyBepusdtLegacyNotify)

		// adtyai A7 返佣 / 提现
		affRoute := apiRouter.Group("/aff")
		affRoute.Use(middleware.UserAuth())
		{
			affRoute.GET("/self", controller.GetAffSelf)
			affRoute.GET("/commissions/self", controller.ListAffCommissionsSelf)
			affRoute.POST("/withdraw", controller.CreateAffWithdraw)
			affRoute.GET("/withdrawals", controller.ListAffWithdrawalsSelf)
			affRoute.GET("/withdrawals/:id", controller.GetAffWithdrawalSelf)
		}
		adminAff := apiRouter.Group("/admin/aff")
		adminAff.Use(middleware.AdminAuth())
		{
			adminAff.GET("/commissions", controller.AdminListAffCommissions)
			adminAff.POST("/commissions/:id/void", controller.AdminVoidAffCommission)
			adminAff.GET("/withdrawals", controller.AdminListAffWithdrawals)
			adminAff.POST("/withdrawals/:id/approve", controller.AdminApproveAffWithdrawal)
			adminAff.POST("/withdrawals/:id/reject", controller.AdminRejectAffWithdrawal)
			adminAff.POST("/withdrawals/:id/mark-paid", controller.AdminMarkAffWithdrawalPaid)
			adminAff.POST("/withdrawals/:id/mark-failed", controller.AdminMarkAffWithdrawalFailed)
			adminAff.POST("/withdrawals/:id/mark-unknown", controller.AdminMarkAffWithdrawalUnknown)
			adminAff.POST("/withdrawals/:id/retry", controller.AdminRetryAffWithdrawal)
			adminAff.GET("/self-invite-reviews", controller.AdminListAffSelfInviteReviews)
		}

		adminBA := apiRouter.Group("/admin/balance-adjustments")
		adminBA.Use(middleware.AdminAuth(), middleware.FeatureFrozen())
		{
			adminBA.POST("", controller.AdminCreateBalanceAdjustment)
			adminBA.GET("", controller.AdminListBalanceAdjustments)
			adminBA.POST("/:id/approve", controller.AdminApproveBalanceAdjustment)
			adminBA.POST("/:id/reject", controller.AdminRejectBalanceAdjustment)
		}
		adminEP := apiRouter.Group("/admin/enterprise-pricing")
		adminEP.Use(middleware.AdminAuth())
		{
			adminEP.GET("", controller.AdminListEnterprisePricing)
			adminEP.POST("", controller.AdminCreateEnterprisePricing)
			adminEP.PUT("/:id", controller.AdminUpdateEnterprisePricing)
			adminEP.DELETE("/:id", controller.AdminDeleteEnterprisePricing)
		}
		adminBP := apiRouter.Group("/admin/business-params")
		adminBP.Use(middleware.AdminAuth())
		{
			adminBP.GET("", controller.AdminGetBusinessParams)
			adminBP.PUT("", controller.AdminPutBusinessParams)
			adminBP.PUT("/:key", controller.AdminPutBusinessParamByKey)
		}

		adminAlerts := apiRouter.Group("/admin/alerts")
		adminAlerts.Use(middleware.AdminAuth())
		{
			adminAlerts.GET("", controller.AdminGetAlerts)
			adminAlerts.PUT("", controller.AdminPutAlerts)
			adminAlerts.POST("/test", controller.AdminTestAlerts)
		}

		// 总裁：停暴露「新增管理员」——表与 controller 保留，不再注册路由。
		// adminAdmins := apiRouter.Group("/admin/admins") ...

		adminChatwoot := apiRouter.Group("/admin/chatwoot")
		adminChatwoot.Use(middleware.AdminAuth())
		{
			adminChatwoot.GET("", controller.AdminGetChatwoot)
			adminChatwoot.PUT("", controller.AdminPutChatwoot)
		}

		adminMenuVis := apiRouter.Group("/admin/menu-visibility")
		adminMenuVis.Use(middleware.AdminAuth())
		{
			adminMenuVis.GET("", controller.AdminGetMenuVisibility)
			adminMenuVis.PUT("", controller.AdminPutMenuVisibility)
		}

		adminGroups := apiRouter.Group("/admin/groups")
		adminGroups.Use(middleware.AdminAuth())
		{
			adminGroups.GET("", controller.AdminGetGroups)
			adminGroups.POST("", controller.AdminPostGroup)
			adminGroups.PUT("/:code", controller.AdminPutGroup)
		}

		// 总裁：停暴露「运营总览」「服务状态」「每日对账」——日报任务仍跑，不再注册管理端路由。
		// adminOverview / adminHealth / adminReconcile 已下线。

		// 侧栏角标：风控/提现/充值待处理条数
		adtyAdmin := apiRouter.Group("/adty/admin")
		adtyAdmin.Use(middleware.AdminAuth())
		{
			adtyAdmin.GET("/sidebar_badges", controller.AdminGetSidebarBadges)
		}

		// adtyai 智能路由管理菜单 API
		adtyRouting := apiRouter.Group("/adty/routing")
		adtyRouting.Use(middleware.AdminAuth())
		{
			adtyRouting.GET("/config", controller.AdtyRoutingGetConfig)
			adtyRouting.PUT("/config", controller.AdtyRoutingPutConfig)
			adtyRouting.GET("/scores", controller.AdtyRoutingGetScores)
			adtyRouting.GET("/scores/:channel_id", controller.AdtyRoutingGetScoreDetail)
			adtyRouting.GET("/rankings", controller.AdtyRoutingGetRankings)
			adtyRouting.GET("/health/:channel_id", controller.AdtyRoutingGetHealth)
			adtyRouting.GET("/weight-schemes", controller.AdtyRoutingListSchemes)
			adtyRouting.POST("/weight-schemes", controller.AdtyRoutingCreateScheme)
			adtyRouting.PUT("/weight-schemes/:id", controller.AdtyRoutingUpdateScheme)
			adtyRouting.DELETE("/weight-schemes/:id", controller.AdtyRoutingDeleteScheme)
			adtyRouting.POST("/weight-schemes/preview", controller.AdtyRoutingPreviewScheme)
			adtyRouting.GET("/weight-schemes/:id/export", controller.AdtyRoutingExportScheme)
			adtyRouting.POST("/weight-schemes/import", controller.AdtyRoutingImportScheme)
			adtyRouting.GET("/weight-schemes/history", controller.AdtyRoutingSchemeHistory)
			adtyRouting.POST("/weight-schemes/rollback", controller.AdtyRoutingSchemeRollback)
			adtyRouting.POST("/simulate", controller.AdtyRoutingSimulate)
			adtyRouting.GET("/tags", controller.AdtyRoutingListTags)
			adtyRouting.POST("/tags", controller.AdtyRoutingCreateTag)
			adtyRouting.DELETE("/tags", controller.AdtyRoutingDeleteTag)
			adtyRouting.POST("/tags/fake_model/clear", controller.AdtyRoutingClearFakeModel)
			adtyRouting.GET("/breaker/events", controller.AdtyRoutingBreakerEvents)
			adtyRouting.GET("/breaker/stages", controller.AdtyRoutingBreakerStages)
			adtyRouting.GET("/prices", controller.AdtyRoutingGetPrices)
			adtyRouting.PUT("/prices", controller.AdtyRoutingPutPrices)
			adtyRouting.GET("/cost/summary", controller.AdtyRoutingCostSummary)
			adtyRouting.GET("/cost/rankings", controller.AdtyRoutingCostRankings)
			adtyRouting.GET("/cost/live", controller.AdtyRoutingCostLive)
			adtyRouting.GET("/cost/guards", controller.AdtyRoutingCostGuards)
			adtyRouting.POST("/cost/backfill", controller.AdtyRoutingCostBackfill)
			adtyRouting.POST("/balance/refresh", controller.AdtyRoutingBalanceRefresh)
			adtyRouting.GET("/auth-flags", controller.AdtyRoutingAuthFlags)
			adtyRouting.PUT("/auth-flags/:id/mark", controller.AdtyRoutingMarkAuthFlag)
			adtyRouting.GET("/decision", controller.AdtyRoutingDecision)
			adtyRouting.GET("/decisions", controller.AdtyRoutingDecisions)
			adtyRouting.POST("/weights/rollback", controller.AdtyRoutingWeightsRollback)
		}

		// adtyai U16 / A17 流水
		walletRoute := apiRouter.Group("/wallet")
		walletRoute.Use(middleware.UserAuth())
		{
			walletRoute.GET("/ledger/self", controller.GetWalletLedgerSelf)
		}
		adminWallet := apiRouter.Group("/admin/wallet")
		adminWallet.Use(middleware.AdminAuth())
		{
			adminWallet.GET("/bonus-ledger", controller.AdminListBonusLedger)
		}

		// Universal secure verification routes
		apiRouter.GET("/verify/methods", middleware.UserAuth(), middleware.DisableCache(), controller.GetVerificationMethods)
		apiRouter.POST("/verify", middleware.UserAuth(), middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("security-verification"), middleware.DisableCache(), controller.UniversalVerify)

		userRoute := apiRouter.Group("/user")
		{
			userRoute.POST("/auth/refresh", middleware.SessionCookieOriginGuard(), middleware.CriticalRateLimit(), middleware.DisableCache(), controller.RefreshAuth)
			userRoute.POST("/auth/logout", middleware.SessionCookieOriginGuard(), middleware.CriticalRateLimit(), middleware.DisableCache(), controller.AuthLogout)
			userRoute.POST("/register", middleware.CriticalRateLimit(), anonymousRequestBodyLimit, middleware.TurnstileCheck(), controller.Register)
			userRoute.GET("/login/encryption-key", middleware.DisableCache(), controller.GetPasswordEncryptionKey)
			userRoute.POST("/login", middleware.CriticalRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, middleware.TurnstileCheck(), controller.Login)
			userRoute.POST("/login/2fa", middleware.CriticalRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, controller.Verify2FALogin)
			userRoute.POST("/login/verify", middleware.CriticalRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, controller.VerifyLogin)
			userRoute.POST("/login/passkey/begin", middleware.CriticalRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, controller.LoginPasskeyBegin)
			userRoute.POST("/login/passkey/finish", middleware.CriticalRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, controller.LoginPasskeyFinish)
			userRoute.POST("/passkey/login/begin", middleware.CriticalRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, controller.PasskeyLoginBegin)
			userRoute.POST("/passkey/login/finish", middleware.CriticalRateLimit(), middleware.DisableCache(), anonymousRequestBodyLimit, controller.PasskeyLoginFinish)
			//userRoute.POST("/tokenlog", middleware.CriticalRateLimit(), controller.TokenLog)
			userRoute.POST("/epay/notify", anonymousRequestBodyLimit, controller.EpayNotify)
			userRoute.GET("/epay/notify", controller.EpayNotify)
			userRoute.GET("/groups", controller.GetUserGroups)

			selfRoute := userRoute.Group("/")
			selfRoute.Use(middleware.DisableCache(), middleware.UserAuth())
			{
				selfRoute.GET("/sessions", middleware.DisableCache(), controller.GetLoginSessions)
				selfRoute.DELETE("/sessions/:sid", middleware.DisableCache(), controller.DeleteLoginSession)
				selfRoute.POST("/sessions/revoke-others", middleware.DisableCache(), controller.RevokeOtherLoginSessions)
				selfRoute.GET("/self/groups", controller.GetUserGroups)
				selfRoute.GET("/self", controller.GetSelf)
				selfRoute.GET("/models", controller.GetUserModels)
				selfRoute.PUT("/self", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.UpdateSelf)
				selfRoute.DELETE("/self", middleware.DisableCache(), controller.DeleteSelf)
				accessTokenRoute := selfRoute.Group("/access_tokens")
				accessTokenRoute.Use(middleware.DisableCache())
				{
					accessTokenRoute.GET("", controller.ListAccessTokens)
					accessTokenRoute.GET("/catalog", controller.GetAccessTokenCatalog)
					accessTokenRoute.GET("/scopes", controller.GetAccessTokenScopes)
					accessTokenRoute.POST("", middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("access-token"), controller.CreateAccessToken)
					accessTokenRoute.PATCH("/:id", middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("access-token"), controller.UpdateAccessToken)
					accessTokenRoute.DELETE("/:id", middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("access-token"), controller.DeleteAccessToken)
					accessTokenRoute.DELETE("/legacy", middleware.CriticalRateLimit(), middleware.UserCriticalRateLimit("access-token"), controller.RevokeLegacyAccessToken)
				}
				selfRoute.GET("/passkey", controller.PasskeyStatus)
				selfRoute.POST("/passkey/register/begin", middleware.UserCriticalRateLimit("security-verification"), middleware.DisableCache(), controller.PasskeyRegisterBegin)
				selfRoute.POST("/passkey/register/finish", middleware.UserCriticalRateLimit("security-verification"), middleware.DisableCache(), controller.PasskeyRegisterFinish)
				selfRoute.POST("/passkey/verify/begin", middleware.UserCriticalRateLimit("security-verification"), middleware.DisableCache(), controller.PasskeyVerifyBegin)
				selfRoute.POST("/passkey/verify/finish", middleware.UserCriticalRateLimit("security-verification"), middleware.DisableCache(), controller.PasskeyVerifyFinish)
				selfRoute.DELETE("/passkey", middleware.DisableCache(), controller.PasskeyDelete)
				selfRoute.GET("/aff", controller.GetAffCode)
				selfRoute.GET("/topup/info", controller.GetTopUpInfo)
				selfRoute.GET("/topup/self", controller.GetUserTopUps)
				selfRoute.POST("/topup", middleware.CriticalRateLimit(), controller.TopUp)
				selfRoute.POST("/pay", middleware.CriticalRateLimit(), controller.RequestEpay)
				selfRoute.POST("/amount", controller.RequestAmount)
				selfRoute.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.RequestStripePay)
				selfRoute.POST("/stripe/amount", controller.RequestStripeAmount)
				selfRoute.POST("/creem/pay", middleware.CriticalRateLimit(), controller.RequestCreemPay)
				selfRoute.POST("/waffo/amount", controller.RequestWaffoAmount)
				selfRoute.POST("/waffo/pay", middleware.CriticalRateLimit(), controller.RequestWaffoPay)
				selfRoute.POST("/waffo-pancake/amount", controller.RequestWaffoPancakeAmount)
				selfRoute.POST("/waffo-pancake/pay", middleware.CriticalRateLimit(), controller.RequestWaffoPancakePay)
				selfRoute.POST("/aff_transfer", middleware.UserCriticalRateLimit("aff-transfer"), controller.TransferAffQuota)
				selfRoute.PUT("/setting", controller.UpdateUserSetting)

				// 2FA routes
				selfRoute.GET("/2fa/status", controller.Get2FAStatus)
				selfRoute.POST("/2fa/setup", middleware.UserCriticalRateLimit("security-verification"), middleware.DisableCache(), controller.Setup2FA)
				selfRoute.POST("/2fa/enable", middleware.UserCriticalRateLimit("security-verification"), middleware.DisableCache(), controller.Enable2FA)
				selfRoute.POST("/2fa/disable", middleware.DisableCache(), controller.Disable2FA)
				selfRoute.POST("/2fa/backup_codes", middleware.DisableCache(), controller.RegenerateBackupCodes)

				// Check-in routes
				selfRoute.GET("/checkin", controller.GetCheckinStatus)
				selfRoute.POST("/checkin", middleware.TurnstileCheck(), controller.DoCheckin)

				// Custom OAuth bindings
				selfRoute.GET("/oauth/bindings", controller.GetUserOAuthBindings)
				selfRoute.DELETE("/oauth/bindings/:provider_id", controller.UnbindCustomOAuth)
			}

			adminRoute := userRoute.Group("/")
			adminRoute.Use(middleware.AdminAuth())
			{
				adminRoute.GET("/", controller.GetAllUsers)
				adminRoute.GET("/topup", controller.GetAllTopUps)
				adminRoute.POST("/topup/complete", controller.AdminCompleteTopUp)
				adminRoute.GET("/search", controller.SearchUsers)
				adminRoute.GET("/:id/oauth/bindings", controller.GetUserOAuthBindingsByAdmin)
				adminRoute.DELETE("/:id/oauth/bindings/:provider_id", controller.UnbindCustomOAuthByAdmin)
				adminRoute.DELETE("/:id/bindings/:binding_type", controller.AdminClearUserBinding)
				adminRoute.GET("/:id", controller.GetUser)
				adminRoute.POST("/", controller.CreateUser)
				adminRoute.POST("/manage", controller.ManageUser)
				adminRoute.PUT("/", controller.UpdateUser)
				adminRoute.DELETE("/:id", controller.DeleteUser)
				adminRoute.DELETE("/:id/reset_passkey", controller.AdminResetPasskey)

				// Admin 2FA routes
				adminRoute.GET("/2fa/stats", controller.Admin2FAStats)
				adminRoute.DELETE("/:id/2fa", controller.AdminDisable2FA)
			}
		}

		// Subscription: keep self status/preference; close purchase + admin management entrances (no table drop).
		subscriptionRoute := apiRouter.Group("/subscription")
		subscriptionRoute.Use(middleware.UserAuth())
		{
			subscriptionRoute.GET("/self", controller.GetSubscriptionSelf)
			subscriptionRoute.PUT("/self/preference", controller.UpdateSubscriptionPreference)
			subscriptionBuy := subscriptionRoute.Group("")
			subscriptionBuy.Use(middleware.SubscriptionClosed())
			{
				subscriptionBuy.GET("/plans", controller.GetSubscriptionPlans)
				subscriptionBuy.POST("/balance/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestBalancePay)
				subscriptionBuy.POST("/epay/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestEpay)
				subscriptionBuy.POST("/stripe/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestStripePay)
				subscriptionBuy.POST("/creem/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestCreemPay)
				subscriptionBuy.POST("/waffo-pancake/pay", middleware.CriticalRateLimit(), controller.SubscriptionRequestWaffoPancakePay)
			}
		}
		subscriptionAdminRoute := apiRouter.Group("/subscription/admin")
		subscriptionAdminRoute.Use(middleware.AdminAuth(), middleware.SubscriptionClosed())
		{
			subscriptionAdminRoute.GET("/plans", controller.AdminListSubscriptionPlans)
			subscriptionAdminRoute.POST("/plans", controller.AdminCreateSubscriptionPlan)
			subscriptionAdminRoute.PUT("/plans/:id", controller.AdminUpdateSubscriptionPlan)
			subscriptionAdminRoute.PATCH("/plans/:id", controller.AdminUpdateSubscriptionPlanStatus)
			subscriptionAdminRoute.POST("/bind", controller.AdminBindSubscription)
			subscriptionAdminRoute.POST("/plans/:id/subscriptions/reset", controller.AdminResetPlanSubscriptions)

			// User subscription management (admin)
			subscriptionAdminRoute.GET("/users/:id/subscriptions", controller.AdminListUserSubscriptions)
			subscriptionAdminRoute.POST("/users/:id/subscriptions", controller.AdminCreateUserSubscription)
			subscriptionAdminRoute.POST("/users/:id/subscriptions/reset", controller.AdminResetUserSubscriptionsByPlan)
			subscriptionAdminRoute.POST("/user_subscriptions/:id/invalidate", controller.AdminInvalidateUserSubscription)
			subscriptionAdminRoute.DELETE("/user_subscriptions/:id", controller.AdminDeleteUserSubscription)
		}

		// Subscription payment callbacks (no auth)
		apiRouter.POST("/subscription/epay/notify", anonymousRequestBodyLimit, controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/notify", controller.SubscriptionEpayNotify)
		apiRouter.GET("/subscription/epay/return", controller.SubscriptionEpayReturn)
		apiRouter.POST("/subscription/epay/return", anonymousRequestBodyLimit, controller.SubscriptionEpayReturn)
		optionRoute := apiRouter.Group("/option")
		optionRoute.Use(middleware.RootAuth())
		{
			optionRoute.GET("/", controller.GetOptions)
			optionRoute.GET("/request_policy", controller.GetRequestPolicy)
			optionRoute.PATCH("/request_policy", controller.UpdateRequestPolicy)
			optionRoute.PUT("/", controller.UpdateOption)
			optionRoute.PUT("/passkey/domains", controller.UpdatePasskeyDomains)
			optionRoute.GET("/model_pricing", controller.GetModelPricingConfig)
			optionRoute.PATCH("/model_pricing", controller.UpdateModelPricingConfig)
			optionRoute.POST("/model_pricing/convert", controller.PreviewModelPricingConversion)
			optionRoute.POST("/model_pricing/preview", controller.PreviewModelPricing)
			optionRoute.GET("/channel_affinity_cache", controller.GetChannelAffinityCacheStats)
			optionRoute.DELETE("/channel_affinity_cache", controller.ClearChannelAffinityCache)
			optionRoute.POST("/rest_model_ratio", controller.ResetModelRatio)
			optionRoute.GET("/waffo-pancake/catalog", controller.ListWaffoPancakeCatalog)
			optionRoute.POST("/waffo-pancake/pair", controller.CreateWaffoPancakePair)
			optionRoute.POST("/waffo-pancake/save", controller.SaveWaffoPancake)
			optionRoute.POST("/waffo-pancake/subscription-product", middleware.SubscriptionClosed(), controller.CreateWaffoPancakeSubscriptionProduct)
			optionRoute.GET("/waffo-pancake/subscription-product-options", middleware.SubscriptionClosed(), controller.ListWaffoPancakeSubscriptionProductOptions)
		}

		// Custom OAuth provider management (root only)
		customOAuthRoute := apiRouter.Group("/custom-oauth-provider")
		customOAuthRoute.Use(middleware.RootAuth())
		{
			customOAuthRoute.POST("/discovery", controller.FetchCustomOAuthDiscovery)
			customOAuthRoute.GET("/", controller.GetCustomOAuthProviders)
			customOAuthRoute.GET("/:id", controller.GetCustomOAuthProvider)
			customOAuthRoute.POST("/", controller.CreateCustomOAuthProvider)
			customOAuthRoute.PUT("/:id", controller.UpdateCustomOAuthProvider)
			customOAuthRoute.DELETE("/:id", controller.DeleteCustomOAuthProvider)
		}
		performanceRoute := apiRouter.Group("/performance")
		performanceRoute.Use(middleware.RootAuth())
		{
			performanceRoute.GET("/stats", controller.GetPerformanceStats)
			performanceRoute.DELETE("/disk_cache", controller.ClearDiskCache)
			performanceRoute.POST("/reset_stats", controller.ResetPerformanceStats)
			performanceRoute.POST("/gc", controller.ForceGC)
			performanceRoute.GET("/logs", controller.GetLogFiles)
			performanceRoute.DELETE("/logs", controller.CleanupLogFiles)
		}
		ratioSyncRoute := apiRouter.Group("/ratio_sync")
		ratioSyncRoute.Use(middleware.RootAuth())
		{
			ratioSyncRoute.GET("/channels", controller.GetSyncableChannels)
			ratioSyncRoute.POST("/fetch", controller.FetchUpstreamRatios)
		}
		taskPluginRoute := apiRouter.Group("/plugin/task")
		taskPluginRoute.Use(middleware.RootAuth())
		{
			taskPluginRoute.GET("", controller.ListTaskPlugins)
			taskPluginRoute.POST("", controller.UploadTaskPlugin)
			taskPluginRoute.PUT("", controller.UploadTaskPlugin)
			taskPluginRoute.GET("/runtime/status", controller.GetTaskPluginRuntime)
			taskPluginRoute.GET("/marketplace/sources", controller.GetTaskPluginMarketplaceSources)
			taskPluginRoute.PUT("/marketplace/sources", controller.UpdateTaskPluginMarketplaceSources)
			taskPluginRoute.GET("/:key", controller.GetTaskPlugin)
			taskPluginRoute.GET("/:key/icon", controller.GetTaskPluginIcon)
			taskPluginRoute.GET("/:key/versions", controller.GetTaskPluginVersions)
			taskPluginRoute.POST("/:key/activate", controller.ActivateTaskPlugin)
			taskPluginRoute.POST("/:key/status", controller.SetTaskPluginStatus)
			taskPluginRoute.POST("/:key/dryrun", controller.DryRunTaskPlugin)
			taskPluginRoute.DELETE("/:key/versions/:version", controller.DeleteTaskPluginVersion)
		}
		handlePermissionRoute(apiRouter.Group("", middleware.AdminAuth()), http.MethodGet, "/task_plugin_options", authz.TaskPluginBind, controller.GetTaskPluginOptions)
		registerChannelRoutes(apiRouter)
		registerAuthzRoutes(apiRouter)
		tokenRoute := apiRouter.Group("/token")
		tokenRoute.Use(middleware.UserAuth())
		tokenRoute.Use(middleware.TokenOperationAudit())
		{
			tokenRoute.GET("/", controller.GetAllTokens)
			tokenRoute.GET("/search", middleware.SearchRateLimit(), controller.SearchTokens)
			tokenRoute.GET("/auto-groups", controller.GetTokenAutoGroups)
			tokenRoute.GET("/:id", controller.GetToken)
			tokenRoute.POST("/:id/key", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.GetTokenKey)
			tokenRoute.POST("/", controller.AddToken)
			tokenRoute.PUT("/", controller.UpdateToken)
			tokenRoute.DELETE("/:id", controller.DeleteToken)
			tokenRoute.POST("/batch", controller.DeleteTokenBatch)
			tokenRoute.POST("/batch/keys", middleware.CriticalRateLimit(), middleware.DisableCache(), controller.GetTokenKeysBatch)
		}

		usageRoute := apiRouter.Group("/usage")
		usageRoute.Use(middleware.CORS(), middleware.CriticalRateLimit())
		{
			tokenUsageRoute := usageRoute.Group("/token")
			tokenUsageRoute.Use(middleware.TokenAuthReadOnly())
			{
				tokenUsageRoute.GET("/", controller.GetTokenUsage)
			}
		}

		redemptionRoute := apiRouter.Group("/redemption")
		redemptionRoute.Use(middleware.AdminAuth())
		{
			redemptionRoute.GET("/", controller.GetAllRedemptions)
			redemptionRoute.GET("/search", controller.SearchRedemptions)
			redemptionRoute.GET("/:id", controller.GetRedemption)
			redemptionRoute.POST("/", controller.AddRedemption)
			redemptionRoute.POST("/batch", controller.DeleteRedemptionBatch)
			redemptionRoute.PUT("/", controller.UpdateRedemption)
			redemptionRoute.DELETE("/invalid", controller.DeleteInvalidRedemption)
			redemptionRoute.DELETE("/:id", controller.DeleteRedemption)
		}
		handlePermissionRoute(apiRouter.Group("", middleware.DisableCache(), middleware.AdminAuth()), http.MethodGet, "/audit", authz.AuditRead, controller.GetAuditLogs)
		apiRouter.GET("/audit/self", middleware.DisableCache(), middleware.UserAuth(), controller.GetAuditLogs)
		logRoute := apiRouter.Group("/log")
		logRoute.GET("/", middleware.AdminAuth(), controller.GetAllLogs)
		logRoute.GET("/stat", middleware.AdminAuth(), controller.GetLogsStat)
		logRoute.GET("/self/stat", middleware.UserAuth(), controller.GetLogsSelfStat)
		logRoute.GET("/channel_affinity_usage_cache", middleware.AdminAuth(), controller.GetChannelAffinityUsageCacheStats)
		logRoute.GET("/search", middleware.AdminAuth(), controller.SearchAllLogs)
		logRoute.GET("/self", middleware.UserAuth(), controller.GetUserLogs)
		logRoute.GET("/export", middleware.AdminAuth(), controller.ExportAllLogs)
		logRoute.GET("/self/export", middleware.UserAuth(), controller.ExportUserLogs)
		logRoute.GET("/self/search", middleware.UserAuth(), middleware.SearchRateLimit(), controller.SearchUserLogs)

		systemTaskRoute := apiRouter.Group("/system-task")
		systemTaskRoute.Use(middleware.RootAuth())
		{
			systemTaskRoute.POST("/log-cleanup", controller.CreateLogCleanupSystemTask)
			systemTaskRoute.GET("/list", controller.ListSystemTasks)
			systemTaskRoute.DELETE("/history", controller.DeleteSystemTaskHistory)
			systemTaskRoute.GET("/current", controller.GetCurrentSystemTask)
			systemTaskRoute.GET("/:task_id", controller.GetSystemTask)
		}
		systemInfoRoute := apiRouter.Group("/system-info")
		systemInfoRoute.Use(middleware.RootAuth())
		{
			systemInfoRoute.GET("/instances", controller.ListSystemInstances)
			systemInfoRoute.DELETE("/stale-instances", controller.DeleteStaleSystemInstances)
			systemInfoRoute.DELETE("/instances/:node_name", controller.DeleteStaleSystemInstance)
		}

		dataRoute := apiRouter.Group("/data")
		dataRoute.GET("/", middleware.AdminAuth(), controller.GetAllQuotaDates)
		dataRoute.GET("/users", middleware.AdminAuth(), controller.GetQuotaDatesByUser)
		dataRoute.GET("/self", middleware.UserAuth(), controller.GetUserQuotaDates)
		dataRoute.GET("/flow", middleware.AdminAuth(), controller.GetAllFlowQuotaDates)
		dataRoute.GET("/flow/self", middleware.UserAuth(), controller.GetUserFlowQuotaDates)

		logRoute.Use(middleware.CORS(), middleware.CriticalRateLimit())
		{
			logRoute.GET("/token", middleware.TokenAuthReadOnly(), controller.GetLogByKey)
		}
		groupRoute := apiRouter.Group("/group")
		groupRoute.Use(middleware.AdminAuth())
		{
			groupRoute.GET("/", controller.GetGroups)
		}

		prefillGroupRoute := apiRouter.Group("/prefill_group")
		prefillGroupRoute.Use(middleware.AdminAuth())
		{
			prefillGroupRoute.GET("/", controller.GetPrefillGroups)
			prefillGroupRoute.POST("/", controller.CreatePrefillGroup)
			prefillGroupRoute.PUT("/", controller.UpdatePrefillGroup)
			prefillGroupRoute.DELETE("/:id", controller.DeletePrefillGroup)
		}

		mjRoute := apiRouter.Group("/mj")
		mjRoute.GET("/self", middleware.UserAuth(), controller.GetUserMidjourney)
		mjRoute.GET("/", middleware.AdminAuth(), controller.GetAllMidjourney)

		taskRoute := apiRouter.Group("/task")
		{
			taskRoute.GET("/self", middleware.UserAuth(), controller.GetUserTask)
			taskRoute.GET("", middleware.AdminAuth(), controller.GetAllTask)
			taskRoute.GET("/:task_id/artifacts", middleware.UserAuth(), controller.GetDashboardTaskArtifacts)
		}

		vendorRoute := apiRouter.Group("/vendors")
		vendorRoute.Use(middleware.AdminAuth())
		{
			vendorRoute.POST("/operations/preview", controller.PreviewVendorOperation)
			vendorRoute.POST("/operations", controller.ApplyVendorOperation)
			vendorRoute.GET("/", controller.GetAllVendors)
			vendorRoute.GET("/search", controller.SearchVendors)
			vendorRoute.GET("/:id", controller.GetVendorMeta)
			vendorRoute.POST("/", controller.CreateVendorMeta)
			vendorRoute.PUT("/", controller.UpdateVendorMeta)
			vendorRoute.DELETE("/:id", controller.DeleteVendorMeta)
		}

		modelsRoute := apiRouter.Group("/models")
		modelsRoute.Use(middleware.AdminAuth())
		{
			modelsRoute.GET("/sync_upstream/preview", controller.SyncUpstreamPreview)
			modelsRoute.POST("/sync_upstream", controller.SyncUpstreamModels)
			modelsRoute.POST("/delete", controller.BatchDeleteModelMeta)
			modelsRoute.GET("/missing", controller.GetMissingModels)
			modelsRoute.GET("/", controller.GetAllModelsMeta)
			modelsRoute.GET("/search", controller.SearchModelsMeta)
			modelsRoute.GET("/:id", controller.GetModelMeta)
			modelsRoute.POST("/", controller.CreateModelMeta)
			modelsRoute.PUT("/", controller.UpdateModelMeta)
			modelsRoute.DELETE("/:id", controller.DeleteModelMeta)
		}

		// Deployments (model deployment management)
		deploymentsRoute := apiRouter.Group("/deployments")
		deploymentsRoute.Use(middleware.AdminAuth())
		{
			deploymentsRoute.GET("/settings", controller.GetModelDeploymentSettings)
			deploymentsRoute.POST("/settings/test-connection", controller.TestIoNetConnection)
			deploymentsRoute.GET("/", controller.GetAllDeployments)
			deploymentsRoute.GET("/search", controller.SearchDeployments)
			deploymentsRoute.POST("/test-connection", controller.TestIoNetConnection)
			deploymentsRoute.GET("/hardware-types", controller.GetHardwareTypes)
			deploymentsRoute.GET("/locations", controller.GetLocations)
			deploymentsRoute.GET("/available-replicas", controller.GetAvailableReplicas)
			deploymentsRoute.POST("/price-estimation", controller.GetPriceEstimation)
			deploymentsRoute.GET("/check-name", controller.CheckClusterNameAvailability)
			deploymentsRoute.POST("/", controller.CreateDeployment)

			deploymentsRoute.GET("/:id", controller.GetDeployment)
			deploymentsRoute.GET("/:id/logs", controller.GetDeploymentLogs)
			deploymentsRoute.GET("/:id/containers", controller.ListDeploymentContainers)
			deploymentsRoute.GET("/:id/containers/:container_id", controller.GetContainerDetails)
			deploymentsRoute.PUT("/:id", controller.UpdateDeployment)
			deploymentsRoute.PUT("/:id/name", controller.UpdateDeploymentName)
			deploymentsRoute.POST("/:id/extend", controller.ExtendDeployment)
			deploymentsRoute.DELETE("/:id", controller.DeleteDeployment)
		}
	}
}

// registerAdtyPaymentAdminRoutes 订单/易支付实例管理路由。
// 审查 P0-2 / P1-1：只读开放给管理员；收款账户写操作（新建/修改/设主用/启停）与人工补单仅 root。
func registerAdtyPaymentAdminRoutes(apiRouter *gin.RouterGroup, adminAuth, rootAuth gin.HandlerFunc) {
	adminOrders := apiRouter.Group("/admin/orders")
	adminOrders.Use(adminAuth)
	{
		adminOrders.GET("", controller.AdminListTopupOrders)
		adminOrders.GET("/:no", controller.AdminGetTopupOrder)
		adminOrders.POST("/:no/refund", controller.AdminRefundTopupOrder)
	}
	rootOrders := apiRouter.Group("/admin/orders")
	rootOrders.Use(rootAuth)
	{
		rootOrders.POST("/:no/complete", controller.AdminCompleteAdtyTopup)
	}
	adminEpay := apiRouter.Group("/admin/epay-instances")
	adminEpay.Use(adminAuth)
	{
		adminEpay.GET("", controller.AdminListEpayInstances)
		adminEpay.GET("/:id", controller.AdminGetEpayInstance)
	}
	rootEpay := apiRouter.Group("/admin/epay-instances")
	rootEpay.Use(rootAuth)
	{
		rootEpay.POST("", controller.AdminCreateEpayInstance)
		rootEpay.PUT("/:id", controller.AdminUpdateEpayInstance)
		rootEpay.POST("/:id/set-primary", controller.AdminSetEpayInstancePrimary)
		rootEpay.POST("/:id/disable", controller.AdminDisableEpayInstance)
		rootEpay.POST("/:id/enable", controller.AdminEnableEpayInstance)
	}
	rootEpayOps := apiRouter.Group("/admin/epay")
	rootEpayOps.Use(rootAuth)
	{
		rootEpayOps.POST("/manual-complete", controller.AdminEpayManualComplete)
	}
}
