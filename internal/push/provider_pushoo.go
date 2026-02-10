package push

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type dingtalkProvider struct{}

func NewDingTalkProvider() Provider { return dingtalkProvider{} }

func (dingtalkProvider) Type() string { return "dingtalk" }

func (dingtalkProvider) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return SendResult{Status: "error", Detail: "empty dingtalk token"}, nil
	}
	urlStr := token
	if !strings.HasPrefix(strings.ToLower(token), "http") {
		urlStr = "https://oapi.dingtalk.com/robot/send?access_token=" + token
	}

	msgtype := "text"
	content := markdownToText(req.Content)
	if req.Title != "" {
		content = req.Title + "\n" + content
	}
	payload := map[string]any{"msgtype": msgtype}
	if msgtype == "text" {
		payload["text"] = map[string]string{"content": content}
	} else if msgtype == "markdown" {
		title := req.Title
		if title == "" {
			title = getTitle(req.Content)
		}
		payload["markdown"] = map[string]string{
			"title": title,
			"text":  req.Content,
		}
	}

	status, body, err := doJSONRequest(ctx, http.MethodPost, urlStr, payload, nil)
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
	if status < 200 || status >= 300 {
		return SendResult{Status: "error", Detail: fmt.Sprintf("status code %d: %s", status, strings.TrimSpace(string(body)))}, nil
	}
	return SendResult{Status: "success", Detail: formatResponseDetail(body)}, nil
}

type wecomProvider struct{}

func NewWeComProvider() Provider { return wecomProvider{} }

func (wecomProvider) Type() string { return "wecom" }

func (wecomProvider) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return SendResult{Status: "error", Detail: "empty wecom token"}, nil
	}
	parts := strings.Split(token, "#")
	corpid := ""
	corpsecret := ""
	agentid := ""
	touser := "@all"
	if len(parts) > 0 {
		corpid = parts[0]
	}
	if len(parts) > 1 {
		corpsecret = parts[1]
	}
	if len(parts) > 2 {
		agentid = parts[2]
	}
	if len(parts) > 3 {
		touser = parts[3]
	}
	if strings.TrimSpace(corpid) == "" || strings.TrimSpace(corpsecret) == "" || strings.TrimSpace(agentid) == "" {
		return SendResult{Status: "error", Detail: "corpid, corpsecret, agentid are required"}, nil
	}

	tokenURL := fmt.Sprintf("https://qyapi.weixin.qq.com/cgi-bin/gettoken?corpid=%s&corpsecret=%s", url.QueryEscape(corpid), url.QueryEscape(corpsecret))
	status, body, err := doGetRequest(ctx, tokenURL, nil, nil)
	if err != nil || status < 200 || status >= 300 {
		return SendResult{Status: "success", Detail: "{}"}, nil
	}
	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	_ = json.Unmarshal(body, &tokenResp)

	msg := markdownToText(req.Content)
	if req.Title != "" {
		msg = req.Title + "\n" + msg
	}
	sendURL := "https://qyapi.weixin.qq.com/cgi-bin/message/send?access_token=" + tokenResp.AccessToken
	payload := map[string]any{
		"touser":  touser,
		"msgtype": "text",
		"agentid": agentid,
		"text": map[string]string{
			"content": msg,
		},
	}
	status, body, err = doJSONRequest(ctx, http.MethodPost, sendURL, payload, nil)
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
	if status < 200 || status >= 300 {
		return SendResult{Status: "error", Detail: fmt.Sprintf("status code %d: %s", status, strings.TrimSpace(string(body)))}, nil
	}
	return SendResult{Status: "success", Detail: formatResponseDetail(body)}, nil
}

type pushPlusProvider struct{}

func NewPushPlusProvider() Provider { return pushPlusProvider{} }

func (pushPlusProvider) Type() string { return "pushplus" }

func (pushPlusProvider) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return SendResult{Status: "error", Detail: "empty pushplus token"}, nil
	}
	title := req.Title
	if title == "" {
		title = getTitle(req.Content)
	}
	payload := map[string]any{
		"token":    token,
		"title":    title,
		"content":  req.Content,
		"template": "markdown",
	}
	status, body, err := doJSONRequest(ctx, http.MethodPost, "http://www.pushplus.plus/send", payload, nil)
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
	if status < 200 || status >= 300 {
		return SendResult{Status: "error", Detail: fmt.Sprintf("status code %d: %s", status, strings.TrimSpace(string(body)))}, nil
	}
	return SendResult{Status: "success", Detail: formatResponseDetail(body)}, nil
}

