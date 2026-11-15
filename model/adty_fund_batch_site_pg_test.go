package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// ISS-R4-0047：批次自带站点。新写入路径要带上 site_id；站点未知时只能是 NULL，
// 0 是主站这个具体值。

func TestAdtyFundBatchStampsSiteOnCreatePG(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&AdtyFundBatch{}))

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CreateAdtyFundBatchesWithSiteTx(tx, 21, AdtyFundBatchSiteID(4), "order-with-site", 80, 20)
	}))
	var rows []AdtyFundBatch
	require.NoError(t, db.Where("user_id = ?", 21).Order("kind").Find(&rows).Error)
	require.Len(t, rows, 2)
	for _, row := range rows {
		require.NotNil(t, row.SiteId, "principal 与 bonus 两条批次都要带站点")
		require.Equal(t, 4, *row.SiteId)
	}

	// 站点未知的调用方保留旧语义：落 NULL，不猜。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CreateAdtyFundBatchesTx(tx, 22, "order-without-site", 10, 0)
	}))
	var unknown AdtyFundBatch
	require.NoError(t, db.Where("user_id = ?", 22).First(&unknown).Error)
	require.Nil(t, unknown.SiteId)

	// 主站是 0，必须是一个值，不能被写成 NULL。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CreateAdtyFundBatchesWithSiteTx(tx, 23, AdtyFundBatchSiteID(0), "order-main-site", 10, 0)
	}))
	var mainSite AdtyFundBatch
	require.NoError(t, db.Where("user_id = ?", 23).First(&mainSite).Error)
	require.NotNil(t, mainSite.SiteId)
	require.Zero(t, *mainSite.SiteId)
}

func TestAdtySetUsersSiteTxMovesFundBatchesPG(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &AdtyFundBatch{}, &AdtyUserSiteMigration{}))

	mover := User{Username: "batch-mover", Password: "x", Group: "default", Status: common.UserStatusEnabled, SiteId: 7}
	other := User{Username: "batch-other", Password: "x", Group: "default", Status: common.UserStatusEnabled, SiteId: 9}
	require.NoError(t, db.Create(&mover).Error)
	require.NoError(t, db.Create(&other).Error)

	site7 := 7
	site9 := 9
	require.NoError(t, db.Create([]AdtyFundBatch{
		{UserId: mover.Id, SiteId: &site7, Kind: "principal", Source: "order-site-7", Original: 100, Remaining: 100},
		{UserId: mover.Id, Kind: "principal", Source: "legacy", Original: 20, Remaining: 20},
		{UserId: other.Id, SiteId: &site9, Kind: "principal", Source: "order-site-9", Original: 5, Remaining: 5},
	}).Error)

	// 关站迁主站：客户和批次必须一起走。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		moved, err := AdtySetUsersSiteTx(tx, 7, 0)
		if err != nil {
			return err
		}
		require.Equal(t, []int{mover.Id}, moved)
		return nil
	}))

	bySite := func(source string) *AdtyFundBatch {
		var row AdtyFundBatch
		require.NoError(t, db.Where("source = ?", source).First(&row).Error)
		return &row
	}
	moved := bySite("order-site-7")
	require.NotNil(t, moved.SiteId)
	require.Zero(t, *moved.SiteId, "客户迁到主站，批次不能还挂在旧站")

	require.Nil(t, bySite("legacy").SiteId, "站点本来就未知的行不能被搬成「已知」")
	require.Equal(t, 9, *bySite("order-site-9").SiteId, "没迁的客户的批次不能被动")

	// 主站 → 分站：显式的 0 要搬走，NULL 仍然留 NULL。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		_, err := AdtySetUsersSiteTx(tx, 0, 3)
		return err
	}))
	require.Equal(t, 3, *bySite("order-site-7").SiteId)
	require.Nil(t, bySite("legacy").SiteId)
	require.Equal(t, 9, *bySite("order-site-9").SiteId)

	var stored User
	require.NoError(t, db.First(&stored, mover.Id).Error)
	require.Equal(t, 3, stored.SiteId, "客户与批次必须落在同一个站")
}

