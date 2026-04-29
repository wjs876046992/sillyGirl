package core

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	_ "embed"

	"github.com/cdle/sillyplus/utils"
)

//go:embed proto3/dist/sillygirl.js
var sillygirlBundle string

type Language struct {
	Name    string   // node
	Version string   // 20230725
	Os      string   // linux
	Arch    string   // amd64
	Links   []string // 下载链接
}

var plugin_dir = filepath.Join(utils.ExecPath, "plugins")
var release = "20230732"

var languages = []Language{
	{
		Name:    "node",
		Version: release,
		Os:      "linux",
		Arch:    "amd64",
		Links:   []string{"https://gitee.com/sillybot/binary/releases/download/" + release + "/node_linux_amd64.zip"},
	},
	{
		Name:    "node",
		Version: release,
		Os:      "darwin",
		Arch:    "arm64",
		Links:   []string{"https://gitee.com/sillybot/binary/releases/download/" + release + "/node_darwin_arm64.zip"},
	},
	{
		Name:    "node",
		Version: release,
		Os:      "windows",
		Arch:    "amd64",
		Links:   []string{"https://gitee.com/sillybot/binary/releases/download/" + release + "/node_windows_amd64.zip"},
	},
}

// lookPath searches for an executable in the system PATH.
func lookPath(name string) string {
	p, err := exec.LookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// SystemNodePath holds the path to system-installed node if available.
var SystemNodePath string

// SystemNpmPath holds the path to system-installed npm if available.
var SystemNpmPath string

// SystemYarnPath holds the path to system-installed yarn if available.
var SystemYarnPath string

// GetNodeBin returns the best available node binary path.
func GetNodeBin() string {
	if SystemNodePath != "" {
		return SystemNodePath
	}
	return utils.ExecPath + "/language/node/node"
}

// GetPackageManager returns the best available package manager command.
// Priority: system npm (ships with node) > system yarn > bundled yarn.
func GetPackageManager() (cmd string, name string) {
	if SystemNpmPath != "" {
		return SystemNpmPath, "npm"
	}
	if SystemYarnPath != "" {
		return SystemYarnPath, "yarn"
	}
	return utils.ExecPath + "/language/node/yarn/bin/yarn", "yarn"
}

// ensureSillyGirlModule writes the webpack-bundled sillygirl.js to
// ExecPath/node_modules/sillygirl/ so Node.js plugins can find it via
// native upward directory search:
//   ExecPath/plugins/xxx/  (plugin runs here)
//   → ExecPath/node_modules/sillygirl/  (found by walking up)
//
// No NODE_PATH needed. No npm install needed.
// The webpack bundle already includes @grpc/grpc-js and google-protobuf.
// Only writes on first run; subsequent starts skip when file exists.
func ensureSillyGirlModule() {
	sgDir := utils.ExecPath + "/node_modules/sillygirl"

	// Already installed? Skip.
	if _, err := os.Stat(sgDir + "/sillygirl.js"); err == nil {
		return
	}

	console.Log("正在初始化 sillygirl 模块...")
	os.MkdirAll(sgDir, 0755)

	if err := os.WriteFile(sgDir+"/sillygirl.js", []byte(sillygirlBundle), 0755); err != nil {
		console.Error("写入 sillygirl.js 失败: %v", err)
		return
	}

	pkgJson := `{"name":"sillygirl","version":"1.0.0","main":"sillygirl.js"}`
	os.WriteFile(sgDir+"/package.json", []byte(pkgJson), 0644)

	console.Log("sillygirl 模块初始化完成")
}

func initLanguage() {
	// Detect system-installed Node.js first
	SystemNodePath = lookPath("node")
	SystemNpmPath = lookPath("npm")
	SystemYarnPath = lookPath("yarn")

	if SystemNodePath != "" {
		console.Log("检测到系统 Node: %s", SystemNodePath)
		if SystemNpmPath != "" {
			console.Log("检测到系统 npm: %s", SystemNpmPath)
		}
		if SystemYarnPath != "" {
			console.Log("检测到系统 yarn: %s", SystemYarnPath)
		}
		node_dir := utils.ExecPath + "/language/node"
		os.MkdirAll(node_dir, 0755)
		os.WriteFile(node_dir+"/version", []byte("system"), 0755)
		// Write bundled sillygirl module (one-time, skips if exists)
		ensureSillyGirlModule()
		return
	}

	// Fallback: no system node found, use the original remote download logic.
	console.Log("未检测到系统 Node，将下载内置执行环境...")
	for _, item := range languages {
		if !(item.Os == runtime.GOOS && item.Arch == runtime.GOARCH) {
			continue
		}
		node_dir := utils.ExecPath + "/language/" + item.Name
		os.MkdirAll(node_dir, 0755)
		path := os.Getenv("PATH")
		newPath := ""
		if path != "" {
			newPath = fmt.Sprintf("%s:%s", node_dir, path)
		} else {
			newPath = node_dir
		}
		os.Setenv("PATH", newPath)
		if len(item.Links) == 0 {
			continue
		}
		if _, err := os.Stat(node_dir + "/yarn"); err != nil {
			resp, err := http.Get("https://gitee.com/sillybot/binary/releases/download/yarn/yarn.zip")
			if err == nil {
				go func() {
					defer resp.Body.Close()
					zipfile := node_dir + "/yarn.zip"
					f, err := os.OpenFile(zipfile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
					if err != nil {
						fmt.Println(err, 2)
						return
					}
					defer f.Close()
					_, err = io.Copy(f, resp.Body)
					if err != nil {
						fmt.Println(err, 3)
						return
					}
					defer os.Remove(zipfile)
					err = unzip(zipfile, 0777, false)
					if err != nil {
						fmt.Println(err, 4)
						return
					}
				}()
			}
		}
		func() {
			dir := utils.ExecPath + "/language/" + item.Name
			data, _ := os.ReadFile(dir + "/version")
			if string(data) == item.Version {
				return
			}
			console.Log("正在安装", item.Name, "执行环境....")
			resp, err := http.Get(item.Links[0])
			if err != nil {
				return
			}
			defer resp.Body.Close()
			zipfile := dir + "/" + item.Name + ".zip"
			f, err := os.OpenFile(zipfile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				return
			}
			defer f.Close()
			_, err = io.Copy(f, resp.Body)
			if err != nil {
				return
			}
			defer os.Remove(zipfile)
			if err := unzip(zipfile, 0755, false); err == nil {
				os.WriteFile(dir+"/version", []byte(item.Version), 0755)
			}
			console.Log("安装", item.Name, "执行环境成功")
		}()
	}

	// For bundled node too, ensure sillygirl module is available
	ensureSillyGirlModule()
}

func unzip(filename string, perm fs.FileMode, pkg bool) error {
	zipFile, err := zip.OpenReader(filename)
	if err != nil {
		return err
	}
	defer zipFile.Close()
	top := ""
	for i := range zipFile.File {
		file := zipFile.File[i]
		if top == "" {
			top = strings.Split(file.Name, "/")[0]
		}
		if strings.HasPrefix(file.Name, "__MACOSX/") {
			continue
		}
		path := filepath.Join(filepath.Dir(filename), file.Name)
		if file.FileInfo().IsDir() {
			err = os.MkdirAll(path, perm)
			if err != nil {
				return err
			}
		} else {
			err = os.MkdirAll(filepath.Dir(path), perm)
			if err != nil {
				return err
			}
			var de = func() error {
				zipFile, err := file.Open()
				if err != nil {
					return err
				}
				defer zipFile.Close()

				localFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
				if err != nil {
					return err
				}
				defer localFile.Close()
				_, err = io.Copy(localFile, zipFile)
				if err != nil {
					return err
				}
				return nil
			}
			if file.Name != top+"/main.js" {
				de()
			} else {
				if pkg {
					defer func() {
						pkgCmd, pkgName := GetPackageManager()
						dir := utils.ExecPath + "/plugins/" + top
						cmd := exec.Command(pkgCmd, "install")
						cmd.Dir = dir
						cmd.Env = os.Environ()
						data, err := cmd.Output()
						if err != nil {
							console.Error("依赖添加失败(%s): %v %s\n\n你可以尝试自救：\n进入插件目录：cd %s\n安装依赖：%s install", pkgName, err, string(data), dir, pkgCmd)
						} else {
							console.Log(string(data))
						}
						de()
					}()
				} else {
					de()
				}
			}
		}
	}
	return nil
}
