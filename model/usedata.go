package model

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// QuotaData 柱状图数据
type QuotaData struct {
	Id                  int    `json:"id"`
	UserID              int    `json:"user_id" gorm:"uniqueIndex:idx_quota_data_bucket,priority:1"`
	Username            string `json:"username" gorm:"uniqueIndex:idx_quota_data_bucket,priority:2;index:idx_qdt_model_user_name,priority:2;size:64;default:''"`
	ModelName           string `json:"model_name" gorm:"uniqueIndex:idx_quota_data_bucket,priority:3;index:idx_qdt_model_user_name,priority:1;size:64;default:''"`
	CreatedAt           int64  `json:"created_at" gorm:"bigint;uniqueIndex:idx_quota_data_bucket,priority:4;index:idx_qdt_created_at,priority:2"`
	UseGroup            string `json:"use_group" gorm:"uniqueIndex:idx_quota_data_bucket,priority:5;size:64;default:''"`
	TokenID             int    `json:"token_id" gorm:"uniqueIndex:idx_quota_data_bucket,priority:6;default:0"`
	ChannelID           int    `json:"channel_id" gorm:"uniqueIndex:idx_quota_data_bucket,priority:7;default:0"`
	NodeName            string `json:"node_name" gorm:"uniqueIndex:idx_quota_data_bucket,priority:8;size:64;default:''"`
	TokenUsed           int    `json:"token_used" gorm:"default:0"`
	Count               int    `json:"count" gorm:"default:0"`
	Quota               int    `json:"quota" gorm:"default:0"`
	CacheReadTokens     int64  `json:"cache_read_tokens" gorm:"default:0"`
	CacheCreationTokens int64  `json:"cache_creation_tokens" gorm:"default:0"`
}

type QuotaDataLogParams struct {
	UserID              int
	Username            string
	ModelName           string
	Quota               int
	CreatedAt           int64
	TokenUsed           int
	CacheReadTokens     int64
	CacheCreationTokens int64
	UseGroup            string
	TokenID             int
	ChannelID           int
	NodeName            string
}

func UpdateQuotaData() {
	for {
		if common.DataExportEnabled {
			common.SysLog(common.LogText("updating dashboard data..."))
			SaveQuotaDataCache()
		}
		time.Sleep(time.Duration(common.DataExportInterval) * time.Minute)
	}
}

var CacheQuotaData = make(map[string]*QuotaData)
var CacheQuotaDataLock = sync.Mutex{}

func logQuotaDataCache(quotaData *QuotaData) {
	key := fmt.Sprintf("%d\x00%s\x00%s\x00%d\x00%s\x00%d\x00%d\x00%s",
		quotaData.UserID,
		quotaData.Username,
		quotaData.ModelName,
		quotaData.CreatedAt,
		quotaData.UseGroup,
		quotaData.TokenID,
		quotaData.ChannelID,
		quotaData.NodeName,
	)
	count := quotaData.Count
	quota := quotaData.Quota
	tokenUsed := quotaData.TokenUsed
	cachedQuotaData, ok := CacheQuotaData[key]
	if ok {
		cachedQuotaData.Count += count
		cachedQuotaData.Quota += quota
		cachedQuotaData.TokenUsed += tokenUsed
		cachedQuotaData.CacheReadTokens += quotaData.CacheReadTokens
		cachedQuotaData.CacheCreationTokens += quotaData.CacheCreationTokens
		quotaData = cachedQuotaData
	}
	CacheQuotaData[key] = quotaData
}

func LogQuotaData(params QuotaDataLogParams) {
	// 只精确到小时
	createdAt := params.CreatedAt - (params.CreatedAt % 3600)
	quotaData := &QuotaData{
		UserID:              params.UserID,
		Username:            params.Username,
		ModelName:           params.ModelName,
		CreatedAt:           createdAt,
		UseGroup:            params.UseGroup,
		TokenID:             params.TokenID,
		ChannelID:           params.ChannelID,
		NodeName:            params.NodeName,
		Count:               1,
		Quota:               params.Quota,
		TokenUsed:           params.TokenUsed,
		CacheReadTokens:     max(0, params.CacheReadTokens),
		CacheCreationTokens: max(0, params.CacheCreationTokens),
	}

	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	logQuotaDataCache(quotaData)
}

func SaveQuotaDataCache() {
	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	failed := make(map[string]*QuotaData)
	saved := 0
	for key, quotaData := range CacheQuotaData {
		if err := saveOneQuotaData(quotaData); err != nil {
			common.SysError(common.LogText("save dashboard data failed user_id=%d model_name=%s error=%q", quotaData.UserID, quotaData.ModelName, err.Error()))
			failed[key] = quotaData
			continue
		}
		saved++
	}
	CacheQuotaData = failed
	common.SysLog(common.LogText("saved dashboard data, %d records, %d kept after write failure", saved, len(failed)))
}

func quotaDataBucket(tx *gorm.DB, quotaData *QuotaData) *gorm.DB {
	return tx.Where(
		"user_id = ? and username = ? and model_name = ? and created_at = ? and use_group = ? and token_id = ? and channel_id = ? and node_name = ?",
		quotaData.UserID, quotaData.Username, quotaData.ModelName, quotaData.CreatedAt, quotaData.UseGroup, quotaData.TokenID, quotaData.ChannelID, quotaData.NodeName,
	)
}

