# 傻妞

一个不太有用的机器人，不生产消息，只搬运消息。

[![Docker Image](https://img.shields.io/docker/pulls/ntwck/sillygirl)](https://hub.docker.com/r/ntwck/sillygirl)

## 特性

- 简单易用的消息搬运功能。
- 简单强大的自定义回复功能。
- **双 JS 引擎**：内置 [goja](https://github.com/dop251/goja) 引擎（ES5.1+，轻量脚本） + 外挂 **Node.js**（完整 npm 生态，复杂插件）。
- 支持通过内置的阉割版 `Express` / `request` ，接入互联网。
- 内置 `Cron` ，轻松实现定时任务。
- 持久化的 `Bucket` 存储模块。
- 支持同时接入多个平台多个机器人。

---

## 📖 目录

- [快速使用](#-快速使用)
  - [Docker 运行（推荐）](#docker-运行推荐)
  - [直接下载运行](#直接下载运行)
  - [命令行参数](#命令行参数)
- [插件开发](#-插件开发)
  - [Hello World](#hello-world)
  - [定时任务](#定时任务)
  - [接入机器人](#接入机器人)
  - [用户交互](#用户交互)
  - [HTTP 接口](#http-接口)
  - [HTTP 请求](#http-请求)
  - [持久化存储](#持久化存储)
  - [管理员](#管理员)
  - [群组消息](#群组消息)
- [API 参考](#-api-参考)
  - [插件注释](#插件注释)
  - [Sender](#sender)
  - [Express Request / Response](#express-request--response)
  - [request](#request)
  - [Adapter](#adapter)
  - [Bucket](#bucket)
  - [Cron](#cron)
  - [插件表单](#插件表单)
  - [其他工具函数](#其他工具函数)
  - [CQ 码](#cq-码)

---

## 🚀 快速使用

### Docker 运行（推荐）

确保已安装 [Docker](https://docs.docker.com/engine/install/)。

**拉取并启动：**

```bash
docker run -d \
  --name sillygirl \
  -p 8080:8080 \
  -v $(pwd)/sillygirl-data:/app \
  --restart unless-stopped \
  ntwck/sillygirl:latest
```

| 参数 | 说明 |
|------|------|
| `-p 8080:8080` | Web 管理端口，按需修改 |
| `-v $(pwd)/sillygirl-data:/app` | 持久化目录（插件、配置、数据都在这里） |
| `--restart unless-stopped` | 容器退出后自动重启 |

**带终端交互启动：**

```bash
docker exec -it sillygirl /app/sillyGirl -t
```

**查看日志：**

```bash
docker logs -f sillygirl
```

**使用自定义镜像 tag：**

```bash
docker pull ntwck/sillygirl:${{ github.sha }}
```

> 所有文件（可执行文件、插件、配置、数据库）都位于 `/app` 目录下，挂载即可实现完整持久化。支持 `linux/amd64` 和 `linux/arm64` 架构。

### 直接下载运行

从 [releases](https://github.com/ntwck/sillyGirl/releases) 下载对应平台的压缩包，解压后运行：

```bash
./sillyGirl -t
```

程序首次运行会自动创建以下目录结构：

```
sillyGirl/
├── sillyGirl              # 可执行文件
├── plugins/               # 插件目录
├── language/              # Node.js 运行时
├── node_modules/          # 内置 sillygirl 模块
└── .sillyplus/            # 配置及数据存储
```

### 命令行参数

```bash
./sillyGirl -h           # 查看帮助
./sillyGirl -t           # 开启终端交互模式
./sillyGirl -d           # 调试模式
```

---

## 📦 插件开发

傻妞支持**两种插件模式**：

| 模式 | 引擎 | 特点 | 适用场景 |
|------|------|------|----------|
| **内置脚本** | [goja](https://github.com/dop251/goja) (ES5.1+) | 零依赖，内嵌执行 | 简单规则、轻量逻辑 |
| **Node.js 外挂插件** | 系统 Node.js | 完整 npm 生态 | 复杂业务、需要 npm 包 |

> 插件通过 `@rule` 触发，两种模式共享 Sender、Bucket 等核心 API。

### Hello World（内置脚本）

```js
/**
 * @title HelleWorld
 * @rule raw ^你好$
 */

s.reply("Helle World!");
```

输入 `你好`，机器人回复 `Helle World!`：

```
你好
Helle World!
```

`@rule raw ^你好$` 中的正则表达式被消息匹配时插件脚本就会被触发。

### 定时任务

使用 `@on_start true` 让插件作为后台服务持续运行。

```js
/**
 * @title 定时任务
 * @on_start true
 */

const task = Cron();
let taskId = 0;
let times = 5;
const { id } = task.add("*/5 * * * * *", () => {
  times--;
  console.log(
    `每5秒执行一次任务，${times ? `${times}次后结束任务` : "这是最后一次任务"}。`
  );
  if (times == 0) {
    task.remove(taskId);
  }
});
taskId = id;
```

输出：

```
2023/05/27 19:57:00.000 [I]  每5秒执行一次任务，4次后结束任务。
2023/05/27 19:57:05.001 [I]  每5秒执行一次任务，3次后结束任务。
...
```

### 接入机器人

```js
/**
 * @title 第一个机器人
 * @on_start true
 */

const task = Cron();
const qq_1700000 = initAdapter("qq", "1700000");

task.add("*/5 * * * * *", function () {
  let message = {
    user_id: 100000,
    content: "你好",
  };
  qq_1700000.receive(message);
});

qq_1700000.setReplyHandler(function (message) {
  console.log(`给用户${message.user_id}发消息：${message.content}`);
});
```

### 用户交互

```js
/**
 * @title 用户交互插件
 * @rule 猜拳
 */

s.reply("你先出，请在10秒内出拳！");
ns = s.listen({
  rules: ["[出拳:剪刀,石头,布]"],
  timeout: 10000,
  handle: (s) => {
    let choose = s.param("出拳");
    s.reply(`我出${choose == "石头" ? "剪刀" : choose == "布" ? "剪刀" : "石头"}，我赢了。`);
  },
});
if (!ns) {
  s.reply("你没出拳，算我赢了！");
}
```

### HTTP 接口（goja 插件）

```js
/**
 * @title 第一个web服务
 * @on_start true
 */

const app = Express();
app.get("/helloWorld", function (req, res) {
  res.send("Hello world!");
});
```

访问 `http://127.0.0.1:8080/helloWorld` 即可看到 `Hello world!`。

### HTTP 接口（Node.js 插件）

Node.js 外挂插件可以通过 `@http` 注释声明 HTTP 路由，傻妞会自动启动反向代理把这些路由绑定到 8080 端口。

**工作原理**：傻妞加载插件时发现 `@http` 注释 → 自动分配随机端口（40000-50000）→ 设置环境变量 `HTTP_LISTEN_PORT` 后启动 Node 子进程 → Node 插件在指定端口启动 HTTP 服务 → 傻妞等待端口就绪后注册 Gin 反向代理路由。

**不需要手动配置端口、不需要 Nginx 反代，一切都自动完成。**

```js
/**
 * @name node-http-demo
 * @title Node.js HTTP 路由示例
 * @version 1.0.0
 * @public false
 * @admin false
 * @disable false
 * @service true
 * @http GET /api/hello
 * @http POST /api/echo
 * @http GET /api/json
 * @http GET /api/query
 * @create_at 2099-01-01 12:10:49
 */

const http = require('http');
const url = require('url');

const port = parseInt(process.env.HTTP_LISTEN_PORT || '30000', 10);
const server = http.createServer((req, res) => {
    const parsed = url.parse(req.url, true);
    const path = parsed.pathname;
    const method = req.method;

    let body = '';
    req.on('data', chunk => body += chunk);
    req.on('end', () => {

        if (method === 'GET' && path === '/api/hello') {
            res.writeHead(200, { 'Content-Type': 'text/plain; charset=utf-8' });
            res.end('Hello from Node.js plugin! 🎉');
            return;
        }

        if (method === 'POST' && path === '/api/echo') {
            res.writeHead(200, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({ method, path, query: parsed.query, body }));
            return;
        }

        if (method === 'GET' && path === '/api/json') {
            res.writeHead(200, { 'Content-Type': 'application/json' });
            res.end(JSON.stringify({
                success: true,
                message: 'Hello from reverse proxy',
                timestamp: Date.now(),
            }));
            return;
        }

        if (method === 'GET' && path === '/api/query') {
            const name = parsed.query.name || 'World';
            res.writeHead(200, { 'Content-Type': 'text/plain; charset=utf-8' });
            res.end(`你好，${name}！`);
            return;
        }

        res.writeHead(404);
        res.end('Not Found');
    });
});

server.listen(port, '127.0.0.1', () => {
    console.log(`[node-http-demo] HTTP server listening on ${port}`);
});
```

**支持的 @http 语法**：

| 示例 | 说明 |
|------|------|
| `@http GET /api/xxx` | 仅匹配 GET 请求 |
| `@http POST /api/xxx` | 仅匹配 POST 请求 |
| `@http ANY /api/xxx` | 匹配任意 HTTP 方法 |

**环境变量**：

| 变量 | 说明 |
|------|------|
| `HTTP_LISTEN_PORT` | 傻妞自动分配的可用端口（40000-50000） |
| `PLUGIN_ID` | 当前插件的唯一标识 |

**注意事项**：
- 插件必须有 `@service true` 才会常驻运行
- Node 插件可以使用原生 `http` 模块或 `express`、`koa`、`fastify` 等任意框架
- 端口由傻妞自动分配，无需关心端口冲突
- 插件进程意外退出时，反向代理路由需重载插件或重启傻妞才能清理

### HTTP 请求

```js
/**
 * @title 实现一个HTTP 请求
 * @on_start true
 */

let api = "/testRequest";

const app = Express();
app.post(api, (req, res) => res.json(req.json()));

const port = Bucket("app").port ?? "8080";
const url = `http://127.0.0.1:${port}${api}`;
fetch({
  url,
  method: "POST",
  body: { value: "test" },
})
  .then((resp) => resp.json())
  .then((data) => console.log(`value is ${data.value}`))
  .catch((e) => console.log(e));
```

### 持久化存储

```js
/**
 * @title 持久化存储
 * @rule raw ^我是谁$
 * @rule 我是[姓名]
 */

const user = Bucket("user");
let name = s.param("姓名");

if (user.name == "") {
  s.reply("我不知道你是谁！");
} else if (name == "谁") {
  s.reply(`你是${user.name}`);
} else {
  user.name = name;
  s.reply(`好的，你的姓名更新为${user.name}`);
}
```

### 管理员

```js
const masters = Bucket("qq")["masters"];
```

管理员账号通过 `&` 拼接，系统默认依此判断用户是否是管理员。

### 群组消息

默认不监听不回复任何群组。管理员在对应群组发送口令控制：

| 口令 | 作用 |
|------|------|
| `listen` | 开始监听该群 |
| `unlisten` | 停止监听该群 |
| `reply` | 开始回复该群 |
| `noreply` | 停止回复该群 |

---

## 📚 API 参考

### 插件注释

| 字段 | 举例 | 用法 |
|------|------|------|
| `title` | `HelloWorld` | 插件标题 |
| `rule` | `raw ^我是([\s\S]+)$` | 可写多行，取括号内参数 `s.param(1)` |
| `priority` | `1` | 插件优先级，越高越优先处理 |
| `on_start` | `true` | 后台任务执行脚本，避免重复运行 |
| `disable` | `true` | 禁用脚本 |
| `form` | `{title: "姓名", key:"user.name"}` | 插件表，key 对应 `存储桶.键名` |
| `public` | `true` | 公开插件 |
| `create_at` | `2023-05-24 15:14:53` | 插件创建时间 |
| `description` | `本插件用于每天向女友问好` | 插件描述 |
| `author` | `cdle` | 插件作者 |
| `version` | `v1.0.0` | 插件版本 |
| `icon` | `url` | 插件图标 |

### Sender

傻妞搬运的核心对象，在插件中为全局变量 `s` 或 `sender`。

```ts
interface Sender {
  getUserId(): string;
  getUserName(): string;
  getChatId(): string;
  getChatName(): string;
  getMessageId(): string;
  getContent(): string;
  continue(): void;
  setContent(content: string): void;
  param(index: string | number): string;
  holdOn(content: string): string;
  listen(options: ListenOptions): Sender;
  isAdmin(): boolean;
  getPlatform(): string;
  getBotId(): string;
  reply(content: string): { message_id: string; error: string };
  recallMessage(meesageId: string | string[] | number): { error: string };
  kick(user_id: string): { error: string };
  unkick(user_id: string): { error: string };
  ban(user_id: string, duration: number): { error: string };
  unban(user_id: string): { error: string };
}
```

#### listen options

```ts
interface ListenOptions {
  rules: string[];             // 匹配规则
  timeout: number;             // 超时（毫秒）
  handle: (s: Sender) => string;
  listen_private: boolean;     // 监听用户群内消息时，同时监听用户消息
  listen_group: boolean;       // 监听用户消息时，同时监听群员消息
  allow_platforms: string[];   // 平台白名单
  prohibit_platforms: string[];// 平台黑名单
  allow_groups: string[];      // 群聊白名单
  prohibit_groups: string[];   // 群聊黑名单
  allow_users: string[];       // 用户白名单
  prohibit_users: string[];    // 用户黑名单
}
```

### Express Request / Response

通过 `Express()` 返回。

**Request:**

```ts
interface Request {
  body(): string;
  json(): any;
  ip(): string;
  originalUrl(): string;
  query(param: string): string;
  param(i: number): string;
  querys(): Record<string, string[]>;
  postForm(s: string): string;
  postForms(): Record<string, string[]>;
  path(): string;
  header(s: string): string;
  get(s: string): string;
  headers(): Record<string, string[]>;
  method(): string;
  cookie(s: string): string;
  cookies(): Record<string, string>;
  continue(): void;
  setSession(k: string, v: string): string;
  getSession(k: string): string;
  getSessionId(): string;
  destroySession(): string;
  logined(): boolean;
}
```

**Response:**

```ts
interface Response {
  send(body: any): Response;
  sendStatus(status: number): Response;
  json(...ps: any[]): Response;
  header(str: string, value: string): Response;
  set(str: string, value: string): void;
  render(view: string, params: Record<string, any>): Response;
  redirect(...is: any[]): void;
  status(i: number, ...s: string[]): Response;
  setCookie(name: string, value: string, ...i: any[]): Response;
  stop(): void;
}
```

### request

```ts
function request(options: {
  url: string;
  method: string;
  headers: { [key: string]: string };
  json: boolean;
  timeout: number;
  form: { [key: string]: any };
  body: any;
  allow_redirects: boolean;
  proxy: {};
}): {
  status: number;
  headers: { [key: string]: string };
  body: any;
};
```

### Adapter

```ts
interface Message {
  message_id: string;
  user_id: string;
  chat_id: string;
  content: string;
  user_name: string;
  chat_name: string;
}

class Adapter(botplt: string, botid: string) {
  isAdapter(botid: string): boolean;
  push(message: Message): string;
  getReplyMessage(): Promise<Message>;
  setReplyHandler(func: (message: Message) => string): void;
  receive(message: Message): Sender;
  setRecallMessage(func: (i: string | string[]) => boolean): void;
  setGroupKick(func: (user_id: string, chat_id: string, reject_add_request: boolean) => void): boolean;
  setGroupBan(func: (user_id: string, chat_id: string, duration: number) => void): boolean;
  setGroupUnban(func: (user_id: string, chat_id: string) => void): boolean;
  setIsAdmin(func: (user_id: string) => boolean): void;
  destroy(): void;
}
```

### Bucket

```ts
interface Bucket(name: string) {
  get(key: string, defaultValue: any): any;
  set(key: string, value: any): Error | null;
  watch(key: string, event: (old: any, new_: any, key: string) => void);
  getAll(): [];
  delete(key: string): Error | null;
  empty(): Error | undefined;
  keys(): string[];
  len(): number | undefined;
  buckets(): string[];
  _name(): string;
}
```

### Cron

```ts
interface Cron {
  add(crontab: string, () => void): { id: number; error: string };
  remove(id: number): void;
}
```

### 插件表单

```js
// 单个表单元素
Form({
  title: "姓名",
  key: "test.name",
});

// 多个表单元素
Form([
  { title: "姓名", key: "test.name" },
  { title: "性别", key: "test.sex" },
]);
```

支持 Ant Design Pro 的 SchemaForm 语法，详见源码示例。

### 其他工具函数

```ts
sleep(millsec: number): void;            // 等待
md5(string): string;                     // MD5 加密
running(): boolean;                      // 服务是否运行
uuid(): string;                          // 获取脚本uuid
genUuid(): string;                       // 生成uuid
```

支持 `Crypto`、`Buffer`。

### CQ 码

```
[CQ:delete,id=message_id]
[CQ:kick,user_id,chat_id,forever=true]
[CQ:ban,user_id,chat_id,duration=0]
```

---



---

## 🛠 本地开发

### 环境要求

- Go 1.20+
- Node.js 20+
- npm / yarn

### 构建

```bash
# 构建 webpack bundle
cd proto3
npm install
npx webpack
mkdir -p ../core/proto3/dist
cp dist/sillygirl.js ../core/proto3/dist/sillygirl.js

# 编译 Go 二进制
cd ..
CGO_ENABLED=0 go build -ldflags "-s -w" -o sillyGirl
```

### Docker 镜像构建（本地）

```bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t ntwck/sillygirl:latest \
  --push .
```

---

## 🙏 项目赞助

打开微信扫一扫，深入了解作者~

![](https://raw.githubusercontent.com/cdle/sillyGirl/main/appreciate.jpg)
