package model

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting"
	"golang.org/x/image/webp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func init() {
	image.RegisterFormat("webp", "RIFF????WEBP", webp.Decode, webp.DecodeConfig)
}

const affPayoutQrMaxBytes = 512 * 1024

var (
	alipayPhonePattern = regexp.MustCompile(`^\+?[0-9][0-9\-]{5,18}$`)
	alipayEmailPattern = regexp.MustCompile(`^[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}$`)
)

// AffPayoutQr 收款码原图。列表接口不读 data。
type AffPayoutQr struct {
	Id           int64     `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId       int       `json:"user_id" gorm:"index;not null"`
	WithdrawalId int64     `json:"withdrawal_id" gorm:"index;default:0"`
	ContentType  string    `json:"content_type" gorm:"type:varchar(32);not null"`
	Size         int       `json:"size" gorm:"not null"`
	Sha256       string    `json:"sha256" gorm:"type:varchar(64);index;not null"`
	Data         []byte    `json:"-" gorm:"type:bytea;not null"`
	CreatedAt    time.Time `json:"created_at" gorm:"autoCreateTime;index"`
}

func (AffPayoutQr) TableName() string { return "aff_payout_qrcodes" }

type AffCommissionView struct {
	AffCommission
	InviterName string `json:"inviter_name,omitempty"`
	InviteeName string `json:"invitee_name,omitempty"`
}

type AffWithdrawalAdminRow struct {
	AffWithdrawal
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

type AffInviteeRow struct {
	UserId          int     `json:"user_id"`
	Name            string  `json:"name"`
	RegisteredAt    int64   `json:"registered_at"`
	PaidUSD         float64 `json:"paid_usd"`
	CommissionQuota int64   `json:"commission_quota"`
	Status          string  `json:"status"`
}

type AffTransferView struct {
	Id           int64     `json:"id"`
	Delta        int64     `json:"delta"`
	BalanceAfter int64     `json:"balance_after"`
	CreatedAt    time.Time `json:"created_at"`
}

type AffOverview struct {
	InvitedUsers          int64 `json:"invited_users"`
	Inviters              int64 `json:"inviters"`
	CommissionFrozen      int64 `json:"commission_frozen"`
	CommissionAvailable   int64 `json:"commission_available"`
	CommissionVoided      int64 `json:"commission_voided"`
	CommissionBlocked     int64 `json:"commission_blocked"`
	WithdrawPendingUSDT   int64 `json:"withdraw_pending_usdt"`
	WithdrawPendingAlipay int64 `json:"withdraw_pending_alipay"`
	WithdrawPendingLegacy int64 `json:"withdraw_pending_legacy"`
	WithdrawPaidQuota     int64 `json:"withdraw_paid_quota"`
}

type AffWithdrawBuckets struct {
	Pending  int64 `json:"pending"`
	Approved int64 `json:"approved"`
	Failed   int64 `json:"failed"`
}

type AffAdminUserDetail struct {
	UserId          int                     `json:"user_id"`
	Username        string                  `json:"username"`
	DisplayName     string                  `json:"display_name"`
	InviterId       int                     `json:"inviter_id"`
	InviterName     string                  `json:"inviter_name"`
	AffCount        int                     `json:"aff_count"`
	AffQuota        int                     `json:"aff_quota"`
	AffHistory      int                     `json:"aff_history"`
	Invitees        []AffInviteeRow         `json:"invitees"`
	InviteeTotal    int64                   `json:"invitee_total"`
	Commissions     []AffCommissionView     `json:"commissions"`
	CommissionTotal int64                   `json:"commission_total"`
	Withdrawals     []AffWithdrawalAdminRow `json:"withdrawals"`
	WithdrawalTotal int64                   `json:"withdrawal_total"`
	Transfers       []AffTransferView       `json:"transfers"`
	TransferTotal   int64                   `json:"transfer_total"`
}

type AffAdminListFilter struct {
	Status    string
	PayMethod string
	UserId    int
	InviterId int
	InviteeId int
	Keyword   string
	Limit     int
	Offset    int
}

func normalizePayoutRef(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if len(s) > 128 || strings.ContainsAny(s, "\r\n\x00") {
		return "", ErrAffPayoutRefInvalid
	}
	return s, nil
}

func affWithdrawPayloadMatches(row *AffWithdrawal, amount int64, method, account, chain, qrSha string) bool {
	if row == nil {
		return false
	}
	return row.AmountQuota == amount &&
		row.PayMethod == method &&
		row.PayAccount == account &&
		row.UsdtChain == chain &&
		row.PayoutQrSha == qrSha
}

func AffWithdrawalDisplayMethod(row *AffWithdrawal) string {
	if row == nil {
		return ""
	}
	switch row.PayMethod {
	case AffPayMethodUSDT, AffPayMethodAlipay:
		return row.PayMethod
	}
	if strings.TrimSpace(row.UsdtChain) != "" {
		return AffPayMethodUSDT
	}
	return AffPayMethodLegacy
}

// ValidateAlipayAccount 只接受手机号或邮箱。错误不回显账号。
func ValidateAlipayAccount(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if len(s) < 6 || len(s) > 64 || strings.ContainsAny(s, " \t\r\n") {
		return "", ErrAffAlipayAccountInvalid
	}
	lower := strings.ToLower(s)
	if strings.Contains(s, "/") || strings.Contains(s, ":") || strings.Contains(lower, "http") {
		return "", ErrAffAlipayAccountInvalid
	}
	if alipayPhonePattern.MatchString(s) || alipayEmailPattern.MatchString(s) {
		return s, nil
	}
	return "", ErrAffAlipayAccountInvalid
}

func normalizeAffWithdrawPayout(userId int, in AffWithdrawCreateInput) (method, account, chain, qrSha string, err error) {
	method = strings.ToLower(strings.TrimSpace(in.PayMethod))
	switch method {
	case AffPayMethodUSDT:
		if in.PayoutQrId != 0 {
			return "", "", "", "", ErrAffPayoutQrInvalid
		}
		chain, account, err = setting.ValidateUsdtPayoutAddress(in.UsdtChain, in.PayAccount)
		if err != nil {
			return "", "", "", "", err
		}
		return method, account, chain, "", nil
	case AffPayMethodAlipay:
		if strings.TrimSpace(in.UsdtChain) != "" {
			return "", "", "", "", ErrAffWithdrawMethodInvalid
		}
		account, err = ValidateAlipayAccount(in.PayAccount)
		if err != nil {
			return "", "", "", "", err
		}
		if in.PayoutQrId <= 0 {
			return "", "", "", "", ErrAffPayoutQrInvalid
		}
		var qr AffPayoutQr
		qerr := DB.Select("id", "user_id", "sha256").Where("id = ?", in.PayoutQrId).First(&qr).Error
		if qerr != nil || qr.UserId != userId || qr.Sha256 == "" {
			return "", "", "", "", ErrAffPayoutQrInvalid
		}
		qrSha = qr.Sha256
		return method, account, "", qrSha, nil
	default:
		return "", "", "", "", ErrAffWithdrawMethodInvalid
	}
}

func ValidatePayoutQrImage(data []byte) (string, error) {
	if len(data) == 0 || len(data) > affPayoutQrMaxBytes {
		return "", ErrAffPayoutQrInvalid
	}
	head := data
	if len(head) > 512 {
		head = head[:512]
	}
	low := strings.ToLower(string(head))
	if strings.Contains(low, "<svg") || strings.Contains(low, "image/svg") {
		return "", ErrAffPayoutQrInvalid
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", ErrAffPayoutQrInvalid
	}
	switch format {
	case "png", "jpeg", "webp":
	default:
		return "", ErrAffPayoutQrInvalid
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 2048 || cfg.Height > 2048 {
		return "", ErrAffPayoutQrInvalid
	}
	if _, _, err = image.Decode(bytes.NewReader(data)); err != nil {
		return "", ErrAffPayoutQrInvalid
	}
	return "image/" + format, nil
}

func SaveAffPayoutQr(userId int, data []byte) (*AffPayoutQr, error) {
	if userId <= 0 {
		return nil, ErrAffPayoutQrInvalid
	}
	contentType, err := ValidatePayoutQrImage(data)
	if err != nil {
		return nil, err
	}
	if err := DB.Where("user_id = ? AND withdrawal_id = 0 AND created_at < ?", userId, time.Now().Add(-24*time.Hour)).
		Delete(&AffPayoutQr{}).Error; err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	row := AffPayoutQr{
		UserId:      userId,
		ContentType: contentType,
		Size:        len(data),
		Sha256:      hex.EncodeToString(sum[:]),
		Data:        append([]byte(nil), data...),
	}
	if err := DB.Create(&row).Error; err != nil {
		return nil, err
	}
	row.Data = nil
	return &row, nil
}

func GetAffPayoutQrForUser(id int64, userId int) (*AffPayoutQr, error) {
	if id <= 0 || userId <= 0 {
		return nil, ErrAffPayoutQrNotFound
	}
	var row AffPayoutQr
	err := DB.Where("id = ? AND user_id = ?", id, userId).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAffPayoutQrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func GetAffPayoutQrForAdmin(id int64) (*AffPayoutQr, error) {
	if id <= 0 {
		return nil, ErrAffPayoutQrNotFound
	}
	var row AffPayoutQr
	err := DB.Where("id = ?", id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrAffPayoutQrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func clampAffPage(limit, offset int) (int, int) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func maskAffDisplayName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if strings.Contains(name, "@") {
		return string([]rune(name)[:1]) + "***"
	}
	runes := []rune(name)
	if len(runes) <= 1 {
		return "*"
	}
	if len(runes) == 2 {
		return string(runes[0]) + "*"
	}
	return string(runes[0]) + "***" + string(runes[len(runes)-1])
}

func affUserLabel(displayName, username string, mask bool) string {
	name := strings.TrimSpace(displayName)
	if name == "" {
		name = strings.TrimSpace(username)
	}
	if mask {
		return maskAffDisplayName(name)
	}
	return name
}

func ListAffInvitees(inviterId int, limit, offset int, mask bool) ([]AffInviteeRow, int64, error) {
	if inviterId <= 0 {
		return nil, 0, errors.New("invalid inviter")
	}
	limit, offset = clampAffPage(limit, offset)
	q := DB.Model(&User{}).Where("inviter_id = ?", inviterId)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var users []User
	if err := q.Select("id", "username", "display_name", "created_at").
		Order("id DESC").Limit(limit).Offset(offset).Find(&users).Error; err != nil {
		return nil, 0, err
	}
	rows := make([]AffInviteeRow, 0, len(users))
	if len(users) == 0 {
		return rows, total, nil
	}
	ids := make([]int, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.Id)
	}
	type agg struct {
		InviteeId int
		Paid      float64
		Quota     int64
		Blocked   int
		Frozen    int
		Available int
		Voided    int
	}
	var aggs []agg
	// 作废行不计入这行实付和佣金。状态仍全部保留，避免被更高优先级盖掉。
	if err := DB.Model(&AffCommission{}).
		Select(`invitee_id,
			COALESCE(SUM(CASE WHEN status <> 'voided' THEN paid_amount_usd ELSE 0 END),0) as paid,
			COALESCE(SUM(CASE WHEN status <> 'voided' THEN commission_quota ELSE 0 END),0) as quota,
			SUM(CASE WHEN status = 'blocked' THEN 1 ELSE 0 END) as blocked,
			SUM(CASE WHEN status = 'frozen' THEN 1 ELSE 0 END) as frozen,
			SUM(CASE WHEN status = 'available' THEN 1 ELSE 0 END) as available,
			SUM(CASE WHEN status = 'voided' THEN 1 ELSE 0 END) as voided`).
		Where("inviter_id = ? AND invitee_id IN ?", inviterId, ids).
		Group("invitee_id").Scan(&aggs).Error; err != nil {
		return nil, 0, err
	}
	byInvitee := map[int]agg{}
	for _, a := range aggs {
		byInvitee[a.InviteeId] = a
	}
	for _, u := range users {
		a := byInvitee[u.Id]
		rows = append(rows, AffInviteeRow{
			UserId:          u.Id,
			Name:            affUserLabel(u.DisplayName, u.Username, mask),
			RegisteredAt:    u.CreatedAt,
			PaidUSD:         a.Paid,
			CommissionQuota: a.Quota,
			Status:          summarizeAffCommissionStatus(a.Blocked, a.Frozen, a.Available, a.Voided),
		})
	}
	return rows, total, nil
}

func summarizeAffCommissionStatus(blocked, frozen, available, voided int) string {
	parts := make([]string, 0, 4)
	if blocked > 0 {
		parts = append(parts, AffCommissionStatusBlocked)
	}
	if frozen > 0 {
		parts = append(parts, AffCommissionStatusFrozen)
	}
	if available > 0 {
		parts = append(parts, AffCommissionStatusAvailable)
	}
	if voided > 0 {
		parts = append(parts, AffCommissionStatusVoided)
	}
	return strings.Join(parts, ",")
}

func loadUserLabels(ids []int) (map[int]User, error) {
	out := map[int]User{}
	if len(ids) == 0 {
		return out, nil
	}
	var users []User
	if err := DB.Select("id", "username", "display_name").Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, u := range users {
		out[u.Id] = u
	}
	return out, nil
}

func ListAffCommissionsAdminFiltered(f AffAdminListFilter, maskInvitee bool) ([]AffCommissionView, int64, error) {
	f.Limit, f.Offset = clampAffPage(f.Limit, f.Offset)
	q := DB.Model(&AffCommission{})
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.InviterId > 0 {
		q = q.Where("inviter_id = ?", f.InviterId)
	}
	if f.InviteeId > 0 {
		q = q.Where("invitee_id = ?", f.InviteeId)
	}
	kw := strings.TrimSpace(f.Keyword)
	if kw != "" {
		like := "%" + strings.ToLower(kw) + "%"
		sub := DB.Model(&User{}).Select("id").Where("LOWER(username) LIKE ? OR LOWER(display_name) LIKE ?", like, like)
		if id, err := strconv.Atoi(kw); err == nil && id > 0 {
			q = q.Where("inviter_id = ? OR invitee_id = ? OR inviter_id IN (?) OR invitee_id IN (?)", id, id, sub, sub)
		} else {
			q = q.Where("inviter_id IN (?) OR invitee_id IN (?)", sub, sub)
		}
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []AffCommission
	if err := q.Order("id DESC").Limit(f.Limit).Offset(f.Offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	ids := make([]int, 0, len(rows)*2)
	for _, row := range rows {
		ids = append(ids, row.InviterId, row.InviteeId)
	}
	labels, err := loadUserLabels(ids)
	if err != nil {
		return nil, 0, err
	}
	views := make([]AffCommissionView, 0, len(rows))
	for _, row := range rows {
		inv := labels[row.InviterId]
		ee := labels[row.InviteeId]
		views = append(views, AffCommissionView{
			AffCommission: row,
			InviterName:   affUserLabel(inv.DisplayName, inv.Username, false),
			InviteeName:   affUserLabel(ee.DisplayName, ee.Username, maskInvitee),
		})
	}
	return views, total, nil
}

func applyWithdrawalMethodFilter(q *gorm.DB, method string) *gorm.DB {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case AffPayMethodUSDT:
		return q.Where("pay_method = ? OR (pay_method = '' AND usdt_chain <> '')", AffPayMethodUSDT)
	case AffPayMethodAlipay:
		return q.Where("pay_method = ?", AffPayMethodAlipay)
	case AffPayMethodLegacy:
		return q.Where("pay_method = '' AND (usdt_chain = '' OR usdt_chain IS NULL)")
	default:
		return q
	}
}

func ListAffWithdrawalsAdminFiltered(f AffAdminListFilter) ([]AffWithdrawalAdminRow, int64, error) {
	f.Limit, f.Offset = clampAffPage(f.Limit, f.Offset)
	q := DB.Model(&AffWithdrawal{})
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.UserId > 0 {
		q = q.Where("user_id = ?", f.UserId)
	}
	q = applyWithdrawalMethodFilter(q, f.PayMethod)
	kw := strings.TrimSpace(f.Keyword)
	if kw != "" {
		like := "%" + strings.ToLower(kw) + "%"
		sub := DB.Model(&User{}).Select("id").Where("LOWER(username) LIKE ? OR LOWER(display_name) LIKE ?", like, like)
		if id, err := strconv.Atoi(kw); err == nil && id > 0 {
			q = q.Where("user_id = ? OR user_id IN (?)", id, sub)
		} else {
			q = q.Where("user_id IN (?)", sub)
		}
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []AffWithdrawal
	if err := q.Order("id DESC").Limit(f.Limit).Offset(f.Offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	ids := make([]int, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.UserId)
	}
	labels, err := loadUserLabels(ids)
	if err != nil {
		return nil, 0, err
	}
	out := make([]AffWithdrawalAdminRow, 0, len(rows))
	for _, row := range rows {
		u := labels[row.UserId]
		out = append(out, AffWithdrawalAdminRow{
			AffWithdrawal: row,
			Username:      u.Username,
			DisplayName:   u.DisplayName,
		})
	}
	return out, total, nil
}

func CountAffWithdrawalBuckets() (AffWithdrawBuckets, error) {
	type row struct {
		Status string
		N      int64
	}
	var rows []row
	if err := DB.Model(&AffWithdrawal{}).Select("status, COUNT(*) as n").Group("status").Scan(&rows).Error; err != nil {
		return AffWithdrawBuckets{}, err
	}
	var out AffWithdrawBuckets
	for _, row := range rows {
		switch row.Status {
		case AffWithdrawStatusPending:
			out.Pending += row.N
		case AffWithdrawStatusApproved:
			out.Approved += row.N
		case AffWithdrawStatusPayFailed, AffWithdrawStatusPayFailedFinal, AffWithdrawStatusPayUnknown:
			out.Failed += row.N
		}
	}
	return out, nil
}

type AffInviterRow struct {
	UserId       int    `json:"user_id"`
	Username     string `json:"username"`
	DisplayName  string `json:"display_name"`
	AffCode      string `json:"aff_code"`
	InviteeCount int    `json:"invitee_count"`
	AffQuota     int    `json:"aff_quota"`
	AffHistory   int    `json:"aff_history"`
}

func clampInviterPage(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func ListAffAdminInviters(keyword string, limit, offset int) ([]AffInviterRow, int64, error) {
	limit, offset = clampInviterPage(limit, offset)
	var total int64
	if err := inviterDirectoryQuery(keyword).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := make([]AffInviterRow, 0)
	err := inviterDirectoryQuery(keyword).
		Select(`u.id AS user_id, u.username AS username, u.display_name AS display_name, u.aff_code AS aff_code,
			(SELECT COUNT(*) FROM users AS invitee WHERE invitee.inviter_id = u.id AND invitee.deleted_at IS NULL) AS invitee_count,
			u.aff_quota AS aff_quota, u.aff_history AS aff_history`).
		Order("u.id ASC").
		Limit(limit).
		Offset(offset).
		Scan(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func inviterDirectoryQuery(keyword string) *gorm.DB {
	q := DB.Table("users AS u").Where("u.deleted_at IS NULL").Where(`(
		EXISTS (SELECT 1 FROM users AS invitee WHERE invitee.inviter_id = u.id AND invitee.deleted_at IS NULL)
		OR EXISTS (SELECT 1 FROM aff_commissions AS c WHERE c.inviter_id = u.id)
	)`)
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return q
	}
	pattern := "%" + escapeLikePattern(strings.ToLower(keyword)) + "%"
	textMatch := `(LOWER(COALESCE(u.username, '')) LIKE ? ESCAPE '\' OR LOWER(COALESCE(u.display_name, '')) LIKE ? ESCAPE '\' OR LOWER(COALESCE(u.aff_code, '')) LIKE ? ESCAPE '\')`
	if id, err := strconv.Atoi(keyword); err == nil && id > 0 && strconv.Itoa(id) == keyword {
		return q.Where(`(u.id = ? OR `+textMatch+`)`, id, pattern, pattern, pattern)
	}
	return q.Where(`(`+textMatch+`)`, pattern, pattern, pattern)
}

func escapeLikePattern(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

func GetAffOverview() (AffOverview, error) {
	var out AffOverview
	// Count only invitees whose inviter is still in the directory. A deleted
	// inviter stays out of the inviter table, so their invitees stay out of this card.
	if err := DB.Model(&User{}).
		Where("inviter_id > 0").
		Where("inviter_id IN (SELECT id FROM users WHERE deleted_at IS NULL)").
		Count(&out.InvitedUsers).Error; err != nil {
		return out, err
	}
	if err := inviterDirectoryQuery("").Count(&out.Inviters).Error; err != nil {
		return out, err
	}
	type sumRow struct {
		Status string
		Total  int64
	}
	var sums []sumRow
	if err := DB.Model(&AffCommission{}).
		Where("inviter_id IN (SELECT id FROM users WHERE deleted_at IS NULL)").
		Select("status, COALESCE(SUM(commission_quota),0) as total").
		Group("status").Scan(&sums).Error; err != nil {
		return out, err
	}
	for _, row := range sums {
		switch row.Status {
		case AffCommissionStatusFrozen:
			out.CommissionFrozen = row.Total
		case AffCommissionStatusAvailable:
			out.CommissionAvailable = row.Total
		case AffCommissionStatusVoided:
			out.CommissionVoided = row.Total
		case AffCommissionStatusBlocked:
			out.CommissionBlocked = row.Total
		}
	}
	var pending []AffWithdrawal
	if err := DB.Select("pay_method", "usdt_chain").
		Where("status = ?", AffWithdrawStatusPending).
		Where("user_id IN (SELECT id FROM users WHERE deleted_at IS NULL)").
		Find(&pending).Error; err != nil {
		return out, err
	}
	for i := range pending {
		switch AffWithdrawalDisplayMethod(&pending[i]) {
		case AffPayMethodUSDT:
			out.WithdrawPendingUSDT++
		case AffPayMethodAlipay:
			out.WithdrawPendingAlipay++
		default:
			out.WithdrawPendingLegacy++
		}
	}
	if err := DB.Model(&AffWithdrawal{}).Where("status = ?", AffWithdrawStatusPaid).
		Where("user_id IN (SELECT id FROM users WHERE deleted_at IS NULL)").
		Select("COALESCE(SUM(net_quota),0)").Scan(&out.WithdrawPaidQuota).Error; err != nil {
		return out, err
	}
	return out, nil
}

type AffAdminUserDetailQuery struct {
	Limit            int
	InviteeOffset    int
	CommissionOffset int
	WithdrawalOffset int
	TransferOffset   int
}

func GetAffAdminUserDetail(userId int, limit int) (*AffAdminUserDetail, error) {
	return GetAffAdminUserDetailPage(userId, AffAdminUserDetailQuery{Limit: limit})
}

func GetAffAdminUserDetailPage(userId int, q AffAdminUserDetailQuery) (*AffAdminUserDetail, error) {
	if userId <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	limit := q.Limit
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	inviteeOffset := q.InviteeOffset
	if inviteeOffset < 0 {
		inviteeOffset = 0
	}
	commissionOffset := q.CommissionOffset
	if commissionOffset < 0 {
		commissionOffset = 0
	}
	withdrawalOffset := q.WithdrawalOffset
	if withdrawalOffset < 0 {
		withdrawalOffset = 0
	}
	transferOffset := q.TransferOffset
	if transferOffset < 0 {
		transferOffset = 0
	}
	var u User
	if err := DB.Select("id", "username", "display_name", "inviter_id", "aff_count", "aff_quota", "aff_history").
		Where("id = ?", userId).First(&u).Error; err != nil {
		return nil, err
	}
	detail := &AffAdminUserDetail{
		UserId:      u.Id,
		Username:    u.Username,
		DisplayName: u.DisplayName,
		InviterId:   u.InviterId,
		AffCount:    u.AffCount,
		AffQuota:    u.AffQuota,
		AffHistory:  u.AffHistoryQuota,
		Invitees:    []AffInviteeRow{},
		Commissions: []AffCommissionView{},
		Withdrawals: []AffWithdrawalAdminRow{},
		Transfers:   []AffTransferView{},
	}
	if u.InviterId > 0 {
		labels, err := loadUserLabels([]int{u.InviterId})
		if err != nil {
			return nil, err
		}
		inv := labels[u.InviterId]
		detail.InviterName = affUserLabel(inv.DisplayName, inv.Username, false)
	}
	invitees, total, err := ListAffInvitees(userId, limit, inviteeOffset, false)
	if err != nil {
		return nil, err
	}
	detail.Invitees = invitees
	detail.InviteeTotal = total
	comms, commTotal, err := ListAffCommissionsAdminFiltered(AffAdminListFilter{
		InviterId: userId, Limit: limit, Offset: commissionOffset,
	}, false)
	if err != nil {
		return nil, err
	}
	detail.Commissions = comms
	detail.CommissionTotal = commTotal
	wds, wdTotal, err := ListAffWithdrawalsAdminFiltered(AffAdminListFilter{
		UserId: userId, Limit: limit, Offset: withdrawalOffset,
	})
	if err != nil {
		return nil, err
	}
	detail.Withdrawals = wds
	detail.WithdrawalTotal = wdTotal
	var transferTotal int64
	if err := DB.Model(&LedgerEntry{}).Where("user_id = ? AND account = ? AND kind = ?", userId, "aff", "aff_transfer").Count(&transferTotal).Error; err != nil {
		return nil, err
	}
	detail.TransferTotal = transferTotal
	var ledgers []LedgerEntry
	if err := DB.Where("user_id = ? AND account = ? AND kind = ?", userId, "aff", "aff_transfer").
		Order("created_at DESC").Limit(limit).Offset(transferOffset).Find(&ledgers).Error; err != nil {
		return nil, err
	}
	for _, row := range ledgers {
		detail.Transfers = append(detail.Transfers, AffTransferView{
			Id:           row.Id,
			Delta:        row.Delta,
			BalanceAfter: row.BalanceAfter,
			CreatedAt:    row.CreatedAt,
		})
	}
	return detail, nil
}

func approveAffSelfInviteLocked(tx *gorm.DB, review *RiskReview, operatorId int) error {
	if review == nil || review.OrderNo == "" {
		return ErrRiskReviewStatusInvalid
	}
	var peek AffCommission
	if err := tx.Where("order_no = ?", review.OrderNo).First(&peek).Error; err != nil {
		return ErrRiskReviewStatusInvalid
	}
	// 锁序与退款一致：订单 → 用户 → 佣金。审核行已由调用方持有。
	base, remain, skipScale, err := lockAffOrderForSelfInvite(tx, &peek)
	if err != nil {
		return err
	}
	if err := lockAffUsersInOrder(tx, []int{peek.InviterId}); err != nil {
		return err
	}
	var comm AffCommission
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", peek.Id).First(&comm).Error; err != nil {
		return ErrRiskReviewStatusInvalid
	}
	if comm.Status == AffCommissionStatusVoided {
		return ErrRiskReviewStatusInvalid
	}
	if comm.Status == AffCommissionStatusBlocked {
		if !skipScale {
			scaleBlockedCommission(&comm, base, remain)
		}
		now := time.Now()
		if comm.CommissionQuota <= 0 {
			comm.Status = AffCommissionStatusVoided
		} else {
			deadline := affSelfInviteFreezeDeadline(&comm, now)
			comm.UnfreezeAt = deadline
			if deadline.After(now) {
				comm.Status = AffCommissionStatusFrozen
			} else {
				comm.Status = AffCommissionStatusAvailable
				if err := creditAffQuotaTx(tx, comm.InviterId, comm.CommissionQuota, "aff_self_invite_release", comm.OrderNo); err != nil {
					return err
				}
			}
		}
		if err := tx.Save(&comm).Error; err != nil {
			return err
		}
	}
	now := time.Now()
	review.Status = RiskReviewStatusApproved
	review.ResolvedBy = operatorId
	review.ResolvedAt = &now
	return tx.Save(review).Error
}

func lockAffOrderForSelfInvite(tx *gorm.DB, comm *AffCommission) (base, remain float64, skipScale bool, err error) {
	if comm.Source == AffCommissionSourceSubscription || strings.HasPrefix(comm.OrderNo, "sub:") {
		trade := strings.TrimPrefix(comm.OrderNo, "sub:")
		var order SubscriptionOrder
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("trade_no = ?", trade).First(&order).Error; err != nil {
			return 0, 0, true, ErrRiskReviewStatusInvalid
		}
		if order.Status != common.TopUpStatusSuccess {
			return 0, 0, true, ErrRiskReviewStatusInvalid
		}
		return 0, 0, true, nil
	}
	var order TopupOrder
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("order_no = ?", comm.OrderNo).First(&order).Error; err != nil {
		return 0, 0, false, ErrRiskReviewStatusInvalid
	}
	base = order.CreditAmountUSD
	if base <= 0 {
		base = order.PaidAmountUSD
	}
	remain = base - order.RefundedAmount
	if order.RefundStatus == RefundStatusFull || remain <= 1e-9 {
		return base, remain, false, &RiskReviewOrderRefundedError{OrderNo: order.OrderNo, RefundStatus: order.RefundStatus}
	}
	return base, remain, false, nil
}

func scaleBlockedCommission(comm *AffCommission, base, remain float64) {
	if comm == nil {
		return
	}
	// 配额是按写入时的 PaidAmountUSD 算的。部分退款之后才写入时，这一列已经是净值，不能再除原额。
	recorded := comm.PaidAmountUSD
	if recorded <= 0 {
		recorded = base
	}
	if recorded <= 0 || remain >= recorded {
		return
	}
	if remain < 0 {
		remain = 0
	}
	scaled := int64(math.Round(float64(comm.CommissionQuota) * remain / recorded))
	if scaled < 0 {
		scaled = 0
	}
	if scaled > comm.CommissionQuota {
		scaled = comm.CommissionQuota
	}
	comm.CommissionQuota = scaled
	comm.PaidAmountUSD = remain
}

type AffSelfInviteReviewView struct {
	Id               int64      `json:"id"`
	Kind             string     `json:"kind"`
	UserId           int        `json:"user_id"`
	OrderNo          string     `json:"order_no"`
	AmountUSD        float64    `json:"amount_usd"`
	Status           string     `json:"status"`
	Reason           string     `json:"reason"`
	CreatedAt        time.Time  `json:"created_at"`
	ResolvedAt       *time.Time `json:"resolved_at"`
	InviterId        int        `json:"inviter_id"`
	InviterName      string     `json:"inviter_name"`
	InviteeId        int        `json:"invitee_id"`
	InviteeName      string     `json:"invitee_name"`
	CommissionQuota  int64      `json:"commission_quota"`
	CommissionStatus string     `json:"commission_status"`
}

func ListAffSelfInviteReviews(status string, limit, offset int) ([]AffSelfInviteReviewView, int64, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := DB.Model(&RiskReview{}).Where("kind = ?", RiskReviewKindAffSelfInvite)
	if status != "" && status != "all" {
		q = q.Where("status = ?", status)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var reviews []RiskReview
	if err := q.Order("id DESC").Limit(limit).Offset(offset).Find(&reviews).Error; err != nil {
		return nil, 0, err
	}
	if len(reviews) == 0 {
		return []AffSelfInviteReviewView{}, total, nil
	}
	orderNos := make([]string, 0, len(reviews))
	for _, row := range reviews {
		if row.OrderNo != "" {
			orderNos = append(orderNos, row.OrderNo)
		}
	}
	var comms []AffCommission
	if len(orderNos) > 0 {
		if err := DB.Where("order_no IN ?", orderNos).Find(&comms).Error; err != nil {
			return nil, 0, err
		}
	}
	byOrder := make(map[string]AffCommission, len(comms))
	ids := map[int]struct{}{}
	for _, comm := range comms {
		byOrder[comm.OrderNo] = comm
		ids[comm.InviterId] = struct{}{}
		ids[comm.InviteeId] = struct{}{}
	}
	for _, row := range reviews {
		ids[row.UserId] = struct{}{}
	}
	idList := make([]int, 0, len(ids))
	for id := range ids {
		if id > 0 {
			idList = append(idList, id)
		}
	}
	labels, err := loadUserLabels(idList)
	if err != nil {
		return nil, 0, err
	}
	out := make([]AffSelfInviteReviewView, 0, len(reviews))
	for _, row := range reviews {
		comm := byOrder[row.OrderNo]
		inviteeId := comm.InviteeId
		if inviteeId == 0 {
			inviteeId = row.UserId
		}
		inv := labels[comm.InviterId]
		ie := labels[inviteeId]
		out = append(out, AffSelfInviteReviewView{
			Id:               row.Id,
			Kind:             row.Kind,
			UserId:           row.UserId,
			OrderNo:          row.OrderNo,
			AmountUSD:        row.AmountUSD,
			Status:           row.Status,
			Reason:           row.Reason,
			CreatedAt:        row.CreatedAt,
			ResolvedAt:       row.ResolvedAt,
			InviterId:        comm.InviterId,
			InviterName:      affUserLabel(inv.DisplayName, inv.Username, false),
			InviteeId:        inviteeId,
			InviteeName:      affUserLabel(ie.DisplayName, ie.Username, false),
			CommissionQuota:  comm.CommissionQuota,
			CommissionStatus: comm.Status,
		})
	}
	return out, total, nil
}

// ReopenAffSelfInviteReview 只把已驳回的自邀审核改回 pending。不入账，不改佣金。
// 佣金已作废或已不是 blocked 时失败。已是 pending 则幂等成功。
func ReopenAffSelfInviteReview(id int64, _ int) (*RiskReview, error) {
	if id <= 0 {
		return nil, ErrRiskReviewNotFound
	}
	var out *RiskReview
	err := DB.Transaction(func(tx *gorm.DB) error {
		var rr RiskReview
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&rr).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrRiskReviewNotFound
			}
			return err
		}
		if rr.Kind != RiskReviewKindAffSelfInvite {
			return ErrRiskReviewStatusInvalid
		}
		if rr.Status == RiskReviewStatusPending {
			out = &rr
			return nil
		}
		if rr.Status != RiskReviewStatusRejected {
			return ErrRiskReviewStatusInvalid
		}
		var peek AffCommission
		if err := tx.Where("order_no = ?", rr.OrderNo).First(&peek).Error; err != nil {
			return ErrRiskReviewStatusInvalid
		}
		if peek.Status == AffCommissionStatusVoided || peek.Status != AffCommissionStatusBlocked {
			return ErrRiskReviewStatusInvalid
		}
		if err := lockAffUsersInOrder(tx, []int{peek.InviterId}); err != nil {
			return err
		}
		var comm AffCommission
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", peek.Id).First(&comm).Error; err != nil {
			return ErrRiskReviewStatusInvalid
		}
		if comm.Status != AffCommissionStatusBlocked {
			return ErrRiskReviewStatusInvalid
		}
		rr.Status = RiskReviewStatusPending
		rr.ResolvedBy = 0
		rr.ResolvedAt = nil
		if err := tx.Save(&rr).Error; err != nil {
			return err
		}
		out = &rr
		return nil
	})
	return out, err
}
