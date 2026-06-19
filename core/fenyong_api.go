package core

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/cdle/sillyplus/utils"
	"github.com/gin-gonic/gin"
)

// Mongo 连接 — 延迟初始化，只连接一次
var fanyongDB *mongo.Database
var fanyongCtx = context.TODO()

func getFanyongCollection() (*mongo.Collection, error) {
	if fanyongDB != nil {
		return fanyongDB.Collection("orders"), nil
	}

	// 从 Bucket 读取 MongoDB 配置
	fanli := MakeBucket("fanli")
	mongodbURL := fanli.GetString("mongodb")
	if mongodbURL == "" {
		// 也尝试从傻妞存储读取（兼容旧配置）
		mongodbURL = sillyGirl.GetString("fanli.mongodb")
	}
	if mongodbURL == "" {
		return nil, fmt.Errorf("未配置 MongoDB 地址 (fanli.mongodb)")
	}

	client, err := mongo.Connect(fanyongCtx, options.Client().ApplyURI(mongodbURL))
	if err != nil {
		return nil, fmt.Errorf("MongoDB 连接失败: %v", err)
	}

	// 验证连接
	if err := client.Ping(fanyongCtx, nil); err != nil {
		return nil, fmt.Errorf("MongoDB ping 失败: %v", err)
	}

	fanyongDB = client.Database("fanyong")
	return fanyongDB.Collection("orders"), nil
}

// FenyongTab 订单统计 tab
type FenyongTab struct {
	Key   string `json:"key"`
	Value int    `json:"value"`
	Title string `json:"title"`
}

// FenyongRowContent 订单详情标签
type FenyongRowContent struct {
	Label  string      `json:"label"`
	Value  interface{} `json:"value"`
	Status string      `json:"status,omitempty"`
}

// FenyongRow 订单行
type FenyongRow struct {
	Name    string                  `json:"name"`
	SkuName string                  `json:"sku_name"`
	Image   string                  `json:"image"`
	Status  string                  `json:"status"`
	Content []FenyongRowContent     `json:"content"`
	Bind    *FenyongBind            `json:"bind"`
	CreatedTime int64              `json:"created_time"`
}

// FenyongBind 用户绑定信息
type FenyongBind struct {
	Platform string `json:"platform"`
	UserID   string `json:"user_id"`
}

// FenyongResult API 返回
type FenyongResult struct {
	Success bool         `json:"success"`
	Data    []FenyongRow `json:"data"`
	Page    int          `json:"page"`
	Total   int          `json:"total"`
	Tabs    []FenyongTab `json:"tabs"`
	Tongji  *FenyongStat `json:"tongji"`
}

// FenyongStat 统计数据
type FenyongStat struct {
	OrderNum             int     `json:"order_num"`
	UserNum              int     `json:"user_num"`
	TotalActual          float64 `json:"total_actual"`
	TotalEstimate        float64 `json:"total_estimate"`
	TotalRakeActual      float64 `json:"total_rake_actual"`
	TotalRakeEstimate    float64 `json:"total_rake_estimate"`
	TotalIRakeActual     float64 `json:"total_irake_actual"`
	TotalIRakeEstimate   float64 `json:"total_irake_estimate"`
	TotalIRakeActualPct  float64 `json:"total_irake_actual_pct"`
	TotalIRakeEstimatePct float64 `json:"total_irake_estimate_pct"`
	TotalIactual         float64 `json:"total_iactual"`
	TotalIestimate       float64 `json:"total_iestimate"`
}

// truncateText 截断字符串
func truncateText(text string, maxLength int) string {
	if len(text) > maxLength {
		return text[:maxLength-3] + "..."
	}
	return text
}

