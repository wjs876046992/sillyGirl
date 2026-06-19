package core

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/cdle/sillyplus/utils"
	"github.com/gin-gonic/gin"
)

// ========== MongoDB 连接管理 ==========

var (
	fanyongDB   *mongo.Database
	fanyongOnce sync.Once
	fanyongCtx  = context.TODO()
)

func getFanyongCollection() (*mongo.Collection, error) {
	var err error
	fanyongOnce.Do(func() {
		fanli := MakeBucket("fanli")
		mongodbURL := fanli.GetString("mongodb")
		if mongodbURL == "" {
			mongodbURL = sillyGirl.GetString("fanli.mongodb")
		}
		if mongodbURL == "" {
			err = fmt.Errorf("未配置 MongoDB 地址 (fanli.mongodb)")
			return
		}
		client, e := mongo.Connect(fanyongCtx, options.Client().ApplyURI(mongodbURL))
		if e != nil {
			err = fmt.Errorf("MongoDB 连接失败: %w", e)
			return
		}
		if e := client.Ping(fanyongCtx, nil); e != nil {
			err = fmt.Errorf("MongoDB ping 失败: %w", e)
			return
		}
		fanyongDB = client.Database("fanyong")
	})
	if err != nil {
		return nil, err
	}
	return fanyongDB.Collection("orders"), nil
}

// 获取结算周期配置（默认30天）
func getSettleDays() int64 {
	fanli := MakeBucket("fanli")
	days := fanli.GetInt("fy_day")
	if days <= 0 {
		days = 30
	}
	return int64(days)
}

// ========== 数据结构 ==========

// FenyongOrder 订单行
type FenyongOrder struct {
	Name        string                  `json:"name"`
	SkuName     string                  `json:"sku_name"`
	Image       string                  `json:"image"`
	Status      string                  `json:"status"`
	Content     []FenyongOrderContent   `json:"content"`
	Bind        *FenyongBind            `json:"bind"`
	Site        string                  `json:"site"`
	Platform    string                  `json:"platform"` // 绑定的平台
	CreatedTime int64                   `json:"created_time"`
	OrderID     string                  `json:"order_id"`
	SkuID       string                  `json:"sku_id"`
}

// FenyongOrderContent 订单详情标签
type FenyongOrderContent struct {
	Label  string      `json:"label"`
	Value  interface{} `json:"value"`
	Status string      `json:"status,omitempty"`
}

// FenyongBind 用户绑定信息
type FenyongBind struct {
	Platform string `json:"platform"`
	UserID   string `json:"user_id"`
	Nickname string `json:"nickname,omitempty"`
}

// FenyongOrderResult 订单列表返回
type FenyongOrderResult struct {
	Success bool            `json:"success"`
	Data    []FenyongOrder  `json:"data"`
	Page    int             `json:"page"`
	Total   int64           `json:"total"`
}

// FenyongPlatformStats 平台统计
type FenyongPlatformStats struct {
	Orders   int64   `json:"orders"`
	Estimate float64 `json:"estimate"`
	Actual   float64 `json:"actual"`
}

// FenyongPeriodStats 时间段统计
type FenyongPeriodStats struct {
	Orders   int64   `json:"orders"`
	Estimate float64 `json:"estimate"`
	Actual   float64 `json:"actual"`
}

// FenyongDashboard 首页仪表盘数据
type FenyongDashboard struct {
	Success     bool                 `json:"success"`
	Today       FenyongPeriodStats   `json:"today"`
	Yesterday   FenyongPeriodStats   `json:"yesterday"`
	Last7Days   FenyongPeriodStats   `json:"last7days"`
	LastMonth   FenyongPeriodStats   `json:"lastMonth"`
	Platforms   map[string]FenyongPlatformStats `json:"platforms"`
	TotalSettled    float64 `json:"total_settled"`
	TotalUnsettled  float64 `json:"total_unsettled"`
	TotalOrders     int64   `json:"total_orders"`
}

