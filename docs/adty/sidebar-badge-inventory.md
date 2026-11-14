# Sidebar badge口径 register (A.19 / UI-007)

This is an in-repo engineering register for functional badges, not a
user-facing marketing document and not a change to the frozen plan.
It records, per module, the six A.19 fields:

- 待办定义
- 数据来源
- 角色范围
- 计数规则
- 清零条件
- 刷新机制

Numeric count口径 that official rules have not confirmed are listed
in part B and must not be invented or implemented as badges (A.19:
具体角标数字口径尚未确认的，不得自行编造). Kinds in part B are
absent from the badge API. This register does not treat the existing
five-minute referral sidebar refetch as a newly signed official
interval (ISS-R1-0050 leftover) and does not fold later leftovers
(ISS-R1-0034 G.13.6 freeze path, ISS-R1-0059 hidden-notice audit,
retired self-apply site flows, official payment-route rewrites).

Refresh mechanism that applies to every confirmed kind below, unless
a row says otherwise:

- Recomputed from live backend data on each successful badge request
  (normal load / refetch).
- After an official state change (approval, payout, cover, mark-read,
  ticket status, disposition review, and similar existing mutations)
  the official client invalidates the badge query so the next read
  matches list/detail/stats (LIST-012, UI-009).
- Opening a menu or fetching badges does not clear a business todo.
- A query or authorization failure must not be presented as a
  successful zero or as a stale number (LIST-012).
- Poll interval is not a newly signed official rule.

## Permission matrix (including team_id)

| Actor | Scope used for badges | team_id |
| --- | --- | --- |
| Root | Global, for kinds Root is the official reviewer or workbench operator of. | Ticket badges include every team. Notifications have no team_id column; see B. |
| Non-root platform admin | Main site only (`users.site_id = 0` / `site_id = 0`). Kinds still require the corresponding admin permission when the permission system is on (finance read, referral read). Unauthorized kinds are omitted, not returned as zero (EMP-017). | Tickets on the main site, including those tickets' `team_id`. |
| Site owner | This site only. | Tickets for this site, including those tickets' `team_id`. |
| Site staff (enabled) | Authorized site only, and only kinds the staff grant actually covers. Disabled or unauthorized staff must not learn that other sites' work exists or how many items there are (EMP-017). | Tickets for the authorized site, including those tickets' `team_id`. |
| Team leader | Self plus the active team (`team_id` of the single active membership). | Ticket scope is `user_id = self OR team_id = this team`. Notification team scope is unconfirmed (part B). |
| Team member | Self only. | Other members' tickets in the same team are not counted. Notification team scope is unconfirmed (part B). |
| Ordinary user | Self only. | Not a team actor. |

Notification unread stays a distinct field from finance, order, and
ticket counts (UI-010). The top bar continues to show the existing
inbox unread cue; sidebar finance/order/ticket badges must not add
that number in.

---

## A. Confirmed口径 (official rules or already landed official states)

### A.1 通知未读 (`notification_unread`)

- 待办定义: Unread official notification publications visible to this
  account on the current site, using the existing inbox unread count
  (`unread_publications`). The top-bar hook may also include the
  current notice / announcement extras already handled by
  `useNotifications`; this API field does not invent a new combined
  formula and does not replace the top bar.
- 数据来源: `GetAdtyNotificationInbox` / the same visibility and
  receipt query (`adty_notification_publications` vs
  `adty_notification_reads` for `user_id` + current `site_id`).
- 角色范围: Every signed-in user, self, current site. No cross-account
  leak. Team_id is not on publications; see B.3.
- 计数规则: The existing `unread_publications` integer. Hidden-notice
  audit adjustments are ISS-R1-0059 and are not folded here.
- 清零条件: Those notices are marked read through the official read
  API. Opening a menu or the badge endpoint does not mark them read.
- 刷新机制: Common rule above. Distinct from finance/order/ticket
  badges.

### A.2 推广合作伙伴申请 (`referral_partners`)

- 待办定义: Platform referral partner applications waiting for review
  (`ReferralPartnerStatusPending`).
- 数据来源: `referral_affiliates.partner_status`, scoped through
  `users.id = referral_affiliates.user_id` and `users.site_id`.
- 角色范围: Root = global. Non-root admin with referral read = main
  site only. Site owners, staff, and users do not receive this kind
  (platform referral admin is not their reviewer role).
- 计数规则: Number of pending partner rows in scope.
- 清零条件: The application leaves pending (approved or rejected).
  Opening the referral menu does not clear it.
- 刷新机制: Common rule. `GetReferralAdminBadges` uses this same
  scoped count and is no longer an unscoped global sum.

