package core

import (
	"bufio"
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/cdle/sillyplus/core/common"
	"github.com/cdle/sillyplus/core/logs"
	"github.com/cdle/sillyplus/core/storage"
	"github.com/cdle/sillyplus/utils"
	"github.com/dop251/goja"
	"github.com/fsnotify/fsnotify"
	"github.com/google/uuid"
	cron "github.com/robfig/cron/v3"
)

func init() {
	go initNodePlugins()
}

var processes sync.Map

// killOrphanNodePlugins 在加载插件前，杀掉所有残留的 node 插件进程（PPID=1 的孤儿进程）
func killOrphanNodePlugins() {
	scanPid := func(bin string) {
		// 通过 /proc 扫描同名子进程
		dir, err := os.Open("/proc")
		if err != nil {
			return
		}
		defer dir.Close()

		entries, _ := dir.Readdirnames(-1)
		for _, entry := range entries {
			pid, err := strconv.Atoi(entry)
			if err != nil || pid == os.Getpid() {
				continue
			}

			// 读取进程 cmdline
			cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
			if err != nil {
				continue
			}
			// cmdline 以 \x00 分隔
			args := strings.Split(strings.TrimRight(string(cmdline), "\x00"), "\x00")
			if len(args) < 2 {
				continue
			}
			// 匹配 node/py 脚本路径中含有 /plugins/ 的进程
			if args[0] == bin && strings.Contains(args[len(args)-1], "/plugins/") {
				// 只杀 PPID=1 的孤儿进程或不属于本进程的子进程
				stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
				if err != nil {
					continue
				}
				// stat 格式: pid (comm) state ppid ...
				fields := strings.Fields(string(stat))
				if len(fields) < 4 {
					continue
				}
				ppid, _ := strconv.Atoi(fields[3])
				if ppid == 1 || ppid != os.Getppid() {
					proc, err := os.FindProcess(pid)
					if err == nil && proc != nil {
						proc.Signal(syscall.SIGTERM)
						logs.Debug("已终止孤儿插件进程: %s (PID %d)", strings.Join(args, " "), pid)
						// 3秒后强杀
						go func(p int) {
							time.Sleep(3 * time.Second)
							if proc, err := os.FindProcess(p); err == nil && proc != nil {
								proc.Kill()
							}
						}(pid)
					}
				}
			}
		}
	}

	scanPid("node")
	scanPid("python3")
}

func initNodePlugins() {
	initLanguage()

	// 启动前先杀掉孤儿进程
	killOrphanNodePlugins()

	root := strings.ReplaceAll(utils.ExecPath+"/plugins", "\\", "/")
	plugins := []string{root}
	os.Mkdir(root, 0755)
	// fmt.Println("root", root)
	files, _ := ioutil.ReadDir(root)
	for _, file := range files {
		if !file.IsDir() {
			continue
		}
		name := file.Name()
		path := root + "/" + name
		plugins = append(plugins, path)
		index, class := FindMainIndex(path)

		if index != "" {
			AddNodePlugin(index, name, class)
		}
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Println("创建监视器失败：", err)
		return
	}
	defer watcher.Close()
	// 要监控的文件夹路径
	for _, dir := range plugins {
		err = watcher.Add(dir)
		if err != nil {
			fmt.Println("添加监视目录失败：", err)
			return
		}
	}

	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			// fmt.Println(event.Name, "op", event.Op.String())
			event.Name = strings.ReplaceAll(event.Name, "\\", "/")
			files := strings.Split(strings.Replace(event.Name, root+"/", "", 1), "/")
			var plugin_dir = false
			var plugin_index = false
			var plugin_name = ""
			var class = ""
			switch len(files) {
			case 1:
				plugin_dir = true
				// fmt.Println("目录事件")
				plugin_name = files[0]
			case 2:
				class, plugin_index = CheckMainIndex(files[1])
				// if files[1] == "main.js" {
				// 	if files[1] == "main.js" {
				// 		plugin_index = true
				// 	}
				// 	// fmt.Println("入口文件事件")
				// }
				plugin_name = files[0]
			}
			if plugin_name == "." {
				continue
			}
			switch event.Op.String() {
			case "CREATE":
				if plugin_dir {
					info, err := os.Stat(event.Name)
					// fmt.Println(err)
					if err == nil && info.IsDir() {
						index, class := FindMainIndex(event.Name)
						switch class {
						case NODE:
							AddNodePlugin(index, plugin_name, class)
						case PYTHON:
						case "":
							// time.Sleep(time.Millisecond * 100) //如果没有
							// if info, err := os.Stat(event.Name + "/demo.main.js"); err == nil && !info.IsDir() {

							// }
						}
						// event_name := event.Name + "/main.js"
						// if info, err := os.Stat(event_name); err == nil && !info.IsDir() {
						// 	AddNodePlugin(event_name, plugin_name)
						// } else {
						// 	time.Sleep(time.Millisecond * 100)
						// 	if info, err := os.Stat(event_name); err == nil && !info.IsDir() {
						// 		AddNodePlugin(event_name, plugin_name)
						// 	}
						// }
						watcher.Add(event.Name)
						// fmt.Println("增加插件目录", event.Name)
					} else {
						// fmt.Println("非插件目录", event.Name)
					}
					tf := event.Name + "/node_modules/sillygirl.d.ts"
					ti := event.Name + "/demo.main.js"
					if _, err := os.Stat(tf); err != nil {
						os.Mkdir(event.Name+"/node_modules", 0700)
						os.WriteFile(tf, []byte(typeat), 0700)
					}
					go func() {
						time.Sleep(time.Second)
						if _, err := os.Stat(ti); err != nil {
							os.Mkdir(event.Name+"/node_modules", 0700)
							os.WriteFile(ti, []byte(defaultScript(plugin_name)), 0700)
						}
					}()
				} else if plugin_index {
					// fmt.Println("增加插件", event.Name)
					// RemNodePlugin(plugin_name)
					AddNodePlugin(event.Name, plugin_name, class)
				}
			case "REMOVE", "RENAME", "REMOVE|RENAME", "REMOVE|WRITE":
				if plugin_dir {
					watcher.Remove(event.Name)
					// fmt.Println("移除插件目录", event.Name)
					// fmt.Println("移除插件", plugin_name)
					AddNodePlugin(event.Name, plugin_name, UNKNOWN)
				} else if plugin_index {
					// fmt.Println("移除插件", plugin_name)
					AddNodePlugin(event.Name, plugin_name, class)
				}
			case "WRITE": //, "CHMOD"
				if plugin_index {
					// 热加载：先清理已加载标记，再重新加载
					uuid := nameUuid(plugin_name)
					loadedPlugins.Delete(uuid)
					AddNodePlugin(event.Name, plugin_name, class)
					// fmt.Println("变更插件", event.Name, plugin_name)
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			fmt.Println("错误：", err)
		}
	}
}

