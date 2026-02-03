package push

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"pushoo-chan-gover/internal/config"
)

type Event struct {
	Time        time.Time `json:"time"`
	ChannelName string    `json:"channel_name"`
	ChannelType string    `json:"channel_type"`
	Status      string    `json:"status"`
	Detail      string    `json:"detail"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
}

type ServiceOptions struct {
	MaxRetry      int
	RetryInterval time.Duration
	HTTPTimeout   time.Duration

	OnEvent func(Event)
}

type Service struct {
	cfg       *config.Manager
	providers *ProviderRegistry
	opts      ServiceOptions
	httpc     *http.Client
}

func NewService(cfg *config.Manager, providers *ProviderRegistry, opts ServiceOptions) *Service {
	if opts.MaxRetry <= 0 {
		opts.MaxRetry = 1
	}
	if opts.HTTPTimeout <= 0 {
		opts.HTTPTimeout = 10 * time.Second
	}
	s := &Service{
		cfg:       cfg,
		providers: providers,
		opts:      opts,
		httpc: &http.Client{
			Timeout: opts.HTTPTimeout,
		},
	}
	// Give providers access to the shared client when they need it.
	SetSharedHTTPClient(s.httpc)
	return s
}

type DeliveryResult struct {
	ChannelName string `json:"channel_name"`
	ChannelType string `json:"channel_type"`
	Status      string `json:"status"`  // "success" | "error"
	Message     string `json:"message"` // human-readable per-channel log (kept compatible with TS style)
}

func (s *Service) ResolveChannels(names []string) ([]config.ChannelConfig, []string, error) {
	cfg := s.cfg.Get()
	chans := map[string]config.ChannelConfig{}
	for _, c := range cfg.Channels {
		chans[c.Name] = c
	}
	groups := map[string]config.ChannelGroupConfig{}
	for _, g := range cfg.ChannelGroups {
		groups[g.Name] = g
	}

	if len(names) == 0 {
		if cfg.DefaultChannel == "" {
			return nil, nil, errors.New("default_channel not specified")
		}
		names = []string{cfg.DefaultChannel}
	}

	var logmsg []string
	process := append([]string(nil), names...)
	seenGroup := map[string]bool{}
	ret := map[string]config.ChannelConfig{}

	for len(process) > 0 {
		n := process[len(process)-1]
		process = process[:len(process)-1]
		if n == "" {
			continue
		}
		if c, ok := chans[n]; ok {
			ret[n] = c
			continue
		}
		if g, ok := groups[n]; ok {
			if seenGroup[n] {
				continue
			}
			seenGroup[n] = true
			if len(g.Use) > 0 {
				process = append(process, g.Use...)
			}
			continue
		}
		logmsg = append(logmsg, fmt.Sprintf("unknown channel or group name: %s", n))
	}

	out := make([]config.ChannelConfig, 0, len(ret))
	for _, c := range ret {
		out = append(out, c)
	}
	return out, logmsg, nil
}

func (s *Service) Push(ctx context.Context, chanList []string, title, content string, dolog func(string)) ([]DeliveryResult, error) {
	chans, logmsg, err := s.ResolveChannels(chanList)
	for _, line := range logmsg {
		dolog(line)
	}
	if err != nil {
		return nil, err
	}

	results := make([]DeliveryResult, 0, len(chans))
	for _, c := range chans {
		status, msg := s.pushOneWithRetry(ctx, c, title, content)
		results = append(results, DeliveryResult{
			ChannelName: c.Name,
			ChannelType: c.Type,
			Status:      status,
			Message:     msg,
		})
	}
	return results, nil
}

func (s *Service) pushOneWithRetry(ctx context.Context, chanCfg config.ChannelConfig, title, content string) (string, string) {
	var out strings.Builder

	finalStatus := "error"
	for i := 0; i < s.opts.MaxRetry; i++ {
		res, retriable, err := s.pushOnce(ctx, chanCfg, title, content)
		round := i + 1
		if err == nil && res.Status == "success" {
			fmt.Fprintf(&out, "push success (round %d): %s", round, res.Detail)
			finalStatus = "success"
			break
		}
		detail := ""
		if err != nil {
			detail = err.Error()
		} else {
			detail = res.Detail
		}
		fmt.Fprintf(&out, "push error (round %d): %s", round, detail)
		if !retriable {
			out.WriteString("\npush failed because not retriable...")
			break
		}
		if i < s.opts.MaxRetry-1 && s.opts.RetryInterval > 0 {
			out.WriteByte('\n')
			select {
			case <-ctx.Done():
				return finalStatus, out.String()
			case <-time.After(s.opts.RetryInterval):
			}
		}
		out.WriteByte('\n')
	}

	return finalStatus, strings.TrimRight(out.String(), "\n")
}

func (s *Service) pushOnce(ctx context.Context, chanCfg config.ChannelConfig, title, content string) (SendResult, bool, error) {
	p, ok := s.providers.Get(chanCfg.Type)
	if !ok {
		return SendResult{Status: "error", Detail: "unknown channel type: " + chanCfg.Type}, false, nil
	}
	req := SendRequest{
		ChannelName: chanCfg.Name,
		ChannelType: chanCfg.Type,
		Token:       chanCfg.Token,
		Title:       title,
		Content:     content,
	}
	res, err := p.Send(ctx, req)
	retriable := shouldRetry(err, res)

	if s.opts.OnEvent != nil {
		detail := res.Detail
		if err != nil {
			detail = err.Error()
		}
		s.opts.OnEvent(Event{
			Time:        time.Now(),
			ChannelName: chanCfg.Name,
			ChannelType: chanCfg.Type,
			Status:      res.Status,
			Detail:      detail,
			Title:       title,
			Content:     content,
		})
	}

	return res, retriable, err
}

func shouldRetry(err error, res SendResult) bool {
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && (ne.Timeout() || ne.Temporary()) {
			return true
		}
		// A lot of network errors show up as plain strings in Go.
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "timeout") || strings.Contains(msg, "network") || strings.Contains(msg, "connection reset") {
			return true
		}
	}
	// Retry on 5xx-like provider messages.
	msg := strings.ToLower(res.Detail)
	if strings.Contains(msg, "status code") {
		// best-effort parse
		if i := strings.LastIndex(msg, "status code"); i >= 0 {
			tail := msg[i+len("status code"):]
			tail = strings.TrimSpace(tail)
			// read first number
			code := 0
			for j := 0; j < len(tail); j++ {
				if tail[j] < '0' || tail[j] > '9' {
					break
				}
				code = code*10 + int(tail[j]-'0')
			}
			if code >= 500 {
				return true
			}
		}
	}
	return false
}