// buildFenyongFilters 构建过滤条件
func buildFenyongFilters(site string, user string, startTime, endTime int) map[string]bson.M {
	filters := map[string]bson.M{
		"tab1": {"estimate": bson.M{"$ne": 0}}, // 有效订单
		"tab2": {},                              // 全部
		"tab3": {"estimate": bson.M{"$eq": 0}},  // 无效订单
	}

	if site != "" {
		for key := range filters {
			filters[key]["site"] = site
		}
	}

	// 处理用户过滤
	if user != "" {
		parts := strings.SplitN(user, "#", 2)
		if len(parts) == 2 {
			platform := parts[0]
			userID := parts[1]
			switch userID {
			case "已绑":
				for key := range filters {
					filters[key]["bind"] = bson.M{"$exists": true}
				}
			case "未绑":
				for key := range filters {
					filters[key]["bind"] = bson.M{"$exists": false}
				}
			default:
				for key := range filters {
					if platform != "" {
						filters[key]["bind.platform"] = platform
						filters[key]["bind.user_id"] = userID
					}
				}
			}
		}
	}

	// 处理时间过滤
	if startTime > 0 || endTime > 0 {
		createdTime := bson.M{}
		if startTime > 0 {
			createdTime["$gte"] = startTime
		}
		if endTime > 0 {
			createdTime["$lt"] = endTime
		}
		for key := range filters {
			filters[key]["created_time"] = createdTime
		}
	}

	return filters
}

// getTabTitle 获取 tab 标题
func getTabTitle(key string) string {
	switch key {
	case "tab1":
		return "有效订单"
	case "tab2":
		return "全部"
	case "tab3":
		return "无效订单"
	case "tab4":
		return "待结算"
	case "tab5":
		return "已结算"
	case "tab6":
		return "已亏本"
	default:
		return key
	}
}

// convertFenyongRows 转换 MongoDB 文档为返回格式
func convertFenyongRows(results []bson.M) []FenyongRow {
	rows := make([]FenyongRow, 0, len(results))

	for _, item := range results {
		row := FenyongRow{}

		// 提取字段
		if name, ok := item["name"].(string); ok {
			row.Name = name
		}
		if skuName, ok := item["sku_name"].(string); ok {
			row.SkuName = truncateText(skuName, 40)
		}
		if image, ok := item["image"].(string); ok {
			row.Image = image
		}
		if createdTime, ok := item["created_time"].(int64); ok {
			row.CreatedTime = createdTime
		}

		// 绑定信息
		if bindData, ok := item["bind"].(bson.M); ok {
			if platform, ok := bindData["platform"].(string); ok {
				if userID, ok := bindData["user_id"].(string); ok {
					row.Bind = &FenyongBind{
						Platform: platform,
						UserID:   userID,
					}
				}
			}
		}

		// 构建 content
		row.Content = []FenyongRowContent{
			{Label: "订单时间", Value: formatTime(item["created_time"])},
			{Label: "订单金额", Value: item["cost"]},
			{Label: "预估佣金", Value: item["estimate"]},
			{Label: "实际佣金", Value: item["actual"]},
		}

		// 构建 status 和图片（根据 site）
		if site, ok := item["site"].(string); ok {
			skuID, _ := item["sku_id"].(string)
			orderID, _ := item["order_id"].(string)
			status, _ := item["status"].(string)
			row.Status = fmt.Sprintf("%s %s %s %s", getSiteName(site), skuID, orderID, status)
			row.Image = getSiteImage(site)
		}

		rows = append(rows, row)
	}

	return rows
}

// formatTime 格式化时间戳
func formatTime(val interface{}) string {
	switch v := val.(type) {
	case int64:
		return time.Unix(v, 0).Format("2006-01-02 15:04:05")
	case float64:
		return time.Unix(int64(v), 0).Format("2006-01-02 15:04:05")
	case primitive.DateTime:
		return time.Time(v.Time()).Format("2006-01-02 15:04:05")
	default:
		return fmt.Sprintf("%v", val)
	}
}

// getSiteName 获取平台名称
func getSiteName(site string) string {
	switch site {
	case "jd":
		return "京东"
	case "tb":
		return "淘宝"
	case "pdd":
		return "拼多多"
	default:
		return site
	}
}

// getSiteImage 获取平台图片
func getSiteImage(site string) string {
	switch site {
	case "jd":
		return "https://img10.360buyimg.com/img/jfs/t1/182127/16/37474/11761/64659c31F0cd84976/21f25b952f03a49a.jpg"
	case "tb":
		return "https://gw.alicdn.com/imgextra/i1/O1CN018qjIZA1yiLUFgmBpM_!!6000000006612-73-tps-64-64.ico"
	case "pdd":
		return "https://t16img.yangkeduo.com/mms_static/c9cf044aadd7ae505d0100e54cd4c608.ico"
	default:
		return ""
	}
}