### A.3 推广提现待审核 (`referral_withdrawals_pending`)

- 待办定义: Platform referral withdrawals in official
  `ReferralWithdrawalStatusPending`.
- 数据来源: `referral_withdrawals.status`, scoped through
  `users.id = referral_withdrawals.user_id` and `users.site_id`.
- 角色范围: Same as A.2. Not merged with site earning withdrawals or
  site-invite commission withdrawals.
- 计数规则: Number of pending rows in scope.
- 清零条件: Review moves the row out of pending (approved, rejected,
  or canceled). Opening the menu does not clear it.
- 刷新机制: Common rule.

### A.4 推广提现待打款 (`referral_withdrawals_approved`)

- 待办定义: Platform referral withdrawals already approved and waiting
  for official payout (`ReferralWithdrawalStatusApproved`). This is
  the existing referral-admin badge slice, not a new policy.
- 数据来源 / 角色范围 / 计数规则: Same scoping as A.3, counting
  approved rows.
- 清零条件: Paid, or otherwise leaving approved. Opening the menu
  does not clear it.
- 刷新机制: Common rule. The referral admin sidebar badge may show
  A.2+A.3+A.4 together because they are one referral-admin module,
  as the existing badge already did.

### A.5 分站收益提现待审核 (`site_withdrawals_pending`)

- 待办定义: Site-owner earning withdrawals in official
  `AdtySiteWithdrawPending`. Root is the official reviewer
  (`/api/adty/admin/site-withdrawals/...`).
- 数据来源: `adty_site_withdrawals.status`.
- 角色范围: Root = global. Site owners are applicants, not reviewers,
  so they do not receive a 待审核 count for their own submissions.
  Staff and ordinary users are omitted. Mid-payout `paying` is an
  official state but is not confirmed as part of 待审核; see B.6.
- 计数规则: Number of pending rows in scope.
- 清零条件: The official review path moves the row out of pending
  (paying, rejected, or cancelled). Opening the menu does not clear it.
- 刷新机制: Common rule. Not merged with A.3 or A.6.

### A.6 分站邀请佣金提现待审核 (`invite_withdrawals_pending`)

- 待办定义: Site-customer invite commission cash withdrawals in
  official `AdtySiteInviteWithdrawPending`. The site owner is the
  official reviewer (`ReviewAdtySiteInviteWithdrawal`).
- 数据来源: `adty_site_invite_withdrawals.status` and `site_id`.
- 角色范围: Site owner = this site. Root who is not that owner is not
  the official reviewer and does not receive this kind. Staff and
  users are omitted.
- 计数规则: Number of pending rows for the owner site.
- 清零条件: Official review or a later terminal path leaves pending.
  Opening the menu does not clear it.
- 刷新机制: Common rule. Not merged with A.3 or A.5.

### A.7 返佣待覆盖 (`site_invite_commissions_pending_coverage`)

- 待办定义: Site invite commissions in the official recorded DEC-025
  status `pending_coverage` (`AdtySiteCommissionPendingCoverage`).
  This does not invent a new commission policy.
- 数据来源: `adty_site_invite_commissions.status` and `site_id`.
- 角色范围: Site owner = this site (owner cover path
  `/api/adty/site/referral/cover-pending`). Root who is not the
  owner is not the official cover actor and does not receive this
  kind. Users, staff, and platform-referral-only admins are omitted.
- 计数规则: Number of rows in `pending_coverage` for the owner site
  (row count, not an invented amount total).
- 清零条件: Official cover (or a later official status) leaves
  `pending_coverage`. Opening the menu does not clear it.
- 刷新机制: Common rule. Derived display states and frozen amounts
  are not mixed into this count; see B.5.

### A.8 冻结中佣金 — 平台推广 (`referral_commissions_frozen`)

- 待办定义: Platform referral commissions in official
  `ReferralCommissionStatusFrozen`.
- 数据来源: `referral_commissions.status`, scoped through
  `users.id = referral_commissions.user_id` and `users.site_id`.
- 角色范围: Same as A.2 (Root global; non-root referral admin = main
  site). Not merged with site-invite frozen / on-hold amounts (B.5).
- 计数规则: Number of frozen commission rows in scope.
- 清零条件: Official commission state leaves `frozen`. Opening the
  menu does not clear it.
- 刷新机制: Common rule. This kind is shown on the referral admin
  item together with A.2–A.4 only when it is the sole positive
  referral kind, or as part of that same referral-admin module;
  it is never added into finance/order or ticket badges.

### A.9 财务异常 (`fund_anomalies_open`)

