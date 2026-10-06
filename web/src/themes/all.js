/**
 * 主题集中注册入口 —— 公开博客的所有 SPA 视图共用（列表页 BlogView / 文章页 BlogPostView）。
 *
 * 为什么集中：主题在 SPA 侧是「源码 + 静态 import」，必须被某个真实入口模块静态引用
 * 才会被 Vite 打进产物（运行时无法热加载 .vue，见规范 §11）。新增主题只需在此加一行，
 * 避免多个视图各写一份清单而导致遗漏（遗漏的表现是：主题在列表页可选、文章页却掉回默认皮）。
 *
 * 注意：本文件只做「注册副作用」，不导出数据；主题数据走 '@/themes' 的 listThemes 等。
 * 爱库录精简发行：不注册求智（采集型）主题。
 * 求智 manifest 声明 dataSources:['collect'] / dataScope:'library'，读的是采集库（非「已分享」
 * 文件集合），且注明「公开部署时不得对外暴露此主题」；而 ThemeSwitch 直接 v-for 渲染
 * listThemes()（= 整个注册表，无 dataScope 门控），一旦注册即对访客可见 → 故取消注册。
 * 如需单机自用（采集看板），在下方主题列表末尾把 qiuzhi 那一行加回即可，源码保留在 themes/qiuzhi/。
 */
import '@/themes/aiklog' // 注册爱库录主题（AiKlog 样板房门面）
import '@/themes/minimal' // 注册极简阅读主题
import '@/themes/docs' // 注册技术文档主题
import '@/themes/paper' // 注册暖纸情报主题（对齐求智站视觉）
import '@/themes/elevated' // 注册高端暗色主题（玉色点缀，AI 侧栏）
import '@/themes/parchment' // 注册暖纸杂志主题（亮色，琥珀强调）
import '@/themes/emforum' // 注册论坛社区主题（深蓝鎏金，版块页签式）
import '@/themes/brutal' // 注册新粗野主题（硬边框硬阴影，高饱和撞色）
import '@/themes/aurora' // 注册极光主题（Bento Grid 卡片式 · Apple 风格企业博客，含内页）
import '@/themes/verdant' // 注册青野主题（暖白 + 森林绿，明亮清爽，含内页与文章页）
import '@/themes/zircon' // 注册青璃主题（青绿胶囊 · 子比风格社区资讯式，含内页与文章页）
import '@/themes/butterfly' // 注册蝶语主题（卡片式双栏 + 侧栏卡片组 + 夜间模式，含内页与文章页）
import '@/themes/chenxi' // 注册晨曦笔记主题（玫粉渐变 Hero + 左图右文卡片 + 毛玻璃顶栏，含内页与文章页）
import '@/themes/jaded' // 注册翡翠主题（暗色翡翠绿 + 毛玻璃 + 文章衬线体，含内页与文章页）
import '@/themes/zhicang' // 注册知藏主题（靛蓝资源课程风 + 学习路径分组，含内页与文章页）
import '@/themes/aiknav' // 注册好站导航主题（蓝色导航站卡片网格 + 左侧分类树，含内页与文章页）
import '@/themes/clawblog' // 注册 ClawBlog 主题（渐变 + 玻璃拟态 + 三段式混合布局 + 侧栏 Widget，含内页与文章页）
import '@/themes/huajian' // 注册花笺主题（暖纸杂志风 + 衬线排版 + 琥珀强调 + 右下角 AI 对话窗，含内页与文章页）