func nameUuid(name string) string {
	hash := sha1.Sum([]byte(name))
	return strings.ReplaceAll(uuid.NewSHA1(uuid.Nil, hash[:]).String(), "-", "_")
}

func isNameUuid(uuid string) bool {
	return strings.Contains(uuid, "_")
}

var loadedPlugins sync.Map // 记录已加载完成的插件 uuid，防止重复加载

func AddNodePlugin(path, name, class string) error {

	if name == "" {
		return nil
	}
	uuid := nameUuid(name)

	// 如果插件已经加载过且正在运行，跳过重复加载
	if _, loaded := loadedPlugins.Load(uuid); loaded {
		logs.Debug("插件 [%s] 已加载，跳过重复加载", name)
		return nil
	}

	pluginLock.Lock()
	defer pluginLock.Unlock()

	// 如果插件已加载且正在运行，跳过重复加载
	for i := range Functions {
		if Functions[i].UUID == uuid && Functions[i].Running {
			return nil
		}
	}

	//移除
	var rf *common.Function
	for i := range Functions {
		if Functions[i].UUID == uuid {
			rf = Functions[i]
			DestroyAdapterByUUID(uuid)
			Functions[i].Running = false
			if len(Functions[i].CronIds) != 0 {
				for _, id := range Functions[i].CronIds {
					CRON.Remove(cron.EntryID(id))
				}
			}
			Functions = append(Functions[:i], Functions[i+1:]...)
			CancelPluginCrons(uuid)
			CancelPluginWebs(uuid)
			CancelPluginlistening(uuid)
			CancelHttpListen(uuid)
			StopNodeProxy(uuid)
			remStatic(uuid)
			// 终止旧的 node/python 进程
			processes.Range(func(key, value any) bool {
				p := key.(*exec.Cmd)
				s := value.(common.Sender)
				if s.GetPluginID() == uuid {
					func() {
						defer func() { recover() }()
						if p.Process != nil {
							p.Process.Signal(syscall.SIGTERM)
							processes.Delete(key)
							// 3秒后强制SIGKILL
							go func(pid int) {
								time.Sleep(3 * time.Second)
								func() {
									defer func() { recover() }()
									proc, _ := os.FindProcess(pid)
									if proc != nil {
										proc.Kill()
									}
								}()
							}(p.Process.Pid)
						}
					}()
				}
				return true
			})
			storage.DisableHandle(uuid)
			break
		}
	}
	file, err := os.Open(path)
	if err != nil {
		if rf != nil {
			console.Log("已卸载 %s%s", rf.Title, rf.Suffix)
		}
		return err
	}
	defer file.Close()
	data, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}
	script := string(data)
	if script == "" {
		return nil
	}
	// plugins_id.Store(uuid, path)
	// fmt.Println("add,", uuid, name)
	f, cbs := pluginParse(script, uuid)
	loadedPlugins.Store(uuid, true)
	f.Reload = func() { //重载
		loadedPlugins.Delete(uuid)
		// 清理 Running 状态，触发重新加载
		for i := range Functions {
			if Functions[i].UUID == uuid {
				Functions[i].Running = false
				break
			}
		}
		AddNodePlugin(path, name, class)
	}

	f.Type = class
	switch f.Type {
	case NODE:
		f.Suffix = ".js"
	case PYTHON:
		f.Suffix = ".py"
	}
	f.Path = path
	f.Handle = func(s common.Sender, _ func(vm *goja.Runtime)) interface{} {
		console := &Console{UUID: uuid}
		s.SetPluginID(uuid)
		plt := s.GetImType()
		bin := ""
		var cmd *exec.Cmd
		switch class {
		case NODE:
			bin = GetNodeBin()
			cmd = exec.Command(bin, path)
		case PYTHON:
			bin = "python3"
			cmd = exec.Command(bin, "-u", path)
			cmd.Env = append(os.Environ(), "PYTHONPATH="+utils.ExecPath+"/proto3")
		}

		// Linux: Pdeathsig 让内核在父进程退出时自动 SIGTERM 子进程
		cmd.SysProcAttr = &syscall.SysProcAttr{}
		setChildPdeathsig(cmd.SysProcAttr)
		cmd.Dir = filepath.Dir(path)
		RUNTIME_ID := utils.GenUUID()
		if len(cmd.Env) == 0 {
			cmd.Env = os.Environ()
		}
		cmd.Env = append(cmd.Env, "RUNTIME_ID="+RUNTIME_ID)
		cmd.Env = append(cmd.Env, "PLUGIN_ID="+uuid)
		// 获取标准输出和标准错误输出的管道
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			// fmt.Printf("获取标准输出管道失败：%v\n", err)
			// os.Exit(1)
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			// fmt.Printf("获取标准错误输出管道失败：%v\n", err)
			// os.Exit(1)
		}

		// file, err := os.Create("output.log")
		// if err != nil {
		// 	fmt.Printf("创建文件失败：%v\n", err)
		// 	os.Exit(1)
		// }
		// defer file.Close()
		var wg sync.WaitGroup
		wg.Add(2)
		// 处理标准输出
		go func() {
			defer wg.Done()

			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				data := scanner.Text()
				fmt.Println(data)

				// if _, err := file.WriteString(data + "\n"); err != nil {
				// 	fmt.Printf("写入文件失败：%v\n", err)
				// }
			}
		}()
		// 处理标准错误输出
		go func() {
			defer wg.Done()
			scanner := bufio.NewScanner(stderr)
			if f.OnStart {
				for scanner.Scan() {
					fmt.Println(scanner.Text())
				}
			} else {
				lines := []string{}
				for scanner.Scan() {
					data := scanner.Text()
					lines = append(lines, data)
				}
				if len(lines) != 0 {
					console.Error(strings.Join(lines, "\n"))
				}

			}
		}()
		processes.Store(cmd, s)
		register := createSenderRegister(RUNTIME_ID)
		if (plt) != "*" {
			cmd.Env = append(cmd.Env, "SENDER_ID="+register(s))
			err = cmd.Start()
			if err != nil {

			}
			defer deleteSenderRegister(RUNTIME_ID)
			defer processes.Delete(cmd)
			err = cmd.Wait()
			if err != nil {
				fmt.Println("命令执行失败：", err)
				return nil
			}
		} else {
			err = cmd.Start()
			if err != nil {

			}
			processes.Range(func(key, value any) bool {
				p := key.(*exec.Cmd)
				if p == cmd {
					return true
				}
				s := value.(common.Sender)
				if s.GetPluginID() == uuid {
					func() {
						defer func() {
							recover()
						}()
						if p.Process.Kill() == nil {
							processes.Delete(key)
						}
					}()
				}
				return true
			})
			go func() {
				defer deleteSenderRegister(RUNTIME_ID)
				defer processes.Delete(cmd)
				err = cmd.Wait()
			}()
		}
		return nil
	}
	for _, cb := range cbs {
		cb()
	}
	if !f.Disable { //!f.OnStart &&
		if rf == nil {
			// console.Log("已加载 %s%s", f.Title, f.Suffix)
		} else {
			// 重载时先停止旧的代理
			StopNodeProxy(uuid)
			console.Log("已重载 %s%s", f.Title, f.Suffix)
		}
	}
	AddCommand([]*common.Function{f})

	// Node 插件有 @http 路由时自动启动反向代理
	if len(f.Https) > 0 && class == NODE {
		StartNodeProxy(f)
	}

	return nil
}