// FenyongRefreshResult 刷新结果
type FenyongRefreshResult struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ========== 工具函数 ==========

func truncateText(text string, maxLen int) string {
	if len(text) > maxLen {
		return text[:maxLen-3] + "..."
	}
	return text
}

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

// ========== API 路由 ==========

func initFenyongAPI() {
	GinApi(GET, "/api/fenyong/dashboard", RequireAuth, handleFenyongDashboard)
	GinApi(GET, "/api/fenyong/orders", RequireAuth, handleFenyongOrders)
	GinApi(GET, "/api/fenyong/tongji", RequireAuth, handleFenyongTongji)
	GinApi(POST, "/api/fenyong/refresh", RequireAuth, handleFenyongRefresh)
}

// 获取时间范围
func getTimeRanges() (todayStart, yesterdayStart, yesterdayEnd, last7DaysStart, lastMonthStart int64) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	yesterday := today.Add(-24 * time.Hour)
	last7 := today.Add(-7 * 24 * time.Hour)
	lastMonth := today.AddDate(0, -1, 0)

	return today.Unix(), yesterday.Unix(), today.Unix(), last7.Unix(), lastMonth.Unix()
}

// handleFenyongDashboard 首页仪表盘
func handleFenyongDashboard(c *gin.Context) {
	coll, err := getFanyongCollection()
	if err != nil {
		c.JSON(200, gin.H{"success": true, "error": err.Error()})
		return
	}

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	yesterdayStart := todayStart - 86400
	last7Start := todayStart - 7*86400
	lastMonthStart := todayStart - 31*86400

	// 平台统计
	platforms := map[string]FenyongPlatformStats{"jd": {}, "tb": {}, "pdd": {}}

	for site := range platforms {
		filter := bson.M{"site": site}
		// 今日
		filter["created_time"] = bson.M{"$gte": todayStart}
		count, _ := coll.CountDocuments(fanyongCtx, filter)
		estimate, actual := getSumStats(coll, filter)
		platforms[site] = FenyongPlatformStats{
			Orders:   count,
			Estimate: estimate,
			Actual:   actual,
		}
	}

	// 按时间段统计
	periods := []struct {
		name string
		start int64
		end   int64
	}{
		{"today", todayStart, 0},
		{"yesterday", yesterdayStart, todayStart},
		{"last7days", last7Start, todayStart},
		{"lastMonth", lastMonthStart, todayStart},
	}

	dash := FenyongDashboard{
		Success:     true,
		Platforms:   platforms,
		TotalOrders: 0,
	}

	for _, p := range periods {
		filter := bson.M{}
		if p.end > 0 {
			filter["created_time"] = bson.M{"$gte": p.start, "$lt": p.end}
		} else {
			filter["created_time"] = bson.M{"$gte": p.start}
		}

		count, _ := coll.CountDocuments(fanyongCtx, filter)
		estimate, actual := getSumStats(coll, filter)

		switch p.name {
		case "today":
			dash.Today = FenyongPeriodStats{Orders: count, Estimate: estimate, Actual: actual}
		case "yesterday":
			dash.Yesterday = FenyongPeriodStats{Orders: count, Estimate: estimate, Actual: actual}
		case "last7days":
			dash.Last7Days = FenyongPeriodStats{Orders: count, Estimate: estimate, Actual: actual}
		case "lastMonth":
			dash.LastMonth = FenyongPeriodStats{Orders: count, Estimate: estimate, Actual: actual}
		}
		dash.TotalOrders += count
	}

	// 已结算 / 待结算
	settledFilter := bson.M{"actual": bson.M{"$ne": 0}, "settled": bson.M{"$exists": true}}
	unsettledFilter := bson.M{"actual": bson.M{"$ne": 0}, "settled": bson.M{"$exists": false}}

	// 待结算需要检查结算周期
	fyDays := getSettleDays()
	nowUnix := time.Now().Unix()
	cutoffTime := nowUnix - fyDays*86400
	unsettledFilter["created_time"] = bson.M{"$lt": cutoffTime}

	dash.TotalSettled, _ = getSumStats(coll, settledFilter)
	dash.TotalUnsettled, _ = getSumStats(coll, unsettledFilter)

	c.JSON(200, dash)
}

