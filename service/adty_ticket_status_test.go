package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestAdtyTicketOfficialStatusMachineAndWorkbenchQueues(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AdtySite{}, &model.AdtySiteStaff{}, &model.AdtyTeam{}, &model.AdtyTeamMember{}))
	require.NoError(t, model.MigrateAdtyTables())

	customer := ticketTestUser(t, db, "st-customer", common.RoleCommonUser)
	owner := ticketTestUser(t, db, "st-owner", common.RoleCommonUser)
	site := model.AdtySite{OwnerUserId: owner.Id, Subdomain: "ticket-status-site", Status: model.AdtySiteStatusActive}
	require.NoError(t, db.Create(&site).Error)
	require.NoError(t, db.Model(customer).Update("site_id", site.Id).Error)
	staff := ticketTestUser(t, db, "st-staff", common.RoleCommonUser)
	require.NoError(t, db.Create(&model.AdtySiteStaff{SiteId: site.Id, OwnerUserId: owner.Id, UserId: staff.Id, Role: "support", Status: "enabled", Permissions: `["customers.read"]`}).Error)

	created, err := CreateAdtyTicket(customer.Id, AdtyTicketInput{Type: "billing", Title: "Invoice", Body: "Please check the charge", Idempotency: "create-1"})
	require.NoError(t, err)
	require.Equal(t, model.AdtyTicketNew, created.Status)
	require.Zero(t, created.AssigneeId)

	again, err := CreateAdtyTicket(customer.Id, AdtyTicketInput{Type: "billing", Title: "Invoice", Body: "retry", Idempotency: "create-1"})
	require.NoError(t, err)
	assert.Equal(t, created.Id, again.Id)
	assert.Equal(t, model.AdtyTicketNew, again.Status)

	pending, err := CreateAdtyTicket(customer.Id, AdtyTicketInput{Type: "api", Title: "Timeout", Body: "requests fail", Idempotency: "create-2"})
	require.NoError(t, err)
	require.NoError(t, SetAdtyTicketStatus(staff.Id, pending.Id, model.AdtyTicketPendingAssignment, "", 0))
	require.Equal(t, model.AdtyTicketPendingAssignment, loadTicket(t, db, pending.Id).Status)

	require.NoError(t, SetAdtyTicketStatus(staff.Id, created.Id, "", "", staff.Id))
	handling := loadTicket(t, db, created.Id)
	require.Equal(t, model.AdtyTicketInProgress, handling.Status)
	require.Equal(t, staff.Id, handling.AssigneeId)

	transferred, err := CreateAdtyTicket(customer.Id, AdtyTicketInput{Type: "account", Title: "Login", Body: "cannot sign in", Idempotency: "create-3"})
	require.NoError(t, err)
	require.NoError(t, SetAdtyTicketStatus(owner.Id, transferred.Id, model.AdtyTicketPendingAssignment, "", 0))
	require.NoError(t, SetAdtyTicketStatus(owner.Id, transferred.Id, "", "", staff.Id))
	require.Equal(t, model.AdtyTicketInProgress, loadTicket(t, db, transferred.Id).Status)

	require.ErrorIs(t, SetAdtyTicketStatus(staff.Id, pending.Id, model.AdtyTicketInProgress, "", 0), ErrAdtyTicketInvalid)
	require.Equal(t, model.AdtyTicketPendingAssignment, loadTicket(t, db, pending.Id).Status)
	require.ErrorIs(t, SetAdtyTicketStatus(staff.Id, pending.Id, model.AdtyTicketNew, "", staff.Id), ErrAdtyTicketInvalid)
	require.ErrorIs(t, SetAdtyTicketStatus(staff.Id, pending.Id, model.AdtyTicketPendingAssignment, "", staff.Id), ErrAdtyTicketInvalid)
	require.ErrorIs(t, SetAdtyTicketStatus(staff.Id, created.Id, model.AdtyTicketPendingAssignment, "", 0), ErrAdtyTicketInvalid)

	require.ErrorIs(t, SetAdtyTicketStatus(customer.Id, created.Id, model.AdtyTicketWaitingUser, "", 0), ErrAdtyTicketForbidden)
	require.NoError(t, SetAdtyTicketStatus(staff.Id, created.Id, model.AdtyTicketWaitingUser, "", 0))
	require.Equal(t, model.AdtyTicketWaitingUser, loadTicket(t, db, created.Id).Status)
	require.NoError(t, SetAdtyTicketStatus(staff.Id, created.Id, model.AdtyTicketWaitingInternal, "", 0))
	require.Equal(t, model.AdtyTicketWaitingInternal, loadTicket(t, db, created.Id).Status)
	require.NoError(t, SetAdtyTicketStatus(staff.Id, created.Id, model.AdtyTicketInProgress, "", 0))
	require.Equal(t, model.AdtyTicketInProgress, loadTicket(t, db, created.Id).Status)
	require.NoError(t, SetAdtyTicketStatus(staff.Id, created.Id, "resolve", "", 0))
	require.Equal(t, model.AdtyTicketResolved, loadTicket(t, db, created.Id).Status)
	require.NoError(t, SetAdtyTicketStatus(staff.Id, created.Id, "close", "", 0))
	require.Equal(t, model.AdtyTicketClosed, loadTicket(t, db, created.Id).Status)

	require.ErrorIs(t, SetAdtyTicketStatus(staff.Id, created.Id, model.AdtyTicketWaitingUser, "", 0), ErrAdtyTicketInvalid)
	require.ErrorIs(t, SetAdtyTicketStatus(staff.Id, created.Id, "", "", owner.Id), ErrAdtyTicketInvalid)

	require.NoError(t, SetAdtyTicketStatus(customer.Id, created.Id, "reopen", "", 0))
	reopened := loadTicket(t, db, created.Id)
	require.Equal(t, model.AdtyTicketInProgress, reopened.Status, "reopen keeps the remaining assignee and lands in 处理中")
	require.Equal(t, staff.Id, reopened.AssigneeId)
	var reopenEvents int64
	require.NoError(t, db.Model(&model.AdtyTicketEvent{}).Where("ticket_id = ? AND action = ?", created.Id, "reopen").Count(&reopenEvents).Error)
	require.EqualValues(t, 1, reopenEvents)
	require.NoError(t, SetAdtyTicketStatus(customer.Id, created.Id, "reopen", "", 0), "immediate reopen retry is idempotent")
	require.NoError(t, db.Model(&model.AdtyTicketEvent{}).Where("ticket_id = ? AND action = ?", created.Id, "reopen").Count(&reopenEvents).Error)
	require.EqualValues(t, 1, reopenEvents)
	require.Equal(t, model.AdtyTicketInProgress, loadTicket(t, db, created.Id).Status)

	require.NoError(t, SetAdtyTicketStatus(staff.Id, created.Id, "resolve", "", 0))
	require.NoError(t, SetAdtyTicketStatus(staff.Id, created.Id, "resolve", "", 0))
	var resolveEvents int64
	require.NoError(t, db.Model(&model.AdtyTicketEvent{}).Where("ticket_id = ? AND action = ? AND detail = ?", created.Id, "status", model.AdtyTicketResolved).Count(&resolveEvents).Error)
	require.EqualValues(t, 2, resolveEvents)

	unassignedClosed, err := CreateAdtyTicket(customer.Id, AdtyTicketInput{Type: "other", Title: "Closed", Body: "close me", Idempotency: "create-4"})
	require.NoError(t, err)
	require.NoError(t, SetAdtyTicketStatus(staff.Id, unassignedClosed.Id, "close", "", 0))
	require.NoError(t, SetAdtyTicketStatus(staff.Id, unassignedClosed.Id, "reopen", "", 0))
	require.Equal(t, model.AdtyTicketPendingAssignment, loadTicket(t, db, unassignedClosed.Id).Status)
	require.ErrorIs(t, SetAdtyTicketStatus(staff.Id, pending.Id, "reopen", "", 0), ErrAdtyTicketInvalid)

	urgent, err := CreateAdtyTicket(customer.Id, AdtyTicketInput{Type: "billing", Title: "Urgent bill", Body: "need help now", Priority: model.AdtyTicketPriorityUrgent, Idempotency: "create-5"})
	require.NoError(t, err)
	require.Equal(t, model.AdtyTicketNew, urgent.Status)

	historicalOpen := model.AdtyTicket{SiteId: site.Id, UserId: customer.Id, Title: "legacy open", Status: model.AdtyTicketLegacyOpen, Priority: model.AdtyTicketPriorityNormal}
	require.NoError(t, db.Create(&historicalOpen).Error)
	historicalAssigned := model.AdtyTicket{SiteId: site.Id, UserId: customer.Id, Title: "legacy assigned", Status: model.AdtyTicketLegacyAssigned, AssigneeId: staff.Id, Priority: model.AdtyTicketPriorityNormal}
	require.NoError(t, db.Create(&historicalAssigned).Error)
	historicalOpenAssigned := model.AdtyTicket{SiteId: site.Id, UserId: customer.Id, Title: "legacy open assigned", Status: model.AdtyTicketLegacyOpen, AssigneeId: staff.Id, Priority: model.AdtyTicketPriorityHigh}
	require.NoError(t, db.Create(&historicalOpenAssigned).Error)

	list, err := ListAdtyTickets(staff.Id, "")
	require.NoError(t, err)
	assert.True(t, list.CanManage)
	assert.True(t, list.CanNote)
	byID := map[int64]model.AdtyTicket{}
	for _, row := range list.Tickets {
		byID[row.Id] = row
		assert.NotEqual(t, model.AdtyTicketLegacyOpen, row.Status)
		assert.NotEqual(t, model.AdtyTicketLegacyAssigned, row.Status)
	}
	require.Equal(t, model.AdtyTicketNew, byID[historicalOpen.Id].Status)
	require.Equal(t, model.AdtyTicketInProgress, byID[historicalAssigned.Id].Status)
	require.Equal(t, model.AdtyTicketInProgress, byID[historicalOpenAssigned.Id].Status)
	require.Equal(t, model.AdtyTicketNew, byID[urgent.Id].Status)
	require.Equal(t, model.AdtyTicketPendingAssignment, byID[pending.Id].Status)

	require.EqualValues(t, 2, list.Badges.New, "新建: urgent + leftover unassigned open")
	require.EqualValues(t, 2, list.Badges.PendingAssignment, "待分配: pending + unassigned reopen")
	require.EqualValues(t, 3, list.Badges.InProgress, "处理中: transferred + two leftover assigned rows")
	require.EqualValues(t, 4, list.Badges.Pending, "待处理 queue includes 新建 and 待分配")
	require.EqualValues(t, 1, list.Badges.Urgent)
	require.Zero(t, list.Badges.WaitingUser)
	require.Zero(t, list.Badges.WaitingInternal)
	require.NotEqual(t, list.Badges.PendingAssignment, list.Badges.InProgress)
	require.NotEqual(t, list.Badges.New, list.Badges.InProgress)

	pendingQueue, err := ListAdtyTickets(staff.Id, "pending")
	require.NoError(t, err)
	pendingIDs := ticketIDs(pendingQueue.Tickets)
	assert.Contains(t, pendingIDs, urgent.Id)
	assert.Contains(t, pendingIDs, pending.Id)
	assert.Contains(t, pendingIDs, historicalOpen.Id)
	assert.Contains(t, pendingIDs, unassignedClosed.Id)
	assert.NotContains(t, pendingIDs, transferred.Id)
	assert.NotContains(t, pendingIDs, historicalAssigned.Id)
	assert.Equal(t, list.Badges, pendingQueue.Badges, "badge counts stay full-scope when a queue is selected")

	progressQueue, err := ListAdtyTickets(staff.Id, "in_progress")
	require.NoError(t, err)
	progressIDs := ticketIDs(progressQueue.Tickets)
	assert.Contains(t, progressIDs, transferred.Id)
	assert.Contains(t, progressIDs, historicalAssigned.Id)
	assert.Contains(t, progressIDs, historicalOpenAssigned.Id)
	assert.NotContains(t, progressIDs, pending.Id)
	assert.NotContains(t, progressIDs, urgent.Id)

	urgentQueue, err := ListAdtyTickets(staff.Id, "urgent")
	require.NoError(t, err)
	assert.Equal(t, []int64{urgent.Id}, ticketIDs(urgentQueue.Tickets))

	customerList, err := ListAdtyTickets(customer.Id, "pending")
	require.NoError(t, err)
	assert.False(t, customerList.CanManage)
	assert.Contains(t, ticketIDs(customerList.Tickets), urgent.Id)

	require.NoError(t, model.RemapAdtyTicketLegacyStatuses(db))
	require.Equal(t, model.AdtyTicketNew, loadTicket(t, db, historicalOpen.Id).Status)
	require.Equal(t, model.AdtyTicketInProgress, loadTicket(t, db, historicalAssigned.Id).Status)
	require.Equal(t, model.AdtyTicketInProgress, loadTicket(t, db, historicalOpenAssigned.Id).Status)
	var leftovers int64
	require.NoError(t, db.Model(&model.AdtyTicket{}).Where("status IN ?", []string{model.AdtyTicketLegacyOpen, model.AdtyTicketLegacyAssigned}).Count(&leftovers).Error)
	assert.Zero(t, leftovers)
	require.NoError(t, model.RemapAdtyTicketLegacyStatuses(db))
	require.Equal(t, model.AdtyTicketNew, loadTicket(t, db, historicalOpen.Id).Status)
}

func ticketTestUser(t *testing.T, db *gorm.DB, name string, role int) *model.User {
	t.Helper()
	user := &model.User{Username: name, Password: "hashed-password", Role: role, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, Email: name + "@example.com"}
	require.NoError(t, db.Create(user).Error)
	return user
}

func loadTicket(t *testing.T, db *gorm.DB, id int64) model.AdtyTicket {
	t.Helper()
	var ticket model.AdtyTicket
	require.NoError(t, db.First(&ticket, id).Error)
	return ticket
}

func ticketIDs(rows []model.AdtyTicket) []int64 {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.Id)
	}
	return ids
}
