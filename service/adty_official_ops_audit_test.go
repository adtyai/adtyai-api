package service

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAdtyVoucherOperationsWriteOfficialAudit(t *testing.T) {
	db := prepareReferralDialectDB(t, "sqlite")
	ResetAdtyVoucherGuesses()
	root := referralUser(t, db, "audit-voucher-root")
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	owner := referralUser(t, db, "audit-voucher-owner")
	customer := referralUser(t, db, "audit-voucher-customer")
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "audit-voucher-site", Status: model.AdtySiteStatusActive, AgreementVer: AdtyCurrentSiteAgreementVersion()}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Model(customer).Update("site_id", site.Id).Error)
	require.NoError(t, EnsureAdtySiteWallets(site.Id))
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.NewFromInt(20), AdtySiteWalletChange{
		Type: "seed", Reason: "seed earnings", IdempotencyKey: "audit-voucher-seed",
	})
	require.NoError(t, err)

	issued, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: owner.Id, Purpose: model.AdtyVoucherPurposeSiteCustomer, SiteId: site.Id, AmountUSD: decimal.NewFromInt(3), Reason: "customer gift"})
	require.NoError(t, err)
	issueLog := requireOfficialAudit(t, "voucher", strconv.FormatInt(issued.Id, 10), "adty.voucher.issue")
	require.NotNil(t, issueLog.SiteId)
	assert.Equal(t, site.Id, *issueLog.SiteId)
	assert.Equal(t, owner.Id, issueLog.UserId)
	assert.Equal(t, "audit-voucher-owner", issueLog.Username)
	assert.Equal(t, "customer gift", issueLog.Reason)
	assert.Empty(t, issueLog.ApprovalRef)
	assert.Equal(t, model.AuditResultSuccess, issueLog.Result)
	assert.True(t, issueLog.Success)
	assert.Contains(t, issueLog.AfterValue, `"status":"pending"`)
	assert.Contains(t, issueLog.AfterValue, `"amount_usd":"3"`)
	assert.NotContains(t, issueLog.AfterValue, issued.Code)
	assert.NotContains(t, issueLog.BeforeValue, issued.Code)
	assert.NotContains(t, issueLog.Content, issued.Code)

	require.NoError(t, FreezeAdtyVoucher(owner.Id, issued.Id))
	freezeLog := requireOfficialAudit(t, "voucher", strconv.FormatInt(issued.Id, 10), "adty.voucher.freeze")
	require.NotNil(t, freezeLog.SiteId)
	assert.Equal(t, site.Id, *freezeLog.SiteId)
	assert.Contains(t, freezeLog.BeforeValue, `"status":"pending"`)
	assert.Contains(t, freezeLog.AfterValue, `"status":"frozen"`)
	assert.NotContains(t, freezeLog.AfterValue, issued.Code)

	held, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: owner.Id, Purpose: model.AdtyVoucherPurposeSiteCustomer, SiteId: site.Id, AmountUSD: decimal.NewFromInt(2), Reason: "to cancel"})
	require.NoError(t, err)
	require.NoError(t, CancelAdtyVoucher(owner.Id, held.Id))
	cancelLog := requireOfficialAudit(t, "voucher", strconv.FormatInt(held.Id, 10), "adty.voucher.cancel")
	assert.Equal(t, owner.Id, cancelLog.UserId)
	assert.Contains(t, cancelLog.BeforeValue, `"status":"pending"`)
	assert.Contains(t, cancelLog.AfterValue, `"status":"cancelled"`)
	assert.NotContains(t, cancelLog.AfterValue, held.Code)

	live, err := IssueAdtyVoucher(AdtyVoucherIssueInput{IssuerId: owner.Id, Purpose: model.AdtyVoucherPurposeSiteCustomer, SiteId: site.Id, AmountUSD: decimal.NewFromInt(4), Reason: "to redeem"})
	require.NoError(t, err)
	redeemed, err := RedeemAdtyVoucher(customer.Id, live.Code)
	require.NoError(t, err)
	require.False(t, redeemed.Replayed)
	replay, err := RedeemAdtyVoucher(customer.Id, live.Code)
	require.NoError(t, err)
	require.True(t, replay.Replayed)
	var redeemCount int64
	require.NoError(t, officialAuditDB().Model(&model.AuditLog{}).Where("object_type = ? AND object_id = ? AND action = ?", "voucher", strconv.FormatInt(live.Id, 10), "adty.voucher.redeem").Count(&redeemCount).Error)
	require.EqualValues(t, 1, redeemCount)
	redeemLog := requireOfficialAudit(t, "voucher", strconv.FormatInt(live.Id, 10), "adty.voucher.redeem")
	require.NotNil(t, redeemLog.SiteId)
	assert.Equal(t, site.Id, *redeemLog.SiteId)
	assert.Equal(t, customer.Id, redeemLog.UserId)
	assert.Contains(t, redeemLog.AfterValue, `"status":"redeemed"`)
	assert.Contains(t, redeemLog.AfterValue, redeemed.TradeNo)
	assert.NotContains(t, redeemLog.AfterValue, live.Code)
}

