<template>
  <div class="store-shell">
    <header class="store-bar">
      <RouterLink to="/store" class="brand">
        <AikIcon name="cart" :size="18" />
        <span>{{ t('store.title') }}</span>
      </RouterLink>
      <nav class="store-nav">
        <RouterLink to="/store" active-class="on">{{ t('store.catalog') }}</RouterLink>
      </nav>
      <div class="spacer"></div>
      <RouterLink to="/store/cart" class="cart-link" :title="t('store.cart')">
        <AikIcon name="cart" :size="20" />
        <span v-if="cartState.count > 0" class="badge">{{ cartState.count }}</span>
      </RouterLink>
      <RouterLink to="/desk" class="console-link">{{ t('store.goConsole') }}</RouterLink>
    </header>
    <main class="store-main">
      <router-view />
    </main>
    <footer class="store-foot">
      <span>·</span>
      <RouterLink to="/blog">{{ t('store.back') }}</RouterLink>
    </footer>
  </div>
</template>

<script setup>
import { onMounted } from 'vue'
import AikIcon from '@/components/AikIcon.vue'
import { t } from '@/i18n'
import { cartState, refreshCart } from '@/stores/storeCart'

onMounted(refreshCart)
</script>

<style scoped>
.store-shell {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--bg, #f6f7f9);
  color: var(--text, #1f2329);
}
.store-bar {
  display: flex;
  align-items: center;
  gap: 18px;
  padding: 12px 24px;
  background: var(--surface, #fff);
  border-bottom: 1px solid var(--border, #e6e8eb);
  position: sticky;
  top: 0;
  z-index: 10;
}
.brand {
  display: flex;
  align-items: center;
  gap: 8px;
  font-weight: 700;
  font-size: 17px;
  color: var(--text, #1f2329);
  text-decoration: none;
}
.store-nav {
  display: flex;
  gap: 14px;
}
.store-nav a {
  color: var(--text-2, #5b6168);
  text-decoration: none;
  font-size: 14px;
  padding: 4px 8px;
  border-radius: 6px;
}
.store-nav a.on {
  color: var(--accent, #2f6bff);
  background: color-mix(in srgb, var(--accent, #2f6bff) 12%, transparent);
}
.spacer { flex: 1; }
.cart-link {
  position: relative;
  color: var(--text, #1f2329);
  display: inline-flex;
  align-items: center;
}
.cart-link .badge {
  position: absolute;
  top: -6px;
  right: -8px;
  background: #e8453c;
  color: #fff;
  font-size: 11px;
  line-height: 1;
  padding: 2px 5px;
  border-radius: 10px;
}
.console-link {
  font-size: 13px;
  color: var(--text-2, #5b6168);
  text-decoration: none;
  border: 1px solid var(--border, #e6e8eb);
  padding: 5px 12px;
  border-radius: 6px;
}
.store-main {
  flex: 1;
  width: 100%;
  max-width: 1080px;
  margin: 0 auto;
  padding: 24px;
  box-sizing: border-box;
}
.store-foot {
  padding: 18px;
  text-align: center;
  color: var(--text-3, #8a9099);
  font-size: 12px;
}
.store-foot a { color: var(--text-2, #5b6168); text-decoration: none; }
</style>
