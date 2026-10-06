<template>
  <div class="landing">
    <!-- 顶栏 -->
    <header class="lp-bar">
      <a class="lp-brand" href="#/">
        <span class="lp-logo">{{  $t('录')  }}</span>
        <span class="lp-name">{{  $t('爱库录')  }}</span>
        <span class="lp-sub">AiKlog</span>
      </a>
      <nav class="lp-nav">
        <a class="lp-link" href="#/blog?view=public">{{  $t('公开博客')  }}</a>
        <GuestEntry variant="bar" />
      </nav>
    </header>

    <!-- Hero -->
    <section class="lp-hero">
      <div class="lp-badge">{{  $t('自部署 · 数据自有')  }}</div>
      <h1 class="lp-title">{{  $t('爱库录 AiKlog')  }}</h1>
      <p class="lp-tagline">{{  $t('自部署 AI 知识库博客系统')  }}</p>
      <p class="lp-desc">
        {{  $t('目录即站点，文件即文章。一套系统，把你的笔记、文档与文章变成可被搜索、可被分享、可被 AI 理解的知识库与博客——内容与服务都运行在你自己的服务器上。')  }}
      </p>
      <div class="lp-cta">
        <a class="lp-btn lp-btn--primary" href="/app#/blog?view=public" rel="noopener">{{  $t('浏览公开博客')  }}</a>
        <a class="lp-btn" href="#/desk" rel="noopener">{{  $t('进入控制台')  }}</a>
      </div>
    </section>

    <!-- 核心能力 -->
    <section class="lp-section">
      <h2 class="lp-sec-title">{{  $t('核心能力')  }}</h2>
      <p class="lp-sec-sub">{{  $t('一套自部署系统，覆盖从内容沉淀、AI 整理到对外分发的完整闭环。')  }}</p>
      <div class="lp-grid">
        <article class="lp-card">
          <span class="lp-ico">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 3l7 3v5c0 4.2-2.8 7.6-7 9-4.2-1.4-7-4.8-7-9V6l7-3z" />
              <path d="M9.2 12l2 2 3.6-3.8" />
            </svg>
          </span>
          <h3>{{  $t('自部署 · 数据自有')  }}</h3>
          <p>{{  $t('你的内容、你的服务器。不绑定任何平台，不担心停服与数据出走，完全掌控自己的知识资产与访问规则。')  }}</p>
        </article>

        <article class="lp-card">
          <span class="lp-ico">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
              <path d="M4 5.5A1.5 1.5 0 015.5 4H11v16H5.5A1.5 1.5 0 014 18.5z" />
              <path d="M20 5.5A1.5 1.5 0 0118.5 4H13v16h5.5A1.5 1.5 0 0120 18.5z" />
              <path d="M8.5 8.5h-1M15.5 8.5h-1M8.5 12h-1M15.5 12h-1" />
            </svg>
          </span>
          <h3>{{  $t('AI 知识库 + 博客引擎')  }}</h3>
          <p>{{  $t('写文件即发文，AI 辅助整理、摘要与发布。同一份内容，对内是知识库，对外是博客，不必维护两套。')  }}</p>
        </article>

        <article class="lp-card">
          <span class="lp-ico">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="9" />
              <path d="M3 12h18M12 3c2.8 2.4 4.3 5.6 4.3 9s-1.5 6.6-4.3 9c-2.8-2.4-4.3-5.6-4.3-9s1.5-6.6 4.3-9z" />
            </svg>
          </span>
          <h3>{{  $t('双轨渲染 · SEO 友好')  }}</h3>
          <p>{{  $t('同一 URL，爬虫看到服务端渲染的纯 HTML，读者看到交互式单页应用。搜索引擎收录与读者体验兼得。')  }}</p>
        </article>

        <article class="lp-card">
          <span class="lp-ico">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
              <rect x="3.5" y="3.5" width="7" height="7" rx="1.4" />
              <rect x="13.5" y="3.5" width="7" height="7" rx="1.4" />
              <rect x="3.5" y="13.5" width="7" height="7" rx="1.4" />
              <rect x="13.5" y="13.5" width="7" height="7" rx="1.4" />
            </svg>
          </span>
          <h3>{{  $t('应用中心 · 按需扩展')  }}</h3>
          <p>{{  $t('主题与插件的在线市场。外观换肤、功能增强都能像安装应用一样完成，按站点需要分发与解锁。')  }}</p>
        </article>

        <article class="lp-card">
          <span class="lp-ico">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="8.5" cy="8" r="3" />
              <path d="M3 19a5.5 5.5 0 0 1 11 0" />
              <circle cx="17" cy="9" r="2.4" />
              <path d="M14.5 19a4.5 4.5 0 0 1 6 0" />
            </svg>
          </span>
          <h3>{{  $t('同构多用户 · 开箱协作')  }}</h3>
          <p>{{  $t('系统自带注册与登录，邮箱验证码 + IP 限流开箱可用。一个实例即可服务多个真实用户，不必外接身份系统。')  }}</p>
        </article>

        <article class="lp-card">
          <span class="lp-ico">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 3a9 9 0 1 0 0 18c1.1 0 1.7-1 1.4-2-.3-1 .3-2 1.4-2H17a4 4 0 0 0 4-4c0-5-4-8-9-8z" />
              <circle cx="7.5" cy="11" r="1" />
              <circle cx="11" cy="7.5" r="1" />
              <circle cx="15.5" cy="8.5" r="1" />
            </svg>
          </span>
          <h3>{{  $t('多主题 · 免编译热投放')  }}</h3>
          <p>{{  $t('博客主题支持服务端渲染免编译热投放——换肤、调版式不必重新构建发布，上传即用、即时生效。')  }}</p>
        </article>

        <article class="lp-card">
          <span class="lp-ico">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
              <path d="M12 3l8 4-8 4-8-4 8-4z" />
              <path d="M4 11l8 4 8-4" />
              <path d="M4 15l8 4 8-4" />
            </svg>
          </span>
          <h3>{{  $t('多源采集 · 一键导入')  }}</h3>
          <p>{{  $t('WebDAV、Org 笔记与标准化源包一键导入；采集、订阅、摘要、收件箱等核心模块已全量接入。')  }}</p>
        </article>

        <article class="lp-card">
          <span class="lp-ico">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
              <rect x="4" y="4" width="16" height="16" rx="3" />
              <path d="M9 9h6v6H9z" />
              <path d="M9 2.2v1.8M15 2.2v1.8M9 20v1.8M15 20v1.8M2.2 9h1.8M2.2 15h1.8M20 9h1.8M20 15h1.8" />
            </svg>
          </span>
          <h3>{{  $t('AI 工具矩阵 · 智能创作')  }}</h3>
          <p>{{  $t('内置一组 AI 工具——智能探路建源、网页剪藏、内容组织与打标等，由统一能力开关门控，可按站点按需启用。')  }}</p>
        </article>

        <article class="lp-card">
          <span class="lp-ico">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="4.5" r="2.2" />
              <circle cx="5.5" cy="15" r="2.2" />
              <circle cx="18.5" cy="15" r="2.2" />
              <path d="M12 6.7v4.3M12 11h-6.5v1.8M12 11h6.5v1.8" />
            </svg>
          </span>
          <h3>{{  $t('组织树 · 团队协作')  }}</h3>
          <p>{{  $t('组织树、部门空间、成员任职与文件移交流转一应俱全。内容按组织节点聚合与授权，适合团队共同维护知识库。')  }}</p>
        </article>

        <article class="lp-card">
          <span class="lp-ico">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">
              <rect x="3" y="4" width="13" height="9" rx="1.6" />
              <rect x="8" y="11" width="13" height="9" rx="1.6" />
            </svg>
          </span>
          <h3>{{  $t('一套多站 · 多租户')  }}</h3>
          <p>{{  $t('同一套自部署实例承载多个相互独立的站点 / 租户：内容、分类、主题与站点设置按站点隔离。自定义域名、子域名或子目录三种接入方式任选，矩阵站群一站管控。')  }}</p>
        </article>
      </div>
    </section>

    <!-- 页脚 -->
    <footer class="lp-foot">
      <span>{{  $t('爱库录 AiKlog · 自部署 AI 知识库博客系统')  }}</span>
      <!-- SSR 静态版入口：专供搜索引擎收录（读者入口见右上角/主按钮的 SPA） -->
      <a href="/blog" target="_blank" rel="noopener">{{  $t('公开博客（静态版）')  }}</a>
    </footer>
  </div>