func saveOneQuotaData(quotaData *QuotaData) error {
	existing := &QuotaData{}
	err := quotaDataBucket(DB, quotaData).First(existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		quotaData.Id = 0
		err = DB.Create(quotaData).Error
		if err == nil {
			return nil
		}
		if !isAdtyUniqueViolation(err) {
			return err
		}
		existing = &QuotaData{}
		if err = quotaDataBucket(DB, quotaData).First(existing).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return increaseQuotaDataByID(existing.Id, quotaData)
}

func increaseQuotaDataByID(id int, quotaData *QuotaData) error {
	if id <= 0 {
		return errors.New("quota data row is missing")
	}
	result := DB.Model(&QuotaData{}).Where("id = ?", id).Updates(map[string]any{
		"count":                 gorm.Expr("count + ?", quotaData.Count),
		"quota":                 gorm.Expr("quota + ?", quotaData.Quota),
		"token_used":            gorm.Expr("token_used + ?", quotaData.TokenUsed),
		"cache_read_tokens":     gorm.Expr("cache_read_tokens + ?", quotaData.CacheReadTokens),
		"cache_creation_tokens": gorm.Expr("cache_creation_tokens + ?", quotaData.CacheCreationTokens),
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("quota data update affected %d rows", result.RowsAffected)
	}
	return nil
}

// mergeQuotaDataDuplicates 把同一统计桶的重复行合成一行，好让唯一索引能建上。
func mergeQuotaDataDuplicates(db *gorm.DB) error {
	if db == nil || db.Dialector.Name() != "postgres" {
		return nil
	}
	if !db.Migrator().HasTable(&QuotaData{}) {
		return nil
	}
	merge := `
WITH ranked AS (
  SELECT id,
         SUM(count) OVER w AS sum_count,
         SUM(quota) OVER w AS sum_quota,
         SUM(token_used) OVER w AS sum_token_used,
         SUM(cache_read_tokens) OVER w AS sum_cache_read,
         SUM(cache_creation_tokens) OVER w AS sum_cache_creation,
         ROW_NUMBER() OVER (PARTITION BY user_id, username, model_name, created_at, use_group, token_id, channel_id, node_name ORDER BY id) AS rn,
         COUNT(*) OVER w AS copies
  FROM quota_data
  WINDOW w AS (PARTITION BY user_id, username, model_name, created_at, use_group, token_id, channel_id, node_name)
)
UPDATE quota_data AS q
SET count = r.sum_count,
    quota = r.sum_quota,
    token_used = r.sum_token_used,
    cache_read_tokens = r.sum_cache_read,
    cache_creation_tokens = r.sum_cache_creation
FROM ranked AS r
WHERE q.id = r.id AND r.rn = 1 AND r.copies > 1`
	if err := db.Exec(merge).Error; err != nil {
		return err
	}
	return db.Exec(`
DELETE FROM quota_data AS q
USING (
  SELECT id,
         ROW_NUMBER() OVER (PARTITION BY user_id, username, model_name, created_at, use_group, token_id, channel_id, node_name ORDER BY id) AS rn
  FROM quota_data
) AS ranked
WHERE q.id = ranked.id AND ranked.rn > 1`).Error
}

func GetQuotaDataByUsername(username string, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	err = DB.Table("quota_data").
		Select("user_id, username, model_name, created_at, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used, sum(cache_read_tokens) as cache_read_tokens, sum(cache_creation_tokens) as cache_creation_tokens").
		Where("username = ? and created_at >= ? and created_at <= ?", username, startTime, endTime).
		Group("user_id, username, model_name, created_at").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaDataByUserId(userId int, startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	err = DB.Table("quota_data").
		Select("user_id, username, model_name, created_at, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used, sum(cache_read_tokens) as cache_read_tokens, sum(cache_creation_tokens) as cache_creation_tokens").
		Where("user_id = ? and created_at >= ? and created_at <= ?", userId, startTime, endTime).
		Group("user_id, username, model_name, created_at").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetQuotaDataGroupByUser(startTime int64, endTime int64) (quotaData []*QuotaData, err error) {
	var quotaDatas []*QuotaData
	err = DB.Table("quota_data").
		Select("username, created_at, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used, sum(cache_read_tokens) as cache_read_tokens, sum(cache_creation_tokens) as cache_creation_tokens").
		Where("created_at >= ? and created_at <= ?", startTime, endTime).
		Group("username, created_at").
		Find(&quotaDatas).Error
	return quotaDatas, err
}

func GetAllQuotaDates(startTime int64, endTime int64, username string) (quotaData []*QuotaData, err error) {
	if username != "" {
		return GetQuotaDataByUsername(username, startTime, endTime)
	}
	var quotaDatas []*QuotaData
	// 从quota_data表中查询数据
	// only select model_name, sum(count) as count, sum(quota) as quota, model_name, created_at from quota_data group by model_name, created_at;
	//err = DB.Table("quota_data").Where("created_at >= ? and created_at <= ?", startTime, endTime).Find(&quotaDatas).Error
	err = DB.Table("quota_data").Select("model_name, sum(count) as count, sum(quota) as quota, sum(token_used) as token_used, sum(cache_read_tokens) as cache_read_tokens, sum(cache_creation_tokens) as cache_creation_tokens, created_at").Where("created_at >= ? and created_at <= ?", startTime, endTime).Group("model_name, created_at").Find(&quotaDatas).Error
	return quotaDatas, err
}
