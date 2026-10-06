<template>
  <div class="settings-view">
    <div class="sv-head">
      <div>
        <h2>{{  $t('设置')  }}</h2>
        <p class="sv-sub">{{  $t('站点 · AI · 消息 · 应用 · 数据')  }}</p>
      </div>
    </div>

    <!-- 顶部分组标签（?tab= 深链可直达） -->
    <nav class="sv-nav-tabs">
      <button v-for="t in navTabs" :key="t.key" class="sv-tab-btn" :class="{ on: tab === t.key }" @click="switchTab(t.key)">
        {{  t.label  }}<span v-if="t.count" class="sv-tab-count">{{  t.count  }}</span>
      </button>
    </nav>

    <!-- 界面语言（框架级 i18n，B22 移植上游） -->
    <section class="sv-card">
      <div class="sv-card-h">
        <h3>{{  t('settings.language')  }}</h3>
        <p>{{  t('settings.languageDesc')  }}</p>
      </div>
      <div class="sv-cap">
        <div class="sv-cap-info">
          <span class="sv-cap-name">{{  t('settings.language')  }}</span>
          <span class="sv-badge on">{{  locale  }}</span>
        </div>
        <div class="sv-cap-actions">
          <select :value="locale" @change="onLocaleChange($event)">
            <option v-for="l in LOCALES" :key="l.value" :value="l.value">{{  t(l.labelKey)  }}</option>
          </select>
        </div>
      </div>
    </section>

    <!-- 站点（多用户：开放注册开关） -->
    <section v-if="multiUser" v-show="tab === 'site'" class="sv-card">
      <div class="sv-card-h">
        <h3>{{  $t('站点')  }}</h3>
        <p>{{  $t('开放注册开关：关闭后新用户无法自助注册（已注册用户不受影响）——分发场景建议开启，私有部署建议关闭')  }}</p>
      </div>
      <div class="sv-cap">
        <div class="sv-cap-info">
          <span class="sv-cap-name">{{  $t('开放注册')  }}</span>
          <span class="sv-badge" :class="{ on: registrationOpen }">{{  registrationOpen ? $t('已开启') : $t('已关闭')  }}</span>
        </div>
        <div class="sv-cap-actions">
          <button class="btn btn-sm" :disabled="saving" @click="toggleRegistration">
            {{  registrationOpen ? $t('关闭注册') : $t('开启注册')  }}
          </button>
        </div>
      </div>
    </section>

    <!-- AI 模型（能力级 provider 绑定） -->
    <section v-show="tab === 'ai'" class="sv-card">
      <div class="sv-card-h">
        <h3>{{  $t('AI 模型')  }}</h3>
        <p>{{  $t('各能力独立绑定 provider 与模型，切换即时生效；「自备模型」支持任意 OpenAI 兼容接口（含本地 Ollama）')  }}</p>
      </div>
      <div v-for="cap in caps" :key="cap.name" class="sv-cap">
        <div class="sv-cap-info">
          <span class="sv-cap-name">{{  cap.label  }}</span>
          <span class="sv-badge" :class="{ on: !!cap.active }">{{  cap.active || $t('未配置')  }}</span>
          <span v-if="cap.model" class="sv-cap-model">{{  cap.model  }}</span>
          <span v-if="cap.custom?.configured" class="sv-custom-tag">{{  $t('自备')  }}</span>
        </div>
        <div class="sv-cap-actions">
          <select :value="cap.active" @change="onSwitch(cap.name, $event)">
            <option :value="cap.active" disabled v-if="cap.active">{{  cap.active  }}</option>
            <option value="">{{  $t('选择 provider…')  }}</option>
            <option v-for="o in cap.options" :key="o" :value="o" :disabled="o === cap.active">{{  o  }}</option>
          </select>
          <button class="btn btn-sm" @click="openCustom(cap.name)">{{  customOpen === cap.name ? $t('收起') : $t('自备模型')  }}</button>
        </div>
        <div v-if="customOpen === cap.name" class="sv-custom">
          <input v-model="customForm.endpoint" :placeholder="$t('API 地址（OpenAI 兼容，如 http://127.0.0.1:11434/v1）')" />
          <input v-model="customForm.model" :placeholder="$t('模型名（如 bge-m3 / glm-5.2）')" />
          <input v-model="customForm.apiKey" type="password" :placeholder="$t('API Key（本地服务可留空）')" />
          <div class="sv-custom-actions">
            <button class="btn btn-sm btn-primary" :disabled="saving" @click="saveCustom(cap.name)">{{  $t('保存')  }}</button>
            <button v-if="cap.custom?.configured" class="btn btn-sm" @click="clearCustom(cap.name)">{{  $t('清除自备')  }}</button>
          </div>
        </div>
        <!-- 语音合成默认参数（后台可配，即时生效）：音色/格式/风格 + 试听 -->
        <div v-if="cap.name === 'tts'" class="sv-voice">
          <div class="sv-voice-grid">
            <div class="sv-voice-field">
              <label>{{  $t('默认音色')  }}</label>
              <input v-model="voiceForm.voice" list="ttsVoiceCands" :placeholder="$t('留空 = provider 默认（如 茉莉 / 冰糖 / Mia）')" />
              <datalist id="ttsVoiceCands">
                <option v-for="v in (cap.voice?.cands || [])" :key="v" :value="v" />
              </datalist>
            </div>
            <div class="sv-voice-field">
              <label>{{  $t('音频格式')  }}</label>
              <select v-model="voiceForm.format" class="sv-voice-select">
                <option value="">{{  $t('默认（mp3）')  }}</option>
                <option value="mp3">mp3</option>
                <option value="wav">wav</option>
                <option value="pcm16">pcm16</option>
              </select>
            </div>
            <div class="sv-voice-field">
              <label>{{  $t('风格指令')  }}</label>
              <input v-model="voiceForm.style" :placeholder="$t('可选，如：温柔自然 / 播报感')" />
            </div>
          </div>
          <div class="sv-voice-actions">
            <button class="btn btn-sm btn-primary" :disabled="saving" @click="saveVoice">{{  $t('保存语音默认值')  }}</button>
            <button class="btn btn-sm" :disabled="listening" @click="testVoice">{{  listening ? $t('合成中…') : $t('试听')  }}</button>
            <audio v-if="voiceAudio" :src="voiceAudio" controls class="sv-voice-audio"></audio>
          </div>
        </div>
      </div>
    </section>

    <!-- 模型提供方管理（可任意增删改 provider + token 池） -->
    <section v-show="tab === 'ai'" class="sv-card">
      <div class="sv-card-h">
        <h3>{{ t('模型提供方') }}</h3>
        <p>{{ t('管理所有大模型提供方：添加任意 OpenAI 兼容服务商，配置 endpoint / 默认模型 / 能力 / 密钥池 / 音色候选；修改即时生效，无需重启') }}</p>
      </div>
      <div class="sv-providers">
        <div v-for="p in providers" :key="p.name" class="sv-prov">
          <div class="sv-prov-main">
            <div class="sv-prov-name">
              {{ p.name }}
              <span v-if="p.builtin" class="sv-badge on">{{ t('内置') }}</span>
            </div>
            <div class="sv-prov-endpoint">{{ p.endpoint || '—' }}</div>
            <div class="sv-prov-meta">
              <span>{{ t('能力') }}: {{ (p.caps || []).join(' / ') || '—' }}</span>
              <span>{{ t('密钥') }}: {{ p.keys_count }}</span>
              <span v-if="p.model">{{ t('默认模型') }}: {{ p.model }}</span>
            </div>
          </div>
          <div class="sv-prov-actions">
            <button class="btn btn-sm" :disabled="testing === p.name" @click="testProvider(p)">{{ testing === p.name ? t('测试中…') : t('测试') }}</button>
            <button class="btn btn-sm" @click="openProviderEdit(p)">{{ t('编辑') }}</button>
            <button class="btn btn-sm btn-danger" :disabled="p.builtin" @click="deleteProvider(p)">{{ t('删除') }}</button>
          </div>
        </div>
        <div v-if="!providers.length" class="sv-empty">{{ t('暂无提供方记录') }}</div>
      </div>
      <div class="sv-prov-add">
        <button class="btn btn-primary" @click="openProviderAdd">{{ t('添加提供方') }}</button>
      </div>

      <!-- 新增/编辑弹窗 -->
      <div v-if="provModal.open" class="sv-modal-mask" @click.self="closeProviderModal">
        <div class="sv-modal">
          <div class="sv-modal-h">
            <h4>{{ provModal.editing ? t('编辑提供方') : t('添加提供方') }}</h4>
            <button class="sv-modal-x" @click="closeProviderModal">×</button>
          </div>
          <div class="sv-modal-body">
            <label>{{ t('标识名（唯一，不可含空格）') }}</label>
            <input v-model="provForm.name" :disabled="provModal.editing" :placeholder="t('如 deepseek / openai / 本地 ollama')" />
            <label>{{ t('API 地址（OpenAI 兼容，可带或不带 /v1）') }}</label>
            <input v-model="provForm.endpoint" :placeholder="t('如 https://api.openai.com/v1')" />
            <label>{{ t('默认模型（未逐能力指定时使用）') }}</label>
            <input v-model="provForm.model" :placeholder="t('如 gpt-4o-mini')" />
            <label>{{ t('支持的能力（声明本提供方提供的 AI 能力）') }}</label>
            <div class="sv-caps-pick">
              <label v-for="c in providerCapList" :key="c.key" class="sv-cap-check">
                <input type="checkbox" :value="c.key" v-model="provForm.caps" /> {{ c.label }}
              </label>
            </div>
            <div v-for="c in provForm.caps" :key="'mc-' + c" class="sv-cap-row">
              <span class="sv-cap-row-name">{{ capLabel(c) }}</span>
              <input v-model="provForm.models[c]" :placeholder="t('该能力专用模型（可空）')" />
              <input v-model="provForm.capCandsText[c]" :placeholder="t('候选模型，逗号分隔（可空）')" />
            </div>
            <label>{{ t('密钥池（每行一个 API Key，轮替调用；本地服务可留空）') }}</label>
            <textarea v-model="provForm.keysText" rows="4" :placeholder="t('sk-... 每行一个；留空 = 不带鉴权（如本地 Ollama）')"></textarea>
            <label>{{ t('语音音色候选（逗号分隔，可空）') }}</label>
            <input v-model="provForm.voiceCandsText" :placeholder="t('如 茉莉,冰糖,Mia,Chloe')" />
            <label>{{ t('备注') }}</label>
            <input v-model="provForm.note" :placeholder="t('可选说明')" />
          </div>
          <div class="sv-modal-foot">
            <button class="btn" @click="closeProviderModal">{{ t('取消') }}</button>
            <button class="btn btn-primary" :disabled="saving" @click="saveProvider">{{ saving ? t('保存中…') : t('保存') }}</button>
          </div>
        </div>
      </div>
    </section>

    <!-- IM 对接 -->
    <section v-show="tab === 'im'" class="sv-card">
      <div class="sv-card-h">
        <h3>{{  $t('IM 对接')  }}</h3>
        <p>{{  $t('对接 Telegram / 企业微信：发消息即可发博客。命令：')  }}<code>{{  $t('/post 正文')  }}</code> {{  $t('发布、')  }}<code>{{  $t('/draft 正文')  }}</code> {{  $t('存草稿、')  }}<code>{{  $t('/search 关键词')  }}</code> {{  $t('检索、')  }}<code>/list</code> {{  $t('最近文章、')  }}<code>/help</code> {{  $t('帮助')  }}</p>
      </div>

      <!-- Telegram -->
      <div class="sv-im">
        <div class="sv-im-block-h">
          <span class="sv-im-plt">Telegram</span>
          <span class="sv-badge" :class="{ on: imTgOn }">{{  imTgOn ? $t('已启用') : $t('未启用')  }}</span>
        </div>
        <div class="sv-im-state-row">
          <span v-if="imTgOn" class="sv-muted">{{  $t('在 Telegram 搜索你的 Bot，发送')  }} <code>{{  $t('/post 正文')  }}</code> {{  $t('即可发博客')  }}</span>
          <span v-else class="sv-muted">{{  $t('填写 Bot Token 并保存后启用')  }}</span>
        </div>
        <label>Bot Token</label>
        <input v-model="tg.botToken" type="password" placeholder="123456789:AAH..." />
        <label>{{  $t('允许的 Chat ID（逗号分隔，留空 = 全部）')  }}</label>
        <input v-model="tg.allowedChats" :placeholder="$t('如 123456789, 987654321')" />
        <div class="sv-im-actions">
          <button class="btn btn-sm btn-primary" :disabled="saving" @click="saveIM">{{  $t('保存配置')  }}</button>
          <span class="sv-muted">Webhook：<code>{{  tgHook  }}</code></span>
        </div>
      </div>

      <!-- 企业微信 -->
      <div class="sv-im">
        <div class="sv-im-block-h">
          <span class="sv-im-plt">{{  $t('企业微信')  }}</span>
          <span class="sv-badge" :class="{ on: imWcOn }">{{  imWcOn ? $t('已启用') : $t('未启用')  }}</span>
        </div>
        <div class="sv-im-state-row">
          <span v-if="imWcOn" class="sv-muted">{{  $t('在企业微信应用/群机器人收发消息，发')  }} <code>{{  $t('/post 正文')  }}</code> {{  $t('即可发博客')  }}</span>
          <span v-else class="sv-muted">{{  $t('填企业微信自建应用信息并保存后启用（群机器人 Webhook 或自建应用二选一）')  }}</span>
        </div>
        <label>{{  $t('群机器人 Webhook 地址（任选其一）')  }}</label>
        <input v-model="wc.webhookUrl" placeholder="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=..." />
        <label>{{  $t('自建应用 CorpID')  }}</label>
        <input v-model="wc.corpId" :placeholder="$t('企业 ID')" />
        <label>AgentID</label>
        <input v-model="wc.agentId" :placeholder="$t('应用 AgentId')" />
        <label>Secret</label>
        <input v-model="wc.secret" type="password" :placeholder="$t('应用 Secret')" />
        <label>{{  $t('回调 Token')  }}</label>
        <input v-model="wc.token" :placeholder="$t('接收消息回调 Token')" />
        <label>EncodingAESKey</label>
        <input v-model="wc.encodingAesKey" :placeholder="$t('接收消息回调 EncodingAESKey')" />
        <div class="sv-im-actions">
          <button class="btn btn-sm btn-primary" :disabled="saving" @click="saveWeCom">{{  $t('保存配置')  }}</button>
          <span class="sv-muted">{{  $t('回调地址：')  }}<code>{{  wcHook  }}</code></span>
        </div>
      </div>
    </section>

    <!-- 用量管控与计费（平台模型） -->
    <section v-show="tab === 'ai'" class="sv-card">
      <div class="sv-card-h">
        <h3>{{ $t('用量管控与计费') }}</h3>
        <p>{{ $t('平台模型用量管控：登录用户与匿名公开访客（读者问答/语音朗读）分别限额；开启增值计费后平台模型按 token 扣余额，余额不足拦截。自备模型（custom）不计量不限量') }}</p>
      </div>
      <div class="sv-quota-grid">
        <label>{{ $t('登录用户·每日调用上限') }}
          <input v-model.number="quota.user_daily_calls" type="number" min="0" />
        </label>
        <label>{{ $t('登录用户·每日 token 上限') }}
          <input v-model.number="quota.user_daily_tokens" type="number" min="0" />
        </label>
        <label>{{ $t('公开访客·每日调用上限') }}
          <input v-model.number="quota.public_daily_calls" type="number" min="0" />
        </label>
        <label>{{ $t('公开访客·每日 token 上限') }}
          <input v-model.number="quota.public_daily_tokens" type="number" min="0" />
        </label>
      </div>
      <div class="sv-quota-check">
        <label>
          <input v-model="quota.billing_enabled" type="checkbox" />
          {{ $t('开启增值计费（按 token 扣余额，余额不足拦截平台模型调用）') }}
        </label>
        <button class="btn btn-sm btn-primary" :disabled="saving" @click="saveQuotaPolicy">{{ $t('保存管控策略') }}</button>
        <span class="sv-muted">{{ $t('0 = 不限；保存即时生效，无需重启') }}</span>
      </div>
      <div class="sv-balance">
        <h4>{{ $t('今日用量（按主体）') }}</h4>
        <div v-if="!quotaToday.length" class="sv-muted">{{ $t('今日暂无平台模型调用') }}</div>
        <div v-for="u in quotaToday" :key="u.subject" class="sv-balance-row">
          <code>{{ u.subject === 'public' ? $t('公开访客（匿名）') : u.subject }}</code>
          <span>{{ u.calls }} {{ $t('次') }}</span>
          <span>{{ fmtTokens(u.tokens) }} token</span>
        </div>
      </div>
      <div class="sv-balance">
        <h4>{{ $t('token 余额（增值包）') }}</h4>
        <div v-if="!balances.length" class="sv-muted">{{ $t('暂无余额记录；开启计费后可在下方为用户/公开池充值') }}</div>
        <div v-for="b in balances" :key="b.subject" class="sv-balance-row">
          <code>{{ b.subject === 'public' ? $t('公开访客（匿名）') : b.subject }}</code>
          <span class="sv-balance-num">{{ fmtTokens(b.balance) }}</span>
          <button class="btn btn-sm" @click="openGrant(b.subject)">{{ $t('充值') }}</button>
        </div>
        <div class="sv-grant">
          <input v-model="grantForm.subject" :placeholder="$t('主体：user:<uid> 或 public')" />
          <input v-model.number="grantForm.delta" type="number" :placeholder="$t('增减 token（正充负扣）')" />
          <input v-model="grantForm.reason" :placeholder="$t('备注（如：增值包 ¥10）')" />
          <button class="btn btn-sm btn-primary" :disabled="saving" @click="doGrant">{{ $t('执行') }}</button>
        </div>
      </div>
    </section>

    <!-- Agent 参数 -->
    <section v-show="tab === 'ai'" class="sv-card">
      <div class="sv-card-h">
        <h3>{{  $t('Agent 参数')  }}</h3>
        <p>{{  $t('工具轮数上限是单次对话最多调用工具的次数（免费/付费分级参考项）')  }}</p>
      </div>
      <div class="sv-agent">
        <label>{{  $t('工具轮数上限（1-32）')  }}</label>
        <input v-model.number="maxRounds" type="number" min="1" max="32" style="width: 120px" />
        <button class="btn btn-sm btn-primary" :disabled="saving" @click="saveMaxRounds">{{  $t('保存')  }}</button>
      </div>
    </section>

    <!-- 入库解读 -->
    <section v-show="tab === 'ai'" class="sv-card">
      <div class="sv-card-h">
        <h3>{{  $t('入库解读')  }}</h3>
        <p>{{  $t('文件入库后后台 AI 自动解读（摘要+标签+实体，进知识库/图谱）；按类型筛选可减少无谓 token 消耗')  }}</p>
      </div>
      <div class="sv-agent">
        <label>{{  $t('解读类型白名单（逗号分隔扩展名，如 .md,.docx,.pdf）')  }}</label>
        <input v-model="summarizeExts" :placeholder="$t('留空 = 全部文本类型都解读')" style="width: 320px" />
        <button class="btn btn-sm btn-primary" :disabled="saving" @click="saveSummarizeExts">{{  $t('保存')  }}</button>
        <p class="sv-muted" style="margin-top:6px">{{  $t('批量导入大批文件时建议先只开 .md 等高频类型；白名单变更即时生效，已解读文件不受影响')  }}</p>
      </div>
    </section>

    <!-- 已装应用（安装 zip / 启停 / 配置） -->
    <section v-show="tab === 'apps'" class="sv-card">
      <div class="sv-card-h">
        <h3>{{  $t('已装应用')  }}</h3>
        <p>{{  $t('本地管理已安装应用：安装 zip（内含')  }} <code>manifest.json</code>{{  $t('）、启用/禁用、卸载与配置。')  }}</p>
      </div>
      <div class="sv-app-install">
        <label class="btn btn-sm sv-up">
          <input type="file" accept=".zip,application/zip" @change="onInstallZip" />
          {{  $t('安装应用 zip')  }}
        </label>
        <span class="sv-muted" style="font-size:12px">{{  $t('协议见 docs/PLUGIN-MARKET.md')  }}</span>
      </div>
      <table class="plug-table" style="margin-top:12px">
        <thead><tr><th>{{  $t('应用')  }}</th><th>{{  $t('版本')  }}</th><th>{{  $t('挂载点')  }}</th><th>{{  $t('状态')  }}</th><th>{{  $t('操作')  }}</th></tr></thead>
        <tbody>
          <tr v-for="p in plugins" :key="'app-' + p.id">
            <td>
              <div class="sv-app-name">{{  p.name || p.id  }}</div>
              <code class="sv-app-id">{{  p.id  }}</code>
              <div v-if="p.description" class="sv-muted" style="font-size:12px">{{  p.description  }}</div>
            </td>
            <td>{{  p.version  }}</td>
            <td>{{  mountPointsText(p)  }}</td>
            <td><span class="sv-badge" :class="{ on: p.enabled }">{{  p.enabled ? $t('启用') : $t('禁用')  }}</span></td>
            <td>
              <button class="btn btn-sm" @click="togglePlugin(p)">{{  p.enabled ? $t('禁用') : $t('启用')  }}</button>
              <button class="btn btn-sm" style="margin-left:6px" @click="removePlugin(p)">{{  $t('卸载')  }}</button>
              <button class="btn btn-sm" style="margin-left:6px" @click="openConfig(p)">{{  $t('配置')  }}</button>
            </td>
          </tr>
        </tbody>
      </table>
      <!-- 插件动态设置（settings_schema → KV；应用中心安装的插件可在此配置，如 RSS 条目数） -->
      <div v-if="cfgId" class="sv-cfg">
        <div class="sv-cfg-h">
          <strong>{{  $t('配置：')  }}{{  cfgName  }}</strong>
          <button class="btn btn-sm" @click="closeConfig">{{  $t('关闭')  }}</button>
        </div>
        <div v-for="f in cfgSchema" :key="f.key" class="sv-cfg-row">
          <label>{{  f.label || f.key  }}<span class="sv-muted">（{{  f.key  }}）</span></label>
          <input v-if="f.type === 'number'" type="number" v-model="cfgValues[f.key]" class="input" />
          <input v-else-if="f.type === 'boolean'" type="checkbox" v-model="cfgValues[f.key]" />
          <input v-else type="text" v-model="cfgValues[f.key]" class="input" />
        </div>
        <button class="btn btn-sm btn-primary" :disabled="saving" @click="saveConfig">{{  $t('保存设置')  }}</button>
      </div>
    </section>

    <!-- 插件调试（协议 v1.1 可视化：manifest / 事件 / KV 抽样） -->
    <section v-show="tab === 'apps'" class="sv-card">
      <div class="sv-card-h">
        <h3>{{  $t('博客插件调试')  }}</h3>
        <p>{{  $t('已登记插件 manifest、可投递事件名、KV 数据抽样——插件开发者对照')  }} <code>{{  $t('docs/博客插件协议.md')  }}</code> {{  $t('排查渲染与数据')  }}</p>
      </div>
      <div class="sv-plug">
        <div v-if="plugins.length === 0" class="sv-muted">{{  $t('暂无插件登记（系统内置插件随博客空间初始化自动注册）')  }}</div>
        <table v-else class="plug-table">
          <thead><tr><th>id</th><th>{{  $t('版本')  }}</th><th>{{  $t('挂载点')  }}</th><th>{{  $t('权限')  }}</th><th>min_core</th><th>{{  $t('状态')  }}</th><th>{{  $t('KV 抽样')  }}</th></tr></thead>
          <tbody>
            <tr v-for="p in plugins" :key="p.id">
              <td><div class="sv-app-name">{{  p.name || p.id  }}</div><code class="sv-app-id">{{  p.id  }}</code></td>
              <td>{{  p.version  }}</td>
              <td>{{  (p.mount_points || []).join(' / ')  }}</td>
              <td>{{  (p.api_permissions || []).join(' / ') || '—'  }}</td>
              <td>{{  p.min_core_version || '0'  }}</td>
              <td><span class="sv-badge" :class="{ on: p.enabled }">{{  p.enabled ? $t('启用') : $t('禁用')  }}</span></td>
              <td>
                <code v-if="kvSamples[p.id]" class="kv-sample" :title="kvSamples[p.id].join('\n')">
                  {{  kvSamples[p.id].length  }} {{ $t('键：') }}{{  kvSamples[p.id].slice(0, 3).join(' · ')  }}{{  kvSamples[p.id].length > 3 ? ' …' : ''  }}
                </code>
                <span v-else class="sv-muted">{{  $t('空')  }}</span>
              </td>
            </tr>
          </tbody>
        </table>
        <div class="sv-events">
          <span class="sv-events-label">{{  $t('事件名（event:post 可投递对象）')  }}</span>
          <span v-for="ev in eventNames" :key="ev" class="ev-chip" @click="copyEvent(ev)" :title="$t('点击复制 {name}', { name: ev })">{{  ev  }}</span>
        </div>
      </div>
    </section>

    <!-- AI 用量（配额分层：日配额消耗 + 当日 Top 来源） -->
    <section v-show="tab === 'ai'" class="sv-card">
      <div class="sv-card-h">
        <h3>{{  $t('AI 用量')  }}</h3>
        <p>{{  $t('博客 AI 问答近 7 日消耗与当日来源 Top（配额键：')  }}<code>blog.ai_ask_*</code>{{  $t('，可在设置接口调整）')  }}</p>
      </div>
      <div class="sv-plug">
        <div v-if="aiUsageDaily.length === 0" class="sv-muted">{{  $t('近 7 日暂无 AI 问答用量')  }}</div>
        <table v-else class="plug-table">
          <thead><tr><th>{{  $t('日期')  }}</th><th>{{  $t('问答次数')  }}</th></tr></thead>
          <tbody>
            <tr v-for="d in aiUsageDaily" :key="d.day">
              <td>{{  d.day  }}</td>
              <td>{{  d.hits  }}</td>
            </tr>
          </tbody>
        </table>
        <div v-if="aiUsageTop.length" class="sv-events">
          <span class="sv-events-label">{{  $t('今日来源 Top（已脱敏）')  }}</span>
          <span v-for="t in aiUsageTop" :key="t.ident" class="ev-chip">{{  t.ident  }} · {{  t.hits  }} {{  $t('次')  }}</span>
        </div>
        <p class="sv-muted" style="font-size:12px">{{  $t('配额默认：游客 3 次/日 · 登录用户 10 次/日 · 管理员不限 · 全站日预算 200 次（超限当日降级）')  }}</p>
      </div>
    </section>

    <!-- 运维：能力模块门控（装配在启动阶段 → 改后需重启） -->
    <section v-show="tab === 'ops'" class="sv-card">
      <div class="sv-card-h">
        <h3>{{  $t('能力模块')  }}</h3>
        <p>
          {{ $t('内核模块的启用判定：站长配置 > 应用中心已装应用记录 > 内置默认开启。关闭后对应端点下线（404 = 能力未启用），数据保留。多数模块在进程启动阶段装配，改动') }}<b>{{  $t('需重启进程')  }}</b>{{  $t('才生效；运行期判定的模块（见各项右侧标记）')  }}<b>{{  $t('即时生效')  }}</b>。
        </p>
      </div>
      <div v-for="c in capStates" :key="c.name" class="sv-cap">
        <div class="sv-cap-info">
          <span class="sv-cap-name">{{  c.label  }}</span>
          <span class="sv-badge" :class="{ on: c.enabled }">{{  c.enabled ? $t('开') : $t('关')  }}</span>
          <span class="sv-badge sv-badge-muted">{{  capSourceText[c.source] || c.source  }}</span>
          <span class="sv-badge sv-badge-muted">{{  capApplyText[c.apply] || capApplyText.restart  }}</span>
          <span class="sv-cap-note">{{  c.note  }}</span>
        </div>
        <div class="sv-cap-actions">
          <button class="btn btn-sm" :disabled="saving || (c.configured && c.enabled)" @click="saveCap(c.name, 'true')">{{  $t('开启')  }}</button>
          <button class="btn btn-sm" :disabled="saving || (c.configured && !c.enabled)" @click="saveCap(c.name, 'false')">{{  $t('关闭')  }}</button>
          <button class="btn btn-sm" :disabled="saving || !c.configured" @click="saveCap(c.name, '')">{{  $t('跟随默认')  }}</button>
        </div>
      </div>
      <p class="sv-muted" style="margin-top: 8px">
        {{  $t('「跟随默认」= 清除站长配置，回到应用中心记录 / 内置默认的判定链。')  }}
      </p>
    </section>

    <!-- 运维：组织模块运行期子开关（即时生效） -->
    <section v-show="tab === 'ops'" class="sv-card">
      <div class="sv-card-h">
        <h3>{{  $t('组织与权限')  }}</h3>
        <p>{{  $t('组织模块的运行期子开关：总开关关闭时，组织树 / 部门空间 / 移交流一律视同关闭。保存即时生效，无需重启。')  }}</p>
      </div>
      <div v-if="!moduleOn('org')" class="sv-warn">
        {{  $t('组织模块当前未启用（见上方「能力模块 · 组织架构」）：需先开启并重启进程，下面的子开关才有落点。')  }}
      </div>
      <div v-for="g in orgGateList" :key="g.key" class="sv-cap">
        <div class="sv-cap-info">
          <span class="sv-cap-name">{{  g.label  }}</span>
          <span class="sv-badge" :class="{ on: orgGates[g.field] }">{{  orgGates[g.field] ? $t('开') : $t('关')  }}</span>
        </div>
        <div class="sv-cap-actions">
          <button
            class="btn btn-sm"
            :disabled="saving || orgGates[g.field] === true || (g.key !== 'org.enabled' && !orgGates.enabled)"
            @click="saveOrg(g.key, 'true')"
          >{{  $t('开启')  }}</button>
          <button class="btn btn-sm" :disabled="saving || orgGates[g.field] !== true" @click="saveOrg(g.key, 'false')">{{  $t('关闭')  }}</button>
        </div>
      </div>
      <p class="sv-muted" style="margin-top: 8px">
        {{  $t('子开关受总开关压制：总开关关闭时，子开关写不写都是关（值保留，总开关一开即恢复）。')  }}
      </p>
    </section>

    <!-- 运维：媒体派生资产（B16）—— 参数即时生效；清理后按新参数懒重生成 -->
    <section v-show="tab === 'ops'" class="sv-card">
      <div class="sv-card-h">
        <h3>{{  $t('媒体派生资产（缩略图）')  }}</h3>
        <p>
          {{  $t('图片/视频缩略图由 ffmpeg 就地生成并缓存为')  }}<b>{{  $t('派生对象')  }}</b>{{  $t('（存放于')  }}
          <code>{{ $t('spaces/<空间>/.derived/<文件>/') }}</code>{{  $t('，不进文件库、不占配额、不进公开列表与 sitemap）。 下面三个参数保存后')  }}<b>{{  $t('即时生效')  }}</b>{{  $t('；但')  }}<b>{{  $t('已生成的缩略图不会自动重做')  }}</b> {{  $t('—— 尺寸改了请点「清空派生缓存」，下次访问即按新参数重生成。')  }}
        </p>
      </div>

      <div v-if="!derivedReady" class="sv-warn">{{  derivedErr || $t('正在读取派生资产状态…')  }}</div>
      <template v-else>
        <div v-for="o in thumbParamList" :key="o.key" class="sv-cap">
          <div class="sv-cap-info">
            <span class="sv-cap-name">{{  o.label  }}</span>
            <span class="sv-badge sv-badge-muted">{{  o.tip  }}</span>
          </div>
          <div class="sv-cap-actions">
            <input v-model="thumbForm[o.field]" type="number" :min="o.min" :max="o.max" style="width: 108px" />
            <button class="btn btn-sm" :disabled="saving" @click="saveThumbParam(o)">{{  $t('保存')  }}</button>
            <button class="btn btn-sm" :disabled="saving" @click="resetThumbParam(o)">{{  $t('默认')  }}</button>
          </div>
        </div>

        <div class="sv-cap">
          <div class="sv-cap-info">
            <span class="sv-cap-name">{{  $t('当前生效')  }}</span>
            <span class="sv-badge sv-badge-muted">{{  $t('长边')  }} {{  derived.options?.max_edge || 480  }}px</span>
            <span class="sv-badge sv-badge-muted">{{  $t('抽帧上限')  }} {{  derived.options?.seek_ms ?? 1000  }}ms</span>
            <span class="sv-badge sv-badge-muted">{{  $t('质量')  }} {{  derived.options?.quality || 5  }}</span>
            <span class="sv-badge" :class="{ on: derived.ffmpeg }">{{  derived.ffmpeg ? $t('ffmpeg 就绪') : $t('未装 ffmpeg')  }}</span>
            <span class="sv-badge" :class="{ on: derived.capability_enabled }">{{  derived.capability_enabled ? $t('能力已开') : $t('能力已关')  }}</span>
          </div>
        </div>

        <div class="sv-cap">
          <div class="sv-cap-info">
            <span class="sv-cap-name">{{  $t('派生对象')  }}</span>
            <span class="sv-badge sv-badge-muted">{{  derived.stats?.objects || 0  }} {{  $t('个 ·')  }} {{  fmtBytes(derived.stats?.bytes || 0)  }}</span>
            <span class="sv-badge sv-badge-muted">{{  $t('覆盖')  }} {{  derived.stats?.files || 0  }} {{  $t('个源文件')  }}</span>
            <span v-if="(derived.stats?.orphans || 0) > 0" class="sv-badge sv-badge-warn">
              {{  $t('孤儿')  }} {{  derived.stats.orphans  }} {{  $t('个 ·')  }} {{  fmtBytes(derived.stats.orphan_bytes || 0)  }}
            </span>
          </div>
          <div class="sv-cap-actions">
            <button class="btn btn-sm" :disabled="saving || !(derived.stats?.objects > 0)" @click="doPurgeDerived()">{{  $t('清空派生缓存')  }}</button>
          </div>
        </div>

        <div class="sv-cap">
          <div class="sv-cap-info">
            <span class="sv-cap-name">{{  $t('孤儿自动清理间隔')  }}</span>
            <span class="sv-badge sv-badge-muted">{{  $t('分钟，0=关闭（默认 1440=24h）')  }}</span>
          </div>
          <div class="sv-cap-actions">
            <input v-model="sweepInterval" type="number" min="0" max="100000" style="width: 108px" />
            <button class="btn btn-sm" :disabled="saving" @click="saveSweepInterval()">{{  $t('保存')  }}</button>
            <button class="btn btn-sm" :disabled="saving" @click="resetSweepInterval()">{{  $t('默认')  }}</button>
            <button class="btn btn-sm" :disabled="saving || !((derived.stats?.orphans || 0) > 0)" @click="doSweepDerived()">{{  $t('立即清理孤儿')  }}</button>
          </div>
        </div>

        <p class="sv-muted" style="margin-top: 8px">
          {{  $t('清理是安全的：派生对象不是用户文件，删掉只会让缩略图在下次访问时重新生成（首次略慢）。 「孤儿」= 源文件已不存在的派生对象（直接改库、回滚数据库备份会留下这类残留），可与缓存一并清掉。 这里设的「自动清理间隔」会让服务定时兜底清孤儿（默认每天一次）；「立即清理孤儿」可随时手动触发一次。')  }}
        </p>
      </template>
    </section>

    <!-- 运维：数据备份（B28）—— 管理员动态开关；VACUUM INTO 一致性快照 → storage 远端 -->
    <section v-if="backupLoaded" v-show="tab === 'ops'" class="sv-card">
      <div class="sv-card-h">
        <h3>{{  $t('数据备份')  }}</h3>
        <p>{{  $t('VACUUM INTO 一致性快照 → storage 远端；开关/周期/保留份数均可后台动态设置（≤1 分钟生效）')  }}</p>
      </div>

      <div class="sv-cap">
        <div class="sv-cap-info">
          <span class="sv-cap-name">{{  $t('备份开关')  }}</span>
          <span class="sv-badge" :class="{ on: backup.enabled }">{{  backup.enabled ? $t('已开启') : $t('已关闭')  }}</span>
        </div>
        <div class="sv-cap-actions">
          <button class="btn btn-sm" :disabled="saving" @click="toggleBackup">{{  backup.enabled ? $t('关闭备份') : $t('开启备份')  }}</button>
        </div>
      </div>

      <div class="sv-cap">
        <div class="sv-cap-info">
          <span class="sv-cap-name">{{  $t('每日备份时刻')  }}</span>
          <span class="sv-badge sv-badge-muted">{{  $t('格式 HH:MM，如 03:00')  }}</span>
        </div>
        <div class="sv-cap-actions">
          <input v-model="backup.schedule" maxlength="5" placeholder="03:00" style="width: 108px" />
          <button class="btn btn-sm" :disabled="saving" @click="saveBackupFields">{{  $t('保存')  }}</button>
        </div>
      </div>

      <div class="sv-cap">
        <div class="sv-cap-info">
          <span class="sv-cap-name">{{  $t('本地保留份数')  }}</span>
          <span class="sv-badge sv-badge-muted">{{  $t('份')  }}</span>
        </div>
        <div class="sv-cap-actions">
          <input v-model.number="backup.keep_local" type="number" min="1" style="width: 108px" />
          <button class="btn btn-sm" :disabled="saving" @click="saveBackupFields">{{  $t('保存')  }}</button>
        </div>
      </div>

      <div class="sv-cap">
        <div class="sv-cap-info">
          <span class="sv-cap-name">{{  $t('远端保留份数')  }}</span>
          <span class="sv-badge sv-badge-muted">{{  $t('份')  }}</span>
        </div>
        <div class="sv-cap-actions">
          <input v-model.number="backup.keep_remote" type="number" min="1" style="width: 108px" />
          <button class="btn btn-sm" :disabled="saving" @click="saveBackupFields">{{  $t('保存')  }}</button>
        </div>
      </div>

      <p class="sv-muted" style="margin-top: 8px">
        {{  $t('最近成功备份：')  }}<code>{{  backup.last_backup || $t('（尚未备份）')  }}</code>
        <span v-if="backup.note" style="margin-left: 8px">{{  $t(backup.note)  }}</span>
      </p>
    </section>

    <!-- 数据（导入导出，独立视图组件整体嵌入） -->
    <section v-show="tab === 'data'" class="sv-data-tab">
      <ImpexView />
    </section>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, nextTick } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import * as api from '@/api'