// handleFenyongOrders 订单列表
func handleFenyongOrders(c *gin.Context) {
	coll, err := getFanyongCollection()
	if err != nil {
		c.JSON(200, FenyongOrderResult{
			Success: true, Data: []FenyongOrder{}, Page: 1, Total: 0,
		})
		return
	}

	pageSize := utils.Int(c.Query("pageSize"))
	if pageSize <= 0 {
		pageSize = 20
	}
	current := utils.Int(c.Query("page"))
	if current <= 0 {
		current = 1
	}
	site := c.Query("site")
	tab := c.Query("tab")
	user := c.Query("user")
	startTime := c.Query("startTime")
	endTime := c.Query("endTime")
	keyword := c.Query("keyword")

	if site == "all" || site == "" {
		site = ""
	}
	if tab == "" {
		tab = "tab1"
	}

	// 构建过滤条件
	filter := buildFenyongFilter(site, tab, user, startTime, endTime, keyword)

	// 总数
	total, _ := coll.CountDocuments(fanyongCtx, filter)

	// 分页查询
	skip := int64(pageSize * (current - 1))
	cursor, err := coll.Find(fanyongCtx, filter,
		options.Find().SetSort(bson.M{"created_time": -1}),
		options.Find().SetSkip(skip),
		options.Find().SetLimit(int64(pageSize)),
		options.Find().SetProjection(bson.M{"data": 0}),
	)
	if err != nil {
		c.JSON(200, FenyongOrderResult{
			Success: true, Data: []FenyongOrder{}, Page: current, Total: 0,
		})
		return
	}
	defer cursor.Close(fanyongCtx)

	var docs []bson.M
	if err := cursor.All(fanyongCtx, &docs); err != nil {
		c.JSON(200, FenyongOrderResult{
			Success: true, Data: []FenyongOrder{}, Page: current, Total: 0,
		})
		return
	}

	orders := convertOrders(docs)

	c.JSON(200, FenyongOrderResult{
		Success: true,
		Data:    orders,
		Page:    current,
		Total:   total,
	})
}

// handleFenyongTongji 统计数据（兼容旧版 API）
func handleFenyongTongji(c *gin.Context) {
	coll, err := getFanyongCollection()
	if err != nil {
		c.JSON(200, gin.H{"success": true, "data": nil})
		return
	}

	startTime := c.Query("startTime")
	endTime := c.Query("endTime")
	site := c.Query("site")
	user := c.Query("user")

	stat := getFenyongStats(startTime, endTime, site, user, coll)
	c.JSON(200, gin.H{"data": stat, "success": true})
}

// handleFenyongRefresh 手动刷新订单
func handleFenyongRefresh(c *gin.Context) {
	// 通知 JS 插件执行刷新（通过 find 一个空文档触发 JS 逻辑）
	c.JSON(200, FenyongRefreshResult{
		Success: true,
		Message: "刷新命令已发送（请确保插件已配置各平台 API Key）",
	})
}

// ========== 辅助函数 ==========

// getSumStats 获取 sum(estimate) 和 sum(actual)
func getSumStats(coll *mongo.Collection, filter bson.M) (estimate, actual float64) {
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: filter}},
		bson.D{{Key: "$group", Value: bson.M{
			"_id":      1,
			"estimate": bson.M{"$sum": "$estimate"},
			"actual":   bson.M{"$sum": "$actual"},
		}}},
	}
	cursor, err := coll.Aggregate(fanyongCtx, pipeline)
	if err != nil {
		return
	}
	defer cursor.Close(fanyongCtx)

	var result struct {
		ID       int     `bson:"_id"`
		Estimate float64 `bson:"estimate"`
		Actual   float64 `bson:"actual"`
	}
	if cursor.Next(fanyongCtx) {
		cursor.Decode(&result)
		estimate = result.Estimate
		actual = result.Actual
	}
	return
}

