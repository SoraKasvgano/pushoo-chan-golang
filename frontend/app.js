(() => {
  const $ = (id) => document.getElementById(id);
  const cfgEl = $("cfg");
  const cfgMsg = $("cfgMsg");
  const pageButtons = Array.from(document.querySelectorAll(".nav-btn"));
  const pageSections = Array.from(document.querySelectorAll(".page-section"));

  // Configuration state
  let configData = {
    auth: { user: "", pass: "" },
    channels: [],
    channel_groups: [],
    default_channel: "",
    push_token: { enabled: false, token: "" },
    webhooks: { tawk: { chan: "", title: "", secret: "" } },
    sqlite: { path: "", cleanup_days: 30, cleanup_interval_hours: 24, record_channel_messages: false },
    security: {
      auth_fail_limit: 5,
      auth_ban_minutes: 10,
      token_fail_limit: 10,
      token_ban_minutes: 10,
      ip_ban_max_entries: 10000,
      ip_ban_cleanup_seconds: 60,
      ip_ban_idle_minutes: 60
    }
  };
  let configRevision = "";
  let configLoaded = false;
  let configDirty = false;
  let activeConfigTab = "graphical";

  function escapeHTML(value) {
    return String(value ?? "").replace(/[&<>'"]/g, (char) => ({
      "&": "&amp;", "<": "&lt;", ">": "&gt;", "'": "&#39;", '"': "&quot;"
    })[char]);
  }

  function setConfigReady(ready) {
    configLoaded = ready;
    ["saveGraphical", "saveYaml", "addChannel", "addGroup"].forEach((id) => {
      const el = $(id);
      if (el) el.disabled = !ready;
    });
  }

  function setConfigDirty(dirty) {
    configDirty = dirty;
    const state = $("configState");
    state.textContent = dirty ? "有未保存更改" : (configLoaded ? "已与服务器同步" : "正在加载");
    state.className = `config-state ${dirty ? "config-state--dirty" : (configLoaded ? "config-state--saved" : "")}`;
  }

  function setMsg(el, text, isErr, isLoading = false) {
    el.textContent = text || "";
    el.className = "msg";
    if (isLoading) {
      el.className += " msg--loading";
      el.innerHTML = `<span class="spinner"></span>${text}`;
    } else if (isErr) {
      el.className += " msg--error fade-in";
    } else if (text) {
      el.className += " msg--success fade-in";
    }
  }

  function switchPage(page) {
    pageSections.forEach((sec) => {
      const match = sec.dataset.page === page;
      sec.classList.toggle("page-section--hidden", !match);
    });
    pageButtons.forEach((btn) => {
      btn.classList.toggle("nav-btn--active", btn.dataset.page === page);
    });
    localStorage.setItem("pushoo_page", page);
    if (page === "overview") {
      refreshOverview().catch(() => {});
    }
  }

  function initPageNav() {
    const saved = localStorage.getItem("pushoo_page") || "overview";
    switchPage(saved);
    pageButtons.forEach((btn) => {
      btn.addEventListener("click", () => switchPage(btn.dataset.page));
    });
  }

  function formatBanMessage(resp) {
    if (resp.status !== 429) return null;
    const retryAfter = resp.headers.get("Retry-After");
    if (retryAfter) {
      const seconds = parseInt(retryAfter, 10);
      if (!Number.isNaN(seconds)) {
        return `操作过于频繁，已临时封禁，请在 ${seconds} 秒后重试。`;
      }
    }
    return "操作过于频繁，已临时封禁，请稍后重试。";
  }

  async function downloadConfig() {
    setConfigReady(false);
    setMsg(cfgMsg, "下载中...", false, true);
    try {
      const resp = await fetch("/config/download");
      const banMsg = formatBanMessage(resp);
      const txt = await resp.text();
      if (!resp.ok) {
        setMsg(cfgMsg, banMsg || `下载失败: ${resp.status} ${txt}`, true);
        return;
      }
      const dataResp = await fetch("/config/data");
      if (!dataResp.ok) {
        throw new Error(`结构化配置加载失败 (${dataResp.status})`);
      }
      const rawRevision = resp.headers.get("X-Config-Revision") || "";
      const dataRevision = dataResp.headers.get("X-Config-Revision") || "";
      if (rawRevision && dataRevision && rawRevision !== dataRevision) {
        return downloadConfig();
      }
      cfgEl.value = txt;
      configData = await dataResp.json();
      configRevision = dataRevision || rawRevision;
      renderGraphicalConfig();
      setConfigReady(true);
      setConfigDirty(false);
      setMsg(cfgMsg, "已加载服务器当前配置", false);
    } catch (err) {
      setConfigReady(false);
      setConfigDirty(false);
      setMsg(cfgMsg, `下载失败: ${err.message}`, true);
    }
  }

  async function uploadConfig() {
    if (!configLoaded) throw new Error("配置尚未成功加载，已阻止保存");
    setMsg(cfgMsg, "上传中...", false, true);
    try {
      const resp = await fetch("/config/upload", {
        method: "POST",
        headers: {
          "content-type": "text/plain; charset=utf-8",
          "If-Match": configRevision
        },
        body: cfgEl.value || "",
      });
      const banMsg = formatBanMessage(resp);
      const txt = await resp.text();
      if (!resp.ok) {
        const message = resp.status === 409
          ? "服务器配置已被修改，请重新加载后再编辑"
          : (banMsg || `上传失败: ${resp.status} ${txt}`);
        setMsg(cfgMsg, message, true);
        return;
      }
      configRevision = resp.headers.get("X-Config-Revision") || configRevision;
      await downloadConfig();
      setMsg(cfgMsg, "配置已保存并重新加载", false);
    } catch (err) {
      setMsg(cfgMsg, `上传失败: ${err.message}`, true);
    }
  }

  function beautifyYAML() {
    // Very small "beautify": normalize tabs to spaces and trim trailing spaces.
    const lines = (cfgEl.value || "").split(/\r?\n/);
    cfgEl.value = lines
      .map((l) => l.replace(/\t/g, "  ").replace(/[ \t]+$/g, ""))
      .join("\n")
      .replace(/\n{4,}/g, "\n\n\n");
    setMsg(cfgMsg, "已做简单整理（不保证严格 YAML 格式化）", false);
  }

  // Legacy parser kept only for compatibility with older embedded pages.
  function parseYamlToGraphical(yamlText) {
    try {
      const lines = yamlText.split('\n');
      configData = {
        auth: { user: "", pass: "" },
        channels: [],
        channel_groups: [],
        default_channel: "",
        push_token: { enabled: false, token: "" },
        webhooks: { tawk: { chan: "", title: "", secret: "" } },
        sqlite: { path: "", cleanup_days: 30, cleanup_interval_hours: 24, record_channel_messages: false },
        security: {
          auth_fail_limit: 5,
          auth_ban_minutes: 10,
          token_fail_limit: 10,
          token_ban_minutes: 10,
          ip_ban_max_entries: 10000,
          ip_ban_cleanup_seconds: 60,
          ip_ban_idle_minutes: 60
        }
      };

      let currentSection = null;
      let currentChannel = null;
      let currentGroup = null;

      for (let line of lines) {
        const trimmed = line.trim();
        if (!trimmed || trimmed.startsWith('#')) continue;

        // Top-level sections
        if (line.startsWith('channels:')) {
          currentSection = 'channels';
          continue;
        }
        if (line.startsWith('channel_groups:')) {
          currentSection = 'channel_groups';
          continue;
        }
        if (line.startsWith('auth:')) {
          currentSection = 'auth';
          continue;
        }
        if (line.startsWith('push_token:')) {
          currentSection = 'push_token';
          continue;
        }
        if (line.startsWith('webhooks:')) {
          currentSection = 'webhooks';
          continue;
        }
        if (line.startsWith('security:')) {
          currentSection = 'security';
          continue;
        }
        if (line.startsWith('sqlite:')) {
          currentSection = 'sqlite';
          continue;
        }
        if (line.startsWith('default_channel:')) {
          configData.default_channel = trimmed.split(':')[1].split('#')[0].trim();
          continue;
        }

        // Parse sections
        if (currentSection === 'channels') {
          if (line.match(/^\s+- name:/)) {
            if (currentChannel) configData.channels.push(currentChannel);
            currentChannel = { name: trimmed.split(':')[1].split('#')[0].trim(), type: "", token: "" };
          } else if (currentChannel && line.match(/^\s+type:/)) {
            currentChannel.type = trimmed.split(':')[1].split('#')[0].trim();
          } else if (currentChannel && line.match(/^\s+token:/)) {
            const tokenPart = trimmed.split(':').slice(1).join(':').split('#')[0].trim().replace(/^[\"']|[\"']$/g, '');
            currentChannel.token = tokenPart;
          }
        } else if (currentSection === 'channel_groups') {
          if (line.match(/^\s+- name:/)) {
            if (currentGroup) configData.channel_groups.push(currentGroup);
            currentGroup = { name: trimmed.split(':')[1].split('#')[0].trim(), use: [] };
          } else if (currentGroup && line.match(/^\s+use:/)) {
            continue;
          } else if (currentGroup && line.match(/^\s+- /)) {
            const channelName = trimmed.substring(2).split('#')[0].trim();
            currentGroup.use.push(channelName);
          }
        } else if (currentSection === 'auth') {
          if (line.match(/^\s+user:/)) {
            configData.auth.user = trimmed.split(':')[1].split('#')[0].trim();
          } else if (line.match(/^\s+pass:/)) {
            configData.auth.pass = trimmed.split(':')[1].split('#')[0].trim();
          }
        } else if (currentSection === 'push_token') {
          if (line.match(/^\s+enabled:/)) {
            const val = trimmed.split(':')[1].split('#')[0].trim().toLowerCase();
            configData.push_token.enabled = val === 'true';
          } else if (line.match(/^\s+token:/)) {
            const tokenVal = trimmed.split(':')[1].split('#')[0].trim().replace(/^[\"']|[\"']$/g, '');
            configData.push_token.token = tokenVal;
          }
        } else if (currentSection === 'webhooks') {
          if (line.match(/^\s+tawk:/)) {
            configData.webhooks.tawk = configData.webhooks.tawk || { chan: "", title: "", secret: "" };
          } else if (line.match(/^\s+chan:/)) {
            configData.webhooks.tawk.chan = trimmed.split(':').slice(1).join(':').split('#')[0].trim().replace(/^[\"']|[\"']$/g, '');
          } else if (line.match(/^\s+title:/)) {
            configData.webhooks.tawk.title = trimmed.split(':').slice(1).join(':').split('#')[0].trim().replace(/^[\"']|[\"']$/g, '');
          } else if (line.match(/^\s+secret:/)) {
            configData.webhooks.tawk.secret = trimmed.split(':').slice(1).join(':').split('#')[0].trim().replace(/^[\"']|[\"']$/g, '');
          }
        } else if (currentSection === 'sqlite') {
          if (line.match(/^\s+path:/)) {
            configData.sqlite.path = trimmed.split(':')[1].split('#')[0].trim();
          } else if (line.match(/^\s+cleanup_days:/)) {
            const val = trimmed.split(':')[1].split('#')[0].trim();
            configData.sqlite.cleanup_days = parseInt(val || "0", 10) || 0;
          } else if (line.match(/^\s+cleanup_interval_hours:/)) {
            const val = trimmed.split(':')[1].split('#')[0].trim();
            configData.sqlite.cleanup_interval_hours = parseInt(val || "0", 10) || 0;
          } else if (line.match(/^\s+record_channel_messages:/)) {
            const val = trimmed.split(':')[1].split('#')[0].trim().toLowerCase();
            configData.sqlite.record_channel_messages = val === "true";
          }
        } else if (currentSection === 'security') {
          if (line.match(/^\s+auth_fail_limit:/)) {
            const val = trimmed.split(':')[1].split('#')[0].trim();
            configData.security.auth_fail_limit = parseInt(val || "0", 10) || 0;
          } else if (line.match(/^\s+auth_ban_minutes:/)) {
            const val = trimmed.split(':')[1].split('#')[0].trim();
            configData.security.auth_ban_minutes = parseInt(val || "0", 10) || 0;
          } else if (line.match(/^\s+token_fail_limit:/)) {
            const val = trimmed.split(':')[1].split('#')[0].trim();
            configData.security.token_fail_limit = parseInt(val || "0", 10) || 0;
          } else if (line.match(/^\s+token_ban_minutes:/)) {
            const val = trimmed.split(':')[1].split('#')[0].trim();
            configData.security.token_ban_minutes = parseInt(val || "0", 10) || 0;
          } else if (line.match(/^\s+ip_ban_max_entries:/)) {
            const val = trimmed.split(':')[1].split('#')[0].trim();
            configData.security.ip_ban_max_entries = parseInt(val || "0", 10) || 0;
          } else if (line.match(/^\s+ip_ban_cleanup_seconds:/)) {
            const val = trimmed.split(':')[1].split('#')[0].trim();
            configData.security.ip_ban_cleanup_seconds = parseInt(val || "0", 10) || 0;
          } else if (line.match(/^\s+ip_ban_idle_minutes:/)) {
            const val = trimmed.split(':')[1].split('#')[0].trim();
            configData.security.ip_ban_idle_minutes = parseInt(val || "0", 10) || 0;
          }
        }
      }

      // Push last items
      if (currentChannel) configData.channels.push(currentChannel);
      if (currentGroup) configData.channel_groups.push(currentGroup);

      renderGraphicalConfig();
    } catch (err) {
      console.error('Failed to parse YAML:', err);
      setMsg(cfgMsg, `解析配置失败: ${err.message}`, true);
    }
  }

  function renderGraphicalConfig() {
    configData.channels = Array.isArray(configData.channels) ? configData.channels : [];
    configData.channel_groups = Array.isArray(configData.channel_groups) ? configData.channel_groups : [];
    configData.auth ||= { user: "", pass: "" };
    configData.push_token ||= { enabled: false, token: "" };
    configData.webhooks ||= { tawk: {} };
    configData.webhooks.tawk ||= {};
    configData.sqlite ||= {};
    configData.security ||= {};

    // Render auth
    $("cfgAuthUser").value = configData.auth.user || "";
    $("cfgAuthPass").value = configData.auth.pass || "";
    renderTargetSelect($("cfgDefaultChannel"), configData.default_channel, "请选择默认发送目标");
    $("cfgSqlitePath").value = configData.sqlite?.path || "";
    $("cfgSqliteCleanupDays").value = String(configData.sqlite?.cleanup_days ?? 0);
    $("cfgSqliteCleanupHours").value = String(configData.sqlite?.cleanup_interval_hours ?? 0);
    $("cfgSqliteRecordMessages").value = configData.sqlite?.record_channel_messages ? "true" : "false";

    // Render push_token
    $("cfgPushTokenEnabled").value = configData.push_token?.enabled ? "true" : "false";
    $("cfgPushTokenToken").value = configData.push_token?.token || "";
    renderTargetSelect($("cfgTawkChan"), configData.webhooks?.tawk?.chan, "使用默认发送目标");
    $("cfgTawkTitle").value = configData.webhooks?.tawk?.title || "";
    $("cfgTawkSecret").value = configData.webhooks?.tawk?.secret || "";

    // Render security
    $("cfgAuthFailLimit").value = String(configData.security?.auth_fail_limit ?? 0);
    $("cfgAuthBanMinutes").value = String(configData.security?.auth_ban_minutes ?? 0);
    $("cfgTokenFailLimit").value = String(configData.security?.token_fail_limit ?? 0);
    $("cfgTokenBanMinutes").value = String(configData.security?.token_ban_minutes ?? 0);
    $("cfgIPBanMaxEntries").value = String(configData.security?.ip_ban_max_entries ?? 0);
    $("cfgIPBanCleanupSeconds").value = String(configData.security?.ip_ban_cleanup_seconds ?? 0);
    $("cfgIPBanIdleMinutes").value = String(configData.security?.ip_ban_idle_minutes ?? 0);

    // Show/hide token hint in send test section
    const tokenHint = $("tokenHint");
    if (configData.push_token?.enabled && configData.push_token?.token) {
      const firstChannel = configData.channels.find((item) => item.name)?.name;
      const firstGroup = configData.channel_groups.find((item) => item.name)?.name;
      const sampleTarget = firstChannel || firstGroup || "TARGET_NAME";
      const sampleURL = `${window.location.origin}/send?token=${encodeURIComponent(configData.push_token.token)}&text=Hello&desp=World`;
      tokenHint.style.display = "block";
      tokenHint.innerHTML = `<strong>已启用 Token 验证</strong>
        <div class="token-examples">
          <code>${escapeHTML(sampleURL)}</code>
          <code>${escapeHTML(sampleURL)}&amp;chan=${escapeHTML(encodeURIComponent(sampleTarget))}</code>
        </div>
        <span>第一条使用默认发送目标；第二条指定${firstChannel ? "单个通知通道" : "发送目标组"} ${escapeHTML(sampleTarget)}。Token 可放在查询参数或表单字段中；JSON 请求请放在 URL 查询参数中。</span>`;
    } else {
      tokenHint.style.display = "none";
    }

    // Render channels
    const channelsList = $("channelsList");
    channelsList.innerHTML = "";
    configData.channels.forEach((channel, idx) => {
      const channelEl = document.createElement("div");
      channelEl.className = "channel-item";
      channelEl.innerHTML = `
        <div class="channel-header">
          <div class="channel-title" id="channelTitle${idx}">通知通道 ${escapeHTML(channel.name) || `#${idx + 1}`}</div>
          <button class="btn btn--danger btn--small" onclick="window.removeChannel(${idx})">🗑️ 删除</button>
        </div>
        <div class="channel-fields">
          <label class="field">
            <div class="field__label">名称 (name)</div>
            <input class="input" value="${escapeHTML(channel.name)}" oninput="window.renameChannel(${idx}, this.value)" placeholder="例如：telegram_work" />
          </label>
          <label class="field">
            <div class="field__label">类型 (type)</div>
            <select class="input" onchange="window.updateChannel(${idx}, 'type', this.value)">
              <option value="">选择类型...</option>
              <option value="telegram" ${channel.type === 'telegram' ? 'selected' : ''}>Telegram</option>
              <option value="bark" ${channel.type === 'bark' ? 'selected' : ''}>Bark</option>
              <option value="dingtalk" ${channel.type === 'dingtalk' ? 'selected' : ''}>钉钉 (DingTalk)</option>
              <option value="wecom" ${channel.type === 'wecom' ? 'selected' : ''}>企业微信 (WeCom)</option>
              <option value="wecombot" ${channel.type === 'wecombot' ? 'selected' : ''}>企业微信机器人 (WeCom Bot)</option>
              <option value="igot" ${channel.type === 'igot' ? 'selected' : ''}>iGot</option>
              <option value="pushplus" ${channel.type === 'pushplus' ? 'selected' : ''}>PushPlus</option>
              <option value="pushplushxtrip" ${channel.type === 'pushplushxtrip' ? 'selected' : ''}>PushPlus HXTrip</option>
              <option value="feishu" ${channel.type === 'feishu' ? 'selected' : ''}>飞书 (Feishu)</option>
              <option value="serverchan" ${channel.type === 'serverchan' ? 'selected' : ''}>ServerChan</option>
              <option value="serverchain" ${channel.type === 'serverchain' ? 'selected' : ''}>ServerChan (serverchain)</option>
              <option value="qmsg" ${channel.type === 'qmsg' ? 'selected' : ''}>Qmsg</option>
              <option value="pushdeer" ${channel.type === 'pushdeer' ? 'selected' : ''}>PushDeer</option>
              <option value="ifttt" ${channel.type === 'ifttt' ? 'selected' : ''}>IFTTT</option>
              <option value="gocqhttp" ${channel.type === 'gocqhttp' ? 'selected' : ''}>Go-CQHTTP</option>
              <option value="atri" ${channel.type === 'atri' ? 'selected' : ''}>Atri</option>
              <option value="discord" ${channel.type === 'discord' ? 'selected' : ''}>Discord</option>
              <option value="wxpusher" ${channel.type === 'wxpusher' ? 'selected' : ''}>WxPusher</option>
              <option value="webhook" ${channel.type === 'webhook' ? 'selected' : ''}>Webhook</option>
              <option value="stub" ${channel.type === 'stub' ? 'selected' : ''}>Stub (测试)</option>
            </select>
          </label>
          <label class="field" style="grid-column: 1 / -1;">
            <div class="field__label">Token / URL</div>
            <input class="input" value="${escapeHTML(channel.token)}" onchange="window.updateChannel(${idx}, 'token', this.value)" />
          </label>
        </div>
      `;
      channelsList.appendChild(channelEl);
    });

    renderGroups();
    renderSendTargets();

    updateApiExamples();
    updateDbMaintenanceVisibility();
  }

  function renderTargetSelect(select, value, emptyLabel) {
    const channels = configData.channels.filter((item) => item.name);
    const groups = configData.channel_groups.filter((item) => item.name);
    select.innerHTML = `<option value="">${emptyLabel}</option>`;
    if (channels.length) {
      select.insertAdjacentHTML("beforeend", `<optgroup label="单个通知通道">${channels.map((item) =>
        `<option value="${escapeHTML(item.name)}">${escapeHTML(item.name)}</option>`).join("")}</optgroup>`);
    }
    if (groups.length) {
      select.insertAdjacentHTML("beforeend", `<optgroup label="发送目标组（同时发送到多个通道）">${groups.map((item) =>
        `<option value="${escapeHTML(item.name)}">${escapeHTML(item.name)}</option>`).join("")}</optgroup>`);
    }
    if (value && ![...channels, ...groups].some((item) => item.name === value)) {
      select.insertAdjacentHTML("beforeend", `<option value="${escapeHTML(value)}">${escapeHTML(value)}（目标不存在，请重新选择）</option>`);
    }
    select.value = value || "";
  }

  function renderGroups() {
    const groupsList = $("groupsList");
    groupsList.innerHTML = "";
    configData.channel_groups.forEach((group, idx) => {
      const groupEl = document.createElement("div");
      groupEl.className = "group-item";
      groupEl.innerHTML = `
        <div class="group-header">
          <div class="group-title" id="groupTitle${idx}">发送目标组 ${escapeHTML(group.name) || `#${idx + 1}`}</div>
          <button class="btn btn--danger btn--small" onclick="window.removeGroup(${idx})">🗑️ 删除</button>
        </div>
        <div class="group-fields">
          <label class="field" style="grid-column: 1 / -1;">
            <div class="field__label">组名 (name)</div>
            <input class="input" value="${escapeHTML(group.name)}" oninput="window.renameGroup(${idx}, this.value)" placeholder="例如：all_devices" />
          </label>
        </div>
        <div class="group-channels">
          <div class="group-channels-label">选择该组包含的通知通道</div>
          <div class="channel-checklist">
            ${configData.channels.filter((ch) => ch.name).map((ch) => `
              <label class="channel-choice">
                <input type="checkbox" ${group.use.includes(ch.name) ? 'checked' : ''}
                  onchange="window.toggleGroupChannel(${idx}, ${configData.channels.indexOf(ch)}, this.checked)" />
                <span>${escapeHTML(ch.name)}</span>
                <small>${escapeHTML(ch.type || '未选择类型')}</small>
              </label>
            `).join('') || '<div class="empty-state">请先在上方添加并命名通知通道</div>'}
          </div>
        </div>
      `;
      groupsList.appendChild(groupEl);
    });

  }

  function renderSendTargets() {
    const container = $("sendTargets");
    if (!container) return;
    const targets = [
      ...configData.channels.filter((item) => item.name).map((item) => ({ ...item, kind: "单个通知通道" })),
      ...configData.channel_groups.filter((item) => item.name).map((item) => ({ ...item, kind: "发送目标组" }))
    ];
    container.innerHTML = targets.map((target, idx) => `
      <label class="channel-choice">
        <input type="checkbox" data-send-target="${escapeHTML(target.name)}" />
        <span>${escapeHTML(target.name)}</span>
        <small>${target.kind}</small>
      </label>
    `).join("") || '<div class="empty-state">尚未配置可用的发送目标</div>';
  }

  // Global functions for inline event handlers
  window.updateChannel = (idx, field, value) => {
    if (configData.channels[idx]) {
      configData.channels[idx][field] = value;
      if (field === "type") renderGroups();
    }
  };

  window.renameChannel = (idx, value) => {
    const channel = configData.channels[idx];
    if (!channel) return;
    const oldName = channel.name;
    channel.name = value.trim();
    if (oldName && oldName !== channel.name) {
      configData.channel_groups.forEach((group) => {
        group.use = group.use.map((name) => name === oldName ? channel.name : name).filter(Boolean);
      });
      if (configData.default_channel === oldName) configData.default_channel = channel.name;
      if (configData.webhooks.tawk.chan === oldName) configData.webhooks.tawk.chan = channel.name;
    }
    $(`channelTitle${idx}`).textContent = `通知通道 ${channel.name || `#${idx + 1}`}`;
    renderGroups();
    renderTargetSelect($("cfgDefaultChannel"), configData.default_channel, "请选择默认发送目标");
    renderTargetSelect($("cfgTawkChan"), configData.webhooks.tawk.chan, "使用默认发送目标");
    renderSendTargets();
    updateApiExamples();
  };

  window.removeChannel = (idx) => {
    const name = configData.channels[idx]?.name;
    const usedBy = configData.channel_groups.filter((group) => group.use.includes(name)).map((group) => group.name);
    if (usedBy.length) {
      setMsg(cfgMsg, `无法删除：通知通道 ${name} 正被发送目标组 ${usedBy.join("、")} 使用`, true);
      return;
    }
    configData.channels.splice(idx, 1);
    if (configData.default_channel === name) configData.default_channel = "";
    renderGraphicalConfig();
    setConfigDirty(true);
  };

  window.updateGroup = (idx, field, value) => {
    if (configData.channel_groups[idx]) {
      configData.channel_groups[idx][field] = value;
    }
  };

  window.renameGroup = (idx, value) => {
    const group = configData.channel_groups[idx];
    if (!group) return;
    const oldName = group.name;
    group.name = value.trim();
    if (configData.default_channel === oldName) configData.default_channel = group.name;
    if (configData.webhooks.tawk.chan === oldName) configData.webhooks.tawk.chan = group.name;
    $(`groupTitle${idx}`).textContent = `发送目标组 ${group.name || `#${idx + 1}`}`;
    renderTargetSelect($("cfgDefaultChannel"), configData.default_channel, "请选择默认发送目标");
    renderTargetSelect($("cfgTawkChan"), configData.webhooks.tawk.chan, "使用默认发送目标");
    renderSendTargets();
    updateApiExamples();
  };

  window.removeGroup = (idx) => {
    const name = configData.channel_groups[idx]?.name;
    configData.channel_groups.splice(idx, 1);
    if (configData.default_channel === name) configData.default_channel = "";
    renderGraphicalConfig();
    setConfigDirty(true);
  };

  window.toggleGroupChannel = (groupIdx, channelIdx, checked) => {
    const group = configData.channel_groups[groupIdx];
    const channelName = configData.channels[channelIdx]?.name;
    if (!group || !channelName) return;
    group.use = group.use.filter((name) => name !== channelName);
    if (checked) group.use.push(channelName);
  };

  function addChannel() {
    configData.channels.push({ name: "", type: "", token: "" });
    renderGraphicalConfig();
    setConfigDirty(true);
  }

  function addGroup() {
    configData.channel_groups.push({ name: "", use: [] });
    renderGraphicalConfig();
    setConfigDirty(true);
  }

  function graphicalToYaml() {
    // Update config from form
    configData.auth.user = $("cfgAuthUser").value.trim();
    configData.auth.pass = $("cfgAuthPass").value.trim();
    configData.default_channel = $("cfgDefaultChannel").value.trim();
    configData.push_token.enabled = $("cfgPushTokenEnabled").value === "true";
    configData.push_token.token = $("cfgPushTokenToken").value.trim();
    configData.webhooks = configData.webhooks || { tawk: { chan: "", title: "", secret: "" } };
    configData.webhooks.tawk = configData.webhooks.tawk || { chan: "", title: "", secret: "" };
    configData.webhooks.tawk.chan = $("cfgTawkChan").value.trim();
    configData.webhooks.tawk.title = $("cfgTawkTitle").value.trim();
    configData.webhooks.tawk.secret = $("cfgTawkSecret").value.trim();
    configData.sqlite.path = $("cfgSqlitePath").value.trim();
    configData.sqlite.cleanup_days = parseInt($("cfgSqliteCleanupDays").value.trim() || "0", 10) || 0;
    configData.sqlite.cleanup_interval_hours = parseInt($("cfgSqliteCleanupHours").value.trim() || "0", 10) || 0;
    configData.sqlite.record_channel_messages = $("cfgSqliteRecordMessages").value === "true";
    configData.security.auth_fail_limit = parseInt($("cfgAuthFailLimit").value.trim() || "0", 10) || 0;
    configData.security.auth_ban_minutes = parseInt($("cfgAuthBanMinutes").value.trim() || "0", 10) || 0;
    configData.security.token_fail_limit = parseInt($("cfgTokenFailLimit").value.trim() || "0", 10) || 0;
    configData.security.token_ban_minutes = parseInt($("cfgTokenBanMinutes").value.trim() || "0", 10) || 0;
    configData.security.ip_ban_max_entries = parseInt($("cfgIPBanMaxEntries").value.trim() || "0", 10) || 0;
    configData.security.ip_ban_cleanup_seconds = parseInt($("cfgIPBanCleanupSeconds").value.trim() || "0", 10) || 0;
    configData.security.ip_ban_idle_minutes = parseInt($("cfgIPBanIdleMinutes").value.trim() || "0", 10) || 0;

    let yaml = "channels:\n";
    configData.channels.forEach(ch => {
      if (ch.name) {
        yaml += `  - name: ${ch.name}\n`;
        yaml += `    type: ${ch.type || 'telegram'}\n`;
        yaml += `    token: ${ch.token || ''}\n`;
      }
    });

    yaml += "\nchannel_groups:\n";
    configData.channel_groups.forEach(grp => {
      if (grp.name) {
        yaml += `  - name: ${grp.name}\n`;
        yaml += `    use:\n`;
        grp.use.forEach(ch => {
          yaml += `      - ${ch}\n`;
        });
      }
    });

    if (configData.default_channel) {
      yaml += `\ndefault_channel: ${configData.default_channel}\n`;
    }

    yaml += "\nauth:\n";
    yaml += `  user: ${configData.auth.user || 'admin'}\n`;
    yaml += `  pass: ${configData.auth.pass || 'yourpassword'}\n`;

    yaml += "\npush_token:\n";
    yaml += `  enabled: ${configData.push_token.enabled}\n`;
    yaml += `  token: ${configData.push_token.token || ''}\n`;

    if (configData.webhooks?.tawk?.chan || configData.webhooks?.tawk?.title || configData.webhooks?.tawk?.secret) {
      yaml += "\nwebhooks:\n";
      yaml += "  tawk:\n";
      if (configData.webhooks.tawk.chan) {
        yaml += `    chan: ${configData.webhooks.tawk.chan}\n`;
      }
      if (configData.webhooks.tawk.title) {
        yaml += `    title: ${configData.webhooks.tawk.title}\n`;
      }
      if (configData.webhooks.tawk.secret) {
        yaml += `    secret: ${configData.webhooks.tawk.secret}\n`;
      }
    }

    yaml += "\nsecurity:\n";
    yaml += `  auth_fail_limit: ${configData.security.auth_fail_limit || 0}\n`;
    yaml += `  auth_ban_minutes: ${configData.security.auth_ban_minutes || 0}\n`;
    yaml += `  token_fail_limit: ${configData.security.token_fail_limit || 0}\n`;
    yaml += `  token_ban_minutes: ${configData.security.token_ban_minutes || 0}\n`;
    yaml += `  ip_ban_max_entries: ${configData.security.ip_ban_max_entries || 0}\n`;
    yaml += `  ip_ban_cleanup_seconds: ${configData.security.ip_ban_cleanup_seconds || 0}\n`;
    yaml += `  ip_ban_idle_minutes: ${configData.security.ip_ban_idle_minutes || 0}\n`;

    if (configData.sqlite.path) {
      yaml += "\nsqlite:\n";
      yaml += `  path: ${configData.sqlite.path}\n`;
      yaml += `  cleanup_days: ${configData.sqlite.cleanup_days || 0}\n`;
      yaml += `  cleanup_interval_hours: ${configData.sqlite.cleanup_interval_hours || 0}\n`;
      yaml += `  record_channel_messages: ${configData.sqlite.record_channel_messages}\n`;
    }

    return yaml;
  }

  async function saveGraphicalConfig() {
    if (!configLoaded) throw new Error("配置尚未成功加载，已阻止保存");
    graphicalToYaml(); // Collect the current form values into configData.
    const names = [...configData.channels, ...configData.channel_groups].map((item) => item.name.trim());
    if (names.some((name) => !name)) throw new Error("通知通道和发送目标组都必须填写名称");
    if (new Set(names).size !== names.length) throw new Error("通知通道与发送目标组的名称不能重复");
    if (configData.channels.some((item) => !item.type)) throw new Error("每个通知通道都必须选择服务类型");
    if (!configData.default_channel) throw new Error("请选择默认发送目标");
    if (!configData.auth.user || !configData.auth.pass) throw new Error("管理用户名和密码不能为空");

    setMsg(cfgMsg, "保存中...", false, true);
    const resp = await fetch("/config/data", {
      method: "POST",
      headers: {
        "content-type": "application/json",
        "If-Match": configRevision
      },
      body: JSON.stringify(configData)
    });
    const text = await resp.text();
    if (!resp.ok) {
      throw new Error(resp.status === 409
        ? "服务器配置已被修改，请重新加载后再编辑"
        : `保存失败 (${resp.status}): ${text}`);
    }
    configRevision = resp.headers.get("X-Config-Revision") || configRevision;
    await downloadConfig();
    setMsg(cfgMsg, "配置已保存并重新加载", false);
  }

  async function reloadGraphicalConfig() {
    if (configDirty && !window.confirm("重新加载会放弃当前未保存的更改，是否继续？")) return;
    await downloadConfig();
  }

  // Tab switching
  async function switchTab(tab) {
    if (tab === activeConfigTab) return;
    if (configDirty) {
      if (!window.confirm("切换编辑模式会放弃当前未保存的更改，是否继续？")) return;
      await downloadConfig();
    }
    activeConfigTab = tab;
    const tabGraphical = $("tabGraphical");
    const tabYaml = $("tabYaml");
    const panelGraphical = $("panelGraphical");
    const panelYaml = $("panelYaml");

    if (tab === 'graphical') {
      tabGraphical.classList.add('tab--active');
      tabYaml.classList.remove('tab--active');
      panelGraphical.style.display = 'block';
      panelYaml.style.display = 'none';
    } else {
      tabGraphical.classList.remove('tab--active');
      tabYaml.classList.add('tab--active');
      panelGraphical.style.display = 'none';
      panelYaml.style.display = 'block';
    }
  }

  async function send(method) {
    const chan = Array.from(document.querySelectorAll("[data-send-target]:checked"))
      .map((item) => item.dataset.sendTarget).join(",");
    const charset = $("sendCharset").value || "";
    const text = $("sendText").value || "";
    const desp = $("sendDesp").value || "";
    const out = $("sendResp");
    out.innerHTML = '<span class="spinner"></span>发送中...';
    out.className = "pre";

    try {
      const qs = new URLSearchParams();
      if (chan) qs.set("chan", chan);
      if (charset) qs.set("charset", charset);
      if (text) qs.set("text", text);
      if (desp) qs.set("desp", desp);

      let resp;
      if (method === "GET") {
        resp = await fetch(`/send?${qs.toString()}`);
      } else {
        // Use x-www-form-urlencoded to keep it compatible with existing usage.
        resp = await fetch(`/send${charset ? `?charset=${encodeURIComponent(charset)}` : ""}`, {
          method: "POST",
          headers: { "content-type": "application/x-www-form-urlencoded; charset=utf-8" },
          body: qs.toString(),
        });
      }

      const banMsg = formatBanMessage(resp);
      const txt = await resp.text();
      if (banMsg) {
        out.textContent = `HTTP ${resp.status}\n${banMsg}`;
        out.className = "pre";
        return;
      }
      out.textContent = `HTTP ${resp.status}\n${txt}`;
      out.className = resp.ok ? "pre fade-in" : "pre fade-in";
    } catch (err) {
      out.textContent = `错误: ${err.message}`;
      out.className = "pre";
    }
  }

  let es = null;
  function eventsOn() {
    if (es) return;
    const out = $("events");
    out.textContent = out.textContent || "connecting...\n";
    es = new EventSource("/api/events");
    es.addEventListener("push", (e) => {
      out.textContent += `${e.data}\n`;
      out.scrollTop = out.scrollHeight;
    });
    es.onerror = () => {
      out.textContent += "event stream error\n";
    };
  }
  function eventsOff() {
    if (!es) return;
    es.close();
    es = null;
    const out = $("events");
    out.textContent += "disconnected\n";
  }

  async function refreshBanStats() {
    const out = $("banStatsOut");
    out.innerHTML = '<span class="spinner"></span>加载中...';
    out.className = "pre pre--fixed";
    try {
      const resp = await fetch("/api/security/ban_stats?limit=50");
      const txt = await resp.text();
      if (!resp.ok) {
        out.textContent = `HTTP ${resp.status}\n${txt}`;
        return;
      }
      const data = JSON.parse(txt);
      const auth = data.auth || {};
      const token = data.token || {};
      $("banAuthTotal").textContent = String(auth.total_entries ?? "-");
      $("banAuthBanned").textContent = String(auth.banned_entries ?? "-");
      $("banTokenTotal").textContent = String(token.total_entries ?? "-");
      $("banTokenBanned").textContent = String(token.banned_entries ?? "-");
      $("banMaxEntries").textContent = String(auth.max_entries ?? token.max_entries ?? "-");
      const bytes = auth.estimated_bytes ?? 0;
      const per = auth.estimated_bytes_per_ip ?? 0;
      $("banEstimatedBytes").textContent = bytes ? `${bytes} B (~${per} B/IP)` : "-";

      const authIps = (auth.sample_ips || []).map((ip) => ({
        ip: ip.ip,
        banned: ip.banned,
        until: ip.banned_until,
        last: ip.last_seen
      }));
      const tokenIps = (token.sample_ips || []).map((ip) => ({
        ip: ip.ip,
        banned: ip.banned,
        until: ip.banned_until,
        last: ip.last_seen
      }));
      const renderList = (label, items, truncated) => {
        const lines = items.map((it) => {
          const status = it.banned ? "BANNED" : "OK";
          const until = it.until ? ` until=${it.until}` : "";
          return `${status} ${it.ip} last=${it.last}${until}`;
        });
        const suffix = truncated ? " (truncated)" : "";
        return [`[${label}]${suffix}`, ...lines, ""].join("\n");
      };
      const parts = [];
      parts.push(renderList("auth", authIps, auth.sample_truncated));
      parts.push(renderList("token", tokenIps, token.sample_truncated));
      out.textContent = parts.join("\n");
      refreshBanTrends().catch(() => {});
    } catch (err) {
      out.textContent = `错误: ${err.message}`;
    }
  }

  let lastOverviewFetch = 0;
  async function refreshOverview() {
    const now = Date.now();
    if (now - lastOverviewFetch < 10000) return;
    lastOverviewFetch = now;

    const healthEl = $("ovHealth");
    const sqliteEl = $("ovSqlite");
    const authEl = $("ovAuthPool");
    const tokenEl = $("ovTokenPool");
    const maxEl = $("ovMaxEntries");
    const updatedEl = $("ovUpdated");
    const notifyTotalEl = $("ovNotifyTotal");
    const lastSentEl = $("ovLastSent");
    const todaySentEl = $("ovTodaySent");
    const todayFailedEl = $("ovTodayFailed");
    if (!healthEl || !sqliteEl) return;

    const sqliteEnabled = Boolean(configData.sqlite?.path);
    sqliteEl.textContent = sqliteEnabled ? "已启用" : "未启用";
    updatedEl.textContent = new Date().toLocaleTimeString();

    try {
      const resp = await fetch("/api/health");
      if (resp.ok) {
        healthEl.textContent = "OK";
      } else {
        healthEl.textContent = `HTTP ${resp.status}`;
      }
    } catch {
      healthEl.textContent = "不可用";
    }

    try {
      const resp = await fetch("/api/security/ban_stats?limit=1");
      if (!resp.ok) {
        authEl.textContent = "不可用";
        tokenEl.textContent = "不可用";
        return;
      }
      const data = await resp.json();
      const auth = data.auth || {};
      const token = data.token || {};
      authEl.textContent = `${auth.total_entries ?? 0} / 封禁 ${auth.banned_entries ?? 0}`;
      tokenEl.textContent = `${token.total_entries ?? 0} / 封禁 ${token.banned_entries ?? 0}`;
      maxEl.textContent = String(auth.max_entries ?? token.max_entries ?? "-");
    } catch {
      authEl.textContent = "不可用";
      tokenEl.textContent = "不可用";
    }

    if (!notifyTotalEl || !lastSentEl) return;
    if (!sqliteEnabled) {
      notifyTotalEl.textContent = "未启用";
      lastSentEl.textContent = "未启用";
      if (todaySentEl) todaySentEl.textContent = "未启用";
      if (todayFailedEl) todayFailedEl.textContent = "未启用";
      return;
    }
    try {
      const resp = await fetch("/api/store/summary");
      if (!resp.ok) {
        notifyTotalEl.textContent = "不可用";
        lastSentEl.textContent = "不可用";
        if (todaySentEl) todaySentEl.textContent = "不可用";
        if (todayFailedEl) todayFailedEl.textContent = "不可用";
        return;
      }
      const data = await resp.json();
      notifyTotalEl.textContent = String(data.notification_total ?? 0);
      if (data.last_sent_at) {
        lastSentEl.textContent = formatTime(data.last_sent_at);
      } else {
        lastSentEl.textContent = "-";
      }
      if (todaySentEl) todaySentEl.textContent = String(data.today_sent ?? 0);
      if (todayFailedEl) todayFailedEl.textContent = String(data.today_failed ?? 0);
    } catch {
      notifyTotalEl.textContent = "不可用";
      lastSentEl.textContent = "不可用";
      if (todaySentEl) todaySentEl.textContent = "不可用";
      if (todayFailedEl) todayFailedEl.textContent = "不可用";
    }
  }

  async function refreshBanTrends() {
    const chart24 = $("banTrend24h");
    const chart7 = $("banTrend7d");
    if (!chart24 || !chart7) return;
    try {
      const resp = await fetch("/api/security/ban_trends");
      if (!resp.ok) {
        chart24.innerHTML = "<div class=\"hint\">趋势数据不可用</div>";
        chart7.innerHTML = "<div class=\"hint\">趋势数据不可用</div>";
        return;
      }
      const data = await resp.json();
      renderTrendChart(chart24, data.last_24h || [], 24);
      renderTrendChart(chart7, data.last_7d || [], 7);
    } catch (err) {
      chart24.innerHTML = "<div class=\"hint\">趋势加载失败</div>";
      chart7.innerHTML = "<div class=\"hint\">趋势加载失败</div>";
    }
  }

  function renderTrendChart(el, points, expected) {
    const items = Array.isArray(points) ? points : [];
    const max = items.reduce((m, p) => Math.max(m, p.count || 0), 0) || 1;
    const html = items.slice(0, expected).map((p, idx) => {
      const h = Math.max(6, Math.round((p.count || 0) / max * 80));
      const label = p.label || String(idx + 1);
      return `<div class="trend-bar" style="height:${h}px" title="${label}: ${p.count || 0}">
        <span class="trend-bar__label">${label}</span>
      </div>`;
    }).join("");
    el.innerHTML = html || "<div class=\"hint\">暂无数据</div>";
  }

  function updateDbMaintenanceVisibility() {
    const el = $("dbMaintenance");
    if (!el) return;
    const enabled = Boolean(configData.sqlite?.path);
    el.style.display = enabled ? "block" : "none";
    if (enabled) {
      const keep = $("dbKeepDays");
      if (keep && !keep.value) {
        keep.value = "30";
      }
    }
    const notif = $("dbNotifications");
    if (notif) {
      notif.style.display = enabled ? "block" : "none";
    }
  }

  async function compactDatabase() {
    const msg = $("dbMsg");
    setMsg(msg, "压缩中...", false, true);
    try {
      const resp = await fetch("/api/store/compact", { method: "POST" });
      const txt = await resp.text();
      if (!resp.ok) {
        setMsg(msg, `压缩失败: ${resp.status} ${txt}`, true);
        return;
      }
      setMsg(msg, "压缩完成", false);
    } catch (err) {
      setMsg(msg, `压缩失败: ${err.message}`, true);
    }
  }

  async function cleanupDatabase() {
    const msg = $("dbMsg");
    const keepDays = parseInt($("dbKeepDays").value.trim() || "30", 10);
    const days = Number.isNaN(keepDays) ? 30 : keepDays;
    setMsg(msg, "清理中...", false, true);
    try {
      const resp = await fetch(`/api/store/cleanup?keep_days=${days}`, { method: "POST" });
      const txt = await resp.text();
      if (!resp.ok) {
        setMsg(msg, `清理失败: ${resp.status} ${txt}`, true);
        return;
      }
      setMsg(msg, "清理完成", false);
      refreshBanStats().catch(() => {});
    } catch (err) {
      setMsg(msg, `清理失败: ${err.message}`, true);
    }
  }

  let notifPage = 1;
  const notifPageSize = 10;

  function formatTime(ts) {
    if (!ts) return "";
    const d = new Date(ts);
    if (Number.isNaN(d.getTime())) return String(ts);
    return d.toLocaleString();
  }

  async function refreshNotifications(page = 1) {
    const body = $("notifBody");
    const msg = $("notifMsg");
    if (!body || !msg) return;
    setMsg(msg, "加载中...", false, true);
    try {
      const resp = await fetch(`/api/store/notifications?page=${page}&page_size=${notifPageSize}`);
      const txt = await resp.text();
      if (!resp.ok) {
        setMsg(msg, `加载失败: ${resp.status} ${txt}`, true);
        return;
      }
      const data = JSON.parse(txt);
      notifPage = data.page || page;
      const total = data.total || 0;
      const totalPages = Math.max(1, Math.ceil(total / notifPageSize));
      $("notifPageInfo").textContent = `第 ${notifPage} 页 / 共 ${totalPages} 页（共 ${total} 条）`;
      $("notifPrev").disabled = notifPage <= 1;
      $("notifNext").disabled = notifPage >= totalPages;

      body.innerHTML = "";
      if (!data.items || data.items.length === 0) {
        const tr = document.createElement("tr");
        const td = document.createElement("td");
        td.colSpan = 6;
        td.textContent = "暂无记录";
        tr.appendChild(td);
        body.appendChild(tr);
        setMsg(msg, "", false);
        return;
      }
      data.items.forEach((it) => {
        const tr = document.createElement("tr");
        const tdTime = document.createElement("td");
        tdTime.textContent = formatTime(it.created_at);
        const tdIP = document.createElement("td");
        tdIP.textContent = it.remote_addr || "";
        const tdChan = document.createElement("td");
        tdChan.textContent = `${it.channel_name || ""} (${it.channel_type || ""})`;
        const tdTitle = document.createElement("td");
        tdTitle.textContent = it.title || "";
        const tdContent = document.createElement("td");
        tdContent.textContent = it.content || "";
        const tdStatus = document.createElement("td");
        tdStatus.textContent = it.status || "";
        tr.appendChild(tdTime);
        tr.appendChild(tdIP);
        tr.appendChild(tdChan);
        tr.appendChild(tdTitle);
        tr.appendChild(tdContent);
        tr.appendChild(tdStatus);
        body.appendChild(tr);
      });
      setMsg(msg, "", false);
    } catch (err) {
      setMsg(msg, `加载失败: ${err.message}`, true);
    }
  }

  // Update API examples with token
  function updateApiExamples() {
    const tokenEnabled = configData.push_token?.enabled;
    const token = configData.push_token?.token || "YOUR_TOKEN";
    const tokenParam = tokenEnabled ? `token=${token}&` : "";
    const tokenParamOnly = tokenEnabled ? `?token=${token}` : "";
    const baseURL = window.location.origin;
    const channelName = configData.channels.find((item) => item.name)?.name || "YOUR_CHANNEL";
    const groupName = configData.channel_groups.find((item) => item.name)?.name || "YOUR_CHANNEL_GROUP";
    const defaultName = configData.default_channel || "未配置";
    const authQuery = tokenEnabled ? `token=${encodeURIComponent(token)}&` : "";
    const makeURL = (target) => `${baseURL}/send?${authQuery}text=Hello&desp=World${target ? `&chan=${encodeURIComponent(target)}` : ""}`;

    $("exampleTargetDefault").textContent = makeURL("");
    $("exampleTargetChannel").textContent = makeURL(channelName);
    $("exampleTargetGroup").textContent = makeURL(groupName);
    $("exampleTargetMultiple").textContent = makeURL(`${channelName},${groupName}`);
    $("exampleTargetMeaning").textContent = `当前默认发送目标：${defaultName}。chan 接受通知通道名称、发送目标组名称，或以英文逗号分隔的多个名称；发送目标组会展开为其包含的通知通道，重复通道只发送一次。`;

    // Update cURL examples (token is accepted in query or form, not JSON body)
    $("curlGet").textContent = `curl "${makeURL(channelName)}"`;
    $("curlPostForm").textContent = `curl -X POST ${baseURL}/send \\
  -d "${tokenParam}text=Hello&desp=World&chan=${groupName}"`;
    $("curlPostJson").textContent = `curl -X POST "${baseURL}/send${tokenParamOnly}" \\
  -H "Content-Type: application/json" \\
  -d '{"text":"Hello","desp":"World","chan":"${channelName},${groupName}"}'`;
    $("curlBarkGetTitleBody").textContent = `curl "${baseURL}/bark/${encodeURIComponent(channelName)}/Hello/World${tokenParamOnly}"`;
    $("curlBarkPostForm").textContent = `curl -X POST ${baseURL}/bark/${encodeURIComponent(channelName)} \\
  -d "${tokenEnabled ? `token=${token}&` : ""}title=Hello&body=World"`;
    $("curlBarkV2").textContent = `curl -X POST "${baseURL}/barkv2${tokenParamOnly}" \\
  -H "Content-Type: application/json" \\
  -d '{"device_key":"${channelName}","title":"Hello","body":"World"}'`;

    // Update Python example (token must be in query or form for POST)
    const pythonTokenLine = tokenEnabled ? `    'token': '${token}',\n` : "";
    const pythonPostUrl = tokenEnabled ? `${baseURL}/send?token=${token}` : `${baseURL}/send`;
    const pythonBarkTitleBodyUrl = `${baseURL}/bark/${encodeURIComponent(channelName)}/Hello/World${tokenParamOnly}`;
    const pythonBarkPostFormUrl = `${baseURL}/bark/${encodeURIComponent(channelName)}`;
    const pythonBarkV2Url = `${baseURL}/barkv2${tokenParamOnly}`;
    $("pythonExample").textContent = `import requests

# GET 请求
response = requests.get('${baseURL}/send', params={
${pythonTokenLine}    'text': 'Hello',
    'desp': 'World',
    'chan': '${channelName}'
})

# POST 请求（表单）
response = requests.post('${baseURL}/send', data={
${pythonTokenLine}    'text': 'Hello',
    'desp': 'World',
    'chan': '${groupName}'
})

# POST 请求（JSON）
response = requests.post('${pythonPostUrl}', json={
    'text': 'Hello',
    'desp': 'World',
    'chan': '${channelName},${groupName}'
})

# Bark GET（标题/内容）
response = requests.get('${pythonBarkTitleBodyUrl}')

# Bark POST（表单）
response = requests.post('${pythonBarkPostFormUrl}', data={
${pythonTokenLine}    'title': 'Hello',
    'body': 'World'
})

# Bark v2 接口（JSON）
response = requests.post('${pythonBarkV2Url}', json={
    'device_key': '${channelName}',
    'title': 'Hello',
    'body': 'World'
})

print(response.json())`;

    // Update Go example
    const goTokenParamLine = tokenEnabled ? `  params.Set("token", "${token}")\n` : "";
    const goTokenFormLine = tokenEnabled ? `  form.Set("token", "${token}")\n` : "";
    const goTokenBarkFormLine = tokenEnabled ? `  barkForm.Set("token", "${token}")\n` : "";
    $("goExample").textContent = `package main

import (
  "bytes"
  "encoding/json"
  "fmt"
  "io"
  "net/http"
  "net/url"
)

func main() {
  // GET 请求
  params := url.Values{}
${goTokenParamLine}  params.Set("text", "Hello")
  params.Set("desp", "World")
  params.Set("chan", "${channelName}")

  resp, err := http.Get("${baseURL}/send?" + params.Encode())
  if err != nil {
    panic(err)
  }
  body, _ := io.ReadAll(resp.Body)
  _ = resp.Body.Close()
  fmt.Println("GET status:", resp.Status, string(body))

  // POST 请求（表单）
  form := url.Values{}
${goTokenFormLine}  form.Set("text", "Hello")
  form.Set("desp", "World")
  form.Set("chan", "${groupName}")
  resp, err = http.Post("${baseURL}/send", "application/x-www-form-urlencoded", bytes.NewBufferString(form.Encode()))
  if err != nil {
    panic(err)
  }
  body, _ = io.ReadAll(resp.Body)
  _ = resp.Body.Close()
  fmt.Println("POST form status:", resp.Status, string(body))

  // POST 请求（JSON）
  body, _ = json.Marshal(map[string]string{
    "text": "Hello",
    "desp": "World",
    "chan": "${channelName},${groupName}",
  })
  resp, err = http.Post("${baseURL}/send${tokenParamOnly}", "application/json", bytes.NewReader(body))
  if err != nil {
    panic(err)
  }
  body, _ = io.ReadAll(resp.Body)
  _ = resp.Body.Close()
  fmt.Println("POST json status:", resp.Status, string(body))

  // Bark GET（标题/内容）
  resp, err = http.Get("${baseURL}/bark/${encodeURIComponent(channelName)}/Hello/World${tokenParamOnly}")
  if err != nil {
    panic(err)
  }
  body, _ = io.ReadAll(resp.Body)
  _ = resp.Body.Close()
  fmt.Println("Bark get title/body status:", resp.Status, string(body))

  // Bark POST（表单）
  barkForm := url.Values{}
${goTokenBarkFormLine}  barkForm.Set("title", "Hello")
  barkForm.Set("body", "World")
  resp, err = http.Post("${baseURL}/bark/${encodeURIComponent(channelName)}", "application/x-www-form-urlencoded", bytes.NewBufferString(barkForm.Encode()))
  if err != nil {
    panic(err)
  }
  body, _ = io.ReadAll(resp.Body)
  _ = resp.Body.Close()
  fmt.Println("Bark post form status:", resp.Status, string(body))

  // Bark v2 接口（JSON）
  barkBody, _ := json.Marshal(map[string]string{
    "device_key": "${channelName}",
    "title": "Hello",
    "body": "World",
  })
  resp, err = http.Post("${baseURL}/barkv2${tokenParamOnly}", "application/json", bytes.NewReader(barkBody))
  if err != nil {
    panic(err)
  }
  body, _ = io.ReadAll(resp.Body)
  _ = resp.Body.Close()
  fmt.Println("Bark v2 status:", resp.Status, string(body))
}`;

    // Update JavaScript example (token must be in query or form for POST)
    const jsTokenLine = tokenEnabled ? `  token: '${token}',\n` : "";
    const jsBarkTitleBodyUrl = `${baseURL}/bark/${encodeURIComponent(channelName)}/Hello/World${tokenParamOnly}`;
    const jsBarkPostFormUrl = `${baseURL}/bark/${encodeURIComponent(channelName)}`;
    const jsBarkV2Url = `${baseURL}/barkv2${tokenParamOnly}`;
    $("jsExample").textContent = `// GET 请求
const params = new URLSearchParams({
${jsTokenLine}  text: 'Hello',
  desp: 'World',
  chan: '${channelName}'
});
fetch(\`${baseURL}/send?\${params}\`)
  .then(res => res.json())
  .then(data => console.log(data));

// POST 请求（表单）
const form = new URLSearchParams({
${jsTokenLine}  text: 'Hello',
  desp: 'World',
  chan: '${groupName}'
});
fetch('${baseURL}/send', {
  method: 'POST',
  headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
  body: form.toString()
})
  .then(res => res.json())
  .then(data => console.log(data));

// POST 请求（JSON）
fetch('${baseURL}/send${tokenParamOnly}', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    text: 'Hello',
    desp: 'World',
    chan: '${channelName},${groupName}'
  })
})
  .then(res => res.json())
  .then(data => console.log(data));

// Bark GET（标题/内容）
fetch('${jsBarkTitleBodyUrl}')
  .then(res => res.json())
  .then(data => console.log(data));

// Bark POST（表单）
const barkForm = new URLSearchParams({
${jsTokenLine}  title: 'Hello',
  body: 'World'
});
fetch('${jsBarkPostFormUrl}', {
  method: 'POST',
  headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
  body: barkForm.toString()
})
  .then(res => res.json())
  .then(data => console.log(data));

// Bark v2 接口（JSON）
fetch('${jsBarkV2Url}', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    device_key: '${channelName}',
    title: 'Hello',
    body: 'World'
  })
})
  .then(res => res.json())
  .then(data => console.log(data));`;

    // Show/hide token hint
    const tokenExampleHint = $("tokenExampleHint");
    if (tokenEnabled) {
      tokenExampleHint.style.display = "block";
    } else {
      tokenExampleHint.style.display = "none";
    }
  }

  // Example tab switching
  function switchExampleTab(tab) {
    const tabCurl = $("tabCurl");
    const tabPython = $("tabPython");
    const tabGo = $("tabGo");
    const tabJavaScript = $("tabJavaScript");
    const panelCurl = $("exampleCurl");
    const panelPython = $("examplePython");
    const panelGo = $("exampleGo");
    const panelJavaScript = $("exampleJavaScript");

    // Remove all active states
    tabCurl.classList.remove('tab--active');
    tabPython.classList.remove('tab--active');
    tabGo.classList.remove('tab--active');
    tabJavaScript.classList.remove('tab--active');
    panelCurl.style.display = 'none';
    panelPython.style.display = 'none';
    panelGo.style.display = 'none';
    panelJavaScript.style.display = 'none';

    // Activate selected tab
    if (tab === 'curl') {
      tabCurl.classList.add('tab--active');
      panelCurl.style.display = 'block';
    } else if (tab === 'python') {
      tabPython.classList.add('tab--active');
      panelPython.style.display = 'block';
    } else if (tab === 'go') {
      tabGo.classList.add('tab--active');
      panelGo.style.display = 'block';
    } else if (tab === 'javascript') {
      tabJavaScript.classList.add('tab--active');
      panelJavaScript.style.display = 'block';
    }
  }

  // Event listeners
  $("beautify").addEventListener("click", beautifyYAML);
  $("sendPost").addEventListener("click", () => send("POST"));
  $("sendGet").addEventListener("click", () => send("GET"));
  $("eventsOn").addEventListener("click", eventsOn);
  $("eventsOff").addEventListener("click", eventsOff);
  $("eventsClear").addEventListener("click", () => ($("events").textContent = ""));
  $("banStatsRefresh").addEventListener("click", () => refreshBanStats().catch((e) => console.error(e)));
  if ($("dbCompact")) {
    $("dbCompact").addEventListener("click", () => compactDatabase().catch((e) => console.error(e)));
  }
  if ($("dbCleanup")) {
    $("dbCleanup").addEventListener("click", () => cleanupDatabase().catch((e) => console.error(e)));
  }
  if ($("notifPrev")) {
    $("notifPrev").addEventListener("click", () => refreshNotifications(notifPage - 1).catch((e) => console.error(e)));
  }
  if ($("notifNext")) {
    $("notifNext").addEventListener("click", () => refreshNotifications(notifPage + 1).catch((e) => console.error(e)));
  }
  if ($("notifRefresh")) {
    $("notifRefresh").addEventListener("click", () => refreshNotifications(notifPage).catch((e) => console.error(e)));
  }

  // Graphical config event listeners
  $("tabGraphical").addEventListener("click", () => switchTab('graphical').catch((e) => setMsg(cfgMsg, String(e), true)));
  $("tabYaml").addEventListener("click", () => switchTab('yaml').catch((e) => setMsg(cfgMsg, String(e), true)));
  $("addChannel").addEventListener("click", addChannel);
  $("addGroup").addEventListener("click", addGroup);
  $("saveGraphical").addEventListener("click", () => saveGraphicalConfig().catch((e) => setMsg(cfgMsg, String(e), true)));
  $("reloadGraphical").addEventListener("click", () => reloadGraphicalConfig().catch((e) => setMsg(cfgMsg, String(e), true)));

  // YAML editor event listeners
  $("saveYaml").addEventListener("click", () => uploadConfig().catch((e) => setMsg(cfgMsg, String(e), true)));
  $("reloadYaml").addEventListener("click", () => reloadGraphicalConfig().catch((e) => setMsg(cfgMsg, String(e), true)));

  $("panelGraphical").addEventListener("input", () => setConfigDirty(true));
  $("panelGraphical").addEventListener("change", () => setConfigDirty(true));
  $("cfgDefaultChannel").addEventListener("change", (event) => {
    configData.default_channel = event.target.value;
    updateApiExamples();
  });
  $("cfgTawkChan").addEventListener("change", (event) => {
    configData.webhooks.tawk.chan = event.target.value;
  });
  $("cfgPushTokenEnabled").addEventListener("change", (event) => {
    configData.push_token.enabled = event.target.value === "true";
    updateApiExamples();
  });
  $("cfgPushTokenToken").addEventListener("input", (event) => {
    configData.push_token.token = event.target.value.trim();
    updateApiExamples();
  });
  cfgEl.addEventListener("input", () => setConfigDirty(true));
  window.addEventListener("beforeunload", (event) => {
    if (!configDirty) return;
    event.preventDefault();
    event.returnValue = "";
  });

  // Example tab event listeners
  $("tabCurl").addEventListener("click", () => switchExampleTab('curl'));
  $("tabPython").addEventListener("click", () => switchExampleTab('python'));
  $("tabGo").addEventListener("click", () => switchExampleTab('go'));
  $("tabJavaScript").addEventListener("click", () => switchExampleTab('javascript'));

  // Initialize - Auto-load config on page load
  setConfigReady(false);
  initPageNav();
  downloadConfig().then(() => {
    updateApiExamples();
    refreshBanStats().catch((e) => console.error("Failed to load ban stats:", e));
    updateDbMaintenanceVisibility();
    if (configData.sqlite?.path) {
      refreshNotifications(1).catch((e) => console.error("Failed to load notifications:", e));
    }
  }).catch((e) => console.error('Failed to auto-load config:', e));
})();