import { useToastStore } from '@/stores/toast'
import { t, locale, setLocale, LOCALES } from '@/i18n'
import ImpexView from '@/views/ImpexView.vue'

const toast = useToastStore()
const saving = ref(false)

// 界面语言切换（B22）：即时生效并记忆，无需刷新
function onLocaleChange(e) {
  setLocale(e.target.value)
}

// ---- 顶部分组标签（站点/AI/消息/应用/数据；?tab= 深链直达）----
const route = useRoute()
const router = useRouter()
const NAV_TAB_KEYS = ['site', 'ai', 'im', 'apps', 'ops', 'data']
const tab = ref(NAV_TAB_KEYS.includes(route.query.tab) ? route.query.tab : 'site')
function switchTab(k) {
  tab.value = k
  router.replace({ query: { ...route.query, tab: k } })
}
const navTabs = computed(() => [
  { key: 'site', label: t('站点') },
  { key: 'ai', label: 'AI' },
  { key: 'im', label: t('消息') },
  { key: 'apps', label: t('已装应用'), count: plugins.value.length },
  { key: 'ops', label: t('运维') },
  { key: 'data', label: t('数据') }
])

// ---- 站点（多用户/开放注册）----
const multiUser = ref(true)
const registrationOpen = ref(true)

async function loadSite() {
  try {
    const d = await api.authConfig()
    if (d) multiUser.value = d.multi_user !== false
  } catch (_) { /* 未登录/旧后端：保持默认展示 */ }
  try {
    const s = await api.settings()
    const e = s && s['site.registration_open']
    if (e) registrationOpen.value = String(e.value) !== 'false'
  } catch (_) { /* 非 admin 无读取权限，保持默认展示 */ }
}