type pushPlusHxtripProvider struct{}

func NewPushPlusHxtripProvider() Provider { return pushPlusHxtripProvider{} }

func (pushPlusHxtripProvider) Type() string { return "pushplushxtrip" }

func (pushPlusHxtripProvider) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return SendResult{Status: "error", Detail: "empty pushplus hxtrip token"}, nil
	}
	title := req.Title
	if title == "" {
		title = getTitle(req.Content)
	}
	payload := map[string]any{
		"token":    token,
		"title":    title,
		"content":  markdownToHTML(req.Content),
		"template": "html",
	}
	status, body, err := doJSONRequest(ctx, http.MethodPost, "http://pushplus.hxtrip.com/send", payload, nil)
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
	if status < 200 || status >= 300 {
		return SendResult{Status: "error", Detail: fmt.Sprintf("status code %d: %s", status, strings.TrimSpace(string(body)))}, nil
	}
	return SendResult{Status: "success", Detail: formatResponseDetail(body)}, nil
}

type feishuProvider struct{}

func NewFeishuProvider() Provider { return feishuProvider{} }

func (feishuProvider) Type() string { return "feishu" }

func (feishuProvider) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return SendResult{Status: "error", Detail: "empty feishu token"}, nil
	}
	const v1 = "https://open.feishu.cn/open-apis/bot/hook/"
	const v2 = "https://open.feishu.cn/open-apis/bot/v2/hook/"
	urlStr := token
	if !strings.HasPrefix(strings.ToLower(token), "http") {
		urlStr = v2 + token
	}

	var payload any
	if strings.HasPrefix(urlStr, v1) {
		title := req.Title
		if title == "" {
			title = getTitle(req.Content)
		}
		payload = map[string]any{
			"title": title,
			"text":  markdownToText(req.Content),
		}
	} else {
		text := markdownToText(req.Content)
		if req.Title != "" {
			text = req.Title + "\n" + text
		}
		payload = map[string]any{
			"msg_type": "text",
			"content": map[string]string{
				"text": text,
			},
		}
	}

	status, body, err := doJSONRequest(ctx, http.MethodPost, urlStr, payload, nil)
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
	if status < 200 || status >= 300 {
		return SendResult{Status: "error", Detail: fmt.Sprintf("status code %d: %s", status, strings.TrimSpace(string(body)))}, nil
	}
	return SendResult{Status: "success", Detail: formatResponseDetail(body)}, nil
}

type igotProvider struct{}

func NewIgotProvider() Provider { return igotProvider{} }

func (igotProvider) Type() string { return "igot" }

func (igotProvider) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return SendResult{Status: "error", Detail: "empty igot token"}, nil
	}
	title := req.Title
	if title == "" {
		title = getTitle(req.Content)
	}
	payload := map[string]any{
		"title":   title,
		"content": markdownToText(req.Content),
	}
	urlStr := "https://push.hellyw.com/" + token
	status, body, err := doJSONRequest(ctx, http.MethodPost, urlStr, payload, nil)
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
	if status < 200 || status >= 300 {
		return SendResult{Status: "error", Detail: fmt.Sprintf("status code %d: %s", status, strings.TrimSpace(string(body)))}, nil
	}
	return SendResult{Status: "success", Detail: formatResponseDetail(body)}, nil
}

type qmsgProvider struct{}

func NewQmsgProvider() Provider { return qmsgProvider{} }

func (qmsgProvider) Type() string { return "qmsg" }

func (qmsgProvider) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return SendResult{Status: "error", Detail: "empty qmsg token"}, nil
	}
	msg := markdownToText(req.Content)
	if req.Title != "" {
		msg = req.Title + "\n" + msg
	}
	msg = removeURLAndIP(msg)

	param := url.Values{}
	param.Set("msg", msg)
	urlStr := "https://qmsg.zendee.cn/send/" + token
	status, body, err := doFormRequest(ctx, urlStr, param, nil)
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
	if status < 200 || status >= 300 {
		return SendResult{Status: "error", Detail: fmt.Sprintf("status code %d: %s", status, strings.TrimSpace(string(body)))}, nil
	}
	return SendResult{Status: "success", Detail: formatResponseDetail(body)}, nil
}

var serverChanTurboRe = regexp.MustCompile(`^sctp(\d+)t`)

type serverChanProvider struct {
	typ string
}

func NewServerChanProvider() Provider { return serverChanProvider{typ: "serverchan"} }

