package core

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
)

func init() {
	// 获取可选的 platforms、bots、users、groups
	// 支持参数：platform（平台筛选）、bot_id（机器人筛选）
	GinApi(GET, "/api/chat/selects", RequireAuth, func(ctx *gin.Context) {
		platformFilter := ctx.Query("platform")
		botIDFilter := ctx.Query("bot_id")

		platforms := map[string][]string{}
		for _, plt := range getPltsArray() {
			platforms[plt] = GetAdapterBotsID(plt)
		}

		var userNames []NicklabeL
		var groupNames = []NicklabeL{{
			Label: "私聊",
			Value: "",
		}}

		nickname.Foreach(func(b1, b2 []byte) error {
			v := &Nickname{}
			err := json.Unmarshal(b2, v)
			if err == nil {
				code := string(b1)

				// 平台筛选
				if platformFilter != "" && v.Platform != platformFilter {
					return nil
				}

				// BotID 筛选
				if botIDFilter != "" {
					found := false
					for _, bid := range v.BotsID {
						if bid == botIDFilter {
							found = true
							break
						}
					}
					if !found {
						return nil
					}
				}

				// 跳过空昵称
				if v.Value == "" {
					return nil
				}

				if v.Group {
					groupNames = append(groupNames, NicklabeL{
						Label:    v.Value + "(" + code + ")",
						Value:    code,
						Platform: v.Platform,
					})
				} else {
					userNames = append(userNames, NicklabeL{
						Label:       v.Value + "(" + code + ")",
						Value:       code,
						Platform:    v.Platform,
						Source:      v.Source,
						SourceChats: v.SourceChats,
					})
				}
			}
			return nil
		})

		ctx.JSON(200, map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"platforms":   platforms,
				"user_names":  userNames,
				"group_names": groupNames,
			},
		})
	})

	// 发送消息（模拟触发）
	GinApi(POST, "/api/send", RequireAuth, func(ctx *gin.Context) {
		var body struct {
			Platform string `json:"platform"`
			BotID    string `json:"bot_id"`
			UserID   string `json:"user_id"`
			ChatID   string `json:"chat_id"`
			Content  string `json:"content"`
		}
		if err := ctx.BindJSON(&body); err != nil {
			ctx.JSON(200, map[string]interface{}{
				"success":      false,
				"errorMessage": "参数格式错误: " + err.Error(),
			})
			return
		}

		// 校验必填参数
		if body.Platform == "" {
			ctx.JSON(200, map[string]interface{}{
				"success":      false,
				"errorMessage": "platform 不能为空",
			})
			return
		}
		if body.Content == "" {
			ctx.JSON(200, map[string]interface{}{
				"success":      false,
				"errorMessage": "content 不能为空",
			})
			return
		}
		if body.UserID == "" && body.ChatID == "" {
			ctx.JSON(200, map[string]interface{}{
				"success":      false,
				"errorMessage": "user_id 和 chat_id 不能同时为空",
			})
			return
		}

		// 校验 platform + bot_id 是否存在在线适配器
		adapter, err := GetAdapter(body.Platform, body.BotID)
		if adapter == nil {
			ctx.JSON(200, map[string]interface{}{
				"success":      false,
				"errorMessage": "未找到平台 [" + body.Platform + "] 的在线适配器" + body.BotID,
			})
			return
		}
		if err != nil {
			// 找到了平台但没有精确匹配的 bot，adapter 已随机选择一个
		}

		// 注入消息到 Messages channel，触发插件系统处理
		sender := adapter.Receive(map[string]interface{}{
			USER_ID:  body.UserID,
			CONETNT:  body.Content,
			CHAT_ID:  body.ChatID,
		})

		ctx.JSON(200, map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"platform":  body.Platform,
				"bot_id":    adapter.GetBotId(),
				"user_id":   body.UserID,
				"chat_id":   body.ChatID,
				"message_id": sender.GetMessageID(),
			},
		})
	})
}