- 待办定义: Fund-anomaly workbench items already recorded as official
  dispositions in `open` or `reviewing` (ISS-R1-0034
  `CollectAdtyFundAnomalies` / `AdtyAnomalyDisposition`). The G.13.6
  write-restriction / freeze path is a leftover and is not a badge
  source.
- 数据来源: `adty_anomaly_dispositions.status` (and `site_id`).
- 角色范围: Root only (the workbench routes are RootAuth). Site
  owners, staff, and users must not learn that an anomaly exists
  (EMP-017).
- 计数规则: Number of dispositions whose status is `open` or
  `reviewing`.
- 清零条件: Official review sets the disposition to `closed`. Opening
  the menu does not clear it.
- 刷新机制: Common rule. Detection may create new open rows; the
  badge counts existing open/reviewing rows and does not invent
  tolerances or repair money.

### A.10 员工余额调整待审批 (`staff_balance_pending`)

- 待办定义: Staff customer-balance adjustment requests in official
  `AdtyStaffBalancePending`, awaiting Root or site-owner approval.
- 数据来源: `adty_staff_balance_requests.status` (and `site_id`,
  `staff_user_id`).
- 角色范围: Root = global. Site owner = this site (reviewer). Enabled
  staff = only their own submitted pending rows (本人相关), never
  other staff or other sites. Unauthorized actors are omitted.
- 计数规则: Number of pending rows in that scope.
- 清零条件: Official approve or reject leaves pending. Opening the
  menu does not clear it.
- 刷新机制: Common rule.

### A.11 客服工单待处理 (`tickets_pending`)

- 待办定义: Official G.12.12 待处理 queue already landed in
  ISS-R1-0044: `new` + `pending_assignment` (including leftover
  unassigned `open` mapped as `new`). This does not rewrite the
  ticket state machine.
- 数据来源: `countAdtyTicketBadges` / `adty_tickets`, using the
  existing actor scope (Root global; non-root admin main site;
  owner this site; enabled staff authorized site).
- 角色范围: Only actors who can manage tickets in that scope (Root,
  platform admin, site owner, enabled staff). Ordinary users and
  team members are not ticket handlers; they do not receive this
  kind (their action cue is A.12). Team leaders who are not
  handlers do not receive `tickets_pending`.
- 计数规则: Existing `AdtyTicketBadges.Pending` (`new` +
  `pending_assignment`) in scope.
- 清零条件: Official status changes leave the 待处理 queue. Opening
  the ticket menu does not clear it.
- 刷新机制: Common rule. Not mixed with notification unread.

### A.12 客服工单等待用户 (`tickets_waiting_user`)

- 待办定义: Official `waiting_user` tickets in the caller's existing
  ticket scope (user: own tickets; team leader: `user_id = self OR
  team_id = this team`; team member: own tickets only; handlers:
  the same scoped set as A.11).
- 数据来源: `countAdtyTicketBadges.WaitingUser`.
- 角色范围: Any signed-in actor who may read those tickets under the
  existing ticket permission matrix, including team_id as above.
- 计数规则: Number of `waiting_user` rows in that scope.
- 清零条件: Official status leaves `waiting_user`. Opening the menu
  does not clear it.
- 刷新机制: Common rule. On the Tickets menu, A.11 and A.12 are
  different queues: if only one is positive the sidebar shows that
  number; if both are positive the sidebar shows a non-numeric
  pending cue rather than summing unlike duties.

### A.13 支付异常 — 待审核孤儿事件 (`payment_orphans_pending_review`)

- 待办定义: Official payment-orphan records in
  `PaymentOrphanStatusPendingReview`. These are already the admin
  review queue on order management.
- 数据来源: `adty_payment_orphan_events.status`. Site scope joins
  `reference_id` to `adty_topup_orders.trade_no` and
  `adty_topup_orders.site_id`. Events that cannot be joined to an
  order have no confirmed site and are visible only to Root.
- 角色范围: Root = all pending-review events. Non-root admin with
  finance read = main-site orders only (`site_id = 0`). Site owners,
  staff, and users are omitted (EMP-017).
- 计数规则: Number of `pending_review` rows in that scope.
- 清零条件: Official resolve/credit/dismiss leaves `pending_review`.
  Opening the menu does not clear it.
- 刷新机制: Common rule. Other payment-exception candidates are in
  part B and are not counted.

---

## B. Unconfirmed count口径 (do not implement as badges)

Each item lists the unconfirmed count口径, the decision still needed,
and the affected acceptance items. This is not a single vague
“角标待确认”.

### B.1 充值订单待处理

