package core

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/cdle/sillyplus/core/logs"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// PluginLogDoc MongoDB 文档结构
type PluginLogDoc struct {
	ID         interface{} `bson:"_id,omitempty"`
	UUID       string      `bson:"uuid"`
	Level      string      `bson:"level"`
	Content    string      `bson:"content"`
	Unix       int64       `bson:"unix"`
	Version    string      `bson:"version"`
	PluginName string      `bson:"plugin_name"`
}

var (
	pluginLogDB   *mongo.Database
	pluginLogMu   sync.Mutex
	pluginLogCtx  = context.TODO()
	pluginLogReady bool
)

// getPluginLogCollection 获取日志集合，未配置或失败返回 (nil, error)
func getPluginLogCollection() (*mongo.Collection, error) {
	if pluginLogReady {
		return pluginLogDB.Collection("plugin_logs"), nil
	}

	pluginLogMu.Lock()
	defer pluginLogMu.Unlock()

	if pluginLogReady {
		return pluginLogDB.Collection("plugin_logs"), nil
	}

	fanli := MakeBucket("fanli")
	mongodbURL := fanli.GetString("mongodb")
	if mongodbURL == "" {
		mongodbURL = sillyGirl.GetString("fanli.mongodb")
	}
	if mongodbURL == "" {
		return nil, fmt.Errorf("MongoDB not configured")
	}

	client, e := mongo.Connect(pluginLogCtx, options.Client().ApplyURI(mongodbURL))
	if e != nil {
		logs.Warn("插件日志 MongoDB 连接失败: %v，降级到 Redis", e)
		return nil, e
	}
	if e := client.Ping(pluginLogCtx, nil); e != nil {
		logs.Warn("插件日志 MongoDB ping 失败: %v，降级到 Redis", e)
		return nil, e
	}
	pluginLogDB = client.Database("fanyong")
	pluginLogReady = true
	logs.Info("插件日志已连接 MongoDB (fanyong.plugin_logs)")
	initPluginLogIndexes()
	return pluginLogDB.Collection("plugin_logs"), nil
}

func initPluginLogIndexes() {
	coll, err := getPluginLogCollection()
	if err != nil {
		return
	}
	_, err = coll.Indexes().CreateMany(pluginLogCtx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "uuid", Value: 1}, {Key: "_id", Value: -1}}},
		{Keys: bson.D{{Key: "uuid", Value: 1}, {Key: "level", Value: 1}, {Key: "_id", Value: -1}}},
	})
	if err != nil {
		logs.Warn("插件日志 MongoDB 索引创建失败: %v", err)
	}
}

// mongoLogFilter 构建通用查询过滤条件
func mongoLogFilter(uuid, level string, since int64, expireSec int) bson.M {
	filter := bson.M{"uuid": uuid}
	conditions := []bson.M{}

	// 过期过滤
	expireAt := time.Now().Unix() - int64(expireSec)
	conditions = append(conditions, bson.M{"unix": bson.M{"$gt": expireAt}})

	if level != "" {
		conditions = append(conditions, bson.M{"level": level})
	}
	if since > 0 {
		conditions = append(conditions, bson.M{"unix": bson.M{"$gte": since}})
	}

	if len(conditions) > 0 {
		filter["$and"] = conditions
	}
	return filter
}

// writeLogMongo 写入日志到 MongoDB（upsert）
func (s *PluginLogStore) writeLogMongo(coll *mongo.Collection, plog PluginLog) {
	filter := bson.M{
		"uuid":    plog.UUID,
		"level":   plog.Level,
		"content": plog.Content,
	}
	update := bson.M{"$set": bson.M{
		"level":       plog.Level,
		"content":     plog.Content,
		"unix":        plog.Unix,
		"version":     plog.Version,
		"plugin_name": plog.PluginName,
	}}
	opts := options.Update().SetUpsert(true)
	_, err := coll.UpdateOne(pluginLogCtx, filter, update, opts)
	if err != nil {
		logs.Warn("插件日志 MongoDB 写入失败: %v", err)
	}
}

// getLogsMongo 从 MongoDB 查询日志（真正分页）
func (s *PluginLogStore) getLogsMongo(coll *mongo.Collection, uuid, level string, since int64, offset, limit int) []PluginLog {
	filter := mongoLogFilter(uuid, level, since, s.expireSec)

	opts := options.Find().
		SetSort(bson.D{{Key: "_id", Value: -1}}).
		SetSkip(int64(offset)).
		SetLimit(int64(limit))

	cursor, err := coll.Find(pluginLogCtx, filter, opts)
	if err != nil {
		logs.Warn("插件日志 MongoDB 查询失败: %v", err)
		return nil
	}
	defer cursor.Close(pluginLogCtx)

	var docs []PluginLogDoc
	if err := cursor.All(pluginLogCtx, &docs); err != nil {
		logs.Warn("插件日志 MongoDB 解码失败: %v", err)
		return nil
	}

	result := make([]PluginLog, len(docs))
	for i, doc := range docs {
		result[i] = PluginLog{
			UUID:       doc.UUID,
			Level:      doc.Level,
			Content:    doc.Content,
			Unix:       doc.Unix,
			Version:    doc.Version,
			PluginName: doc.PluginName,
		}
	}
	return result
}

// countLogsMongo 从 MongoDB 计数
func (s *PluginLogStore) countLogsMongo(coll *mongo.Collection, uuid, level string, since int64) int {
	filter := mongoLogFilter(uuid, level, since, s.expireSec)
	count, err := coll.CountDocuments(pluginLogCtx, filter)
	if err != nil {
		logs.Warn("插件日志 MongoDB 计数失败: %v", err)
		return 0
	}
	return int(count)
}

// getStatsMongo 从 MongoDB 聚合统计
func (s *PluginLogStore) getStatsMongo(coll *mongo.Collection, uuid string) *PluginLogStats {
	expireAt := time.Now().Unix() - int64(s.expireSec)
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"uuid": uuid,
			"unix": bson.M{"$gt": expireAt},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$level",
			"count": bson.M{"$sum": 1},
		}}},
	}

	cursor, err := coll.Aggregate(pluginLogCtx, pipeline)
	if err != nil {
		logs.Warn("插件日志 MongoDB 聚合失败: %v", err)
		return &PluginLogStats{ByLevel: map[string]int{}}
	}
	defer cursor.Close(pluginLogCtx)

	stats := &PluginLogStats{ByLevel: map[string]int{}}
	for cursor.Next(pluginLogCtx) {
		var result struct {
			ID    string `bson:"_id"`
			Count int    `bson:"count"`
		}
		if err := cursor.Decode(&result); err != nil {
			continue
		}
		stats.Total += result.Count
		stats.ByLevel[result.ID] = result.Count
	}
	return stats
}

// cleanExpiredMongo 清理 MongoDB 中的过期日志
func (s *PluginLogStore) cleanExpiredMongo(coll *mongo.Collection) {
	expireAt := time.Now().Unix() - int64(s.expireSec)
	result, err := coll.DeleteMany(pluginLogCtx, bson.M{"unix": bson.M{"$lt": expireAt}})
	if err != nil {
		logs.Warn("插件日志 MongoDB 清理失败: %v", err)
		return
	}
	if result.DeletedCount > 0 {
		logs.Info("插件日志 MongoDB 清理过期 %d 条", result.DeletedCount)
	}
}
