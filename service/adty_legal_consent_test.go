package service

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/stretchr/testify/assert"
)

func TestMainLegalConsentFollowsPublishedText(t *testing.T) {
	legal := system_setting.GetLegalSettings()
	savedAgreement, savedPrivacy := legal.UserAgreement, legal.PrivacyPolicy
	t.Cleanup(func() {
		legal.UserAgreement, legal.PrivacyPolicy = savedAgreement, savedPrivacy
	})
	legal.UserAgreement, legal.PrivacyPolicy = "", ""
	assert.False(t, AdtyMainLegalConsentRequired())
	legal.UserAgreement = "current terms"
	assert.True(t, AdtyMainLegalConsentRequired())
}
