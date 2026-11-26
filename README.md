# hello-zb — MacWordLookup 词单服务

Mac 端按窗口发布词单 JSON，手机用只读链接复习（PWA，可添加到主屏幕）。

## 环境变量

| 变量 | 必填 | 说明 |
|---|---|---|
| `PUBLISH_TOKEN` | 是 | Mac 端发布用，`Authorization: Bearer <token>` |
| `READ_KEY` | 是 | 只读路径里的随机串，例如 `/r/<READ_KEY>/` |
| `DATA_DIR` | 否 | 数据目录，默认 `./data` |
| `PORT` | 否 | 监听端口，默认 `8080` |

未设置 `PUBLISH_TOKEN` / `READ_KEY` 时：发布返回 503，读取一律 404。

## 接口

`POST /publish`

- Header：`Authorization: Bearer <PUBLISH_TOKEN>`
- Body：

```json
{
  "version": 1,
  "windowStart": "2026-11-09T13:00:00Z",
  "windowEnd": "2026-11-10T13:00:00Z",
  "generatedAt": "2026-11-10T13:00:05Z",
  "words": [
    { "word": "Focus", "translation": "专注", "phonetic": "/ˈfəʊkəs/" }
  ]
}
```

- 返回：`200 {"ok":true,"words":1}`；鉴权失败 401，结构不合法 400，超过 1MB 413

`GET /r/<READ_KEY>/`

- 返回：PWA 页面（HTML）

`GET /r/<READ_KEY>/latest.json`

- 返回：最近一次发布的词单；从未发布时返回 `{"version":1,"words":[]}`

`GET /r/<READ_KEY>/manifest.webmanifest`、`GET /r/<READ_KEY>/sw.js`

- 返回：PWA manifest 与 service worker

`GET /health`

- 返回：`ok`

## Mac 端配置

`~/Library/Application Support/MacWordLookup/.env`：

```
PUBLISH_URL=https://<你的域名>/publish
PUBLISH_TOKEN=<PUBLISH_TOKEN>
```

## 部署（Render）

1. 在 Render 新建 Blueprint 或直接用 `render.yaml`
2. 设置 `PUBLISH_TOKEN`、`READ_KEY`（`sync: false` 的两个变量在面板里填）
3. 部署后用 `https://<service>.onrender.com/r/<READ_KEY>/` 在手机打开

**数据持久化**：Render free 实例的文件系统是临时的，重启/重新部署后 `DATA_DIR` 会清空，手机在下一次 Mac 发布前会看到「本周期没有新词」。需要长期保留可挂一个持久磁盘（付费），把 `DATA_DIR` 指到挂载点（如 `/data`）。

## 本地开发

```bash
PUBLISH_TOKEN=secret READ_KEY=rk DATA_DIR=./data go run .

curl -X POST localhost:8080/publish \
  -H 'Authorization: Bearer secret' \
  -H 'Content-Type: application/json' \
  -d '{"version":1,"windowStart":"2026-11-09T13:00:00Z","windowEnd":"2026-11-10T13:00:00Z","generatedAt":"2026-11-10T13:00:05Z","words":[{"word":"Focus","translation":"专注"}]}'

open http://localhost:8080/r/rk/
```

## 测试

```bash
go test ./...
```
