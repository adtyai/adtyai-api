#!/usr/bin/env bash
# Closed-loop checks that do not need an external payment, mail, or supplier account.
# A missing ADTY_TEST_PG_DSN is a failure, not a skip.
set -euo pipefail
if [ -z "${ADTY_TEST_PG_DSN:-}" ]; then
  echo "e2e: ADTY_TEST_PG_DSN is unset" >&2
  exit 1
fi
export GOWORK=off
go test ./model -count=1 -timeout 20m -run 'TestCompleteAdtyTopUpCreditsFaceAndBonusOnce|TestEpayDailyLimitCountsPendingReservations|TestReplacedPendingOrderIsNotAutoCredited'
go test ./router -count=1 -timeout 20m -run 'TestAdminReferralAdjustIsIdempotent|TestUserWriteAccessTokenCannotAdjustQuotaOrPromote'
echo "e2e: closed-loop checks passed"
