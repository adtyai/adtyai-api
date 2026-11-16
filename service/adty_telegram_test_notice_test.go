package service

import (
	"strconv"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each bot test is its own delivery. The old fixed event key turned every
// test after the first into a silent duplicate, so the button stopped working.
func TestAdtyTelegramBotTestQueuesANewDeliveryEachTime(t *testing.T) {
	t.Setenv("CRYPTO_SECRET", "synthetic-encryption-key-for-local-tests-only")
	t.Setenv("ADTY_TELEGRAM_ALLOW_NETWORK", "")
	oldGet := AdtyTelegramGetMe
	AdtyTelegramGetMe = func(string) (int64, string, error) { return 0, "", ErrAdtyTelegramUnverified }
	t.Cleanup(func() { AdtyTelegramGetMe = oldGet })

	db := prepareReferralDialectDB(t, "sqlite")
	root := referralUser(t, db, "tg-root")
	_, err := SaveAdtyTelegramBot(model.AdtyTelegramPurposeAdmin, "424242:ABCDEFGHIJKLMNOPQRSTUV", true)
	require.NoError(t, err)

	require.NoError(t, TestAdtyTelegramBot(model.AdtyTelegramPurposeAdmin, root.Id))
	require.NoError(t, TestAdtyTelegramBot(model.AdtyTelegramPurposeAdmin, root.Id))

	var deliveries int64
	require.NoError(t, db.Model(&model.AdtyTelegramDelivery{}).
		Where("event_key LIKE ?", "telegram-test:admin:"+strconv.Itoa(root.Id)+":%").Count(&deliveries).Error)
	assert.EqualValues(t, 2, deliveries)
	var bot model.AdtyTelegramBot
	require.NoError(t, db.Where("purpose = ?", model.AdtyTelegramPurposeAdmin).First(&bot).Error)
	assert.NotZero(t, bot.TestedAt)
}
