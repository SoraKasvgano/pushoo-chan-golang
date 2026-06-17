package httpapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"pushoo-chan-gover/internal/store"
)

type tawkPayload struct {
	Event    string `json:"event"`
	ChatID   string `json:"chatId"`
	Time     string `json:"time"`
	Domain   string `json:"domain"`
	Referrer string `json:"referrer"`
	Message  *struct {
		Text   string `json:"text"`
		Type   string `json:"type"`
		Sender *struct {
			Type string `json:"type"`
		} `json:"sender"`
	} `json:"message"`
	Visitor   *tawkVisitor `json:"visitor"`
	Requester *tawkVisitor `json:"requester"`
	Property  *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"property"`
	Chat *struct {
		ID       string                  `json:"id"`
		Visitor  *tawkVisitor            `json:"visitor"`
		Messages []tawkTranscriptMessage `json:"messages"`
	} `json:"chat"`
	Ticket *struct {
		ID      string `json:"id"`
		HumanID any    `json:"humanId"`
		Subject string `json:"subject"`
		Message string `json:"message"`
	} `json:"ticket"`
}

type tawkVisitor struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	City    string `json:"city"`
	Country string `json:"country"`
}

type tawkTranscriptMessage struct {
	Sender *struct {
		T string `json:"t"`
		N string `json:"n"`
	} `json:"sender"`
	Type   string `json:"type"`
	Msg    string `json:"msg"`
	Time   string `json:"time"`
	Attchs []any  `json:"attchs"`
}

func (a *API) handleTawkWebhook(w http.ResponseWriter, r *http.Request) {
	rc := &reqContext{}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"msg": "Method Not Allowed"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"msg": "read body failed: " + err.Error()})
		return
	}

	cfg := a.opts.Config.Get()
	query := r.URL.Query()
	chanName := firstNonEmpty(query.Get("chan"), cfg.Webhooks.Tawk.Chan)
	titlePrefix := firstNonEmpty(query.Get("title"), cfg.Webhooks.Tawk.Title, "Tawk.to")
	secret := firstNonEmpty(query.Get("secret"), cfg.Webhooks.Tawk.Secret)

	if secret != "" && !verifyTawkSignature(body, r.Header.Get("X-Tawk-Signature"), secret) {
		rc.log("tawk: signature verification failed")
		writeJSON(w, http.StatusUnauthorized, map[string]any{"msg": "Invalid signature"})
		return
	}

	var payload tawkPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"msg": "invalid JSON: " + err.Error()})
		return
	}

	title, content := formatTawkPayload(payload, titlePrefix)
	log.Printf("[tawk] webhook request event=%s title=%q chan=%q", valueOr(payload.Event, "undefined"), title, valueOr(chanName, "undefined"))

	chanList := splitChan(chanName)
	if len(chanList) == 0 {
		rc.log("Warning: no channel specified, using default channel!")
	}

	results, err := a.opts.Push.Push(r.Context(), chanList, title, content, func(msg string) {
		rc.log("tawk: " + msg)
	})
	if err != nil {
		rc.log("tawk: send failed: " + err.Error())
	}

	resp := map[string]any{}
	if len(results) > 0 {
		flat := make([]string, 0, len(results))
		for _, result := range results {
			flat = append(flat, result.Message)
		}
		resp["results"] = flat
	}
	if len(rc.logs) > 0 {
		resp["msg"] = rc.logs
	}

	status := http.StatusOK
	if len(results) == 0 {
		status = http.StatusInternalServerError
	}
	writeJSON(w, status, resp)

	if a.opts.Store != nil && len(results) > 0 {
		msg := store.Message{
			CreatedAt:  time.Now(),
			RemoteAddr: r.RemoteAddr,
			Path:       r.URL.Path,
			Format:     "tawk",
			Chan:       chanName,
			Title:      title,
			Content:    content,
		}
		deliveries := make([]store.Delivery, 0, len(results))
		for _, dr := range results {
			deliveries = append(deliveries, store.Delivery{
				CreatedAt:   time.Now(),
				ChannelName: dr.ChannelName,
				ChannelType: dr.ChannelType,
				Status:      dr.Status,
				Detail:      dr.Message,
			})
		}
		if err := a.opts.Store.Record(context.Background(), msg, deliveries); err != nil {
			rc.log("Failed to store tawk push history: " + err.Error())
		}
	}
}

func verifyTawkSignature(body []byte, signature, secret string) bool {
	signature = strings.TrimSpace(strings.ToLower(signature))
	if signature == "" {
		return false
	}
	mac := hmac.New(sha1.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func formatTawkPayload(payload tawkPayload, defaultTitle string) (string, string) {
	event := firstNonEmpty(payload.Event, "tawk:webhook")
	eventLabel := getTawkEventLabel(event)
	title := fmt.Sprintf("%s - %s", defaultTitle, eventLabel)
	visitor := getTawkVisitor(payload)
	chatID := getTawkChatID(payload)

	lines := []string{
		line("事件", fmt.Sprintf("%s (%s)", eventLabel, event)),
		line("时间", payload.Time),
		line("站点", payload.Domain),
		line("来源", payload.Referrer),
		line("Property", getTawkProperty(payload)),
		line("Chat ID", chatID),
		line("访客", visitor.Name),
		line("邮箱", visitor.Email),
		line("地区", strings.Join(nonEmpty(visitor.City, visitor.Country), ", ")),
	}
	lines = filterNonEmpty(lines)

	if payload.Message != nil && payload.Message.Text != "" {
		lines = append(lines, "", "首条消息:", payload.Message.Text)
	}

	if payload.Ticket != nil {
		lines = append(lines,
			"",
			line("工单编号", firstNonEmpty(textValue(payload.Ticket.HumanID), payload.Ticket.ID)),
			line("工单主题", payload.Ticket.Subject),
			line("工单内容", payload.Ticket.Message),
		)
	}

	if transcript := formatTawkTranscript(payload); transcript != "" {
		lines = append(lines, "", "聊天记录:", transcript)
	}

	return title, strings.Join(filterNonEmptyPreserveBlank(lines), "\n")
}

func getTawkEventLabel(event string) string {
	switch event {
	case "chat:start":
		return "新聊天开始"
	case "chat:end":
		return "聊天结束"
	case "chat:transcript_created":
		return "聊天记录生成"
	case "ticket:create":
		return "新工单创建"
	default:
		return firstNonEmpty(event, "Tawk.to 事件")
	}
}

func getTawkVisitor(payload tawkPayload) tawkVisitor {
	if payload.Visitor != nil {
		return *payload.Visitor
	}
	if payload.Chat != nil && payload.Chat.Visitor != nil {
		return *payload.Chat.Visitor
	}
	if payload.Requester != nil {
		return *payload.Requester
	}
	return tawkVisitor{}
}

func getTawkChatID(payload tawkPayload) string {
	if payload.ChatID != "" {
		return payload.ChatID
	}
	if payload.Chat != nil && payload.Chat.ID != "" {
		return payload.Chat.ID
	}
	if payload.Ticket != nil {
		return payload.Ticket.ID
	}
	return ""
}

func getTawkProperty(payload tawkPayload) string {
	if payload.Property == nil {
		return ""
	}
	return firstNonEmpty(payload.Property.Name, payload.Property.ID)
}

func formatTawkTranscript(payload tawkPayload) string {
	if payload.Chat == nil || len(payload.Chat.Messages) == 0 {
		return ""
	}
	messages := payload.Chat.Messages
	if len(messages) > 20 {
		messages = messages[len(messages)-20:]
	}
	lines := make([]string, 0, len(messages))
	for _, msg := range messages {
		content := msg.Msg
		if content == "" && len(msg.Attchs) > 0 {
			content = "[附件]"
		}
		if content == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", getTawkSenderName(msg), content))
	}
	return strings.Join(lines, "\n")
}

func getTawkSenderName(msg tawkTranscriptMessage) string {
	if msg.Sender == nil {
		return "未知"
	}
	if msg.Sender.N != "" {
		return msg.Sender.N
	}
	switch msg.Sender.T {
	case "a":
		return "客服"
	case "v":
		return "访客"
	case "s":
		return "系统"
	default:
		return firstNonEmpty(msg.Sender.T, "未知")
	}
}

func line(label, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return label + ": " + value
}

func textValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func valueOr(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func nonEmpty(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func filterNonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func filterNonEmptyPreserveBlank(values []string) []string {
	out := make([]string, 0, len(values))
	for i, value := range values {
		if value != "" || (i > 0 && i < len(values)-1 && values[i-1] != "" && values[i+1] != "") {
			out = append(out, value)
		}
	}
	return out
}

func writeJSON(w http.ResponseWriter, status int, body map[string]any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "    ")
	_ = enc.Encode(body)
}
