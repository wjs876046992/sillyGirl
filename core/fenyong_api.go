package core

import (
	"context"
	"encoding/json"
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
	Name        string                 `json:"name"`
	SkuName     string                 `json:"sku_name"`
	Image       string                 `json:"image"`
	Status      string                 `json:"status"`
	Content     []FenyongOrderContent  `json:"content"`
	Bind        *FenyongBind           `json:"bind"`
	Site        string                 `json:"site"`
	Platform    string                 `json:"platform"`
	CreatedTime int64                  `json:"created_time"`
	UpdatedTime int64                  `json:"updated_time"`
	OrderID     string                 `json:"order_id"`
	SkuID       string                 `json:"sku_id"`
}

type FenyongOrderContent struct {
	Label  string      `json:"label"`
	Value  interface{} `json:"value"`
	Status string      `json:"status,omitempty"`
}

type FenyongBind struct {
	Platform string `json:"platform"`
	UserID   string `json:"user_id"`
	UserName string `json:"user_name,omitempty"`
}

type FenyongOrderResult struct {
	Success bool           `json:"success"`
	Data    []FenyongOrder `json:"data"`
	Page    int            `json:"page"`
	Total   int            `json:"total"`
}

// FenyongStats 统一的佣金统计结构
type FenyongStats struct {
	Orders   int64   `json:"orders"`
	Estimate float64 `json:"estimate"`
	Actual   float64 `json:"actual"`
}

// FenyongCrossItem site × 时段 交叉统计
type FenyongCrossItem struct {
	Site     string  `json:"site"`
	Period   string  `json:"period"`
	Orders   int64   `json:"orders"`
	Estimate float64 `json:"estimate"`
	Actual   float64 `json:"actual"`
}

// FenyongDashboard 仪表盘数据（双维度 + 交叉）
type FenyongDashboard struct {
	Success bool                    `json:"success"`
	ByTime  map[string]FenyongStats `json:"by_time"`
	BySite  map[string]FenyongStats `json:"by_site"`
	Cross   []FenyongCrossItem      `json:"cross"`
}

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

// getGoodsImage 从 MongoDB 订单记录中提取商品图片 URL
// 拼多多: doc.data.goods_thumbnail_url
// 淘宝: doc.data.item_img
// 京东: doc.data.skuImg（由京东订单更新插件从转链记录补充）
func getGoodsImage(site string, doc bson.M) string {
	raw := doc["data"]
	if raw == nil {
		return ""
	}
	// 通用方案：Marshal → Unmarshal 任何 map 类型
	b, err := json.Marshal(raw)
	if err != nil {
		return ""
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return ""
	}
	var key string
	switch site {
	case "pdd":
		key = "goods_thumbnail_url"
	case "tb":
		key = "item_img"
	case "jd":
		key = "skuImg"
	default:
		return ""
	}
	if s, ok := m[key].(string); ok && s != "" {
		return s
	}
	return ""
}

// ========== API 路由 ==========

func initFenyongAPI() {
	GinApi(GET, "/api/fenyong/dashboard", RequireAuth, handleFenyongDashboard)
	GinApi(GET, "/api/fenyong/orders", RequireAuth, handleFenyongOrders)
	GinApi(GET, "/api/fenyong/tongji", RequireAuth, handleFenyongTongji)
	GinApi(POST, "/api/fenyong/refresh", RequireAuth, handleFenyongRefresh)
}

// ---------- 辅助类型（内联函数内） ----------

// facetGroupResult $group 后的公共子文档
type fgPlatform struct {
	Site     string  `bson:"_id"`
	Orders   int64   `bson:"orders"`
	Estimate float64 `bson:"estimate"`
	Actual   float64 `bson:"actual"`
}
type fgPeriod struct {
	Orders   int64   `bson:"orders"`
	Estimate float64 `bson:"estimate"`
	Actual   float64 `bson:"actual"`
}
type fgAmount struct {
	Estimate float64 `bson:"estimate"`
	Actual   float64 `bson:"actual"`
}
type fgCross struct {
	Site     string  `bson:"site"`
	Period   string  `bson:"period"`
	Orders   int64   `bson:"orders"`
	Estimate float64 `bson:"estimate"`
	Actual   float64 `bson:"actual"`
}
type facetDashboardResult struct {
	Today     []fgPeriod   `bson:"today"`
	Last7Days []fgPeriod   `bson:"last7days"`
	LastMonth []fgPeriod   `bson:"lastMonth"`
	AllTime   []fgPeriod   `bson:"all_time"`
	BySite    []fgPlatform `bson:"by_site"`
	Cross     []fgCross    `bson:"cross"`
}