</template>

<script setup>
import GuestEntry from '@/components/GuestEntry.vue'
</script>

<style scoped>
.landing {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--app-bg, #f5f6f8);
  color: var(--text, #16182b);
}

/* 顶栏 */
.lp-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  height: 60px;
  background: var(--surface, #fff);
  border-bottom: 1px solid var(--border, #e6e8ee);
  position: sticky;
  top: 0;
  z-index: 10;
}

.lp-brand {
  display: flex;
  align-items: center;
  gap: 10px;
  text-decoration: none;
}

.lp-logo {
  width: 30px;
  height: 30px;
  border-radius: 9px;
  background: linear-gradient(135deg, var(--primary, #4f46e5), #6366f1);
  color: #fff;
  font-size: 13px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  letter-spacing: 0.5px;
}

.lp-name {
  font-weight: 600;
  font-size: 16px;
  color: var(--text);
}

.lp-sub {
  font-size: 12px;
  color: var(--text-3, #939ab0);
}

.lp-nav {
  display: flex;
  align-items: center;
  gap: 14px;
}

.lp-link {
  font-size: 14px;
  color: var(--text-2, #5b6079);
  text-decoration: none;
}

.lp-link:hover {
  color: var(--primary, #4f46e5);
}

/* Hero */
.lp-hero {
  max-width: 880px;
  margin: 0 auto;
  padding: 84px 24px 56px;
  text-align: center;
}

.lp-badge {
  display: inline-block;
  padding: 4px 12px;
  border-radius: 999px;
  background: var(--primary-soft, #eef2ff);
  color: var(--primary, #4f46e5);
  font-size: 13px;
  font-weight: 600;
  margin-bottom: 22px;
}

.lp-title {
  margin: 0;
  font-size: 44px;
  font-weight: 800;
  letter-spacing: -0.5px;
  line-height: 1.15;
}

.lp-tagline {
  margin: 14px 0 0;
  font-size: 19px;
  font-weight: 600;
  color: var(--text-2, #5b6079);
}

.lp-desc {
  margin: 18px auto 0;
  max-width: 620px;
  font-size: 15px;
  line-height: 1.8;
  color: var(--text-2, #5b6079);
}

.lp-cta {
  margin-top: 30px;
  display: flex;
  gap: 14px;
  justify-content: center;
  flex-wrap: wrap;
}

.lp-btn {
  display: inline-flex;
  align-items: center;
  height: 44px;
  padding: 0 26px;
  border-radius: 10px;
  border: 1px solid var(--border, #e6e8ee);
  background: var(--surface, #fff);
  color: var(--text, #16182b);
  font-size: 15px;
  font-weight: 600;
  text-decoration: none;
  transition: transform 0.12s ease, box-shadow 0.12s ease, border-color 0.12s ease;
}

.lp-btn:hover {
  transform: translateY(-1px);
  box-shadow: var(--shadow, 0 4px 14px rgba(22, 24, 43, 0.08));
  border-color: var(--primary, #4f46e5);
}

.lp-btn--primary {
  background: var(--primary, #4f46e5);
  border-color: var(--primary, #4f46e5);
  color: #fff;
}

.lp-btn--primary:hover {
  background: #4338ca;
  border-color: #4338ca;
}

/* 区块 */
.lp-section {
  max-width: 1080px;
  margin: 0 auto;
  padding: 40px 24px;
}

.lp-sec-title {
  margin: 0 0 28px;
  text-align: center;
  font-size: 26px;
  font-weight: 700;
}

.lp-sec-sub {
  margin: -14px 0 28px;
  text-align: center;
  font-size: 15px;
  color: var(--text-2, #5b6079);
}

.lp-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 18px;
}

.lp-card {
  background: var(--surface, #fff);
  border: 1px solid var(--border, #e6e8ee);
  border-radius: var(--radius-lg, 16px);
  padding: 26px 22px;
  transition: transform 0.14s ease, box-shadow 0.14s ease, border-color 0.14s ease;
}

.lp-card:hover {
  transform: translateY(-3px);
  box-shadow: var(--shadow-lg, 0 12px 30px rgba(22, 24, 43, 0.12));
  border-color: color-mix(in srgb, var(--primary, #4f46e5) 40%, var(--border, #e6e8ee));
}

.lp-ico {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 46px;
  height: 46px;
  border-radius: 12px;
  background: var(--primary-soft, #eef2ff);
  color: var(--primary, #4f46e5);
  margin-bottom: 16px;
}

.lp-ico svg {
  width: 24px;
  height: 24px;
}

.lp-card h3 {
  margin: 0 0 10px;
  font-size: 17px;
  font-weight: 700;
  color: var(--text, #16182b);
}

.lp-card p {
  margin: 0;
  font-size: 14px;
  line-height: 1.7;
  color: var(--text-2, #5b6079);
}

.lp-card--soon {
  position: relative;
  border-style: dashed;
  border-color: color-mix(in srgb, var(--primary, #4f46e5) 35%, var(--border, #e6e8ee));
}

.lp-soon {
  position: absolute;
  top: 16px;
  right: 16px;
  padding: 3px 10px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.3px;
  color: var(--primary, #4f46e5);
  background: var(--primary-soft, #eef2ff);
}

/* 页脚 */
.lp-foot {
  margin-top: auto;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 22px 24px;
  font-size: 13px;
  color: var(--text-3, #939ab0);
  background: var(--surface, #fff);
  border-top: 1px solid var(--border, #e6e8ee);
}

.lp-foot a {
  color: var(--primary, #4f46e5);
  text-decoration: none;
}

@media (max-width: 980px) {
  .lp-grid {
    grid-template-columns: repeat(2, 1fr);
  }
}

@media (max-width: 767px) {
  .lp-hero {
    padding: 56px 18px 40px;
  }
  .lp-title {
    font-size: 34px;
  }
  .lp-tagline {
    font-size: 17px;
  }
  .lp-grid {
    grid-template-columns: 1fr;
  }
  .lp-nav .lp-link {
    display: none;
  }
}
</style>
