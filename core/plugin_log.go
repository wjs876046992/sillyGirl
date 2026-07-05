package core

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/cdle/sillyplus/core/common"
	"github.com/cdle/sillyplus/core/logs"
	"github.com/cdle/sillyplus/core/storage"
	"github.com/cdle/sillyplus/utils"
)

// PluginLog 插件日志结构
type PluginLog struct {
	UUID       string `json:"uuid"`        // 插件UUID
	Level      string `json:"level"`       // 日志级别: info/debug/warn/error/log
	Content    string `json:"content"`     // 日志内容
	Unix       int64  `json:"unix"`        // 时间戳
	Version    string `json:"version"`     // 插件版本
	PluginName string `json:"plugin_name"` // 插件名称
}

// PluginLogStats 日志统计
type PluginLogStats struct {
	Total   int            `json:"total"`    // 总数
	ByLevel map[string]int `json:"by_level"` // 按级别统计
}

// PluginLogStore 日志存储管理器
type PluginLogStore struct {
	bucket    storage.Bucket
	mu        sync.RWMutex
	expireSec int // 过期时间（秒），默认86400=1天
}

var pluginLogs = &PluginLogStore{
	bucket:    MakeBucket("plugin_logs"),
	expireSec: 86400, // 1天
}

// SetExpireSec 设置过期时间（秒）
func (s *PluginLogStore) SetExpireSec(sec int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireSec = sec
}

// WriteLog 写入日志
func (s *PluginLogStore) WriteLog(uuid, level, content string) {
	if uuid == "" || content == "" {
		return
	}

	f := GetFunctionByUUID(uuid)
	plog := PluginLog{
		UUID:       uuid,
		Level:      level,
		Content:    content,
		Unix:       time.Now().Unix(),
		Version:    getVersion(f),
		PluginName: getTitle(f),
	}

	// 优先 MongoDB
	if coll, err := getPluginLogCollection(); err == nil {
		s.writeLogMongo(coll, plog)
		return
	}

	// 降级 Redis
	s.mu.Lock()
	defer s.mu.Unlock()

	plogs := s.getLogs(uuid)
	if !s.deduplicate(plogs, plog) {
		plogs = append(plogs, plog)
	}
	if len(plogs) > 1000 {
		plogs = plogs[len(plogs)-1000:]
	}
	s.setLogs(uuid, plogs)
}

// GetLogs 获取插件日志（支持过滤、分页）
func (s *PluginLogStore) GetLogs(uuid, level string, since int64, offset, limit int) []PluginLog {
	if uuid == "" {
		return nil
	}

	// 优先 MongoDB
	if coll, err := getPluginLogCollection(); err == nil {
		return s.getLogsMongo(coll, uuid, level, since, offset, limit)
	}

	// 降级 Redis
	s.mu.RLock()
	defer s.mu.RUnlock()

	plogs := s.getLogs(uuid)
	now := time.Now().Unix()

	var filtered []PluginLog
	for i := len(plogs) - 1; i >= 0; i-- {
		plog := plogs[i]
		if now-plog.Unix > int64(s.expireSec) {
			continue
		}
		if level != "" && plog.Level != level {
			continue
		}
		if since > 0 && plog.Unix < since {
			continue
		}
		filtered = append(filtered, plog)
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Unix > filtered[j].Unix
	})

	total := len(filtered)
	if offset >= total {
		return []PluginLog{}
	}
	end := offset + limit
	if limit <= 0 || end > total {
		end = total
	}
	return filtered[offset:end]
}

// CountLogs 统计插件日志数量（支持级别和时间过滤）
func (s *PluginLogStore) CountLogs(uuid, level string, since int64) int {
	if uuid == "" {
		return 0
	}

	// 优先 MongoDB
	if coll, err := getPluginLogCollection(); err == nil {
		return s.countLogsMongo(coll, uuid, level, since)
	}

	// 降级 Redis
	s.mu.RLock()
	plogs := s.getLogs(uuid)
	s.mu.RUnlock()
	now := time.Now().Unix()
	count := 0
	for i := len(plogs) - 1; i >= 0; i-- {
		plog := plogs[i]
		if now-plog.Unix > int64(s.expireSec) {
			continue
		}
		if level != "" && plog.Level != level {
			continue
		}
		if since > 0 && plog.Unix < since {
			continue
		}
		count++
	}
	return count
}