async function toggleRegistration() {
  const next = !registrationOpen.value
  saving.value = true
  try {
    await api.updateSetting('site.registration_open', next ? 'true' : 'false')
    registrationOpen.value = next
    toast.success(next ? t('已开启开放注册') : t('已关闭开放注册'))
  } catch (e) {
    toast.error(e?.message || t('保存失败'))
  } finally {
    saving.value = false
  }
}

// ---- 运维：能力模块门控（B14）----
// 判定优先级由后端 service.CapabilityEnabled 统一给出：站长配置 > 应用中心记录 > 内置默认。
// 面板只读生效态与来源，避免与真实装配结果漂移。
const capStates = ref([])

const capSourceText = {
  config: t('站长配置'),
  plugin: t('应用中心记录'),
  'plugin-capability': t('应用中心能力声明'),
  default: t('内置默认')
}

// 生效时机（B15）：后端 CapabilityApply 逐项给出 —— 装配期模块要重启，运行期判定的即时生效。
// 面板必须照实显示，否则站长会去白等一次重启（或反过来以为已经生效）。
const capApplyText = { live: t(t('即时生效')), restart: t('需重启进程') }

async function loadCapStates() {
  try {
    const d = await api.capabilityStates()
    capStates.value = (d && d.items) || []
  } catch (_) { capStates.value = [] }
}

