package service

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// 工单回复的推送方向：作者是谁决定通知谁，作者本人永远不进队列。
// 六面清单里只有这两面（客户回帖→处理人、客服回帖→客户）走这条代码路径。
func TestAdtyTicketReplyNotifiesTheCounterpart(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AdtySite{}, &model.AdtySiteStaff{}, &model.AdtyTeam{}, &model.AdtyTeamMember{}))
	require.NoError(t, model.MigrateAdtyTables())

	customer := ticketTestUser(t, db, "notify-customer", common.RoleCommonUser)
	owner := ticketTestUser(t, db, "notify-owner", common.RoleCommonUser)
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "notify-ticket-site", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Model(customer).Update("site_id", site.Id).Error)
	staff := ticketTestUser(t, db, "notify-staff", common.RoleCommonUser)
	require.NoError(t, db.Create(&model.AdtySiteStaff{SiteId: site.Id, OwnerUserId: owner.Id, UserId: staff.Id, Role: "support", Status: "enabled", Permissions: `["customers.read"]`}).Error)

	ticket, err := CreateAdtyTicket(customer.Id, AdtyTicketInput{Type: "billing", Title: "Invoice", Body: "please check the charge", Idempotency: "notify-create-1"})
	require.NoError(t, err)
	require.NoError(t, SetAdtyTicketStatus(staff.Id, ticket.Id, "", "", staff.Id))
	require.Equal(t, staff.Id, loadTicket(t, db, ticket.Id).AssigneeId)

	// 客户回帖 → 处理人，admin purpose；作者本人不收到自己触发的提醒。
	customerReply, err := ReplyAdtyTicket(int64(customer.Id), ticket.Id, "any progress?", "notify-customer-1")
	require.NoError(t, err)
	toHandler := requireTicketNotice(t, db, ticket.Id, customerReply.Id)
	assert.Equal(t, staff.Id, toHandler.UserId, "客户回帖要落到处理人，而不是客户自己")
	assert.Equal(t, model.AdtyTelegramPurposeAdmin, toHandler.Purpose)
	assert.NotEqual(t, customer.Id, toHandler.UserId, "客户不得收到自己回帖的提醒")

	// 客服回帖 → 客户，user purpose。
	staffReply, err := ReplyAdtyTicket(int64(staff.Id), ticket.Id, "we are on it", "notify-staff-1")
	require.NoError(t, err)
	toCustomer := requireTicketNotice(t, db, ticket.Id, staffReply.Id)
	assert.Equal(t, customer.Id, toCustomer.UserId)
	assert.Equal(t, model.AdtyTelegramPurposeUser, toCustomer.Purpose)

	// 内部备注不推送：它不是对客户说的话。
	note, err := NoteAdtyTicket(staff.Id, ticket.Id, "internal only", "notify-note-1")
	require.NoError(t, err)
	var noteMessages []model.AdtyTicketMessage
	require.NoError(t, db.Where("ticket_id = ? AND visibility = ?", ticket.Id, model.AdtyTicketVisibilityNote).Find(&noteMessages).Error)
	require.Len(t, noteMessages, 1)
	assertTicketNoticeCount(t, db, ticket.Id, note.Id, 0)

	// 未分配的站内工单：客户回帖落站长，仍然不是客户自己。
	unassigned, err := CreateAdtyTicket(customer.Id, AdtyTicketInput{Type: "api", Title: "Timeout", Body: "requests fail", Idempotency: "notify-create-2"})
	require.NoError(t, err)
	unassignedReply, err := ReplyAdtyTicket(int64(customer.Id), unassigned.Id, "still failing", "notify-customer-2")
	require.NoError(t, err)
	toOwner := requireTicketNotice(t, db, unassigned.Id, unassignedReply.Id)
	assert.Equal(t, owner.Id, toOwner.UserId, "未分配的站内工单由站长接手")
	assert.Equal(t, model.AdtyTelegramPurposeAdmin, toOwner.Purpose)
	assert.NotEqual(t, customer.Id, toOwner.UserId)
}

func ticketNoticeKey(ticketId, messageId int64) string {
	return "ticket:" + strconv.FormatInt(ticketId, 10) + ":msg:" + strconv.FormatInt(messageId, 10)
}

func assertTicketNoticeCount(t *testing.T, db *gorm.DB, ticketId, messageId int64, want int64) {
	t.Helper()
	var got int64
	require.NoError(t, db.Model(&model.AdtyTelegramDelivery{}).Where("event_key = ?", ticketNoticeKey(ticketId, messageId)).Count(&got).Error)
	assert.EqualValues(t, want, got)
}

func requireTicketNotice(t *testing.T, db *gorm.DB, ticketId, messageId int64) model.AdtyTelegramDelivery {
	t.Helper()
	assertTicketNoticeCount(t, db, ticketId, messageId, 1)
	var row model.AdtyTelegramDelivery
	require.NoError(t, db.Where("event_key = ?", ticketNoticeKey(ticketId, messageId)).First(&row).Error)
	return row
}
