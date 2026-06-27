package core

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// requireLogAuth 日志API认证中间件
func requireLogAuth(c *gin.Context) {
	// 如果密码为空，允许访问（未设置密码时无需认证）
	if password == "" {
		return
	}

	// 支持两种认证方式：Cookie token 和 X-Token header
	token, _ := c.Cookie("token")
	if token == "" {
		token = c.GetHeader("X-Token")
	}

	_, err := CheckAuth(token)
	if err != nil {
		c.JSON(401, map[string]interface{}{
			"success": false,
			"error":   "请先登录",
		})
		panic(err)
	}
}

// initPluginLogAPI 初始化插件日志API
func initPluginLogAPI() {
	// 获取插件日志（需要登录）
	GinApi(GET, "/api/plugin/logs", requireLogAuth, func(ctx *gin.Context) {
		uuid := ctx.Query("uuid")
		level := ctx.Query("level")      // 可选：info/debug/warn/error/log
		since := ctx.Query("since")      // 可选：时间戳
		limit := ctx.Query("limit")      // 可选：数量限制，默认100

		if uuid == "" {
			ctx.JSON(400, map[string]interface{}{
				"success": false,
				"error":   "uuid is required",
			})
			return
		}

		var sinceInt int64
		if since != "" {
			sinceInt, _ = strconv.ParseInt(since, 10, 64)
		}

		limitInt := 100
		if limit != "" {
			if v, err := strconv.Atoi(limit); err == nil {
				limitInt = v
			}
		}

		plogs := pluginLogs.GetLogs(uuid, level, sinceInt, limitInt)

		ctx.JSON(200, map[string]interface{}{
			"success": true,
			"data":    plogs,
			"total":   len(plogs),
		})
	})

	// 获取所有插件日志（按插件分组，需要登录）
	GinApi(GET, "/api/plugin/logs/all", requireLogAuth, func(ctx *gin.Context) {
		level := ctx.Query("level")
		since := ctx.Query("since")
		limit := ctx.Query("limit")

		var sinceInt int64
		if since != "" {
			sinceInt, _ = strconv.ParseInt(since, 10, 64)
		}

		limitInt := 50
		if limit != "" {
			if v, err := strconv.Atoi(limit); err == nil {
				limitInt = v
			}
		}

		result := map[string]interface{}{}
		for _, f := range Functions {
			if f.UUID == "" {
				continue
			}
			plogs := pluginLogs.GetLogs(f.UUID, level, sinceInt, limitInt)
			if len(plogs) > 0 {
				result[f.UUID] = map[string]interface{}{
					"title":  f.Title,
					"logs":   plogs,
					"total":  len(plogs),
				}
			}
		}

		ctx.JSON(200, map[string]interface{}{
			"success": true,
			"data":    result,
		})
	})

	// 获取日志统计（需要登录）
	GinApi(GET, "/api/plugin/logs/stats", requireLogAuth, func(ctx *gin.Context) {
		uuid := ctx.Query("uuid")

		if uuid == "" {
			// 获取所有插件的统计
			stats := map[string]*PluginLogStats{}
			for _, f := range Functions {
				if f.UUID == "" {
					continue
				}
				stats[f.UUID] = pluginLogs.GetStats(f.UUID)
			}
			ctx.JSON(200, map[string]interface{}{
				"success": true,
				"data":    stats,
			})
			return
		}

		stats := pluginLogs.GetStats(uuid)
		ctx.JSON(200, map[string]interface{}{
			"success": true,
			"data":    stats,
		})
	})

	// 清理过期日志（管理员接口，需要登录）
	GinApi(POST, "/api/plugin/logs/clean", requireLogAuth, func(ctx *gin.Context) {
		pluginLogs.CleanExpired()
		ctx.JSON(200, map[string]interface{}{
			"success": true,
			"message": "清理完成",
		})
	})

	// 设置日志过期时间（管理员接口，需要登录）
	GinApi(POST, "/api/plugin/logs/expire", requireLogAuth, func(ctx *gin.Context) {
		secondsStr := ctx.Query("seconds")
		if secondsStr == "" {
			ctx.JSON(400, map[string]interface{}{
				"success": false,
				"error":   "seconds is required",
			})
			return
		}

		seconds, err := strconv.Atoi(secondsStr)
		if err != nil || seconds <= 0 {
			ctx.JSON(400, map[string]interface{}{
				"success": false,
				"error":   "invalid seconds value",
			})
			return
		}

		pluginLogs.SetExpireSec(seconds)
		ctx.JSON(200, map[string]interface{}{
			"success": true,
			"message": "过期时间已设置",
			"seconds": seconds,
		})
	})
}
