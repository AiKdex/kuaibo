# 博客迁移插件设计方案（WordPress / emlog / zblog → AiKlog / KMap）

> 部分内容由豆包生成
> 
> 

方案结论：迁移采取"三件套"设计——旧站导出插件产出「md 文章 \+ 本地图片包 \+ 重定向表」三个部分；新站通过目录兼容挂载实现图片路径零改写；链接保持拆成「slug 保新链接 \+ 301 保旧链接」两层。核心原则是\<bold\>迁移不改内容，只改系统兼容性\</bold\>。

# 一、背景与目标

存量博客用户大量分布在 WordPress、emlog、zblog 等 PHP 系统上。迁移到 AiKlog / KMap 知识库博客系统的价值在于：博客引擎与知识库打通、AI 能力全量可用、数据自持。但迁移成本（文章格式转换、图片搬运、链接失效）是用户最大的顾虑。

## 1\.1 迁移目标

- \<bold\>一键导出\</bold\>：旧站安装导出插件，一个 zip 包带走全部文章与本地图片。

- \<bold\>图片不裂\</bold\>：本地图片按 WordPress 目录惯例原样落盘，图床外链保留原 URL。

- \<bold\>链接不失效\</bold\>：旧站原链接全部 301 到新链接，SEO 权重不丢，旧书签不落空。

- \<bold\>数据自持\</bold\>：导出产物为标准 md \+ 图片，用户可随时自行部署。

## 1\.2 适用系统

|系统|语言/形态|迁移重点|
|---|---|---|
|WordPress|PHP / CMS|post\_name 保留 slug；uploads 图片打包|
|emlog|PHP / 轻量博客|老版本仅 ID 无 slug；uploadfile 图片打包|
|zblog|PHP / 轻量博客|ID \+ alias 并存；upload 图片打包|

# 二、方案总览：三件套设计

整个迁移方案由三件套构成，各司其职、互不耦合：

1. \<bold\>旧站导出插件\</bold\>：跑在旧站（PHP 侧），产出标准迁移包 zip。

2. \<bold\>新站目录兼容挂载\</bold\>：AiKlog / KMap 增加静态挂载点，服务 WordPress 目录惯例的图片路径。

3. \<bold\>链接保持层\</bold\>：slug 决定新链接形态，301 重定向表保证旧链接不失效。

# 三、旧站导出插件设计

## 3\.1 产出物规范

导出插件输出一个 zip 包，内部结构固定：

```text
migration-{site}.zip
├── posts/                  # 每篇一个 md，文件名 = slug（无 slug 用 ID）
│   ├── hello-world.md
│   └── ...
├── images/                 # 本地图片，按 wp-content/uploads/{年}/{月}/ 原样存放
│   └── wp-content/uploads/2026/09/photo.jpg
└── redirect-map.csv        # 旧URL → 新URL 重定向表

```

## 3\.2 md front matter 字段

|字段|必填|说明|
|---|---|---|
|title|是|文章标题|
|date|是|发布时间（影响排序与归档）|
|modified|否|修改时间|
|categories|是|旧站分类，导入时映射为新站目录|
|tags|否|标签列表|
|slug|是|新站 URL 别名，md 文件名默认取它|
|original\_url|是|旧站完整 URL，用于生成 301 重定向表|

## 3\.3 图片处理规则

- \<bold\>本地图片\</bold\>：随包打包，按 wp\-content/uploads/\{年\}/\{月\}/ 原样保留目录层级，不重命名。

- \<bold\>图床 / 外链图片\</bold\>：不下载、不迁移，md 引用保留原 URL 原样。

- \<bold\>相对路径改写\</bold\>：md 内对本地图片的引用保持相对路径，导入后由新站静态挂载直接服务。

## 3\.4 各系统差异表

|维度|WordPress|emlog|zblog|
|---|---|---|---|
|slug 来源|post\_name（原生有 slug）|老版本仅 ID，无 slug；链接 /post\-\{id\}\.html|ID 与 alias 并存，/post/\{id\}\.html 或 alias|
|图片位置|wp\-content/uploads/\{年\}/\{月\}/|content/uploadfile/|zb\_users/upload/|
|导出要点|保留 slug 做 md 文件名；原 URL 带日期前缀，重定向按整条 URL 记录|无 slug 用 ID 当文件名/slug；重定向 /post\-\{id\}\.html → /s/blog/\{id\}|有 alias 用 alias，否则用 ID|
|实现方式|插件或脚本走 REST API / 数据库|PHP 插件读取数据库|PHP 插件读取数据库|

> 踩坑提示：中文站点老文章的 URL 可能是中文路径或纯 ID。导出时要么生成新 slug（拼音/英文），要么直接用 ID 当 slug；重定向表必须按「旧 URL 原文」记录，不能靠猜。
> 
> 

# 四、新站目录兼容挂载

## 4\.1 设计思路