// buildFenyongFilter 构建订单查询过滤条件
func buildFenyongFilter(site, tab, user, startTime, endTime, keyword string) bson.M {
	filter := bson.M{}

	// Tab 筛选
	switch tab {
	case "tab1": // 有效订单
		filter["estimate"] = bson.M{"$ne": 0}
	case "tab3": // 无效订单
		filter["estimate"] = bson.M{"$eq": 0}
	case "tab4": // 待结算
		filter["actual"] = bson.M{"$ne": 0}
		filter["settled"] = bson.M{"$exists": false}
		fyDays := getSettleDays()
		filter["created_time"] = bson.M{"$lt": time.Now().Unix() - fyDays*86400}
	case "tab5": // 已结算
		filter["actual"] = bson.M{"$ne": 0}
		filter["settled"] = bson.M{"$exists": true}
	case "tab6": // 已亏本
		filter["actual"] = bson.M{"$eq": 0}
		filter["settled"] = bson.M{"$exists": true}
	}

	// 平台筛选
	if site != "" {
		filter["site"] = site
	}

	// 用户筛选
	if user != "" {
		parts := strings.SplitN(user, "#", 2)
		if len(parts) == 2 {
			platform, userID := parts[0], parts[1]
			switch userID {
			case "已绑":
				filter["bind"] = bson.M{"$exists": true}
			case "未绑":
				filter["bind"] = bson.M{"$exists": false}
			default:
				if platform != "" {
					filter["bind.platform"] = platform
					filter["bind.user_id"] = userID
				}
			}
		}
	}

	// 时间范围
	if startTime != "" || endTime != "" {
		timeFilter := bson.M{}
		if startTime != "" {
			t, _ := strconv.ParseInt(startTime, 10, 64)
			if t > 0 {
				timeFilter["$gte"] = t
			}
		}
		if endTime != "" {
			t, _ := strconv.ParseInt(endTime, 10, 64)
			if t > 0 {
				timeFilter["$lt"] = t
			}
		}
		filter["created_time"] = timeFilter
	}

	// 关键词搜索
	if keyword != "" {
		filter["$or"] = []bson.M{
			{"sku_name": bson.M{"$regex": keyword, "$options": "i"}},
			{"order_id": bson.M{"$regex": keyword, "$options": "i"}},
			{"sku_id": bson.M{"$regex": keyword, "$options": "i"}},
		}
	}

	return filter
}

// convertOrders 转换 MongoDB 文档
func convertOrders(docs []bson.M) []FenyongOrder {
	orders := make([]FenyongOrder, 0, len(docs))

	for _, item := range docs {
		order := FenyongOrder{}

		if name, ok := item["name"].(string); ok {
			order.Name = name
		}
		if skuName, ok := item["sku_name"].(string); ok {
			order.SkuName = truncateText(skuName, 40)
		}
		if image, ok := item["image"].(string); ok {
			order.Image = image
		}
		if site, ok := item["site"].(string); ok {
			order.Site = site
			skuID, _ := item["sku_id"].(string)
			orderID, _ := item["order_id"].(string)
			status, _ := item["status"].(string)
			order.Status = fmt.Sprintf("%s %s %s %s", getSiteName(site), skuID, orderID, status)
			order.Image = getSiteImage(site)
		}
		if createdTime, ok := item["created_time"].(int64); ok {
			order.CreatedTime = createdTime
		} else if ct, ok := item["created_time"].(primitive.DateTime); ok {
			order.CreatedTime = ct.Time().Unix()
		}
		if orderID, ok := item["order_id"].(string); ok {
			order.OrderID = orderID
		}
		if skuID, ok := item["sku_id"].(string); ok {
			order.SkuID = skuID
		}

		// 绑定信息
		if bindData, ok := item["bind"].(bson.M); ok {
			if platform, ok := bindData["platform"].(string); ok {
				if userID, ok := bindData["user_id"].(string); ok {
					order.Bind = &FenyongBind{Platform: platform, UserID: userID}
				}
			}
		}

		// 订单详情
		order.Content = []FenyongOrderContent{
			{Label: "订单时间", Value: formatTime(item["created_time"])},
			{Label: "订单金额", Value: item["cost"]},
			{Label: "预估佣金", Value: item["estimate"]},
			{Label: "实际佣金", Value: item["actual"]},
		}

		orders = append(orders, order)
	}

	return orders
}

