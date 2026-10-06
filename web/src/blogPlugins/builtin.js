// blogPlugins/builtin.js 内置博客插件注册（协议示例；第三方插件按 docs/博客插件协议.md 开发）
import { registerBlogPlugin } from './index'
import PostMeta from './builtin/postMeta.vue'
import TagBadges from './builtin/tagBadges.vue'
import MetaCard from './builtin/metaCard.vue'
import SeoMeta from './builtin/seoMeta.vue'
import Comments from './builtin/comments.vue'
import RelatedPosts from './builtin/relatedPosts.vue'
import Archives from './builtin/archives.vue'
import TagCloud from './builtin/tagCloud.vue'
import Cover from './builtin/cover.vue'
import ReaderAi from './builtin/readerAi.vue'
import VoiceRead from './builtin/voiceRead.vue'
import Stats from './builtin/stats.vue'

// 文章元信息：挂载文章页正文下方（post_bottom）
// manifest 由后端 EnsureBlogSpace 种子注册（id=blog-meta-footer，mount_points=['post_bottom']）
registerBlogPlugin({
  id: 'blog-meta-footer',
  mountPoints: ['post_bottom'],
  component: PostMeta
})

// 列表徽章：挂载公开首页文章列表项（list_item）
// manifest：id=blog-tag-badges，mount_points=['list_item']
registerBlogPlugin({
  id: 'blog-tag-badges',
  mountPoints: ['list_item'],
  component: TagBadges
})

// 文章信息卡：挂载文章页侧栏（sidebar）
// manifest：id=blog-meta-card，mount_points=['sidebar']
registerBlogPlugin({
  id: 'blog-meta-card',
  mountPoints: ['sidebar'],
  component: MetaCard
})

// SEO meta：挂载公开页 head（head）
// manifest：id=blog-seo-meta，mount_points=['head']
registerBlogPlugin({
  id: 'blog-seo-meta',
  mountPoints: ['head'],
  component: SeoMeta
})

// 评论：挂载文章页正文下方（post_bottom；评论核 API 展示层，写需登录）
// manifest：id=blog-comments，mount_points=['post_bottom']
registerBlogPlugin({
  id: 'blog-comments',
  mountPoints: ['post_bottom'],
  component: Comments
})

// 相关推荐：同分类优先（公开数据契约，无需 AI）
registerBlogPlugin({
  id: 'blog-related-posts',
  mountPoints: ['post_bottom'],
  component: RelatedPosts
})

// 归档：按月分组
registerBlogPlugin({
  id: 'blog-archives',
  mountPoints: ['sidebar'],
  component: Archives
})

// 分类云：path 首段计数
registerBlogPlugin({
  id: 'blog-tag-cloud',
  mountPoints: ['sidebar'],
  component: TagCloud
})

// 封面：从 preview 提取首图
registerBlogPlugin({
  id: 'blog-cover',
  mountPoints: ['list_item'],
  component: Cover
})

// 读者 AI 问答
registerBlogPlugin({
  id: 'blog-reader-ai',
  mountPoints: ['post_bottom'],
  component: ReaderAi
})

// 访问统计 PV
registerBlogPlugin({
  id: 'blog-stats',
  mountPoints: ['post_bottom'],
  component: Stats
})

// 语音朗读：文章页朗读播放器（复用 ai.tts 语音合成，服务端磁盘缓存）
registerBlogPlugin({
  id: 'blog-voice-read',
  mountPoints: ['post_bottom'],
  component: VoiceRead
})
