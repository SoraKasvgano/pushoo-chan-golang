(() => {
  const $ = (id) => document.getElementById(id);
  const cfgEl = $("cfg");
  const cfgMsg = $("cfgMsg");

  // Configuration state
  let configData = {
    auth: { user: "", pass: "" },
    channels: [],
    channel_groups: [],
    default_channel: "",
    sqlite: { path: "" }
  };

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

  async function downloadConfig() {
    setMsg(cfgMsg, "下载中...", false, true);
    try {
      const resp = await fetch("/config/download");
      const txt = await resp.text();
      if (!resp.ok) {
        setMsg(cfgMsg, `下载失败: ${resp.status} ${txt}`, true);
        return;
      }
      cfgEl.value = txt;
      parseYamlToGraphical(txt);
      setMsg(cfgMsg, "✓ 下载完成", false);
    } catch (err) {
      setMsg(cfgMsg, `下载失败: ${err.message}`, true);
    }
  }

  async function uploadConfig() {
    setMsg(cfgMsg, "上传中...", false, true);
    try {
      const resp = await fetch("/config/upload", {
        method: "POST",
        headers: { "content-type": "text/plain; charset=utf-8" },
        body: cfgEl.value || "",
      });
      const txt = await resp.text();
      if (!resp.ok) {
        setMsg(cfgMsg, `上传失败: ${resp.status} ${txt}`, true);
        return;
      }
      setMsg(cfgMsg, "✓ 上传完成", false);
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
    setMsg(cfgMsg, "✓ 已做简单整理（不保证严格 YAML 格式化）", false);
  }

  // Simple YAML parser (basic implementation)
  function parseYamlToGraphical(yamlText) {
    try {
      const lines = yamlText.split('\n');
      configData = {
        auth: { user: "", pass: "" },
        channels: [],
        channel_groups: [],
        default_channel: "",
        sqlite: { path: "" }
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
        if (line.startsWith('sqlite:')) {
          currentSection = 'sqlite';
          continue;
        }
        if (line.startsWith('default_channel:')) {
          configData.default_channel = trimmed.split(':')[1].trim();
          continue;
        }

        // Parse sections
        if (currentSection === 'channels') {
          if (line.match(/^\s+- name:/)) {
            if (currentChannel) configData.channels.push(currentChannel);
            currentChannel = { name: trimmed.split(':')[1].trim(), type: "", token: "" };
          } else if (currentChannel && line.match(/^\s+type:/)) {
            currentChannel.type = trimmed.split(':')[1].trim();
          } else if (currentChannel && line.match(/^\s+token:/)) {
            currentChannel.token = trimmed.split(':').slice(1).join(':').trim().replace(/^["']|["']$/g, '');
          }
        } else if (currentSection === 'channel_groups') {
          if (line.match(/^\s+- name:/)) {
            if (currentGroup) configData.channel_groups.push(currentGroup);
            currentGroup = { name: trimmed.split(':')[1].trim(), use: [] };
          } else if (currentGroup && line.match(/^\s+use:/)) {
            continue;
          } else if (currentGroup && line.match(/^\s+- /)) {
            currentGroup.use.push(trimmed.substring(2).trim());
          }
        } else if (currentSection === 'auth') {
          if (line.match(/^\s+user:/)) {
            configData.auth.user = trimmed.split(':')[1].trim();
          } else if (line.match(/^\s+pass:/)) {
            configData.auth.pass = trimmed.split(':')[1].trim();
          }
        } else if (currentSection === 'sqlite') {
          if (line.match(/^\s+path:/)) {
            configData.sqlite.path = trimmed.split(':')[1].trim();
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
    // Render auth
    $("cfgAuthUser").value = configData.auth.user || "";
    $("cfgAuthPass").value = configData.auth.pass || "";
    $("cfgDefaultChannel").value = configData.default_channel || "";
    $("cfgSqlitePath").value = configData.sqlite?.path || "";

    // Render channels
    const channelsList = $("channelsList");
    channelsList.innerHTML = "";
    configData.channels.forEach((channel, idx) => {
      const channelEl = document.createElement("div");
      channelEl.className = "channel-item";
      channelEl.innerHTML = `
        <div class="channel-header">
          <div class="channel-title">通道 #${idx + 1}</div>
          <button class="btn btn--danger btn--small" onclick="window.removeChannel(${idx})">🗑️ 删除</button>
        </div>
        <div class="channel-fields">
          <label class="field">
            <div class="field__label">名称 (name)</div>
            <input class="input" value="${channel.name || ""}" onchange="window.updateChannel(${idx}, 'name', this.value)" />
          </label>
          <label class="field">
            <div class="field__label">类型 (type)</div>
            <select class="input" onchange="window.updateChannel(${idx}, 'type', this.value)">
              <option value="">选择类型...</option>
              <option value="telegram" ${channel.type === 'telegram' ? 'selected' : ''}>Telegram</option>
              <option value="bark" ${channel.type === 'bark' ? 'selected' : ''}>Bark</option>
              <option value="dingtalk" ${channel.type === 'dingtalk' ? 'selected' : ''}>钉钉 (DingTalk)</option>
              <option value="wecom" ${channel.type === 'wecom' ? 'selected' : ''}>企业微信 (WeCom)</option>
              <option value="igot" ${channel.type === 'igot' ? 'selected' : ''}>iGot</option>
              <option value="pushplus" ${channel.type === 'pushplus' ? 'selected' : ''}>PushPlus</option>
              <option value="feishu" ${channel.type === 'feishu' ? 'selected' : ''}>飞书 (Feishu)</option>
              <option value="webhook" ${channel.type === 'webhook' ? 'selected' : ''}>Webhook</option>
            </select>
          </label>
          <label class="field" style="grid-column: 1 / -1;">
            <div class="field__label">Token / URL</div>
            <input class="input" value="${channel.token || ""}" onchange="window.updateChannel(${idx}, 'token', this.value)" />
          </label>
        </div>
      `;
      channelsList.appendChild(channelEl);
    });

    // Render groups
    const groupsList = $("groupsList");
    groupsList.innerHTML = "";
    configData.channel_groups.forEach((group, idx) => {
      const groupEl = document.createElement("div");
      groupEl.className = "group-item";
      groupEl.innerHTML = `
        <div class="group-header">
          <div class="group-title">通道组 #${idx + 1}</div>
          <button class="btn btn--danger btn--small" onclick="window.removeGroup(${idx})">🗑️ 删除</button>
        </div>
        <div class="group-fields">
          <label class="field" style="grid-column: 1 / -1;">
            <div class="field__label">组名 (name)</div>
            <input class="input" value="${group.name || ""}" onchange="window.updateGroup(${idx}, 'name', this.value)" />
          </label>
        </div>
        <div class="group-channels">
          <div class="group-channels-label">使用的通道 (use):</div>
          <div class="group-channels-list" id="groupChannels${idx}">
            ${group.use.map((ch, chIdx) => `
              <div class="channel-tag">
                ${ch}
                <span class="channel-tag-remove" onclick="window.removeGroupChannel(${idx}, ${chIdx})">×</span>
              </div>
            `).join('')}
          </div>
          <div class="row">
            <input class="input" id="newChannelInput${idx}" placeholder="输入通道名称" style="flex: 1;" />
            <button class="btn btn--small" onclick="window.addGroupChannel(${idx})">➕ 添加</button>
          </div>
        </div>
      `;
      groupsList.appendChild(groupEl);
    });
  }

  // Global functions for inline event handlers
  window.updateChannel = (idx, field, value) => {
    if (configData.channels[idx]) {
      configData.channels[idx][field] = value;
    }
  };

  window.removeChannel = (idx) => {
    configData.channels.splice(idx, 1);
    renderGraphicalConfig();
  };

  window.updateGroup = (idx, field, value) => {
    if (configData.channel_groups[idx]) {
      configData.channel_groups[idx][field] = value;
    }
  };

  window.removeGroup = (idx) => {
    configData.channel_groups.splice(idx, 1);
    renderGraphicalConfig();
  };

  window.addGroupChannel = (idx) => {
    const input = $(`newChannelInput${idx}`);
    const channelName = input.value.trim();
    if (channelName && configData.channel_groups[idx]) {
      configData.channel_groups[idx].use.push(channelName);
      input.value = "";
      renderGraphicalConfig();
    }
  };

  window.removeGroupChannel = (groupIdx, channelIdx) => {
    if (configData.channel_groups[groupIdx]) {
      configData.channel_groups[groupIdx].use.splice(channelIdx, 1);
      renderGraphicalConfig();
    }
  };

  function addChannel() {
    configData.channels.push({ name: "", type: "", token: "" });
    renderGraphicalConfig();
  }

  function addGroup() {
    configData.channel_groups.push({ name: "", use: [] });
    renderGraphicalConfig();
  }

  function graphicalToYaml() {
    // Update config from form
    configData.auth.user = $("cfgAuthUser").value.trim();
    configData.auth.pass = $("cfgAuthPass").value.trim();
    configData.default_channel = $("cfgDefaultChannel").value.trim();
    configData.sqlite.path = $("cfgSqlitePath").value.trim();

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

    if (configData.sqlite.path) {
      yaml += "\nsqlite:\n";
      yaml += `  path: ${configData.sqlite.path}\n`;
    }

    return yaml;
  }

  async function saveGraphicalConfig() {
    const yaml = graphicalToYaml();
    cfgEl.value = yaml;
    await uploadConfig();
  }

  async function reloadGraphicalConfig() {
    await downloadConfig();
  }

  // Tab switching
  function switchTab(tab) {
    const tabGraphical = $("tabGraphical");
    const tabYaml = $("tabYaml");
    const panelGraphical = $("panelGraphical");
    const panelYaml = $("panelYaml");

    if (tab === 'graphical') {
      tabGraphical.classList.add('tab--active');
      tabYaml.classList.remove('tab--active');
      panelGraphical.style.display = 'block';
      panelYaml.style.display = 'none';
      // Sync from YAML to graphical
      if (cfgEl.value) {
        parseYamlToGraphical(cfgEl.value);
      }
    } else {
      tabGraphical.classList.remove('tab--active');
      tabYaml.classList.add('tab--active');
      panelGraphical.style.display = 'none';
      panelYaml.style.display = 'block';
      // Sync from graphical to YAML
      cfgEl.value = graphicalToYaml();
    }
  }

  async function send(method) {
    const chan = $("sendChan").value || "";
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

      const txt = await resp.text();
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

  // Event listeners
  $("beautify").addEventListener("click", beautifyYAML);
  $("sendPost").addEventListener("click", () => send("POST"));
  $("sendGet").addEventListener("click", () => send("GET"));
  $("eventsOn").addEventListener("click", eventsOn);
  $("eventsOff").addEventListener("click", eventsOff);
  $("eventsClear").addEventListener("click", () => ($("events").textContent = ""));

  // Graphical config event listeners
  $("tabGraphical").addEventListener("click", () => switchTab('graphical'));
  $("tabYaml").addEventListener("click", () => switchTab('yaml'));
  $("addChannel").addEventListener("click", addChannel);
  $("addGroup").addEventListener("click", addGroup);
  $("saveGraphical").addEventListener("click", () => saveGraphicalConfig().catch((e) => setMsg(cfgMsg, String(e), true)));
  $("reloadGraphical").addEventListener("click", () => reloadGraphicalConfig().catch((e) => setMsg(cfgMsg, String(e), true)));

  // YAML editor event listeners
  $("saveYaml").addEventListener("click", () => uploadConfig().catch((e) => setMsg(cfgMsg, String(e), true)));
  $("reloadYaml").addEventListener("click", () => downloadConfig().catch((e) => setMsg(cfgMsg, String(e), true)));

  // Initialize - Auto-load config on page load
  downloadConfig().catch((e) => console.error('Failed to auto-load config:', e));
})();
