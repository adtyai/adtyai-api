#!/usr/bin/env bash
# topup bonus accept (cloud PG)
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck disable=SC1091
source "${SCRIPT_DIR}/_accept_env.sh"
# A13 static: CompleteTopupOrder must not call increaseUserQuotaTx directly
if grep -n 'increaseUserQuotaTx' model/topup_order.go; then
  echo "FAIL A13: model/topup_order.go still references increaseUserQuotaTx" >&2
  exit 1
fi
# A5 static: ApproveRiskReview must not write bonus / ledger directly
if grep -nE 'writeLedgerEntryTx|UserBonusWallet\{\}\)\.Where|Update\("balance"' model/risk_review.go; then
  echo "FAIL A5: model/risk_review.go still directly updates bonus or writeLedgerEntryTx" >&2
  exit 1
fi
go run ./scripts/accept_topup_bonus.go
