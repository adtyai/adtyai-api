package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ISS-R4-0012：工单的四个关联字段和 Title/Body 一样是用户手输的自由文本。
// 修前 related_* 原样落库，related_ref 还会因为命中黑名单被拒单；修后一律
// 先脱敏再存，且不再拒单。
func TestAdtyTicketRedactsRelatedFields(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AdtySite{}, &model.AdtySiteStaff{}, &model.AdtyTeam{}, &model.AdtyTeamMember{}))
	require.NoError(t, model.MigrateAdtyTables())

	customer := ticketTestUser(t, db, "redact-customer", common.RoleCommonUser)

	// related_error 是最常夹带 key 的字段：用户贴报错时会把请求体或 header 一起带上。
	created, err := CreateAdtyTicket(customer.Id, AdtyTicketInput{
		Type:         "api",
		Title:        "Upstream rejects the call",
		Body:         "See the attached error text",
		RelatedError: "401 upstream: api_key=sk-abcdefgh12345678 rejected",
		RelatedModel: "gpt-4o sk-abcdefgh12345678",
		RelatedGroup: "default password=hunter2",
		Idempotency:  "redact-1",
	})
	require.NoError(t, err)

	var stored model.AdtyTicket
	require.NoError(t, db.First(&stored, created.Id).Error)
	assert.NotContains(t, stored.RelatedError, "sk-abcdefgh12345678")
	assert.Contains(t, stored.RelatedError, "[redacted]")
	assert.NotContains(t, stored.RelatedModel, "sk-abcdefgh12345678")
	assert.NotContains(t, stored.RelatedGroup, "hunter2")
}

// related_ref 修前是「命中黑名单就拒单」。拒单既没告诉用户哪句有问题，也挡住了
// 正常建单；修后和其它字段一样脱敏落库。
func TestAdtyTicketAcceptsRelatedRefThatLooksLikeASecret(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AdtySite{}, &model.AdtySiteStaff{}, &model.AdtyTeam{}, &model.AdtyTeamMember{}))
	require.NoError(t, model.MigrateAdtyTables())

	customer := ticketTestUser(t, db, "redact-ref-customer", common.RoleCommonUser)
	created, err := CreateAdtyTicket(customer.Id, AdtyTicketInput{
		Type:        "billing",
		Title:       "Order not credited",
		Body:        "Paid but quota missing",
		RelatedRef:  "sk-abcdefgh12345678",
		Idempotency: "redact-ref-1",
	})
	require.NoError(t, err, "命中敏感串的 related_ref 不应拒单，应脱敏后落库")

	var stored model.AdtyTicket
	require.NoError(t, db.First(&stored, created.Id).Error)
	assert.NotContains(t, stored.RelatedRef, "sk-abcdefgh12345678")
	assert.Contains(t, stored.RelatedRef, "[redacted]")
}