func NewServerChainProvider() Provider { return serverChanProvider{typ: "serverchain"} }

func (p serverChanProvider) Type() string { return p.typ }

func (p serverChanProvider) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return SendResult{Status: "error", Detail: "empty serverchan token"}, nil
	}

	title := req.Title
	if title == "" {
		title = getTitle(req.Content)
	}

	urlStr := ""
	params := url.Values{}

	if strings.HasPrefix(token, "sctp") {
		m := serverChanTurboRe.FindStringSubmatch(token)
		if len(m) < 2 {
			return SendResult{Status: "error", Detail: "invalid sctp token"}, nil
		}
		urlStr = fmt.Sprintf("https://%s.push.ft07.com/send", m[1])
		params.Set("title", title)
		params.Set("desp", req.Content)
	} else if strings.ToLower(token[:min(3, len(token))]) == "sct" {
		urlStr = "https://sctapi.ftqq.com"
		params.Set("title", title)
		params.Set("desp", req.Content)
	} else {
		urlStr = "https://sc.ftqq.com"
		params.Set("text", title)
		params.Set("desp", req.Content)
	}

	endpoint := fmt.Sprintf("%s/%s.send", urlStr, token)
	status, body, err := doFormRequest(ctx, endpoint, params, nil)
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
	if status < 200 || status >= 300 {
		return SendResult{Status: "error", Detail: fmt.Sprintf("status code %d: %s", status, strings.TrimSpace(string(body)))}, nil
	}
	return SendResult{Status: "success", Detail: formatResponseDetail(body)}, nil
}

type gocqhttpProvider struct{}

func NewGoCqhttpProvider() Provider { return gocqhttpProvider{} }

func (gocqhttpProvider) Type() string { return "gocqhttp" }

func (gocqhttpProvider) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return SendResult{Status: "error", Detail: "empty gocqhttp token"}, nil
	}
	message := markdownToText(req.Content)
	if req.Title != "" {
		message = req.Title + "\n" + message
	}
	params := url.Values{}
	params.Set("message", message)
	status, body, err := doFormRequest(ctx, token, params, nil)
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
	if status < 200 || status >= 300 {
		return SendResult{Status: "error", Detail: fmt.Sprintf("status code %d: %s", status, strings.TrimSpace(string(body)))}, nil
	}
	return SendResult{Status: "success", Detail: formatResponseDetail(body)}, nil
}

type atriProvider struct{}

func NewAtriProvider() Provider { return atriProvider{} }

func (atriProvider) Type() string { return "atri" }

func (atriProvider) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return SendResult{Status: "error", Detail: "empty atri token"}, nil
	}
	message := markdownToText(req.Content)
	if req.Title != "" {
		message = req.Title + "\n" + message
	}
	params := url.Values{}
	params.Set("user_id", token)
	params.Set("message", message)
	headers := map[string]string{
		"X-Requested-By": "pushoo",
	}
	status, body, err := doFormRequest(ctx, "http://pushoo.tianli0.top/", params, headers)
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
	if status < 200 || status >= 300 {
		return SendResult{Status: "error", Detail: fmt.Sprintf("status code %d: %s", status, strings.TrimSpace(string(body)))}, nil
	}
	return SendResult{Status: "success", Detail: formatResponseDetail(body)}, nil
}

type pushDeerProvider struct{}

func NewPushDeerProvider() Provider { return pushDeerProvider{} }

func (pushDeerProvider) Type() string { return "pushdeer" }

func (pushDeerProvider) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return SendResult{Status: "error", Detail: "empty pushdeer token"}, nil
	}
	title := req.Title
	if title == "" {
		title = getTitle(req.Content)
	}
	payload := map[string]any{
		"pushkey": token,
		"text":    title,
		"desp":    req.Content,
	}
	status, body, err := doJSONRequest(ctx, http.MethodPost, "https://api2.pushdeer.com/message/push", payload, nil)
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
	if status < 200 || status >= 300 {
		return SendResult{Status: "error", Detail: fmt.Sprintf("status code %d: %s", status, strings.TrimSpace(string(body)))}, nil
	}
	return SendResult{Status: "success", Detail: formatResponseDetail(body)}, nil
}

type iftttProvider struct{}

func NewIftttProvider() Provider { return iftttProvider{} }

func (iftttProvider) Type() string { return "ifttt" }