// ---------- 首页仪表盘 ----------

func handleFenyongDashboard(c *gin.Context) {
	coll, err := getFanyongCollection()
	if err != nil {
		c.JSON(200, FenyongDashboard{Success: true})
		return
	}

	now := time.Now()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).Unix()
	last7Start := todayStart - 7*86400
	last30Start := todayStart - 30*86400

	// 通用 $group 阶段：汇总所有匹配文档
	allGroup := bson.D{{Key: "$group", Value: bson.M{
		"_id":      nil,
		"orders":   bson.M{"$sum": 1},
		"estimate": bson.M{"$sum": "$estimate"},
		"actual":   bson.M{"$sum": "$actual"},
	}}}

	// 按 site 分组
	siteGroup := bson.D{{Key: "$group", Value: bson.M{
		"_id":      "$site",
		"orders":   bson.M{"$sum": 1},
		"estimate": bson.M{"$sum": "$estimate"},
		"actual":   bson.M{"$sum": "$actual"},
	}}}

	// 时间范围 match 辅助函数
	timeMatch := func(start, end int64) bson.D {
		cond := bson.M{"$gte": start}
		if end > 0 {
			cond["$lt"] = end
		}
		return bson.D{{Key: "$match", Value: bson.M{"created_time": cond}}}
	}

	// 单次 $facet 查询，同时完成两个维度的聚合
	facetFacets := bson.M{
		// ===== 时间维度 =====
		"today": bson.A{
			timeMatch(todayStart, 0),
			allGroup,
		},
		"last7days": bson.A{
			timeMatch(last7Start, todayStart),
			allGroup,
		},
		"lastMonth": bson.A{
			timeMatch(last30Start, todayStart),
			allGroup,
		},
		"all_time": bson.A{
			allGroup,
		},
		// ===== 平台维度 =====
		"by_site": bson.A{
			siteGroup,
		},
		// ===== 交叉维度：site × 时段 =====
		"cross": bson.A{
			// 按 created_time 归类到 today / last7days / lastMonth
			bson.D{{Key: "$addFields", Value: bson.M{
				"period": bson.M{
					"$switch": bson.M{
						"branches": bson.A{
							bson.M{"case": bson.M{"$gte": []interface{}{"$created_time", todayStart}}, "then": "today"},
							bson.M{"case": bson.M{"$gte": []interface{}{"$created_time", last7Start}}, "then": "last7days"},
							bson.M{"case": bson.M{"$gte": []interface{}{"$created_time", last30Start}}, "then": "lastMonth"},
						},
						"default": "_skip_",
					},
				},
			}}},
			// 过滤掉不属于任何时段的文档
			bson.D{{Key: "$match", Value: bson.M{"period": bson.M{"$ne": "_skip_"}}}},
			// 按 site + period 分组
			bson.D{{Key: "$group", Value: bson.M{
				"_id":     bson.M{"site": "$site", "period": "$period"},
				"orders":  bson.M{"$sum": 1},
				"estimate": bson.M{"$sum": "$estimate"},
				"actual":  bson.M{"$sum": "$actual"},
			}}},
			// 扁平化输出
			bson.D{{Key: "$project", Value: bson.M{
				"_id":      0,
				"site":     "$_id.site",
				"period":   "$_id.period",
				"orders":   1,
				"estimate": 1,
				"actual":   1,
			}}},
		},
	}

	facetPipeline := mongo.Pipeline{
		bson.D{{Key: "$facet", Value: facetFacets}},
	}

	cursor, err := coll.Aggregate(fanyongCtx, facetPipeline)
	if err != nil {
		c.JSON(200, FenyongDashboard{Success: true})
		return
	}
	defer cursor.Close(fanyongCtx)

	var results []facetDashboardResult
	if err := cursor.All(fanyongCtx, &results); err != nil || len(results) == 0 {
		c.JSON(200, FenyongDashboard{Success: true})
		return
	}

	r := results[0]

	// 提取单条 $group 聚合结果
	first := func(arr []fgPeriod) FenyongStats {
		if len(arr) > 0 {
			return FenyongStats{Orders: arr[0].Orders, Estimate: arr[0].Estimate, Actual: arr[0].Actual}
		}
		return FenyongStats{}
	}

	// --- 时间维度 ---
	byTime := map[string]FenyongStats{
		"today":     first(r.Today),
		"last7days": first(r.Last7Days),
		"lastMonth": first(r.LastMonth),
		"total":     first(r.AllTime),
	}

	// --- 平台维度 ---
	bySite := make(map[string]FenyongStats)
	for _, s := range r.BySite {
		bySite[s.Site] = FenyongStats{Orders: s.Orders, Estimate: s.Estimate, Actual: s.Actual}
	}
	// 保证已知平台始终存在（即使无数据）
	for _, s := range []string{"jd", "tb", "pdd"} {
		if _, ok := bySite[s]; !ok {
			bySite[s] = FenyongStats{}
		}
	}
	// 平台维度的总计 = 全量总和
	bySite["total"] = byTime["total"]

	// --- 交叉维度 ---
	cross := make([]FenyongCrossItem, 0, len(r.Cross))
	for _, c := range r.Cross {
		cross = append(cross, FenyongCrossItem{
			Site:     c.Site,
			Period:   c.Period,
			Orders:   c.Orders,
			Estimate: c.Estimate,
			Actual:   c.Actual,
		})
	}

	c.JSON(200, FenyongDashboard{
		Success: true,
		ByTime:  byTime,
		BySite:  bySite,
		Cross:   cross,
	})
}