function moduleOn(name) {
  const it = capStates.value.find((c) => c.name === name)
  return it ? it.enabled === true : true
}

async function saveCap(name, value) {
  saving.value = true
  try {
    // 生效时机取保存前的生效态（保存后重载可能改变来源，别再据此推断）
    const cur = capStates.value.find((c) => c.name === name)
    const how = cur && cur.apply === 'live' ? t('即时生效') : t('重启进程后生效')
    await api.updateSetting('capability.' + name, value)
    await loadCapStates()
    toast.success(value === '' ? `已改为跟随默认（${how}）` : `已保存（${how}）`)
  } catch (e) {
    toast.error(e?.message || t('保存失败'))
  } finally {
    saving.value = false
  }
}

// ---- 运维：组织模块运行期子开关（B14）----
// 与 capability.org 的区别：这里即时生效，无需重启；总开关关闭时子开关一律视同关闭。
const orgGates = ref({})
const orgGateList = computed(() => [
  { key: 'org.enabled', field: 'enabled', label: t('组织模块总开关') },
  { key: 'org.tree', field: 'tree', label: t('组织树（建/改/删节点）') },
  { key: 'org.department', field: 'department', label: t('部门空间') },
  { key: 'org.transfer', field: 'transfer', label: t('岗位移交流') }
])

