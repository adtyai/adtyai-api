package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaymentScenesRejectRetiredHostedProviders(t *testing.T) {
	db := prepareReferralDialectDB(t, "postgres")
	require.NoError(t, db.AutoMigrate(&model.AuditIntent{}))
	root := referralUser(t, db, "retired-scene-root")
	require.NoError(t, db.Model(root).Update("role", common.RoleRootUser).Error)
	admin := referralUser(t, db, "retired-scene-admin")
	require.NoError(t, db.Model(admin).Update("role", common.RoleAdminUser).Error)

	view, err := GetAdtyPaymentScenes()
	require.NoError(t, err)
	require.NotEmpty(t, view.ProviderOptions)
	for _, option := range view.ProviderOptions {
		assert.True(t, model.IsOperableAdtyPaymentProvider(option.Provider), option.Provider)
		assert.False(t, model.IsRetiredHostedPaymentProvider(option.Provider), option.Provider)
	}
	for _, option := range view.MethodOptions {
		assert.NotContains(t, []string{"stripe", "creem", "waffo", "waffo_pancake"}, option.Id)
	}

	scene := view.Scenes[0]
	scene.Key, scene.Name = "wallet_topup", "Wallet top-up"
	scene.SiteScope = "all"
	scene.Business = "wallet_topup"
	for _, provider := range []string{
		model.PaymentProviderStripe,
		model.PaymentProviderCreem,
		model.PaymentProviderWaffo,
		model.PaymentProviderWaffoPancake,
	} {
		scene.Providers = []string{provider}
		_, err = SaveAdtyPaymentScenes(root.Id, view.Version, []model.AdtyPaymentScene{scene}, model.AuditLog{})
		require.ErrorIs(t, err, model.ErrRetiredHostedPaymentChannel, provider)
	}

	mixed := scene
	mixed.Providers = []string{model.PaymentProviderAdtyBEpusdt, model.PaymentProviderStripe}
	_, err = SaveAdtyPaymentScenes(root.Id, view.Version, []model.AdtyPaymentScene{mixed}, model.AuditLog{})
	require.ErrorIs(t, err, model.ErrRetiredHostedPaymentChannel)

	official := scene
	official.Providers = []string{model.PaymentProviderAdtyBEpusdt, model.PaymentProviderAdtyEpay}
	official.Methods = []string{}
	_, err = SaveAdtyPaymentScenes(admin.Id, view.Version, []model.AdtyPaymentScene{official}, model.AuditLog{})
	require.Error(t, err)
	assert.NotErrorIs(t, err, model.ErrAdtyPaymentSceneConflict)

	saved, err := SaveAdtyPaymentScenes(root.Id, view.Version, []model.AdtyPaymentScene{official}, model.AuditLog{})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{model.PaymentProviderAdtyBEpusdt, model.PaymentProviderAdtyEpay}, saved.Scenes[0].Providers)
	for _, option := range saved.ProviderOptions {
		assert.True(t, model.IsOperableAdtyPaymentProvider(option.Provider), option.Provider)
	}
}