func TestAdtyTicketOperationsWriteOfficialAudit(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AdtySite{}, &model.AdtySiteStaff{}, &model.AdtyTeam{}, &model.AdtyTeamMember{}, &model.AuditLog{}))
	require.NoError(t, model.MigrateAdtyTables())
	require.NoError(t, model.MigrateAuditLogs())

	customer := ticketTestUser(t, db, "audit-ticket-customer", common.RoleCommonUser)
	owner := ticketTestUser(t, db, "audit-ticket-owner", common.RoleCommonUser)
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "audit-ticket-site", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Model(customer).Update("site_id", site.Id).Error)
	staff := ticketTestUser(t, db, "audit-ticket-staff", common.RoleCommonUser)
	require.NoError(t, db.Create(&model.AdtySiteStaff{SiteId: site.Id, OwnerUserId: owner.Id, UserId: staff.Id, Role: "support", Status: "enabled", Permissions: `["customers.read"]`}).Error)

	created, err := CreateAdtyTicket(customer.Id, AdtyTicketInput{Type: "billing", Title: "Invoice", Body: "Please check the charge", Idempotency: "audit-create-1"})
	require.NoError(t, err)
	again, err := CreateAdtyTicket(customer.Id, AdtyTicketInput{Type: "billing", Title: "Invoice", Body: "retry", Idempotency: "audit-create-1"})
	require.NoError(t, err)
	assert.Equal(t, created.Id, again.Id)
	var createCount int64
	require.NoError(t, officialAuditDB().Model(&model.AuditLog{}).Where("object_type = ? AND object_id = ? AND action = ?", "ticket", strconv.FormatInt(created.Id, 10), "adty.ticket.create").Count(&createCount).Error)
	require.EqualValues(t, 1, createCount)
	createLog := requireOfficialAudit(t, "ticket", strconv.FormatInt(created.Id, 10), "adty.ticket.create")
	require.NotNil(t, createLog.SiteId)
	assert.Equal(t, site.Id, *createLog.SiteId)
	assert.Equal(t, customer.Id, createLog.UserId)
	assert.Equal(t, "audit-ticket-customer", createLog.Username)
	assert.Empty(t, createLog.ApprovalRef)
	assert.Equal(t, model.AuditResultSuccess, createLog.Result)
	assert.Contains(t, createLog.AfterValue, `"title":"Invoice"`)
	assert.Contains(t, createLog.AfterValue, `"status":"new"`)
	assert.NotContains(t, createLog.AfterValue, "Please check the charge")

	reply, err := ReplyAdtyTicket(int64(customer.Id), created.Id, "more details", "audit-reply-1")
	require.NoError(t, err)
	_, err = ReplyAdtyTicket(int64(customer.Id), created.Id, "more details again", "audit-reply-1")
	require.NoError(t, err)
	var replyCount int64
	require.NoError(t, officialAuditDB().Model(&model.AuditLog{}).Where("object_type = ? AND object_id = ? AND action = ?", "ticket", strconv.FormatInt(created.Id, 10), "adty.ticket.reply").Count(&replyCount).Error)
	require.EqualValues(t, 1, replyCount)
	replyLog := requireOfficialAudit(t, "ticket", strconv.FormatInt(created.Id, 10), "adty.ticket.reply")
	require.NotNil(t, replyLog.SiteId)
	assert.Equal(t, site.Id, *replyLog.SiteId)
	assert.Contains(t, replyLog.AfterValue, `"message_id":`+strconv.FormatInt(reply.Id, 10))
	assert.NotContains(t, replyLog.AfterValue, "more details")

	_, err = NoteAdtyTicket(staff.Id, created.Id, "internal note", "audit-note-1")
	require.NoError(t, err)
	require.NoError(t, officialAuditDB().Model(&model.AuditLog{}).Where("object_type = ? AND object_id = ? AND action = ?", "ticket", strconv.FormatInt(created.Id, 10), "adty.ticket.reply").Count(&replyCount).Error)
	require.EqualValues(t, 1, replyCount, "internal notes must not write a public reply audit")

	require.NoError(t, SetAdtyTicketStatus(staff.Id, created.Id, "", "", staff.Id))
	require.NoError(t, SetAdtyTicketStatus(staff.Id, created.Id, "", "", staff.Id))
	var statusCount int64
	require.NoError(t, officialAuditDB().Model(&model.AuditLog{}).Where("object_type = ? AND object_id = ? AND action = ?", "ticket", strconv.FormatInt(created.Id, 10), "adty.ticket.status").Count(&statusCount).Error)
	require.EqualValues(t, 1, statusCount)
	statusLog := requireOfficialAudit(t, "ticket", strconv.FormatInt(created.Id, 10), "adty.ticket.status")
	require.NotNil(t, statusLog.SiteId)
	assert.Equal(t, site.Id, *statusLog.SiteId)
	assert.Equal(t, staff.Id, statusLog.UserId)
	assert.Contains(t, statusLog.BeforeValue, `"status":"new"`)
	assert.Contains(t, statusLog.AfterValue, `"status":"in_progress"`)
	assert.Contains(t, statusLog.AfterValue, `"assignee_id":`+strconv.Itoa(staff.Id))
}