async function loadOrgGates() {
  try {
    const d = await api.orgSettings()
    orgGates.value = (d && d.org) || {}
  } catch (_) { orgGates.value = {} }
}

async function saveOrg(key, value) {
  saving.value = true
  try {
    await api.updateSetting(key, value)
    await loadOrgGates()
    toast.success(value === 'true' ? t('已开启') : t('已关闭'))
  } catch (e) {
    toast.error(e?.message || t('保存失败'))
  } finally {
    saving.value = false
  }
}

// ---- 运维：媒体派生资产（B16）----
// 三个参数由后端 service.ThumbOptionsFrom 在**生成时**运行期读取 → 保存即生效（无需重启）；
// 但已生成的缩略图不会自动重做，面板必须显式给出「清空派生缓存」动作，
// 否则站长改完尺寸看到的仍是旧图，会误判成「没生效」（B15 已踩过同类判定漂移）。
const derived = ref({ stats: {}, options: {}, ffmpeg: false, capability_enabled: true })
const derivedReady = ref(false)
const derivedErr = ref('')
const thumbForm = ref({ max_edge: '', seek_ms: '', quality: '' })

// 区间只作前端提示与粗校验；权威仍在后端白名单（service 层同一组常量），两处若漂移以后端为准。
const thumbParamList = [
  { key: 'media.thumb_max_edge', field: 'max_edge', label: t('缩略图长边上限'), tip: t('像素 64-4096，只缩不放'), min: 64, max: 4096 },
  { key: 'media.thumb_seek_ms', field: 'seek_ms', label: t('视频抽帧位置上限'), tip: t('毫秒 0-60000，取时长 1/10 并夹在此值内'), min: 0, max: 60000 },
  { key: 'media.thumb_quality', field: 'quality', label: t('JPEG 质量'), tip: t('2-31，数值越小越清晰、体积越大'), min: 2, max: 31 }
]

async function loadDerived() {
  try {
    const d = await api.fetchDerivedStats()
    derived.value = d || {}
    derivedReady.value = true
    derivedErr.value = ''
    const o = (d && d.options) || {}
    thumbForm.value = {
      max_edge: String(o.max_edge ?? ''),
      seek_ms: String(o.seek_ms ?? ''),
      quality: String(o.quality ?? '')
    }
    sweepInterval.value = String(d?.sweep_interval_min ?? 1440)
  } catch (e) {
    derivedReady.value = false
    derivedErr.value = e?.message || t('读取派生资产状态失败')
  }
}

async function saveThumbParam(o) {
  const v = String(thumbForm.value[o.field] ?? '').trim()
  if (v !== '' && (!/^\d+$/.test(v) || Number(v) < o.min || Number(v) > o.max)) {
    toast.error(`${o.label} 需为 ${o.min}-${o.max} 的整数`)
    return
  }
  saving.value = true
  try {
    await api.updateSetting(o.key, v)
    await loadDerived()
    toast.success(t('已保存（参数即时生效；尺寸已变时请再点「清空派生缓存」让旧图重做）'))
  } catch (e) {
    toast.error(e?.message || t('保存失败'))
  } finally {
    saving.value = false
  }
}

async function resetThumbParam(o) {
  saving.value = true
  try {
    await api.updateSetting(o.key, '') // 空串 = 清除显式配置 → 回退内置默认
    await loadDerived()
    toast.success(t('已恢复默认值'))
  } catch (e) {
    toast.error(e?.message || t('保存失败'))
  } finally {
    saving.value = false
  }
}

async function doPurgeDerived() {
  if (!window.confirm(t('清空全部派生对象（缩略图缓存）？ 这不影响任何用户文件，只是让缩略图在下次访问时按当前参数重新生成。'))) return
  saving.value = true
  try {
    const r = await api.purgeDerived('')
    await loadDerived()
    toast.success(`已清理 ${r?.removed ?? 0} 个派生对象`)
  } catch (e) {
    toast.error(e?.message || t('清理失败'))
  } finally {
    saving.value = false
  }
}

// ---- 孤儿自动清理间隔（B18）----
async function saveSweepInterval() {
  const v = String(sweepInterval.value ?? '').trim()
  if (v !== '' && (!/^\d+$/.test(v) || Number(v) > 100000)) {
    toast.error(t('间隔需为 0-100000 的整数（分钟，0=关闭）'))
    return
  }
  saving.value = true
  try {
    await api.updateSetting('media.derived_sweep_interval_min', v)
    await loadDerived()
    toast.success(v === '0' ? t('已关闭自动清理（保留手动）') : `已保存，每 ${v} 分钟自动清理一次孤儿`)
  } catch (e) {
    toast.error(e?.message || t('保存失败'))
  } finally {
    saving.value = false
  }
}

async function resetSweepInterval() {
  saving.value = true
  try {
    await api.updateSetting('media.derived_sweep_interval_min', '') // 空串 = 回退内置默认（1440）
    await loadDerived()
    toast.success(t('已恢复默认（1440 分钟 / 24 小时）'))
  } catch (e) {
    toast.error(e?.message || t('保存失败'))
  } finally {
    saving.value = false
  }
}

async function doSweepDerived() {
  saving.value = true
  try {
    const r = await api.sweepDerived()
    await loadDerived()
    toast.success(`已清理 ${r?.removed ?? 0} 个孤儿派生对象`)
  } catch (e) {
    toast.error(e?.message || t('清理失败'))
  } finally {
    saving.value = false
  }
}

function fmtBytes(n) {
  const v = Number(n) || 0
  if (v < 1024) return `${v} B`
  if (v < 1024 * 1024) return `${(v / 1024).toFixed(1)} KB`
  return `${(v / 1024 / 1024).toFixed(2)} MB`
}

// ---- AI 模型 ----
const caps = ref([])
const customOpen = ref('')
const customForm = ref({ endpoint: '', model: '', apiKey: '' })

const capLabels = {
  llm: t('对话'), embedding: t('向量'), asr: t('语音识别'), tts: t('语音合成'),
  image: t('图像'), rerank: t('重排序'), ocr: 'OCR', code: t('代码')
}

async function loadProviders() {
  const d = await api.aiProviders()
  // name 与 cap 同值：模板沿用 cap.name 取键（后端字段是 cap，此处补齐避免 undefined）
  caps.value = (d.capabilities || []).map((c) => ({ ...c, name: c.cap, label: capLabels[c.cap] || c.cap }))
  syncVoiceForm()
}

function openCustom(cap) {
  if (customOpen.value === cap) {
    customOpen.value = ''
    return
  }
  const c = caps.value.find((x) => x.cap === cap)
  customOpen.value = cap
  customForm.value = {
    endpoint: c?.custom?.endpoint || '',
    model: c?.custom?.model || '',
    apiKey: ''
  }
}

async function onSwitch(cap, e) {
  const provider = e.target.value
  if (!provider) return
  try {
    await api.aiProviderSwitch(cap, provider)
    toast.success(t('已切换 ') + provider)
    await loadProviders()
  } catch (err) {
    toast.error(err.message)
  }
}

