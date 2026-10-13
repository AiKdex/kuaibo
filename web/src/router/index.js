import { createRouter, createWebHashHistory } from 'vue-router'
import { isAuthed } from '@/api'

const routes = [
  // 后台入口（爱库录：路径不叫 login，降低扫描撞库；公开博客不暴露）
  {
    path: '/desk',
    name: 'login',
    component: () => import('@/views/LoginView.vue'),
    meta: { title: '控制台' }
  },
  // 兼容旧入口（仍可进，但对外不宣传）
  {
    path: '/login',
    redirect: '/desk'
  },
  // 公开博客页（分享/发布）：脱离应用外壳，仅凭 token 访问
  {
    path: '/p/:token',
    name: 'public',
    component: () => import('@/views/PublicView.vue'),
    meta: { title: '分享', public: true }
  },
  // 对外博客首页：公开文章列表
  {
    path: '/blog',
    name: 'public-home',
    component: () => import('@/views/BlogView.vue'),
    meta: { title: '博客', public: true }
  },
  // 对外博客内页（分类 / 作者 / 标签 / 归档 / 搜索）：同一 BlogView 壳，
  // 由 BlogView 解析页面参数注入 ctx.page，主题按 entries.{cat,author,tag,archive,search} 接管；
  // 主题未声明该内页时回退系统默认内页（套主题 --th-* 令牌）。
  // 同时支持 query 形态：#/blog?view=public&cat=xxx（两条路径等价，主题无需区分）。
  {
    path: '/blog/cat/:cat',
    name: 'blog-cat',
    component: () => import('@/views/BlogView.vue'),
    meta: { title: '分类', public: true }
  },
  {
    path: '/blog/author/:author',
    name: 'blog-author',
    component: () => import('@/views/BlogView.vue'),
    meta: { title: '作者', public: true }
  },
  {
    path: '/blog/tag/:tag',
    name: 'blog-tag',
    component: () => import('@/views/BlogView.vue'),
    meta: { title: '标签', public: true }
  },
  {
    path: '/blog/archive/:archive',
    name: 'blog-archive',
    component: () => import('@/views/BlogView.vue'),
    meta: { title: '归档', public: true }
  },
  {
    path: '/blog/search',
    name: 'blog-search',
    component: () => import('@/views/BlogView.vue'),
    meta: { title: '搜索', public: true }
  },
  // 对外博客文章页（SPA 交互版）：人看侧主链，与 SSR /{slug} 构成双轨
  {
    path: '/post/:token',
    name: 'public-post',
    component: () => import('@/views/BlogPostView.vue'),
    meta: { title: '文章', public: true }
  },
  {
    path: '/',
    component: () => import('@/components/AppShell.vue'),
    children: [
      // 落地页（根路径）：未登录 → 对外博客首页（给人看的门面）；
      // 已登录 → 工作台文件库。避免访客落到登录页。
      {
        path: '',
        name: 'home',
        redirect: () => (isAuthed() ? '/files' : { path: '/blog', query: { view: 'public' } })
      },
      {
        path: 'files',
        name: 'files',
        component: () => import('@/views/FilesView.vue'),
        meta: { title: '文件库' }
      },
      {
        path: 'read/:id',
        name: 'read',
        component: () => import('@/views/ReadWorkspace.vue'),
        meta: { title: '阅读', reading: true }
      },
      {
        path: 'drafts',
        name: 'drafts',
        component: () => import('@/views/DraftsView.vue'),
        meta: { title: '草稿箱' }
      },
      {
        path: 'trash',
        name: 'trash',
        component: () => import('@/views/TrashView.vue'),
        meta: { title: '回收站' }
      },
      {
        path: 'search',
        name: 'search',
        component: () => import('@/views/SearchView.vue'),
        meta: { title: '检索' }
      },
      {
        path: 'kb',
        name: 'kb',
        component: () => import('@/views/KnowledgeView.vue'),
        meta: { title: '知识库' }
      },
      {
        path: 'tags',
        name: 'tags',
        component: () => import('@/views/TagManagerView.vue'),
        meta: { title: '标签管理' }
      },
      {
        path: 'write',
        name: 'write',
        component: () => import('@/views/WriteView.vue'),
        meta: { title: '写作工作台' }
      },
      {
        path: 'manage/blog',
        name: 'blog-manage',
        component: () => import('@/views/BlogManage.vue'),
        meta: { title: '博客管理' }
      },
      {
        path: 'shares',
        name: 'shares',
        component: () => import('@/views/SharesView.vue'),
        meta: { title: '分享' }
      },
      {
        path: 'settings',
        name: 'settings',
        component: () => import('@/views/SettingsView.vue'),
        meta: { title: '设置' }
      },
      {
        path: 'dev',
        name: 'dev-console',
        component: () => import('@/views/DeveloperConsoleView.vue'),
        meta: { title: '开发者中心' }
      },
      {
        path: 'admin/market-review',
        name: 'admin-market-review',
        component: () => import('@/views/AdminMarketReviewView.vue'),
        meta: { title: '市场审核' }
      },
      {
        path: 'impex',
        name: 'impex',
        redirect: { path: '/settings', query: { tab: 'data' } }, // 导入导出已并入设置页「数据」标签
        meta: { title: '导入导出' }
      }
    ]
  }
]

const router = createRouter({
  history: createWebHashHistory(),
  routes
})

// 登录守卫（安全加固）：除公开页与登录页外，未登录一律跳登录页
router.beforeEach((to) => {
  if (to.meta?.public || to.name === 'login') return true
  if (to.name === 'public-home' || to.path === '/blog' || to.path.startsWith('/p/') || to.path.startsWith('/post/')) return true
  if (!isAuthed()) {
    return { path: '/desk', query: { next: to.fullPath } }
  }
  return true
})

router.afterEach((to) => {
  document.title = to.meta?.title ? `${to.meta.title} · 爱库录` : '爱库录'
})

export default router