// getFenyongTongji 获取统计数据
func getFenyongTongji(startTime, endTime, site, user string, coll *mongo.Collection) *FenyongStat {
	// 构建 match 条件
	match := bson.M{}
	startTimeInt := utils.Int(startTime)
	endTimeInt := utils.Int(endTime)

	if startTimeInt > 0 || endTimeInt > 0 {
		createdTime := bson.M{}
		if startTimeInt > 0 {
			createdTime["$gte"] = startTimeInt
		}
		if endTimeInt > 0 {
			createdTime["$lt"] = endTimeInt
		}
		match["created_time"] = createdTime
	}
	if site != "" {
		match["site"] = site
	}
	if user != "" {
		parts := strings.SplitN(user, "#", 2)
		if len(parts) == 2 {
			platform := parts[0]
			userID := parts[1]
			if userID == "已绑" {
				match["bind"] = bson.M{"$exists": true}
			} else if userID == "未绑" {
				match["bind"] = bson.M{"$exists": false}
			} else if platform != "" {
				match["bind.platform"] = platform
				match["bind.user_id"] = userID
			}
		}
	}

	// 聚合管道：$match → $group → $project
	pipeline := mongo.Pipeline{}
	if len(match) > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: match}})
	}
	pipeline = append(pipeline, bson.D{{Key: "$group", Value: bson.M{
		"_id": bson.M{
			"platform": "$bind.platform",
			"user_id":  "$bind.user_id",
		},
		"count":          bson.M{"$sum": 1},
		"cost":           bson.M{"$sum": "$cost"},
		"actual":         bson.M{"$sum": "$actual"},
		"estimate":       bson.M{"$sum": "$estimate"},
		"rake_actual":    bson.M{"$sum": bson.M{"$multiply": []interface{}{"$actual", "$rake"}}},
		"rake_estimate":  bson.M{"$sum": bson.M{"$multiply": []interface{}{"$estimate", "$rake"}}},
		"irake_actual":   bson.M{"$sum": bson.M{"$multiply": []interface{}{"$actual", bson.M{"$subtract": []interface{}{1, "$rake"}}}}},
		"irake_estimate": bson.M{"$sum": bson.M{"$multiply": []interface{}{"$estimate", bson.M{"$subtract": []interface{}{1, "$rake"}}}}},
	}}})
	pipeline = append(pipeline, bson.D{{Key: "$project", Value: bson.M{
		"_id":            0,
		"bind":           bson.M{"platform": "$_id.platform", "user_id": "$_id.user_id"},
		"actual":         0.1,
		"estimate":       0.1,
		"rake_actual":    0.1,
		"rake_estimate":  0.1,
		"irake_actual":   0.1,
		"irake_estimate": 0.1,
		"count":          1,
		"cost":           0.1,
	}}})

	cursor, err := coll.Aggregate(fanyongCtx, pipeline)
	if err != nil {
		return &FenyongStat{}
	}
	defer cursor.Close(fanyongCtx)

	var allResults []bson.M
	if err := cursor.All(fanyongCtx, &allResults); err != nil {
		return &FenyongStat{}
	}

	stat := &FenyongStat{}
	userNum := -1
	orderNum := 0
	userOrderNum := 0

	for _, result := range allResults {
		userNum++
		count := 0
		if countVal, ok := result["count"].(int32); ok {
			count = int(countVal)
		}
		orderNum += count

		totalActual, _ := result["actual"].(float64)
		totalEstimate, _ := result["estimate"].(float64)
		stat.TotalActual += totalActual
		stat.TotalEstimate += totalEstimate

		if bind, ok := result["bind"].(bson.M); ok {
			if userID, ok := bind["user_id"].(string); ok && userID != "" {
				userOrderNum += count
				rakeActual, _ := result["rake_actual"].(float64)
				rakeEstimate, _ := result["rake_estimate"].(float64)
				irakeActual, _ := result["irake_actual"].(float64)
				irakeEstimate, _ := result["irake_estimate"].(float64)

				stat.TotalRakeActual += rakeActual
				stat.TotalRakeEstimate += rakeEstimate
				stat.TotalIRakeActual += irakeActual
				stat.TotalIRakeEstimate += irakeEstimate
			}
		}
	}

	if userNum < 0 {
		userNum = 0
	}
	stat.OrderNum = orderNum
	stat.UserNum = userNum

	// 计算不含返佣的数据
	stat.TotalIactual = stat.TotalActual - stat.TotalRakeActual
	stat.TotalIestimate = stat.TotalEstimate - stat.TotalRakeEstimate

	// 计算收益率
	if stat.TotalIRakeActual+stat.TotalRakeActual != 0 {
		stat.TotalIRakeActualPct = stat.TotalIRakeActual / (stat.TotalIRakeActual + stat.TotalRakeActual)
	}
	if stat.TotalIRakeEstimate+stat.TotalRakeEstimate != 0 {
		stat.TotalIRakeEstimatePct = stat.TotalIRakeEstimate / (stat.TotalIRakeEstimate + stat.TotalRakeEstimate)
	}

	return stat
}