async function saveCustom(cap) {
  saving.value = true
  try {
    await api.aiCustomSave(cap, customForm.value.endpoint, customForm.value.model, customForm.value.apiKey)
    toast.success(t('自备模型已保存'))
    customOpen.value = ''
    await loadProviders()
  } catch (err) {
    toast.error(err.message)
  } finally {
    saving.value = false
  }
}

async function clearCustom(cap) {
  try {
    await api.aiCustomClear(cap)
    toast.success(t('已清除自备模型'))
    await loadProviders()
  } catch (err) {
    toast.error(err.message)
  }
}

// ---- 语音合成默认参数（family.8.1）：ai.tts.voice/format/style 走设置白名单，即时生效 ----
const voiceForm = ref({ voice: '', format: '', style: '' })
const voiceAudio = ref('')
const listening = ref(false)

function syncVoiceForm() {
  const c = caps.value.find((x) => x.cap === 'tts')
  const v = c?.voice || {}
  voiceForm.value = { voice: v.current || '', format: v.format || '', style: v.style || '' }
}

async function saveVoice() {
  saving.value = true
  try {
    await api.updateSetting('ai.tts.voice', voiceForm.value.voice.trim())
    await api.updateSetting('ai.tts.format', voiceForm.value.format)
    await api.updateSetting('ai.tts.style', voiceForm.value.style.trim())
    toast.success(t('语音默认值已保存（即时生效）'))
    await loadProviders()
  } catch (err) {
    toast.error(err.message)
  } finally {
    saving.value = false
  }
}

async function testVoice() {
  listening.value = true
  try {
    const { blob } = await api.aiTTSListen({
      text: t('你好，这里是爱库录语音试听。'),
      voice: voiceForm.value.voice.trim(),
      style: voiceForm.value.style.trim(),
      format: voiceForm.value.format || 'mp3'
    })
    if (voiceAudio.value) URL.revokeObjectURL(voiceAudio.value)
    voiceAudio.value = URL.createObjectURL(blob)
    await nextTick()
    const el = document.querySelector('.sv-voice-audio')
    if (el) el.play().catch(() => {})
  } catch (err) {
    toast.error(err.message)
  } finally {
    listening.value = false
  }
}

// ---- 模型提供方管理（可任意增删改 + token 池）----
const providers = ref([])
const testing = ref('')
const provModal = ref({ open: false, editing: false })
const provForm = ref({
  name: '', endpoint: '', model: '', caps: [], models: {}, capCandsText: {},
  keysText: '', voiceCandsText: '', note: ''
})
const providerCapList = Object.keys(capLabels).map((c) => ({ key: c, label: capLabels[c] || c }))

function capLabel(c) { return capLabels[c] || c }

async function loadProvidersManage() {
  try {
    const d = await api.aiProvidersManage()
    providers.value = d.providers || []
  } catch (err) {
    toast.error(err.message)
  }
}

function openProviderAdd() {
  provModal.value = { open: true, editing: false }
  provForm.value = {
    name: '', endpoint: '', model: '', caps: [], models: {}, capCandsText: {},
    keysText: '', voiceCandsText: '', note: ''
  }
}

function openProviderEdit(p) {
  const models = {}
  const capCandsText = {}
  for (const k of Object.keys(p.models || {})) models[k] = p.models[k]
  for (const k of Object.keys(p.model_cands || {})) capCandsText[k] = (p.model_cands[k] || []).join(', ')
  provModal.value = { open: true, editing: true }
  provForm.value = {
    name: p.name, endpoint: p.endpoint || '', model: p.model || '',
    caps: (p.caps || []).slice(), models,
    capCandsText, keysText: (p.keys || []).join('\n'),
    voiceCandsText: (p.voice_cands || []).join(', '), note: p.note || ''
  }
}

function closeProviderModal() {
  provModal.value = { open: false, editing: false }
}

function splitList(s) {
  return (s || '')
    .split(/[\n,]/)
    .map((x) => x.trim())
    .filter((x) => x.length > 0)
}

async function saveProvider() {
  saving.value = true
  try {
    const f = provForm.value
    const models = {}
    const modelCands = {}
    for (const c of f.caps) {
      if (f.models[c]) models[c] = f.models[c].trim()
      const cands = splitList(f.capCandsText[c])
      if (cands.length) modelCands[c] = cands
    }
    const payload = {
      name: f.name.trim(),
      endpoint: f.endpoint.trim(),
      model: f.model.trim(),
      caps: f.caps,
      models,
      model_cands: modelCands,
      keys: splitList(f.keysText),
      voice_cands: splitList(f.voiceCandsText),
      note: f.note.trim()
    }
    await api.aiProviderUpsert(payload)
    toast.success(t('提供方已保存'))
    closeProviderModal()
    await loadProvidersManage()
    await loadProviders()
  } catch (err) {
    toast.error(err.message)
  } finally {
    saving.value = false
  }
}

async function deleteProvider(p) {
  if (!confirm(t('确认删除提供方 ') + p.name + t('？关联的能力绑定将被解除'))) return
  try {
    await api.aiProviderDelete(p.name)
    toast.success(t('已删除 ') + p.name)
    await loadProvidersManage()
    await loadProviders()
  } catch (err) {
    toast.error(err.message)
  }
}

async function testProvider(p) {
  testing.value = p.name
  try {
    const cap = p.caps && p.caps.length ? p.caps[0] : 'llm'
    const r = await api.aiProviderTest(p.name, cap)
    if (r.ok) {
      toast.success(t('连接成功（') + p.name + ' / ' + capLabel(cap) + '）')
    } else {
      toast.error(t('连接失败：') + (r.error || t('未知错误')))
    }
  } catch (err) {
    toast.error(err.message)
  } finally {
    testing.value = ''
  }
}

// ---- 用量管控与计费（平台模型 token 级配额 + 余额增值包）----
const quota = ref({
  user_daily_calls: 0, user_daily_tokens: 0,
  public_daily_calls: 0, public_daily_tokens: 0,
  billing_enabled: false
})
const quotaToday = ref([])
const balances = ref([])
const grantForm = ref({ subject: '', delta: 0, reason: '' })

function fmtTokens(n) {
  if (n == null) return '0'
  if (n >= 1000000) return (n / 1000000).toFixed(2) + 'M'
  if (n >= 1000) return (n / 1000).toFixed(1) + 'k'
  return String(n)
}

async function loadQuotaPolicy() {
  try {
    const d = await api.aiQuotaPolicy()
    if (d.policy) quota.value = {
      user_daily_calls: d.policy.user_daily_calls || 0,
      user_daily_tokens: d.policy.user_daily_tokens || 0,
      public_daily_calls: d.policy.public_daily_calls || 0,
      public_daily_tokens: d.policy.public_daily_tokens || 0,
      billing_enabled: !!d.policy.billing_enabled
    }
    quotaToday.value = d.today || []
    balances.value = d.balances || []
  } catch (err) {
    toast.error(err.message)
  }
}

async function saveQuotaPolicy() {
  saving.value = true
  try {
    await api.aiQuotaPolicySave({
      user_daily_calls: Math.max(0, quota.value.user_daily_calls || 0),
      user_daily_tokens: Math.max(0, quota.value.user_daily_tokens || 0),
      public_daily_calls: Math.max(0, quota.value.public_daily_calls || 0),
      public_daily_tokens: Math.max(0, quota.value.public_daily_tokens || 0),
      billing_enabled: !!quota.value.billing_enabled
    })
    toast.success(t('管控策略已保存，即时生效'))
    await loadQuotaPolicy()
  } catch (err) {
    toast.error(err.message)
  } finally {
    saving.value = false
  }
}

function openGrant(subject) {
  grantForm.value = { subject, delta: 100000, reason: t('增值包充值') }
}

async function doGrant() {
  const f = grantForm.value
  if (!f.subject || !f.delta) {
    toast.error(t('请填写主体与增减 token 数'))
    return
  }
  saving.value = true
  try {
    const r = await api.aiBalanceGrant(f.subject.trim(), f.delta, f.reason || '')
    toast.success(t('已执行，新余额 ') + fmtTokens(r.balance))
    grantForm.value = { subject: '', delta: 0, reason: '' }
    await loadQuotaPolicy()
  } catch (err) {
    toast.error(err.message)
  } finally {
    saving.value = false
  }
}

// ---- IM 对接 ----
const tg = ref({ botToken: '', allowedChats: '' })
const wc = ref({ webhookUrl: '', corpId: '', agentId: '', secret: '', token: '', encodingAesKey: '' })
const imTgOn = ref(false)
const imWcOn = ref(false)
const tgHook = computed(() => `${location.origin}/api/v1/im/webhook/telegram`)
const wcHook = computed(() => `${location.origin}/api/v1/im/webhook/wecom`)

async function loadIM() {
  try {
    const d = await api.imStatus()
    imTgOn.value = !!(d.telegram && d.telegram.enabled)
    imWcOn.value = !!(d.wecom && d.wecom.enabled)
  } catch (_) {
    /* 后端未更新时忽略 */
  }
}

async function saveIM() {
  saving.value = true
  try {
    await api.imTelegramConfig({ botToken: tg.value.botToken, allowedChats: tg.value.allowedChats })
    toast.success(t('Telegram 配置已保存'))
    tg.value.botToken = ''
    await loadIM()
  } catch (err) {
    toast.error(err.message)
  } finally {
    saving.value = false
  }
}

async function saveWeCom() {
  saving.value = true
  try {
    await api.imWeComConfig({
      webhookUrl: wc.value.webhookUrl,
      corpId: wc.value.corpId,
      agentId: wc.value.agentId,
      secret: wc.value.secret,
      token: wc.value.token,
      encodingAesKey: wc.value.encodingAesKey
    })
    toast.success(t('企业微信配置已保存'))
    wc.value.secret = ''
    await loadIM()
  } catch (err) {
    toast.error(err.message)
  } finally {
    saving.value = false
  }
}

// ---- Agent 参数 ----
const maxRounds = ref(6)

async function loadAgent() {
  try {
    const s = await api.settings()
    const v = s['ai.agent.max_rounds']
    if (v && v.value) maxRounds.value = Number(v.value)
  } catch (_) { /* 忽略 */ }
}

