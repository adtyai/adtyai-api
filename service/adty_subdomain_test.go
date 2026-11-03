package service

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/adty_setting"
	"github.com/stretchr/testify/require"
)

func TestSubdomainRejectsPunycodeAndInfrastructureLabels(t *testing.T) {
	cfg := adty_setting.GetResellerSetting()
	saved := *cfg
	t.Cleanup(func() { *adty_setting.GetResellerSetting() = saved })
	cfg.RootDomains = []string{"sites.example.com"}
	cfg.ReservedSubdomains = []string{"www"}
	cfg.CnameTarget = "customers.sites.example.com"
	cfg.FallbackOrigin = "origin.sites.example.com"

	_, err := AdtyNormalizeSubdomain("XN--e1a")
	require.ErrorIs(t, err, ErrAdtySiteSubdomainBad)
	_, err = AdtyNormalizeSubdomain("customers")
	require.ErrorIs(t, err, ErrAdtySiteSubdomainUsed)
	_, err = AdtyNormalizeSubdomain("origin")
	require.ErrorIs(t, err, ErrAdtySiteSubdomainUsed)
	_, err = AdtyNormalizeSubdomain("www")
	require.ErrorIs(t, err, ErrAdtySiteSubdomainUsed)

	got, err := AdtyNormalizeSubdomain(" Shop ")
	require.NoError(t, err)
	require.Equal(t, "shop", got)
}