// getFenyongStats 统计数据（兼容旧版 API）
func getFenyongStats(startTime, endTime, site, user string, coll *mongo.Collection) bson.M {
	// 构建 match
	match := bson.M{}
	if startTime != "" {
		if t, err := strconv.ParseInt(startTime, 10, 64); err == nil && t > 0 {
			match["created_time"] = bson.M{"$gte": t}
		}
	}
	if endTime != "" {
		if t, err := strconv.ParseInt(endTime, 10, 64); err == nil && t > 0 {
			if match["created_time"] != nil {
				ct := match["created_time"].(bson.M)
				ct["$lt"] = t
			} else {
				match["created_time"] = bson.M{"$lt": t}
			}
		}
	}
	if site != "" {
		match["site"] = site
	}
	if user != "" {
		parts := strings.SplitN(user, "#", 2)
		if len(parts) == 2 {
			platform, userID := parts[0], parts[1]
			switch userID {
			case "已绑":
				match["bind"] = bson.M{"$exists": true}
			case "未绑":
				match["bind"] = bson.M{"$exists": false}
			default:
				if platform != "" {
					match["bind.platform"] = platform
					match["bind.user_id"] = userID
				}
			}
		}
	}

	pipeline := mongo.Pipeline{}
	if len(match) > 0 {
		pipeline = append(pipeline, bson.D{{"$match", match}})
	}
	pipeline = append(pipeline, bson.D{{"$group", bson.M{
		"_id": bson.M{"platform": "$bind.platform", "user_id": "$bind.user_id"},
		"count": bson.M{"$sum": 1},
		"cost": bson.M{"$sum": "$cost"},
		"actual": bson.M{"$sum": "$actual"},
		"estimate": bson.M{"$sum": "$estimate"},
		"rake_actual": bson.M{"$sum": bson.M{"$multiply": []interface{}{"$actual", "$rake"}}},
		"rake_estimate": bson.M{"$sum": bson.M{"$multiply": []interface{}{"$estimate", "$rake"}}},
		"irake_actual": bson.M{"$sum": bson.M{"$multiply": []interface{}{"$actual", bson.M{"$subtract": []interface{}{1, "$rake"}}}}},
		"irake_estimate": bson.M{"$sum": bson.M{"$multiply": []interface{}{"$estimate", bson.M{"$subtract": []interface{}{1, "$rake"}}}}},
	}}})
	pipeline = append(pipeline, bson.D{{"$project", bson.M{
		"_id": 0,
		"bind": bson.M{"platform": "$_id.platform", "user_id": "$_id.user_id"},
		"actual": 0.1, "estimate": 0.1,
		"rake_actual": 0.1, "rake_estimate": 0.1,
		"irake_actual": 0.1, "irake_estimate": 0.1,
		"count": 1, "cost": 0.1,
	}}})

	cursor, err := coll.Aggregate(fanyongCtx, pipeline)
	if err != nil {
		return bson.M{}
	}
	defer cursor.Close(fanyongCtx)

	var results []bson.M
	if err := cursor.All(fanyongCtx, &results); err != nil {
		return bson.M{}
	}

	totalActual, totalEstimate := 0.0, 0.0
	totalRakeActual, totalRakeEstimate := 0.0, 0.0
	totalIRakeActual, totalIRakeEstimate := 0.0, 0.0
	orderNum, userNum, userOrderNum := int64(0), int64(-1), int64(0)

	for _, r := range results {
		userNum++
		count := 0
		if c, ok := r["count"].(int32); ok {
			count = int(c)
		}
		orderNum += int64(count)

		if a, ok := r["actual"].(float64); ok {
			totalActual += a
		}
		if e, ok := r["estimate"].(float64); ok {
			totalEstimate += e
		}

		if bind, ok := r["bind"].(bson.M); ok {
			if uid, ok := bind["user_id"].(string); ok && uid != "" {
				userOrderNum += int64(count)
				if ra, ok := r["rake_actual"].(float64); ok {
					totalRakeActual += ra
				}
				if re, ok := r["rake_estimate"].(float64); ok {
					totalRakeEstimate += re
				}
				if ira, ok := r["irake_actual"].(float64); ok {
					totalIRakeActual += ira
				}
				if ire, ok := r["irake_estimate"].(float64); ok {
					totalIRakeEstimate += ire
				}
			}
		}
	}
	if userNum < 0 {
		userNum = 0
	}

	totalIactual := totalActual - totalRakeActual
	totalIestimate := totalEstimate - totalRakeEstimate
	var pctActual, pctEstimate float64
	if totalIRakeActual+totalRakeActual != 0 {
		pctActual = totalIRakeActual / (totalIRakeActual + totalRakeActual)
	}
	if totalIRakeEstimate+totalRakeEstimate != 0 {
		pctEstimate = totalIRakeEstimate / (totalIRakeEstimate + totalRakeEstimate)
	}

	result := bson.M{
		"user_num":              userNum,
		"order_num":             orderNum,
		"total_actual":          totalActual,
		"total_estimate":        totalEstimate,
		"total_rake_actual":     totalRakeActual,
		"total_rake_estimate":   totalRakeEstimate,
		"total_irake_actual":    totalIRakeActual,
		"total_irake_estimate":  totalIRakeEstimate,
		"total_iactual":         totalIactual,
		"total_iestimate":       totalIestimate,
		"total_irake_actual_pct":  pctActual,
		"total_irake_estimate_pct": pctEstimate,
	}

	// 用户列表
	type UserResult struct {
		Bound struct {
			Platform string `bson:"platform"`
			UserID   string `bson:"user_id"`
		} `bson:"bind"`
		Count int32 `bson:"count"`
	}
	type UserItem struct {
		Label string `bson:"label"`
		Value string `bson:"value"`
		Count int32  `bson:"count"`
	}

	var userResults []UserResult
	if cursor, err := coll.Aggregate(fanyongCtx, pipeline); err == nil {
		defer cursor.Close(fanyongCtx)
		cursor.All(fanyongCtx, &userResults)
	}

	var userList []UserItem
	for _, r := range userResults {
		var value string
		var label string
		if r.Bound.Platform != "" {
			value = fmt.Sprintf("%s#%s", r.Bound.Platform, r.Bound.UserID)
			label = value
		} else {
			label = "--未绑--"
			value = "#未绑"
		}
		userList = append(userList, UserItem{Label: label, Value: value, Count: r.Count})
	}
	userList = append(userList, UserItem{Label: "--全部--", Value: "#", Count: int32(orderNum)})
	userList = append(userList, UserItem{Label: "--已绑--", Value: "#已绑", Count: int32(userOrderNum)})

	// 按 count 排序
	for i := 0; i < len(userList); i++ {
		for j := i + 1; j < len(userList); j++ {
			if userList[j].Count > userList[i].Count {
				userList[i], userList[j] = userList[j], userList[i]
			}
		}
	}
	result["results"] = userList

	return result
}