不对图片做"翻译"，而是让新站"容纳"WordPress 的目录惯例：图片按 wp\-content/uploads/\{年\}/\{月\}/ 原样落盘，md 引用原样保留，新站加一个静态挂载点把这些目录服务出去。核心收益：图片路径全程零改写，md 不用动、图片不用重命名、旧外链引用保持有效。

## 4\.2 AiKlog（Go）改动点

路由层加一个静态目录挂载，与现有 StaticHandler 并列：

```go
// 新增：博客媒体兼容目录（WordPress 风格）
mux.Handle("/wp-content/uploads/",
    http.StripPrefix("/wp-content/uploads/",
        http.FileServer(http.Dir("<data_dir>/media/wp-content/uploads"))))

```

## 4\.3 KMap（FastAPI）改动点

main\.py 增加一个 StaticFiles 挂载：

```python
app.mount("/wp-content/uploads",
    StaticFiles(directory=settings.data_dir / "media/wp-content/uploads"),
    name="blog-media")

```

## 4\.4 安全边界

- \<bold\>扩展名白名单\</bold\>：挂载点只服务图片扩展名（jpg/png/webp/gif/svg/avif），服务前做扩展名校验，避免任意文件被静态暴露。

- \<bold\>防路径穿越\</bold\>：http\.FileServer / StaticFiles 自带目录边界，仍需确认不允许 \.\. 逃逸。

- \<bold\>可预测路径\</bold\>：WordPress 路径是公开可预测的，无鉴权静态服务为预期行为，不放敏感文件到该目录。

# 五、链接保持：slug 与 301 的分工

## 5\.1 两层机制

|层|作用|实现|
|---|---|---|
|slug|决定新链接长什么样（/s/blog/\{slug\}），导入时落库|md 文件名 / front matter 的 slug 字段|
|301 重定向|保证旧链接不失效（旧 URL → 新 URL）|redirect\-map\.csv → 目标系统重定向注册 / nginx rewrite|

## 5\.2 redirect\-map 格式

```csv
old_url,new_url
https://old.com/2024/03/hello-world/,https://new.com/s/blog/hello-world
https://old.com/post-123.html,https://new.com/s/blog/123

```

## 5\.3 目标系统落地

- \<bold\>方案 A（系统内置）\</bold\>：AiKlog / KMap 增加"重定向注册"能力（插件或配置），导入时读取 redirect\-map 注册；请求旧路径时返回 301。

- \<bold\>方案 B（边缘层）\</bold\>：由 redirect\-map 生成 nginx rewrite 规则或 CDN 规则，在 Web 层完成 301。

- \<bold\>建议\</bold\>：初期用方案 B（快、零代码改动），后续在系统内做管理界面归入方案 A。

# 六、导入流程

1. 用户在旧站安装导出插件，生成 migration\-\{site\}\.zip。

2. 用户将 zip 上传新站（AiKlog ImpexView / KMap 导入页）。

3. 导入器解析 posts/\*\.md：front matter 落库（title/date/categories/tags/slug），正文按相对路径保留图片引用。

4. 图片包按目录结构原样解压到媒体目录，静态挂载立即生效。

5. redirect\-map 交给 301 层（nginx / 系统注册），旧链接全部跳转。

6. 导入完成校验：文章数、图片数、抽查链接可访问。

> 注意：导入器要支持"目录结构导入"（不是只扫平铺 md）。AiKlog 的 ZIP 导入 / Obsidian 导入已具备基础，KMap import\-md 需扩展目录层级支持。
> 
> 

# 七、验收标准

|项|验收口径|
|---|---|
|导出完整性|旧站全部文章导出为 md，篇数与旧站一致；front matter 字段无缺失|
|图片完整性|本地图片全部入包且目录层级保留；图床图片未下载、引用未改写|
|图片可访问|导入后 md 内图片在新站全部可打开，无裂图|
|链接保持|旧站原链接（含 /\{年\}/\{月\}/\{slug\}/ 与 /post\-\{id\}\.html）全部 301 到新链接，状态码 301 且目标正确|
|slug 稳定|新站文章 URL 与 md 文件名 slug 一致；改名不碎链|
|SEO 衔接|分类/标签映射到新站目录/标签，归档页与排序正常|

# 八、下一步行动

1. \<bold\>定稿导出插件字段规范\</bold\>：确认 front matter 字段与 zip 结构（本文档 3\.1 / 3\.2）为统一标准。

2. \<bold\>确认新站挂载改动\</bold\>：AiKlog / KMap 侧各加一处静态挂载（第 4 章），评估改动量。

3. \<bold\>确认 301 落地方式\</bold\>：初期走 nginx rewrite，后续评估系统内置重定向注册。

4. \<bold\>开发优先级\</bold\>：先做 WordPress 导出插件（存量最大）→ emlog / zblog → 新站挂载与导入器。

> （注：部分内容可能由 AI 生成）
