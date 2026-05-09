package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/cdle/sillyplus/core/storage"
	"github.com/cdle/sillyplus/utils"
)

var version = compiled_at

// getLatestReleaseTag 从 GitHub API 获取最新 release tag
func getLatestReleaseTag() (string, error) {
	client := &http.Client{
		Timeout: 15 * time.Second,
	}
	resp, err := client.Get("https://api.github.com/repos/wjs876046992/sillyGirl/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if result.TagName == "" {
		return "", errors.New("无法获取最新 release tag")
	}
	return result.TagName, nil
}

// stripDevSuffix 去掉 -dev.时间戳 后缀，返回版本本体
func stripDevSuffix(v string) string {
	if idx := strings.Index(v, "-dev."); idx > 0 {
		return v[:idx]
	}
	return v
}

// versionGreater 比较两个版本号，a > b 返回 true
// 支持格式：v2.1.3 / v2.1-dev.xxxx / v2.1.3-dev.xxxx
func versionGreater(a, b string) bool {
	// 简单字符串比较即可
	// v2.1.3-dev.xxx > v2.1.3（前缀相同，短的更小，不降级）
	// v2.1.3-dev.xxx < v2.1.4（跨补丁号，能升级）
	// v2.2.0 > v2.1.999-dev.xxx（跨大版本，能升级）
	return a > b
}

// getLatestReleaseAssetURL 构造 release asset 下载地址
func getLatestReleaseAssetURL(tag string) string {
	assetName := fmt.Sprintf("sillyGirl_%s_%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetName += ".exe"
	}
	return fmt.Sprintf("https://github.com/wjs876046992/sillyGirl/releases/download/%s/%s", tag, assetName)
}

func Init() {
	initLoc()
	sillyGirl = MakeBucket("sillyGirl")
	// utils.ReadYaml(utils.ExecPath+"/conf/", &Config, "https://raw.githubusercontent.com/cdle/sillyplus/main/conf/demo_config.yaml")
	initToHandleMessage()
	sillyGirl.Set("compiled_at", compiled_at)
	console.Log("编译版本: %s", compiled_at)
	initWeb()
	initCarry()
	sillyGirl.Set("started_at", time.Now().Format("2006-01-02 15:04:05"))
	var updates = 0
	storage.Watch(sillyGirl, "compiled_at", func(old, new, key string) *storage.Final {
		if old != new {
			if updates == 1 {
				return &storage.Final{
					Message: "升级中，请耐心等待...",
				}
			}
			updates++
			defer func() {
				updates--
			}()
			var latest_version = ""

			// 从 GitHub API 获取最新 release tag
			console.Debug("正在从 GitHub API 获取最新 release 版本号...")
			latest_version, err := getLatestReleaseTag()
			if err != nil {
				console.Error("获取最新 release tag 错误：%s", err)
				return &storage.Final{
					Error: fmt.Errorf("获取最新版本失败：%s", err),
				}
			}
			console.Debug("最新 release 版本: %s, 当前版本: %s", latest_version, compiled_at)

			// 版本比较：latest_version > compiled_at
			// 支持 dev 版本（v2.1-dev.时间戳）正确比较，不会降级到旧 release
			if !versionGreater(latest_version, compiled_at) {
				console.Debug("当前版本 %s 已是最新，无需升级", compiled_at)
				return &storage.Final{
					Message: fmt.Sprintf("当前版本 %s 已是最新，无需升级", compiled_at),
				}
			}

			// 从 GitHub release 下载对应平台的二进制
			qurl := getLatestReleaseAssetURL(latest_version)
			console.Debug("正在从 release 获取最新版本 %s 编译文件: %s", latest_version, qurl)

			client := &http.Client{
				Timeout: 60 * time.Second,
			}
			resp, err := client.Get(qurl)
			if err != nil || resp.StatusCode != 200 {
				console.Error("获取最新编译文件错误：%v (status: %d)", err, func() int {
					if resp != nil {
						return resp.StatusCode
					}
					return 0
				}())
				return &storage.Final{
					Error: fmt.Errorf("升级时貌似网络不太行啊"),
				}
			}
			defer resp.Body.Close()

			console.Debug("正在创建编译文件...")
			filename := utils.ExecPath + "/" + utils.ProcessName
			ready := ""
			if runtime.GOOS == "windows" {
				ready = strings.Replace(filename, ".exe", ".ready.exe", -1)
			} else {
				ready += ".ready"
			}
			f, err := os.OpenFile(ready, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				console.Error("创建编译文件错误：%v", err)
				return &storage.Final{
					Error: fmt.Errorf("创建编译文件错误：%v", err),
				}
			}
			defer f.Close()
			i, err := io.Copy(f, resp.Body)
			if i < 2646140 || err != nil {
				console.Error("创建编译文件错误：%v %v", i, err)
				return &storage.Final{
					Error: fmt.Errorf("创建编译文件错误：%v %v", i, err),
				}
			}
			if runtime.GOOS == "windows" {
				console.Log("正在准备重启...")
				go func() {
					time.Sleep(time.Second)
					utils.Daemon("ready")
				}()
				return &storage.Final{
					Message: "1秒重启升级！",
				}
			} else {
				console.Debug("正在删除旧程序错误...")
				if err = os.RemoveAll(filename); err != nil {
					plugins.Set2(key, "")
					console.Error("删除旧程序错误：%v", err)
					return &storage.Final{
						Error: fmt.Errorf("删除旧程序错误：%v", err),
					}
				}
			}
			console.Debug("正在移动新程序错误...")
			if err = os.Rename(ready, filename); err != nil {
				console.Error("移动新程序错误：%v", err)
				return &storage.Final{
					Error: fmt.Errorf("移动新程序错误：%v", err),
				}
			}
			go func() {
				console.Debug("正在重启...")
				time.Sleep(time.Second)
				utils.Daemon()
			}()
			return &storage.Final{
				Message: "升级成功，即将重启！",
			}
		}
		return nil
	})
	storage.Watch(sillyGirl, "started_at", func(old, new, key string) *storage.Final {
		if old != new {
			go func() {
				time.Sleep(time.Second)
				utils.Daemon()
			}()
			return &storage.Final{
				Message: "1秒重启！",
			}
		}
		return nil
	})

	api_key := sillyGirl.GetString("api_key")
	if api_key == "" {
		api_key := time.Now().UnixNano()
		sillyGirl.Set("api_key", api_key)
	}
	// if sillyplus.GetString("uuid") == "" {
	sillyGirl.Set("uuid", utils.GenUUID())
	// }
	initPlugins()
	initReboot()
	initListenReply()
	// initPluginFile()
	initWebPluginList()
	go initPluginList()
	initPluginPublish()

}