func initFenyongAPI() {
	GinApi(GET, "/api/fanyong", RequireAuth, func(c *gin.Context) {
		pageSize := utils.Int(c.Query("pageSize"))
		if pageSize == 0 {
			pageSize = 10
		}
		current := utils.Int(c.Query("current"))
		if current == 0 {
			current = 1
		}
		startTime := utils.Int(c.Query("startTime"))
		endTime := utils.Int(c.Query("endTime"))
		tab := c.Query("activeKey")
		site := c.Query("site")
		init := c.Query("init")
		user := c.Query("user")

		if tab == "" {
			tab = "tab1"
		}
		if site == "all" {
			site = ""
		}

		// 构建过滤条件
		filters := buildFenyongFilters(site, user, startTime, endTime)

		coll, err := getFanyongCollection()
		if err != nil {
			c.JSON(200, FenyongResult{
				Success: true,
				Data:    []FenyongRow{},
				Page:    current,
				Total:   0,
				Tabs:    []FenyongTab{},
			})
			return
		}

		// 获取各 tab 的数量
		tabs := []FenyongTab{}
		for key, filter := range filters {
			count, _ := coll.CountDocuments(fanyongCtx, filter)
			tabs = append(tabs, FenyongTab{
				Key:   key,
				Value: int(count),
				Title: getTabTitle(key),
			})
		}

		// 查询数据
		count := tabs[0].Value
		for _, t := range tabs {
			if t.Key == tab {
				count = t.Value
				break
			}
		}

		filter := filters[tab]
		if filter == nil {
			filter = bson.M{"estimate": bson.M{"$ne": 0}}
		}

		skip := int64(pageSize * (current - 1))
		cursor, err := coll.Find(fanyongCtx, filter,
			options.Find().SetSort(bson.M{"created_time": -1}),
			options.Find().SetSkip(skip),
			options.Find().SetLimit(int64(pageSize)),
			options.Find().SetProjection(bson.M{"data": 0}),
		)
		if err != nil {
			c.JSON(500, gin.H{"success": true, "data": []FenyongRow{}, "page": current, "total": 0, "tabs": tabs})
			return
		}
		defer cursor.Close(fanyongCtx)

		var results []bson.M
		if err := cursor.All(fanyongCtx, &results); err != nil {
			c.JSON(500, gin.H{"success": true, "data": []FenyongRow{}, "page": current, "total": 0, "tabs": tabs})
			return
		}

		// 转换数据格式
		rows := convertFenyongRows(results)

		// 统计
		var tongji *FenyongStat
		if init == "true" {
			tongji = getFenyongTongji(c.Query("startTime"), c.Query("endTime"), site, user, coll)
		}

		c.JSON(200, FenyongResult{
			Success: true,
			Data:    rows,
			Page:    current,
			Total:   count,
			Tabs:    tabs,
			Tongji:  tongji,
		})
	})

	// 统计接口
	GinApi(GET, "/api/fanyong/tongji", RequireAuth, func(c *gin.Context) {
		startTime := c.Query("startTime")
		endTime := c.Query("endTime")

		coll, err := getFanyongCollection()
		if err != nil {
			c.JSON(200, gin.H{
				"data":    nil,
				"success": true,
				"message": err.Error(),
			})
			return
		}

		tongji := getFenyongTongji(startTime, endTime, "", "", coll)
		c.JSON(200, gin.H{
			"data":    tongji,
			"success": true,
		})
	})
}