async function saveMaxRounds() {
  const n = Number(maxRounds.value)
  if (!n || n < 1 || n > 32) {
    toast.error(t('轮数需在 1-32 之间'))
    return
  }
  saving.value = true
  try {
    await api.updateSetting('ai.agent.max_rounds', String(n))
    toast.success(t('已保存，即时生效'))
  } catch (err) {
    toast.error(err.message)
  } finally {
    saving.value = false
  }
}

// ---- 入库解读 ----
const summarizeExts = ref('')

async function loadSummarize() {
  try {
    const st = await api.settings()
    const v = st['ai.summarize.exts']
    if (v) summarizeExts.value = v.value || ''
  } catch (_) { /* 忽略 */ }
}

async function saveSummarizeExts() {
  saving.value = true
  try {
    await api.updateSetting('ai.summarize.exts', summarizeExts.value.trim())
    toast.success(t('解读类型已保存，即时生效'))
  } catch (err) {
    toast.error(err.message)
  } finally {
    saving.value = false
  }
}

// ---- 插件调试（协议 v1.1）----
const plugins = ref([])
const kvSamples = ref({})

// 事件名（对应审计动作；event:post 权限可投递的对象清单）
const eventNames = [
  'auth.login', 'auth.logout', 'file.create', 'file.update', 'file.delete',
  'file.move', 'share.create', 'share.delete', 'blog.post', 'blog.update',
  'comment.create', 'collection.create', 'tag.create', 'collect.run',
  'export.created', 'plugin.register', 'im.telegram.message', 'settings.update',
]

function copyEvent(ev) {
  navigator.clipboard?.writeText(ev).catch(() => {})
  toast.success(t('已复制 ') + ev)
}

// 挂载点枚举 → 中文标签。两种命名并存：博客插件用裸名（post_bottom），
// 应用/主题用带壳前缀的（blog.head）。未知值原样透出（第三方可自定义）。
const MOUNT_POINT_LABELS = {
  post_bottom: '文章正文下方',
  sidebar: '侧栏',
  list_item: '列表项',
  head: '页面头部',
  'blog.post_bottom': '文章正文下方',
  'blog.sidebar': '侧栏',
  'blog.list_item': '列表项',
  'blog.head': '页面头部',
}

function mountPointsText(p) {
  const mps = p?.mount_points || []
  if (!mps.length) return '—'
  return mps.map((m) => t(MOUNT_POINT_LABELS[m] || m)).join(' / ')
}

async function loadPlugins() {
  try {
    const d = await api.publicPlugins()
    plugins.value = d.items || []
    for (const p of plugins.value) {
      try {
        const kv = await api.pluginKVList(p.id)
        kvSamples.value[p.id] = (kv.items || []).map((x) => x.key).slice(0, 8)
      } catch (_) {
        kvSamples.value[p.id] = []
      }
    }
  } catch (_) { /* 后端未更新时忽略 */ }
}

async function onInstallZip(ev) {
  const f = ev.target.files?.[0]
  ev.target.value = ''
  if (!f) return
  try {
    const d = await api.installAppZip(f)
    toast.success(`已安装 ${d.id}`)
    await loadPlugins()
  } catch (e) {
    toast.error(e.message || t('安装失败'))
  }
}

async function togglePlugin(p) {
  try {
    await api.pluginToggle(p.id, !p.enabled)
    p.enabled = !p.enabled
    toast.success(p.enabled ? t('已启用') : t('已禁用'))
  } catch (e) {
    toast.error(e.message)
  }
}

async function removePlugin(p) {
  if (!confirm(`卸载插件 ${p.id}？`)) return
  try {
    await api.pluginDelete(p.id)
    plugins.value = plugins.value.filter((x) => x.id !== p.id)
    toast.success(t('已卸载'))
  } catch (e) {
    toast.error(e.message)
  }
}

// ---- 插件动态设置（settings_schema → KV）----
const cfgId = ref('')
const cfgName = ref('')
const cfgSchema = ref([])
const cfgValues = ref({})

async function openConfig(p) {
  cfgId.value = p.id
  cfgName.value = p.name || p.id
  try {
    const r = await api.pluginSettingsSchema(p.id)
    cfgSchema.value = r.settings_schema || []
  } catch (_) {
    cfgSchema.value = []
  }
  cfgValues.value = {}
  try {
    const kv = await api.pluginKVList(p.id)
    for (const k of (kv.items || [])) cfgValues.value[k.key] = k.value
  } catch (_) {}
}
function closeConfig() {
  cfgId.value = ''
  cfgSchema.value = []
  cfgValues.value = {}
}
async function saveConfig() {
  saving.value = true
  try {
    for (const f of cfgSchema.value) {
      const v = cfgValues.value[f.key]
      await api.pluginKVSet(cfgId.value, f.key, v == null ? '' : String(v))
    }
    toast.success(t('已保存设置'))
    closeConfig()
  } catch (e) {
    toast.error(e?.message || t('保存失败'))
  } finally {
    saving.value = false
  }
}

// ---- AI 用量（近 7 日 + 当日 Top）----
const aiUsageDaily = ref([])
const aiUsageTop = ref([])

async function loadAiUsage() {
  try {
    const d = await api.aiUsage(7)
    aiUsageDaily.value = d?.daily || []
    aiUsageTop.value = d?.top || []
  } catch (_) { /* 非 admin / 旧后端：静默隐藏 */ }
}

// ---- 数据备份（B28，管理员：动态开关 + 周期 + 保留份数）----
const backupLoaded = ref(false)
const backup = ref({ enabled: false, schedule: '03:00', keep_local: 7, keep_remote: 30, last_backup: '', note: '' })
async function loadBackup() {
  try {
    const d = await api.systemBackupGet()
    if (d && d.ok && d.backup) { backup.value = d.backup; backupLoaded.value = true }
  } catch (_) { /* 非管理员或旧后端：不展示卡片 */ }
}
async function toggleBackup() {
  saving.value = true
  try {
    const d = await api.systemBackupPut({ enabled: !backup.value.enabled })
    if (d && d.ok) { backup.value = d.backup; toast.success(t('备份开关已更新')) } else toast.error(t('更新失败'))
  } catch (e) { toast.error(t('更新失败：') + (e.message || e)) }
  saving.value = false
}
async function saveBackupFields() {
  saving.value = true
  try {
    const d = await api.systemBackupPut({
      schedule: backup.value.schedule,
      keep_local: Number(backup.value.keep_local),
      keep_remote: Number(backup.value.keep_remote)
    })
    if (d && d.ok) { backup.value = d.backup; toast.success(t('备份周期/保留份数已保存')) } else toast.error(t('保存失败'))
  } catch (e) { toast.error(t('保存失败：') + (e.message || e)) }
  saving.value = false
}

onMounted(() => {
  loadSite()
  loadProviders()
  loadProvidersManage()
  loadQuotaPolicy()
  loadIM()
  loadAgent()
  loadSummarize()
  loadPlugins()
  loadAiUsage()
  loadCapStates()
  loadOrgGates()
  loadDerived()
  loadBackup()
})
</script>