// 站点迁移的测试夹具经常只建 users/tokens。没有批次表就没有批次可搬，不能因此
// 让关站迁移整笔失败。
func TestAdtySetUsersSiteTxWithoutFundBatchTablePG(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&User{}, &Token{}, &AdtyUserSiteMigration{}))
	require.False(t, db.Migrator().HasTable(&AdtyFundBatch{}))

	user := User{Username: "no-batch-table", Password: "x", Group: "default", Status: common.UserStatusEnabled, SiteId: 5}
	require.NoError(t, db.Create(&user).Error)

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		moved, err := AdtySetUsersSiteTx(tx, 5, 0)
		if err != nil {
			return err
		}
		require.Equal(t, []int{user.Id}, moved)
		return nil
	}))

	var stored User
	require.NoError(t, db.First(&stored, user.Id).Error)
	require.Zero(t, stored.SiteId)
}

// ISS-R4-0025 + ISS-R4-0047 的交叉点：这一条是补强，不是新缺陷。
//
// 0025 把 CreateAdtyFundBatches* 的幂等判据从「(user_id, source) 计数」改成
// 「(user_id, source, kind) 按 kind 补建」，0047 给批次加了 site_id。
//
// 这两条各自都有测试，而且**「把 site_id 整个从 wanted 里删掉」这种错误
// 0047 自己的测试就会红**（实测：TestAdtyFundBatchStampsSiteOnCreatePG
// 在这条反向改动下 FAIL）。所以这条交叉测试的价值不在那里，而在这里：
//
//	「第一次插入带站点、补建缺 kind 时不带」——这种解法 0047 的测试全绿
//	（它一次调用就把两种 kind 都传进去，走的是「还没有任何行」那条），
//	只有这条会红。合并 w35 那处冲突时，这正是最容易写出来的形状：
//	把站点留在「首次写入」的语义上，忘了补建路径也属于同一次入账。
//
// 走「同一 source 先只传 principal、后补 bonus」那条补建路径，两次都带站点，
// 断言最终两行 site_id 都等于传入值。
func TestAdtyFundBatchTopUpKeepsSiteOnTheMissingKindPG(t *testing.T) {
	db := adtyTestDB(t)
	require.NoError(t, db.AutoMigrate(&AdtyFundBatch{}))

	const userID = 31
	const siteID = 6
	site := siteID

	// 第一次：同一 source 只落 principal。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CreateAdtyFundBatchesWithSiteTx(tx, userID, &site, "order-topup-later", 80, 0)
	}))

	// 第二次：同一 source 补 bonus（0025 的 missing 补建路径）。
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CreateAdtyFundBatchesWithSiteTx(tx, userID, &site, "order-topup-later", 80, 20)
	}))

	var rows []AdtyFundBatch
	require.NoError(t, db.Where("user_id = ? AND source = ?", userID, "order-topup-later").
		Order("kind").Find(&rows).Error)
	require.Len(t, rows, 2, "principal 与 bonus 各一行")
	for _, row := range rows {
		require.NotNil(t, row.SiteId, "%s 这一行必须带站点：补建缺 kind 时不能把 site_id 丢掉", row.Kind)
		require.Equal(t, siteID, *row.SiteId, "%s 这一行的站点必须是传入值", row.Kind)
	}

	// 主站（0）同样是一个具体值，不能被补建成 NULL。
	const mainUser = 32
	mainSite := 0
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CreateAdtyFundBatchesWithSiteTx(tx, mainUser, &mainSite, "order-main-later", 10, 0)
	}))
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		return CreateAdtyFundBatchesWithSiteTx(tx, mainUser, &mainSite, "order-main-later", 10, 5)
	}))
	var mainRows []AdtyFundBatch
	require.NoError(t, db.Where("user_id = ? AND source = ?", mainUser, "order-main-later").
		Order("kind").Find(&mainRows).Error)
	require.Len(t, mainRows, 2)
	for _, row := range mainRows {
		require.NotNil(t, row.SiteId, "%s 这一行必须是主站而不是 NULL", row.Kind)
		require.Zero(t, *row.SiteId, "%s 这一行的站点必须是主站 0", row.Kind)
	}
}