// GetStats 获取日志统计
func (s *PluginLogStore) GetStats(uuid string) *PluginLogStats {
	if uuid == "" {
		return &PluginLogStats{
			Total:   0,
			ByLevel: map[string]int{},
		}
	}

	// 优先 MongoDB
	if coll, err := getPluginLogCollection(); err == nil {
		return s.getStatsMongo(coll, uuid)
	}

	// 降级 Redis
	s.mu.RLock()
	plogs := s.getLogs(uuid)
	s.mu.RUnlock()
	now := time.Now().Unix()
	stats := &PluginLogStats{
		ByLevel: map[string]int{},
	}
	for _, plog := range plogs {
		if now-plog.Unix > int64(s.expireSec) {
			continue
		}
		stats.Total++
		stats.ByLevel[plog.Level]++
	}
	return stats
}

// CleanExpired 清理过期日志
func (s *PluginLogStore) CleanExpired() {
	// 优先 MongoDB
	if coll, err := getPluginLogCollection(); err == nil {
		s.cleanExpiredMongo(coll)
		return
	}

	// 降级 Redis
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()
	keys, err := s.bucket.Keys()
	if err != nil {
		logs.Error("获取插件日志keys失败: %v", err)
		return
	}

	cleaned := 0
	for _, key := range keys {
		if len(key) < 36 {
			continue
		}
		plogs := s.getLogsRaw(key)
		if plogs == nil {
			continue
		}
		var valid []PluginLog
		for _, plog := range plogs {
			if now-plog.Unix <= int64(s.expireSec) {
				valid = append(valid, plog)
			}
		}
		if len(valid) == 0 {
			bkt := s.bucket.Copy(key)
			bkt.Delete()
			cleaned++
		} else if len(valid) != len(plogs) {
			s.setLogsRaw(key, valid)
			cleaned++
		}
	}

	if cleaned > 0 {
		logs.Info("清理过期插件日志 %d 个", cleaned)
	}
}

// 内部方法

func (s *PluginLogStore) getLogs(uuid string) []PluginLog {
	data := s.bucket.GetBytes(uuid)
	if data == nil {
		return []PluginLog{}
	}
	var result []PluginLog
	if err := json.Unmarshal(data, &result); err != nil {
		return []PluginLog{}
	}
	return result
}

func (s *PluginLogStore) getLogsRaw(key string) []PluginLog {
	data := s.bucket.GetBytes(key)
	if data == nil {
		return nil
	}
	var result []PluginLog
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result
}

func (s *PluginLogStore) setLogs(uuid string, plogs []PluginLog) {
	data := utils.JsonMarshal(plogs)
	s.bucket.Set(uuid, string(data))
}

func (s *PluginLogStore) setLogsRaw(key string, plogs []PluginLog) {
	data := utils.JsonMarshal(plogs)
	s.bucket.Set(key, string(data))
}

func (s *PluginLogStore) deduplicate(plogs []PluginLog, new PluginLog) bool {
	for i := range plogs {
		if plogs[i].Level == new.Level && plogs[i].Content == new.Content {
			plogs[i] = new
			return true
		}
	}
	return false
}

// 辅助函数

func getVersion(f *common.Function) string {
	if f == nil {
		return ""
	}
	return f.Version
}

func getTitle(f *common.Function) string {
	if f == nil {
		return ""
	}
	return fmt.Sprintf("%s%s", f.Title, f.Suffix)
}

// InitPluginLogs 初始化日志系统
func InitPluginLogs() {
	// 启动时清理一次
	go func() {
		time.Sleep(5 * time.Second)
		pluginLogs.CleanExpired()
	}()

	// 每小时清理一次
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		for range ticker.C {
			pluginLogs.CleanExpired()
		}
	}()

	// 初始化API
	initPluginLogAPI()
}
