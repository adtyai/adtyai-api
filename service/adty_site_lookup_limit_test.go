package service

import (
	"errors"
	"sync"
	"testing"

	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestOwnerCustomerLookupLimitHoldsUnderConcurrency(t *testing.T) {
	db := prepareSiteDB(t)
	cfg := resellerSetting()
	cfg.CustomerLookupPerHour = 1
	_, site := openActiveSite(t, db, "lookup")

	const n = 8
	var wg sync.WaitGroup
	limited := make(chan struct{}, n)
	for range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := LookupAdtyOwnerCustomer(site, site.OwnerUserId, "nobody-exact")
			if errors.Is(err, ErrAdtySiteLookupLimited) {
				limited <- struct{}{}
			}
		}()
	}
	wg.Wait()
	close(limited)
	got := 0
	for range limited {
		got++
	}
	require.Equal(t, n-1, got)
	var rows int64
	require.NoError(t, db.Model(&model.AdtySiteLookup{}).Where("site_id = ?", site.Id).Count(&rows).Error)
	require.Equal(t, int64(1), rows)
}