func (iftttProvider) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return SendResult{Status: "error", Detail: "empty ifttt token"}, nil
	}
	parts := strings.Split(token, "#")
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return SendResult{Status: "error", Detail: "ifttt token must be 'token#eventName'"}, nil
	}
	key := parts[0]
	eventName := parts[1]
	urlStr := fmt.Sprintf("https://maker.ifttt.com/trigger/%s/with/key/%s", eventName, key)
	value1 := markdownToText(req.Title)
	value2 := markdownToText(req.Content)
	payload := map[string]any{
		"value1": value1,
		"value2": value2,
		"value3": "",
	}
	status, body, err := doJSONRequest(ctx, http.MethodPost, urlStr, payload, nil)
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
	if status < 200 || status >= 300 {
		return SendResult{Status: "error", Detail: fmt.Sprintf("status code %d: %s", status, strings.TrimSpace(string(body)))}, nil
	}
	return SendResult{Status: "success", Detail: formatResponseDetail(body)}, nil
}

type wecombotProvider struct{}

func NewWecombotProvider() Provider { return wecombotProvider{} }

func (wecombotProvider) Type() string { return "wecombot" }

func (wecombotProvider) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return SendResult{Status: "error", Detail: "empty wecombot token"}, nil
	}
	urlStr := "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=" + token
	content := markdownToText(req.Content)
	payload := map[string]any{
		"msgtype": "text",
		"text": map[string]string{
			"content": content,
		},
	}
	status, body, err := doJSONRequest(ctx, http.MethodPost, urlStr, payload, nil)
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
	if status < 200 || status >= 300 {
		return SendResult{Status: "error", Detail: fmt.Sprintf("status code %d: %s", status, strings.TrimSpace(string(body)))}, nil
	}
	return SendResult{Status: "success", Detail: formatResponseDetail(body)}, nil
}

type discordProvider struct{}

func NewDiscordProvider() Provider { return discordProvider{} }

func (discordProvider) Type() string { return "discord" }

func (discordProvider) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return SendResult{Status: "error", Detail: "empty discord token"}, nil
	}
	urlStr := token
	if !strings.HasPrefix(strings.ToLower(token), "http") {
		urlStr = "https://discord.com/api/webhooks/" + strings.Replace(token, "#", "/", 1)
	}
	payload := map[string]any{
		"content": req.Content,
	}
	status, body, err := doJSONRequest(ctx, http.MethodPost, urlStr, payload, nil)
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
	if status < 200 || status >= 300 {
		return SendResult{Status: "error", Detail: fmt.Sprintf("status code %d: %s", status, strings.TrimSpace(string(body)))}, nil
	}
	return SendResult{Status: "success", Detail: fmt.Sprintf("Delivered successfully, code %d.", status)}, nil
}

type wxPusherProvider struct{}

func NewWxPusherProvider() Provider { return wxPusherProvider{} }

func (wxPusherProvider) Type() string { return "wxpusher" }

func (wxPusherProvider) Send(ctx context.Context, req SendRequest) (SendResult, error) {
	token := strings.TrimSpace(req.Token)
	if token == "" {
		return SendResult{Status: "error", Detail: "empty wxpusher token"}, nil
	}
	parts := strings.Split(token, "#")
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return SendResult{Status: "error", Detail: "wxpusher token must be 'appToken#topicIds'"}, nil
	}
	appToken := parts[0]
	topicStr := parts[1]
	var topicIDs []int
	for _, id := range strings.Split(topicStr, ",") {
		id = strings.TrimSpace(id)
		if id == "" {
			topicIDs = append(topicIDs, 0)
			continue
		}
		if n, err := strconv.Atoi(id); err == nil {
			topicIDs = append(topicIDs, n)
		} else {
			topicIDs = append(topicIDs, 0)
		}
	}
	summary := req.Title
	if summary == "" {
		summary = getTitle(req.Content)
	}
	payload := map[string]any{
		"appToken":      appToken,
		"content":       req.Content,
		"summary":       summary,
		"contentType":   3,
		"topicIds":      topicIDs,
		"uids":          []string{},
		"url":           "",
		"verifyPayload": false,
	}
	status, body, err := doJSONRequest(ctx, http.MethodPost, "http://wxpusher.zjiecode.com/api/send/message", payload, nil)
	if err != nil {
		return SendResult{Status: "error", Detail: err.Error()}, err
	}
	if status < 200 || status >= 300 {
		return SendResult{Status: "error", Detail: fmt.Sprintf("status code %d: %s", status, strings.TrimSpace(string(body)))}, nil
	}
	return SendResult{Status: "success", Detail: formatResponseDetail(body)}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