// ---------- 订单列表 ----------

func handleFenyongOrders(c *gin.Context) {
	coll, err := getFanyongCollection()
	if err != nil {
		c.JSON(200, FenyongOrderResult{Success: true, Data: []FenyongOrder{}, Page: 1, Total: 0})
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

	filter := buildFenyongFilter(site, tab, user, startTime, endTime, keyword)
	total, _ := coll.CountDocuments(fanyongCtx, filter)

	skip := int64(pageSize * (current - 1))
	cursor, e := coll.Find(fanyongCtx, filter,
		options.Find().SetSort(bson.M{"created_time": -1}),
		options.Find().SetSkip(skip),
		options.Find().SetLimit(int64(pageSize)),
	)
	if e != nil {
		c.JSON(200, FenyongOrderResult{Success: true, Data: []FenyongOrder{}, Page: current, Total: 0})
		return
	}
	defer cursor.Close(fanyongCtx)

	var docs []bson.M
	if e := cursor.All(fanyongCtx, &docs); e != nil {
		c.JSON(200, FenyongOrderResult{Success: true, Data: []FenyongOrder{}, Page: current, Total: 0})
		return
	}

	c.JSON(200, FenyongOrderResult{
		Success: true,
		Data:    convertOrders(docs),
		Page:    current,
		Total:   int(total),
	})
}

// ---------- 统计（兼容旧版 API） ----------

func handleFenyongTongji(c *gin.Context) {
	coll, err := getFanyongCollection()
	if err != nil {
		c.JSON(200, gin.H{"success": true, "data": nil})
		return
	}
	stat := getFenyongStats(c.Query("startTime"), c.Query("endTime"), c.Query("site"), c.Query("user"), coll)
	c.JSON(200, gin.H{"data": stat, "success": true})
}

// ---------- 手动刷新 ----------

func handleFenyongRefresh(c *gin.Context) {
	c.JSON(200, FenyongRefreshResult{
		Success: true,
		Message: "刷新命令已发送（请确保插件已配置各平台 API Key）",
	})
}

// ========== 辅助函数 ==========

// buildFenyongFilter 构建订单过滤条件
func buildFenyongFilter(site, tab, user, startTime, endTime, keyword string) bson.M {
	filter := bson.M{}

	switch tab {
	case "tab1":
		filter["estimate"] = bson.M{"$ne": 0}
	case "tab3":
		filter["estimate"] = bson.M{"$eq": 0}
	case "tab4":
		filter["actual"] = bson.M{"$ne": 0}
		filter["settled"] = bson.M{"$exists": false}
		filter["created_time"] = bson.M{"$lt": time.Now().Unix() - getSettleDays()*86400}
	case "tab5":
		filter["actual"] = bson.M{"$ne": 0}
		filter["settled"] = bson.M{"$exists": true}
	case "tab6":
		filter["actual"] = bson.M{"$eq": 0}
		filter["settled"] = bson.M{"$exists": true}
	}

	if site != "" {
		filter["site"] = site
	}

	if user != "" {
		parts := strings.SplitN(user, "#", 2)
		if len(parts) == 2 {
			_, userID := parts[0], parts[1]
			switch userID {
			case "已绑":
				filter["bind"] = bson.M{"$exists": true}
			case "未绑":
				filter["bind"] = bson.M{"$exists": false}
			default:
				filter["bind.platform"] = parts[0]
				filter["bind.user_id"] = userID
			}
		}
	}

	if startTime != "" || endTime != "" {
		tf := bson.M{}
		if t, e := strconv.ParseInt(startTime, 10, 64); e == nil && t > 0 {
			tf["$gte"] = t
		}
		if t, e := strconv.ParseInt(endTime, 10, 64); e == nil && t > 0 {
			tf["$lt"] = t
		}
		filter["created_time"] = tf
	}

	if keyword != "" {
		filter["$or"] = []bson.M{
			{"sku_name": bson.M{"$regex": keyword, "$options": "i"}},
			{"order_id": bson.M{"$regex": keyword, "$options": "i"}},
			{"sku_id": bson.M{"$regex": keyword, "$options": "i"}},
		}
	}

	return filter
}

// convertOrders MongoDB 文档 → FenyongOrder
func convertOrders(docs []bson.M) []FenyongOrder {
	orders := make([]FenyongOrder, 0, len(docs))
	for _, item := range docs {
		var o FenyongOrder

		if v, ok := item["name"].(string); ok {
			o.Name = v
		}
		if v, ok := item["sku_name"].(string); ok {
			o.SkuName = v
		}
		if v, ok := item["order_id"].(string); ok {
			o.OrderID = v
		}
		if v, ok := item["sku_id"].(string); ok {
			o.SkuID = v
		}
		// created_time 可能是 int64 / primitive.DateTime / float64 / int32
		switch ct := item["created_time"].(type) {
		case int64:
			o.CreatedTime = ct
		case int32:
			o.CreatedTime = int64(ct)
		case float64:
			o.CreatedTime = int64(ct)
		case primitive.DateTime:
			o.CreatedTime = ct.Time().Unix()
		}

		// updated_time 可能是 int64 / primitive.DateTime / float64 / int32
		switch ut := item["updated_time"].(type) {
		case int64:
			o.UpdatedTime = ut
		case int32:
			o.UpdatedTime = int64(ut)
		case float64:
			o.UpdatedTime = int64(ut)
		case primitive.DateTime:
			o.UpdatedTime = ut.Time().Unix()
		}

		if site, ok := item["site"].(string); ok {
			o.Site = site
			skuID, _ := item["sku_id"].(string)
			o.SkuID = skuID
			orderID, _ := item["order_id"].(string)
			o.OrderID = orderID
			status, _ := item["status"].(string)
			o.Status = status
			// Image: 从 data 子文档提取
			if o.Image == "" {
				if d, _ := item["data"].(bson.M); d != nil {
					switch site {
					case "jd":
						if img, _ := d["skuImg"].(string); img != "" {
							o.Image = img
						}
					case "pdd":
						if img, _ := d["goods_thumbnail_url"].(string); img != "" {
							o.Image = img
						}
					case "tb":
						if img, _ := d["item_img"].(string); img != "" {
							if strings.HasPrefix(img, "//") {
								img = "https:" + img
							}
							o.Image = img
						}
					}
				}
				if o.Image == "" {
					o.Image = getSiteImage(site)
				}
			}
		}

		// 绑定信息
		if bind, ok := item["bind"].(bson.M); ok {
			if platform, ok := bind["platform"].(string); ok {
				if userID, ok := bind["user_id"].(string); ok {
					b := &FenyongBind{Platform: platform, UserID: userID}
					if userName, ok := bind["user_name"].(string); ok {
						b.UserName = userName
					}
					o.Bind = b
				}
			}
		}

		o.Content = []FenyongOrderContent{
			{Label: "订单时间", Value: formatTime(item["created_time"])},
			{Label: "订单金额", Value: item["cost"]},
			{Label: "预估佣金", Value: item["estimate"]},
			{Label: "实际佣金", Value: item["actual"]},
		}

		orders = append(orders, o)
	}
	return orders
}

// getFenyongStats 兼容旧版 API 的统计数据
func getFenyongStats(startTime, endTime, site, user string, coll *mongo.Collection) bson.M {
	match := bson.M{}
	if t, e := strconv.ParseInt(startTime, 10, 64); e == nil && t > 0 {
		match["created_time"] = bson.M{"$gte": t}
	}
	if t, e := strconv.ParseInt(endTime, 10, 64); e == nil && t > 0 {
		if ct, ok := match["created_time"]; ok {
			ct.(bson.M)["$lt"] = t
		} else {
			match["created_time"] = bson.M{"$lt": t}
		}
	}
	if site != "" {
		match["site"] = site
	}
	if user != "" {
		parts := strings.SplitN(user, "#", 2)
		if len(parts) == 2 {
			_, userID := parts[0], parts[1]
			switch userID {
			case "已绑":
				match["bind"] = bson.M{"$exists": true}
			case "未绑":
				match["bind"] = bson.M{"$exists": false}
			default:
				match["bind.platform"] = parts[0]
				match["bind.user_id"] = userID
			}
		}
	}

	pipeline := mongo.Pipeline{}
	if len(match) > 0 {
		pipeline = append(pipeline, bson.D{{Key: "$match", Value: match}})
	}
	pipeline = append(pipeline, bson.D{{Key: "$group", Value: bson.M{
		"_id": bson.M{"platform": "$bind.platform", "user_id": "$bind.user_id"},
		"count":          bson.M{"$sum": 1},
		"cost":           bson.M{"$sum": "$cost"},
		"actual":         bson.M{"$sum": "$actual"},
		"estimate":       bson.M{"$sum": "$estimate"},
		"rake_actual":    bson.M{"$sum": bson.M{"$multiply": []interface{}{"$actual", "$rake"}}},
		"rake_estimate":  bson.M{"$sum": bson.M{"$multiply": []interface{}{"$estimate", "$rake"}}},
		"irake_actual":   bson.M{"$sum": bson.M{"$multiply": []interface{}{"$actual", bson.M{"$subtract": []interface{}{1, "$rake"}}}}},
		"irake_estimate": bson.M{"$sum": bson.M{"$multiply": []interface{}{"$estimate", bson.M{"$subtract": []interface{}{1, "$rake"}}}}},
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

	var totalActual, totalEstimate, totalRakeActual, totalRakeEstimate float64
	var totalIRakeActual, totalIRakeEstimate float64
	var orderNum, userNum, userOrderNum int64
	userNum = -1

	for _, r := range results {
		userNum++
		var cnt int64
		switch v := r["count"].(type) {
		case int32:
			cnt = int64(v)
		case int64:
			cnt = v
		case float64:
			cnt = int64(v)
		}
		orderNum += cnt

		if a, ok := r["actual"].(float64); ok {
			totalActual += a
		}
		if e, ok := r["estimate"].(float64); ok {
			totalEstimate += e
		}
		if bind, ok := r["bind"].(bson.M); ok {
			if uid, ok := bind["user_id"].(string); ok && uid != "" {
				userOrderNum += cnt
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
		"user_num":               userNum,
		"order_num":              orderNum,
		"total_actual":           totalActual,
		"total_estimate":         totalEstimate,
		"total_rake_actual":      totalRakeActual,
		"total_rake_estimate":    totalRakeEstimate,
		"total_irake_actual":     totalIRakeActual,
		"total_irake_estimate":   totalIRakeEstimate,
		"total_iactual":          totalIactual,
		"total_iestimate":        totalIestimate,
		"total_irake_actual_pct":   pctActual,
		"total_irake_estimate_pct": pctEstimate,
	}

	// 用户列表
	// 需要单独带 $match 查询
	userPipeline := mongo.Pipeline{}
	if len(match) > 0 {
		userPipeline = append(userPipeline, bson.D{{Key: "$match", Value: match}})
	}
	userPipeline = append(userPipeline, bson.D{{Key: "$group", Value: bson.M{
		"_id": bson.M{"platform": "$bind.platform", "user_id": "$bind.user_id"},
		"count": bson.M{"$sum": 1},
	}}})

	var userResults []struct {
		Bound struct {
			Platform string `bson:"platform"`
			UserID   string `bson:"user_id"`
		} `bson:"_id"`
		Count int32 `bson:"count"`
	}
	if uc, err := coll.Aggregate(fanyongCtx, userPipeline); err == nil {
		defer uc.Close(fanyongCtx)
		uc.All(fanyongCtx, &userResults)
	}

	type userItem struct {
		Label string `json:"label"`
		Value string `json:"value"`
		Count int32  `json:"count"`
	}
	var userList []userItem
	for _, r := range userResults {
		value := fmt.Sprintf("%s#%s", r.Bound.Platform, r.Bound.UserID)
		userList = append(userList, userItem{Label: value, Value: value, Count: r.Count})
	}
	if userOrderNum > 0 {
		userList = append(userList, userItem{Label: "--已绑--", Value: "#已绑", Count: int32(userOrderNum)})
	}
	userList = append(userList, userItem{Label: "--全部--", Value: "#", Count: int32(orderNum)})
	userList = append(userList, userItem{Label: "--未绑--", Value: "#未绑", Count: int32(orderNum - userOrderNum)})

	// 按 count 降序
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