var typeat = `declare class Sender {
	private uuid;
	private destoried;
	constructor(uuid: string);
	destroy(): void;
	getUserId(): Promise<string>;
	getUserName(): Promise<string>;
	getChatId(): Promise<string>;
	getChatName(): Promise<string>;
	getMessageId(): Promise<string>;
	getPlatform(): Promise<string>;
	getBotId(): Promise<string>;
	getContent(): Promise<string>;
	isAdmin(): Promise<boolean>;
	param(key: number | string): Promise<string>;
	setContent(content: string): Promise<undefined>;
	continue(): Promise<undefined>;
	getAdapter(): Promise<Adapter>;
	listen(options?: {
			rules?: string[];
			timeout?: number;
			handle?: (s: Sender) => Promise<string | void> | string | void;
			listen_private?: boolean;
			listen_group?: boolean;
			allow_platforms?: string[];
			prohibit_platforms?: string[];
			allow_groups?: string[];
			prohibit_groups?: string[];
			allow_users?: string[];
			prohibit_users?: string[];
	}): Promise<Sender | undefined>;
	holdOn(str: string): string;
	reply(content: string): Promise<string>;
	doAction(options: Record<string, any>): Promise<any>;
	getEvent(): Promise<Record<string, any>>;
}
declare class Bucket {
	private name;
	constructor(name: string);
	transform(v: string | undefined): string | number | boolean | undefined;
	reverseTransform(value: any): string;
	get(key: string, defaultValue?: any): Promise<any>;
	set(key: string, value: any): Promise<{
			message?: string;
			changed?: boolean;
	}>;
	getAll(): Promise<Record<string, any>>;
	delete(key: string): Promise<{
			message?: string;
			changed?: boolean;
	}>;
	deleteAll(): Promise<undefined>;
	keys(): Promise<string[]>;
	len(): Promise<number>;
	buckets(): Promise<string[]>;
	watch(key: string, handle: (old: any, now: any, key: string) => StorageModifier | void): void;
	getName(): Promise<string>;
}
interface StorageModifier {
	echo?: string;
	now?: any;
	message?: string;
	error?: string;
}
interface Message {
	message_id?: string;
	user_id: string;
	chat_id?: string;
	content: string;
	user_name?: string;
	chat_name?: string;
}
declare class Adapter {
	platform: string;
	bot_id: string;
	call: any;
	constructor(options: {
			platform: string;
			bot_id: string;
			replyHandler?: (message: Message) => Promise<string | undefined>;
			actionHandler?: (message: Message) => Promise<string | undefined>;
	});
	receive(message: Message): Promise<undefined>;
	push(message: Message): Promise<string>;
	destroy(): Promise<void>;
	sender(options: any): Promise<Sender>;
}
declare let sender: Sender;
declare function sleep(ms?: number): Promise<unknown>;
interface CQItem {
	type: string;
	params: Record<string, string>;
}
interface CQParams {
	[key: string]: string | number | boolean;
}
declare let utils: {
	buildCQTag: (type: string, params: CQParams, prefix?: string) => string;
	parseCQText: (text: string, prefix?: string) => (string | CQItem)[];
	image: (url: string) => string;
	video: (url: string) => string;
};
declare let console: {
	log(...args: any[]): void;
	info(...args: any[]): void;
	error(...args: any[]): void;
	debug(...args: any[]): void;
};
export { Adapter, Bucket, sender, sleep, utils, console };
`

func defaultScript(title string) string {
	create_at := time.Now().Format("2006-01-02 15:04:05")
	return `/**
* @title ` + title + `
* @create_at ` + create_at + `
* @description 🐒这个人很懒什么都没有留下
* @author ` + sillyGirl.GetString("author", "佚名") + `
* @version v1.0.0
*/

const {
  sender: s,
  Bucket,
  utils: { buildCQTag, image, video },
} = require("sillygirl");
`
}

const (
	NODE    = "node"
	PYTHON  = "python3"
	UNKNOWN = "unknown"
)

func FindMainIndex(home string) (string, string) {
	if info, err := os.Stat(home + "/main.js"); err == nil && !info.IsDir() {
		return home + "/main.js", NODE
	}
	if info, err := os.Stat(home + "/main.py"); err == nil && !info.IsDir() {
		return home + "/main.py", PYTHON
	}
	return "", ""
}

func CheckMainIndex(filename string) (string, bool) {
	switch filename {
	case "main.js":
		return NODE, true
	case "main.py":
		return PYTHON, true
	}
	return "", false
}
