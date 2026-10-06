# _pending_upstream —— 待接线模块源码（不影响构建）

本目录存放上游「阻塞项解除包」中**已收到但尚未接线**的 handler 源码。
目录名以 `_` 开头，Go 工具链会**忽略**该目录，因此放在这里不会参与编译。

## 为什么没接线

上游 `handler/{webdav,org}.go` 依赖本 fork 核心存储层尚未具备的 4 项能力
（本 fork 是 AiKmap 早期 fork，核心层有分歧）：

| # | 缺口 | 用途 | 涉及 |
|---|---|---|---|
| 1 | `service.File` 缺 `ViewCount` 字段（DB 列已存在，结构体未扫） | 文件列表展示 | WebDAV |
| 2 | `service.FileStore` 缺 `ReplaceContentBinary` | 写回远端文件 | WebDAV |
| 3 | `service.NotifyStore` 缺 `AddUser` | 成员变更通知 | Org |
| 4 | handler 侧缺 `spaceRole` / `quotaUse` / `quotaUploadMB` 等共享辅助 | 权限与配额 | Org / WebDAV |

补齐上述 4 项后，按 `handler/routes.go` 中 CoreModules 的注册方式接线即可：

```go
if a.webdav != nil { /* webdavMountsList ... */ }
if a.org != nil    { /* orgSettings / orgTreeList / orgTransfers* ... */ }
```

装配点在 `server/cmd/aikmap/capabilities.go`（`buildCoreModules` 中的 TODO 段）。

## 对照的本壳实现

- 装配与门控：`server/cmd/aikmap/capabilities.go`
- CoreModules 定义与路由：`server/internal/handler/routes.go`
- service 层已到位并编译通过：`server/internal/service/{webdavmount,org}.go`
- 建表迁移已落地：`server/internal/repo/db.go`（webdav_mounts；Org 树存 settings 键）
