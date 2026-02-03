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
    push_token: { enabled: false, token: "" },
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
        push_token: { enabled: false, token: "" },
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
        if (line.startsWith('push_token:')) {
          currentSection = 'push_token';
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
            const tokenPart = trimmed.split(':').slice(1).join(':').split('#')[0].trim().replace(/^["']|["']$/g, '');
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
            const tokenVal = trimmed.split(':')[1].split('#')[0].trim().replace(/^["']|["']$/g, '');
            configData.push_token.token = tokenVal;
          }
        } else if (currentSection === 'sqlite') {
          if (line.match(/^\s+path:/)) {
            configData.sqlite.path = trimmed.split(':')[1].split('#')[0].trim();
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

    // Render push_token
    $("cfgPushTokenEnabled").value = configData.push_token?.enabled ? "true" : "false";
    $("cfgPushTokenToken").value = configData.push_token?.token || "";

    // Show/hide token hint in send test section
    const tokenHint = $("tokenHint");
    if (configData.push_token?.enabled && configData.push_token?.token) {
      tokenHint.style.display = "block";
      tokenHint.innerHTML = `🔑 Token 验证已启用，请求示例：<br>
        <code style="background: rgba(255,255,255,0.1); padding: 4px 8px; border-radius: 4px; display: inline-block; margin-top: 4px;">
          /send?token=${configData.push_token.token}&text=Hello&desp=World
        </code>`;
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

    updateApiExamples();
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
    configData.push_token.enabled = $("cfgPushTokenEnabled").value === "true";
    configData.push_token.token = $("cfgPushTokenToken").value.trim();
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

    yaml += "\npush_token:\n";
    yaml += `  enabled: ${configData.push_token.enabled}\n`;
    yaml += `  token: ${configData.push_token.token || ''}\n`;

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

  // Update API examples with token
  function updateApiExamples() {
    const tokenEnabled = configData.push_token?.enabled;
    const token = configData.push_token?.token || "YOUR_TOKEN";
    const tokenParam = tokenEnabled ? `token=${token}&` : "";
    const tokenParamOnly = tokenEnabled ? `?token=${token}` : "";

    // Update cURL examples (token is accepted in query or form, not JSON body)
    $("curlGet").textContent = `curl "http://localhost:8084/send?${tokenParam}text=Hello&desp=World&chan=telegram"`;
    $("curlPostForm").textContent = `curl -X POST http://localhost:8084/send \\
  -d "${tokenParam}text=Hello&desp=World&chan=telegram"`;
    $("curlPostJson").textContent = `curl -X POST "http://localhost:8084/send${tokenParamOnly}" \\
  -H "Content-Type: application/json" \\
  -d '{"text":"Hello","desp":"World","chan":"telegram"}'`;
    $("curlBarkGetTitleBody").textContent = `curl "http://localhost:8084/bark/telegram/Hello/World${tokenParamOnly}"`;
    $("curlBarkPostForm").textContent = `curl -X POST http://localhost:8084/bark/telegram \\
  -d "${tokenEnabled ? `token=${token}&` : ""}title=Hello&body=World"`;
    $("curlBarkV2").textContent = `curl -X POST "http://localhost:8084/barkv2${tokenParamOnly}" \\
  -H "Content-Type: application/json" \\
  -d '{"device_key":"telegram","title":"Hello","body":"World"}'`;

    // Update Python example (token must be in query or form for POST)
    const pythonTokenLine = tokenEnabled ? `    'token': '${token}',\n` : "";
    const pythonPostUrl = tokenEnabled ? `http://localhost:8084/send?token=${token}` : "http://localhost:8084/send";
    const pythonBarkTitleBodyUrl = tokenEnabled ? `http://localhost:8084/bark/telegram/Hello/World?token=${token}` : "http://localhost:8084/bark/telegram/Hello/World";
    const pythonBarkPostFormUrl = "http://localhost:8084/bark/telegram";
    const pythonBarkV2Url = tokenEnabled ? `http://localhost:8084/barkv2?token=${token}` : "http://localhost:8084/barkv2";
    $("pythonExample").textContent = `import requests

# GET 请求
response = requests.get('http://localhost:8084/send', params={
${pythonTokenLine}    'text': 'Hello',
    'desp': 'World',
    'chan': 'telegram'
})

# POST 请求（表单）
response = requests.post('http://localhost:8084/send', data={
${pythonTokenLine}    'text': 'Hello',
    'desp': 'World',
    'chan': 'telegram'
})

# POST 请求（JSON）
response = requests.post('${pythonPostUrl}', json={
    'text': 'Hello',
    'desp': 'World',
    'chan': 'telegram'
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
    'device_key': 'telegram',
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
  params.Set("chan", "telegram")

  resp, err := http.Get("http://localhost:8084/send?" + params.Encode())
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
  form.Set("chan", "telegram")
  resp, err = http.Post("http://localhost:8084/send", "application/x-www-form-urlencoded", bytes.NewBufferString(form.Encode()))
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
    "chan": "telegram",
  })
  resp, err = http.Post("http://localhost:8084/send${tokenParamOnly}", "application/json", bytes.NewReader(body))
  if err != nil {
    panic(err)
  }
  body, _ = io.ReadAll(resp.Body)
  _ = resp.Body.Close()
  fmt.Println("POST json status:", resp.Status, string(body))

  // Bark GET（标题/内容）
  resp, err = http.Get("http://localhost:8084/bark/telegram/Hello/World${tokenParamOnly}")
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
  resp, err = http.Post("http://localhost:8084/bark/telegram", "application/x-www-form-urlencoded", bytes.NewBufferString(barkForm.Encode()))
  if err != nil {
    panic(err)
  }
  body, _ = io.ReadAll(resp.Body)
  _ = resp.Body.Close()
  fmt.Println("Bark post form status:", resp.Status, string(body))

  // Bark v2 接口（JSON）
  barkBody, _ := json.Marshal(map[string]string{
    "device_key": "telegram",
    "title": "Hello",
    "body": "World",
  })
  resp, err = http.Post("http://localhost:8084/barkv2${tokenParamOnly}", "application/json", bytes.NewReader(barkBody))
  if err != nil {
    panic(err)
  }
  body, _ = io.ReadAll(resp.Body)
  _ = resp.Body.Close()
  fmt.Println("Bark v2 status:", resp.Status, string(body))
}`;

    // Update JavaScript example (token must be in query or form for POST)
    const jsTokenLine = tokenEnabled ? `  token: '${token}',\n` : "";
    const jsBarkTitleBodyUrl = tokenEnabled ? `http://localhost:8084/bark/telegram/Hello/World?token=${token}` : "http://localhost:8084/bark/telegram/Hello/World";
    const jsBarkPostFormUrl = "http://localhost:8084/bark/telegram";
    const jsBarkV2Url = tokenEnabled ? `http://localhost:8084/barkv2?token=${token}` : "http://localhost:8084/barkv2";
    $("jsExample").textContent = `// GET 请求
const params = new URLSearchParams({
${jsTokenLine}  text: 'Hello',
  desp: 'World',
  chan: 'telegram'
});
fetch(\`http://localhost:8084/send?\${params}\`)
  .then(res => res.json())
  .then(data => console.log(data));

// POST 请求（表单）
const form = new URLSearchParams({
${jsTokenLine}  text: 'Hello',
  desp: 'World',
  chan: 'telegram'
});
fetch('http://localhost:8084/send', {
  method: 'POST',
  headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
  body: form.toString()
})
  .then(res => res.json())
  .then(data => console.log(data));

// POST 请求（JSON）
fetch('http://localhost:8084/send${tokenParamOnly}', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    text: 'Hello',
    desp: 'World',
    chan: 'telegram'
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
    device_key: 'telegram',
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

  // Example tab event listeners
  $("tabCurl").addEventListener("click", () => switchExampleTab('curl'));
  $("tabPython").addEventListener("click", () => switchExampleTab('python'));
  $("tabGo").addEventListener("click", () => switchExampleTab('go'));
  $("tabJavaScript").addEventListener("click", () => switchExampleTab('javascript'));

  // Initialize - Auto-load config on page load
  downloadConfig().then(() => {
    updateApiExamples();
  }).catch((e) => console.error('Failed to auto-load config:', e));
})();