<style scoped>
.settings-view {
  max-width: 1200px;
  margin: 0 auto;
  padding: 20px 24px 40px;
}
/* 顶部分组标签栏 */
.sv-nav-tabs {
  display: flex;
  gap: 6px;
  margin: 4px 0 16px;
  border-bottom: 1px solid var(--border);
  padding-bottom: 10px;
  flex-wrap: wrap;
}
.sv-tab-btn {
  border: 1px solid transparent;
  background: none;
  cursor: pointer;
  font-size: 13px;
  color: var(--muted, #666);
  padding: 6px 14px;
  border-radius: 999px;
  transition: all 0.15s;
}
.sv-tab-btn:hover {
  color: var(--text, #222);
  background: var(--hover, rgba(127, 127, 127, 0.08));
}
.sv-tab-btn.on {
  color: var(--primary, #4f7cff);
  background: var(--primary-soft, rgba(79, 124, 255, 0.1));
  border-color: var(--primary-soft, rgba(79, 124, 255, 0.2));
  font-weight: 600;
}
.sv-tab-count {
  margin-left: 5px;
  font-size: 11px;
  opacity: 0.75;
}
.sv-data-tab {
  margin-top: 4px;
}
/* 插件调试 */
.sv-plug {
  border-top: 1px solid var(--border);
  padding-top: 12px;
}
.plug-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12.5px;
}
.plug-table th {
  text-align: left;
  color: var(--text-3);
  font-weight: 500;
  padding: 6px 8px;
  border-bottom: 1px solid var(--border);
}
.plug-table td {
  padding: 8px;
  border-bottom: 1px solid var(--border);
  vertical-align: top;
  color: var(--text);
}
.kv-sample {
  font-size: 11.5px;
  color: var(--text-2);
  background: var(--bg);
  padding: 2px 6px;
  border-radius: 4px;
  display: inline-block;
  max-width: 260px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.sv-events {
  margin-top: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}
.sv-events-label {
  font-size: 12px;
  color: var(--text-3);
  margin-right: 4px;
}
.ev-chip {
  font-size: 11.5px;
  font-family: var(--mono, monospace);
  background: var(--bg);
  border: 1px solid var(--border);
  color: var(--text-2);
  padding: 3px 8px;
  border-radius: 10px;
  cursor: pointer;
  user-select: none;
}
.ev-chip:hover {
  border-color: var(--accent, #3b82f6);
  color: var(--accent, #3b82f6);
}
/* 应用中心（在线目录） */
.sv-tabs {
  display: flex;
  gap: 8px;
  margin: 4px 0 12px;
}
.sv-tab {
  padding: 6px 14px;
  border-radius: 6px;
  border: 1px solid var(--border);
  background: transparent;
  cursor: pointer;
  font-size: 13px;
  color: var(--text);
}
.sv-tab.on {
  background: var(--accent, #2563eb);
  color: #fff;
  border-color: transparent;
}
.sv-app-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 12px;
}
.sv-app-item {
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 12px 14px;
  background: var(--card-bg, #fff);
}
.sv-app-cover {
  width: 100%;
  aspect-ratio: 8 / 5;
  object-fit: cover;
  border-radius: 6px;
  margin-bottom: 10px;
  border: 1px solid var(--border);
  background: var(--bg-2, #f2f4f7);
}




.sv-app-t {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
}
.sv-app-name {
  font-weight: 600;
  font-size: 14px;
  color: var(--text);
}
.sv-app-desc {
  color: var(--text-2, #666);
  font-size: 13px;
  margin: 8px 0 10px;
  line-height: 1.5;
}
.sv-app-foot {
  display: flex;
  justify-content: space-between;
  align-items: center;
}
.sv-badge {
  font-size: 11px;
  padding: 2px 7px;
  border-radius: 4px;
}
.sv-badge-paid {
  background: #fef3c7;
  color: #92400e;
}
.sv-badge-ok {
  background: #e7f5ec;
  color: #177245;
}
.sv-badge-muted {
  background: #eee;
  color: #777;
}
/* 插件动态设置面板 */
.sv-cfg {
  margin-top: 14px;
  border: 1px dashed var(--border);
  border-radius: 8px;
  padding: 12px 14px;
  background: var(--bg);
}
.sv-cfg-h {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 10px;
}
.sv-cfg-row {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: 10px;
}
.sv-cfg-row label {
  font-size: 12.5px;
  color: var(--text-2);
}
.sv-cfg-row .input {
  width: 260px;
  max-width: 100%;
}

.sv-head h2 {
  font-size: 17px;
  font-weight: 600;
  color: var(--text);
  margin-bottom: 4px;
}
.sv-sub {
  font-size: 12.5px;
  color: var(--text-3);
  margin-bottom: 18px;
}
.sv-card {
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
  padding: 16px;
  margin-bottom: 16px;
}
.sv-card-h h3 {
  font-size: 14px;
  font-weight: 600;
  color: var(--text);
  margin-bottom: 4px;
}
.sv-card-h p {
  font-size: 12px;
  color: var(--text-3);
  margin-bottom: 12px;
}
.sv-cap {
  border-top: 1px solid var(--border);
  padding: 12px 0;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 10px;
}
.sv-cap:first-of-type {
  border-top: none;
}
.sv-cap-info {
  display: flex;
  align-items: center;
  gap: 8px;
  flex: 1;
  min-width: 200px;
}
.sv-cap-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--text);
  width: 64px;
}
.sv-badge {
  font-size: 11px;
  padding: 1px 8px;
  border-radius: 10px;
  background: var(--surface-2);
  color: var(--text-3);
}
.sv-badge.on {
  background: var(--primary-soft);
  color: var(--primary);
}
.sv-cap-model {
  font-size: 12px;
  color: var(--text-2);
}
.sv-custom-tag {
  font-size: 11px;
  padding: 1px 8px;
  border-radius: 10px;
  background: rgba(250, 173, 20, 0.14);
  color: #b45309;
}
.sv-cap-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}
.sv-cap-actions select {
  height: 28px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
  color: var(--text);
  padding: 0 8px;
  font-size: 12px;
}
.sv-custom {
  flex-basis: 100%;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 8px;
  padding: 12px;
  background: var(--surface-2);
  border-radius: 8px;
}
.sv-custom input,
.sv-im input,
.sv-agent input {
  height: 30px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
  color: var(--text);
  padding: 0 10px;
  font-size: 12.5px;
  flex: 1;
  min-width: 180px;
}
.sv-custom-actions {
  display: flex;
  gap: 8px;
}
/* 语音合成默认参数面板（family.8.1）：与 .sv-custom 同一视觉语言 */
.sv-voice {
  flex-basis: 100%;
  margin-top: 8px;
  padding: 12px;
  background: var(--surface-2);
  border-radius: 8px;
}
.sv-voice-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
}
.sv-voice-field {
  flex: 1;
  min-width: 180px;
}
.sv-voice-field label {
  display: block;
  font-size: 12px;
  color: var(--text-2);
  margin: 0 0 4px;
}
.sv-voice-field input,
.sv-voice-select {
  width: 100%;
  height: 30px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
  color: var(--text);
  padding: 0 10px;
  font-size: 12.5px;
}
.sv-voice-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 10px;
  flex-wrap: wrap;
}
.sv-voice-audio {
  height: 30px;
  max-width: 320px;
}
.sv-im label,
.sv-agent label {
  display: block;
  font-size: 12px;
  color: var(--text-2);
  margin: 10px 0 4px;
}
.sv-im-actions,
.sv-agent {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-top: 12px;
}
.sv-im-state-row {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
}
.sv-im + .sv-im {
  border-top: 1px solid var(--border);
  margin-top: 16px;
  padding-top: 16px;
}
.sv-im-block-h {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 6px;
}
.sv-im-plt {
  font-size: 13px;
  font-weight: 600;
  color: var(--text);
}
.sv-muted {
  font-size: 12px;
  color: var(--text-3);
}
.sv-muted code {
  background: var(--surface-2);
  padding: 1px 6px;
  border-radius: 6px;
  font-size: 11px;
}
/* 已装应用：中文名作主标题（复用 .sv-app-name），插件 id 降为灰色小字
   —— id 仍需可见，排障/提交反馈时要用，只是不该抢主标题。 */
.sv-app-id {
  background: var(--surface-2);
  color: var(--text-3);
  padding: 1px 6px;
  border-radius: 6px;
  font-size: 11px;
}
.btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  height: 32px;
  padding: 0 14px;
  border-radius: 8px;
  font-size: 13px;
  background: var(--surface-2);
  border: 1px solid var(--border);
  color: var(--text-2);
  white-space: nowrap;
  cursor: pointer;
}
.btn:hover:not(:disabled) {
  color: var(--text);
  border-color: var(--text-3);
}
.btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
.btn-primary {
  background: var(--primary);
  border-color: var(--primary);
  color: #fff;
}
.btn-primary:hover:not(:disabled) {
  color: #fff;
  opacity: 0.92;
}
.btn-sm {
  height: 28px;
  padding: 0 10px;
  font-size: 12px;
}
.sv-cap-note {
  font-size: 12px;
  color: var(--text-3);
}
.sv-warn {
  font-size: 12px;
  color: var(--text-2, #92400e);
  background: var(--surface-2);
  border: 1px solid var(--border);
  border-left: 3px solid #d97706;
  border-radius: 8px;
  padding: 8px 10px;
  margin-bottom: 10px;
}
/* 孤儿计数徽章：与 .sv-custom-tag 同一警示配色，避免新增色板 */
.sv-badge-warn {
  background: rgba(250, 173, 20, 0.14);
  color: #b45309;
}
/* ---- 模型提供方管理面板 ---- */
.sv-providers {
  display: flex;
  flex-direction: column;
  gap: 10px;
  margin: 4px 0 12px;
}
.sv-prov {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 14px;
  border: 1px solid var(--border);
  border-radius: 10px;
  background: var(--surface);
}
.sv-prov-main { min-width: 0; flex: 1; }
.sv-prov-name {
  font-weight: 600;
  font-size: 14px;
  color: var(--text);
  display: flex;
  align-items: center;
  gap: 8px;
}
.sv-prov-endpoint {
  font-size: 12px;
  color: var(--text-2, #6b7280);
  margin: 2px 0;
  word-break: break-all;
}
.sv-prov-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  font-size: 12px;
  color: var(--text-2, #6b7280);
}
.sv-prov-actions { display: flex; gap: 6px; flex-shrink: 0; }
.sv-empty {
  font-size: 13px;
  color: var(--text-2, #9ca3af);
  padding: 8px 2px;
}
.sv-prov-add { display: flex; justify-content: flex-end; }
.sv-modal-mask {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.45);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}
.sv-modal {
  width: min(560px, 92vw);
  max-height: 88vh;
  overflow: auto;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 12px;
  box-shadow: 0 16px 48px rgba(0, 0, 0, 0.3);
}
.sv-modal-h {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 18px;
  border-bottom: 1px solid var(--border);
}
.sv-modal-h h4 { margin: 0; font-size: 15px; color: var(--text); }
.sv-modal-x {
  border: none;
  background: transparent;
  font-size: 22px;
  line-height: 1;
  color: var(--text-2, #6b7280);
  cursor: pointer;
}
.sv-modal-body { padding: 16px 18px; display: flex; flex-direction: column; gap: 6px; }
.sv-modal-body label {
  font-size: 12px;
  color: var(--text-2, #6b7280);
  margin-top: 8px;
}
.sv-modal-body input,
.sv-modal-body textarea {
  height: 32px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--input-bg, var(--surface));
  color: var(--text);
  padding: 0 10px;
  font-size: 13px;
}
.sv-modal-body textarea { height: 88px; padding: 8px 10px; resize: vertical; }
.sv-caps-pick {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin: 4px 0;
}
.sv-cap-check {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 13px;
  color: var(--text);
  cursor: pointer;
}
.sv-cap-row {
  display: grid;
  grid-template-columns: 64px 1fr 1fr;
  gap: 8px;
  align-items: center;
}
.sv-cap-row-name {
  font-size: 12px;
  color: var(--text-2, #6b7280);
}
.sv-modal-foot {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 14px 18px;
  border-top: 1px solid var(--border);
}

/* ---- 用量管控与计费 ---- */
.sv-quota-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 10px 16px;
  margin-bottom: 12px;
}

.sv-quota-grid label {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 12px;
  color: var(--text-2, #666);
}

.sv-quota-grid input {
  max-width: 200px;
}

.sv-quota-check {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 16px;
}

.sv-quota-check label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
}

.sv-balance {
  margin-top: 14px;
  padding-top: 12px;
  border-top: 1px dashed var(--border);
}

.sv-balance h4 {
  margin: 0 0 8px;
  font-size: 13px;
}

.sv-balance-row {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 5px 0;
  font-size: 13px;
  flex-wrap: wrap;
}

.sv-balance-row code {
  font-size: 12px;
  background: var(--bg-2, rgba(0, 0, 0, 0.04));
  padding: 2px 8px;
  border-radius: 4px;
}

.sv-balance-num {
  font-weight: 600;
}

.sv-grant {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
  margin-top: 10px;
}

.sv-grant input {
  font-size: 12px;
}
</style>