func TestAdtyGiftAndMarginOperationsWriteOfficialAudit(t *testing.T) {
	db := prepareSiteDB(t)
	owner, site := openActiveSite(t, db, "audit-gift")
	_, err := ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.RequireFromString("4"), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "audit-gift-earn-4", OperatorID: owner.Id,
	})
	require.NoError(t, err)
	require.NoError(t, OpenAdtyMarginCycle(site.Id, decimal.RequireFromString("20"), "audit-gift-cycle"))

	openLog := requireOfficialAudit(t, "margin", strconv.Itoa(site.Id), "adty.margin.margin_cycle_open")
	require.NotNil(t, openLog.SiteId)
	assert.Equal(t, site.Id, *openLog.SiteId)
	assert.Equal(t, model.AuditResultSuccess, openLog.Result)
	assert.True(t, openLog.Success)
	assert.Empty(t, openLog.ApprovalRef)
	assert.Contains(t, openLog.AfterValue, `"type":"margin_cycle_open"`)
	assert.Contains(t, openLog.AfterValue, `"balance":"20"`)

	require.NoError(t, OpenAdtyMarginCycle(site.Id, decimal.RequireFromString("20"), "audit-gift-cycle"))
	var openCount int64
	require.NoError(t, officialAuditDB().Model(&model.AuditLog{}).Where("object_type = ? AND object_id = ? AND action = ?", "margin", strconv.Itoa(site.Id), "adty.margin.margin_cycle_open").Count(&openCount).Error)
	require.EqualValues(t, 1, openCount)

	adty_setting.AuthorizeSiteBonusMarginFill(site.Id, true)
	t.Cleanup(func() { adty_setting.AuthorizeSiteBonusMarginFill(site.Id, false) })
	ext := &model.AdtyTopupOrder{
		TradeNo: "AUDIT-GIFT-1", UserId: owner.Id, SiteId: site.Id,
		BonusUSD: decimal.RequireFromString("10"), Method: "usdt.trc20", BonusMethod: "usdt",
	}
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ReserveAdtySiteGiftTx(tx, ext)
	}))
	var reserveCount int64
	require.NoError(t, officialAuditDB().Model(&model.AuditLog{}).Where("object_type = ? AND object_id = ? AND action = ?", "gift", ext.TradeNo, "adty.gift.reserve").Count(&reserveCount).Error)
	require.EqualValues(t, 1, reserveCount)
	reserveLog := requireOfficialAudit(t, "gift", ext.TradeNo, "adty.gift.reserve")
	require.NotNil(t, reserveLog.SiteId)
	assert.Equal(t, site.Id, *reserveLog.SiteId)
	assert.Equal(t, owner.Id, reserveLog.UserId)
	assert.Contains(t, reserveLog.AfterValue, `"source":"earning+margin"`)
	assert.Contains(t, reserveLog.AfterValue, `"earning":"4"`)
	assert.Contains(t, reserveLog.AfterValue, `"margin":"6"`)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ReleaseAdtySiteGiftTx(tx, ext)
	}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return ReleaseAdtySiteGiftTx(tx, ext)
	}))
	var releaseCount int64
	require.NoError(t, officialAuditDB().Model(&model.AuditLog{}).Where("object_type = ? AND object_id = ? AND action = ?", "gift", ext.TradeNo, "adty.gift.release").Count(&releaseCount).Error)
	require.EqualValues(t, 1, releaseCount)
	releaseLog := requireOfficialAudit(t, "gift", ext.TradeNo, "adty.gift.release")
	require.NotNil(t, releaseLog.SiteId)
	assert.Equal(t, site.Id, *releaseLog.SiteId)
	assert.Contains(t, releaseLog.BeforeValue, `"earning":"4"`)
	assert.Contains(t, releaseLog.AfterValue, `"earning":"0"`)

	_, err = ApplyAdtySiteWalletChange(site.Id, model.AdtySiteWalletEarnings, decimal.RequireFromString("1000"), AdtySiteWalletChange{
		Type: "earning_settlement", IdempotencyKey: "audit-xfer-earn", OperatorID: owner.Id,
	})
	require.NoError(t, err)
	require.NoError(t, TransferAdtyEarningToMargin(site.Id, owner.Id, decimal.RequireFromString("8"), decimal.RequireFromString("8"), "audit-xfer-1"))
	require.NoError(t, TransferAdtyEarningToMargin(site.Id, owner.Id, decimal.RequireFromString("8"), decimal.RequireFromString("8"), "audit-xfer-1"))
	xferLog := requireOfficialAudit(t, "margin", strconv.Itoa(site.Id), "adty.margin.earning_to_margin")
	require.NotNil(t, xferLog.SiteId)
	assert.Equal(t, site.Id, *xferLog.SiteId)
	assert.Equal(t, owner.Id, xferLog.UserId)
	assert.Contains(t, xferLog.AfterValue, `"type":"earning_to_margin"`)
	var xferCount int64
	require.NoError(t, officialAuditDB().Model(&model.AuditLog{}).Where("object_type = ? AND object_id = ? AND action = ?", "margin", strconv.Itoa(site.Id), "adty.margin.earning_to_margin").Count(&xferCount).Error)
	require.EqualValues(t, 1, xferCount)
}

func officialAuditDB() *gorm.DB {
	if model.LOG_DB != nil {
		return model.LOG_DB
	}
	return model.DB
}

func requireOfficialAudit(t *testing.T, objectType, objectID, action string) model.AuditLog {
	t.Helper()
	var log model.AuditLog
	require.NoError(t, officialAuditDB().Where("object_type = ? AND object_id = ? AND action = ?", objectType, objectID, action).Take(&log).Error)
	return log
}