- Unconfirmed: Whether the badge should count every
  `TopUpStatusPending` order (many are only awaiting customer
  payment), only orders that already need manual completion, expired
  or failed rows, some combination of those, or something else.
- Decision needed: Official 待处理 definition for recharge-audit
  (which stored statuses count, whether amount or row count is the
  figure, and who is the actor: Root, finance admin, site owner).
- Affected acceptance items: A.19 checklist「充值订单待处理」;
  UI-006, UI-008, UI-009; LIST-012 consistency between list and
  badge.

### B.2 支付异常 besides official `pending_review` orphans

- Unconfirmed: Whether failed, expired, or amount-mismatch top-ups
  that have no `PaymentOrphanEvent`, or other exception records,
  should form additional badge kinds.
- Decision needed: Which official exception tables and statuses
  besides A.13 count, and how they are scoped to a site when no
  order row exists.
- Affected acceptance items: A.19「支付异常」; UI-006; UI-008.

### B.3 团队通知未读的 team_id 过滤

- Unconfirmed: How team leader/member notification unread should
  include `team_id`. Official publications and receipts are keyed by
  `user_id` + `site_id` and have no `team_id`.
- Decision needed: Whether team-targeted notices exist, how they are
  stored, and how receipts are shared or split inside a team
  without leaking another member's unread.
- Affected acceptance items: A.19 team_id permission matrix; UI-008;
  UI-010.

### B.4 分站待处理事项 (site status / retired flows)

- Unconfirmed: Whether `AdtySiteStatusPending` (including
  admin-opened sites that are not yet active) should be a Root
  badge, and what else would count as 分站待处理. Retired self-apply
  site flows and retired pause actions must not be revived as badge
  sources.
- Decision needed: Official pending-site review definition after the
  self-apply retirement, and whether paused/closing/hold reasons are
  in scope.
- Affected acceptance items: A.19「分站待处理事项」; UI-006; UI-008.

### B.5 分站邀请佣金「冻结中」派生状态

- Unconfirmed: Site invite commissions expose DEC-025 *derived*
  amounts (`on_hold`, `FrozenUSD` / `withdrawal_requested`) rather
  than a single recorded frozen status. Which of those (row count
  with FrozenUSD > 0, on_hold only, both, or an amount) should be
  the badge is not signed. They must not be merged with A.8.
- Decision needed: Official frozen-count rule for site invite
  commissions, and who sees it (owner, inviter, Root).
- Affected acceptance items: A.19「返佣待覆盖」/ UI-006「待覆盖或冻结中佣金」
  for the frozen half of site invite commissions; UI-008.

### B.6 分站收益提现 `paying` 是否计入待审核

- Unconfirmed: `AdtySiteWithdrawPaying` is an official mid-payout
  state. It is not confirmed as part of 提现待审核 (A.5 counts
  `pending` only, matching the review queue).
- Decision needed: Whether Root should also see a 待打款 badge for
  `paying`, analogous to A.4.
- Affected acceptance items: A.19「提现待审核」; UI-006; UI-009.

### B.7 人工退款已提交待审批

- Unconfirmed: `AdtyManualRefundStatusSubmitted` is an official
  state, but A.19 does not list manual refunds as their own badge
  kind, and folding them into B.1 is also unconfirmed.
- Decision needed: Whether submitted manual refunds are a distinct
  finance badge, part of 充值订单待处理, or not badged.
- Affected acceptance items: UI-009 (refund then badge update);
  A.19 finance/order todos; LIST-012.

### B.8 刷新轮询间隔

- Unconfirmed: No newly signed official poll interval. The existing
  referral sidebar refetch (at most every five minutes) remains the
  ISS-R1-0050 leftover and must not be treated as a new official
  interval or made worse.
- Decision needed: Official refresh cadence, if any, for sidebar
  badges.
- Affected acceptance items: A.19 刷新机制; UI-007; ISS-R1-0050.

---

## Sidebar placement notes

- Notification unread: top bar, not mixed into finance/order/ticket
  items.
- Referral admin item: A.2–A.4 and A.8 (one official referral-admin
  module).
- Order management item: A.13 only (B.1 and B.2 are not implemented).
- Reseller sites item: A.5, A.9, and Root's A.10. These are different
  duties, so the parent shows a single kind's number when only one is
  positive, and a non-numeric pending cue when more than one is
  positive — it does not sum them.
- Site owner center item: A.6, A.7, and the owner's A.10, with the
  same non-mixing rule.
- Tickets item: A.11 and/or A.12, with the same non-mixing rule.
- Staff workspace item: A.11 (handler queue). Staff A.10 (own
  submissions) is returned in the API but is not mixed into that
  badge.
