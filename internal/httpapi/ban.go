package httpapi

import (
	"context"
	"hash/fnv"
	"net"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type banEntry struct {
	failCount   int
	bannedUntil time.Time
	lastSeen    time.Time
}

type banShard struct {
	mu      sync.RWMutex
	entries map[string]*banEntry
}

type IPBanOptions struct {
	MaxEntries      int
	CleanupInterval time.Duration
	IdleTTL         time.Duration
	Kind            string
	Persistor       BanPersistor
}

type BanPersistor interface {
	UpsertBan(ctx context.Context, kind, ip string, failCount int, bannedUntil, lastSeen time.Time) error
	DeleteBan(ctx context.Context, kind, ip string) error
}

// IPBanStore keeps temporary bans in memory.
type IPBanStore struct {
	shards    []banShard
	shardMask uint32

	maxEntries      int
	cleanupInterval time.Duration
	idleTTL         time.Duration

	totalEntries int64

	stopOnce sync.Once
	stopCh   chan struct{}

	kind      string
	persistor BanPersistor
}

type BanIPInfo struct {
	IP          string `json:"ip"`
	Banned      bool   `json:"banned"`
	BannedUntil string `json:"banned_until,omitempty"`
	LastSeen    string `json:"last_seen,omitempty"`
}

type BanStats struct {
	TotalEntries        int         `json:"total_entries"`
	BannedEntries       int         `json:"banned_entries"`
	MaxEntries          int         `json:"max_entries"`
	IdleTTLSeconds      int         `json:"idle_ttl_seconds"`
	CleanupIntervalSec  int         `json:"cleanup_interval_seconds"`
	Shards              int         `json:"shards"`
	SampleIPs           []BanIPInfo `json:"sample_ips"`
	SampleTruncated     bool        `json:"sample_truncated"`
	EstimatedBytes      int64       `json:"estimated_bytes"`
	EstimatedBytesPerIP int64       `json:"estimated_bytes_per_ip"`
	EstimatedBytesNote  string      `json:"estimated_bytes_note"`
}

func NewIPBanStore(opts IPBanOptions) *IPBanStore {
	if opts.MaxEntries < 0 {
		opts.MaxEntries = 0
	}
	if opts.CleanupInterval <= 0 {
		opts.CleanupInterval = 60 * time.Second
	}
	if opts.IdleTTL <= 0 {
		opts.IdleTTL = 60 * time.Minute
	}
	shardCount := defaultShardCount()
	shards := make([]banShard, shardCount)
	for i := range shards {
		shards[i] = banShard{entries: make(map[string]*banEntry)}
	}

	s := &IPBanStore{
		shards:          shards,
		shardMask:       uint32(shardCount - 1),
		maxEntries:      opts.MaxEntries,
		cleanupInterval: opts.CleanupInterval,
		idleTTL:         opts.IdleTTL,
		stopCh:          make(chan struct{}),
		kind:            opts.Kind,
		persistor:       opts.Persistor,
	}
	s.startJanitor()
	return s
}

func (s *IPBanStore) IsBanned(ip string) (bool, time.Time) {
	now := time.Now()
	shard := s.shardFor(ip)

	shard.mu.RLock()
	entry, ok := shard.entries[ip]
	if !ok {
		shard.mu.RUnlock()
		return false, time.Time{}
	}
	bannedUntil := entry.bannedUntil
	shard.mu.RUnlock()

	if now.Before(bannedUntil) {
		shard.mu.Lock()
		entry, ok = shard.entries[ip]
		if ok && now.Before(entry.bannedUntil) {
			entry.lastSeen = now
			until := entry.bannedUntil
			shard.mu.Unlock()
			return true, until
		}
		shard.mu.Unlock()
		return false, time.Time{}
	}

	shard.mu.Lock()
	entry, ok = shard.entries[ip]
	if ok {
		if !now.Before(entry.bannedUntil) {
			if entry.failCount == 0 || (s.idleTTL > 0 && now.Sub(entry.lastSeen) >= s.idleTTL) {
				delete(shard.entries, ip)
				shard.mu.Unlock()
				atomic.AddInt64(&s.totalEntries, -1)
				s.persistDelete(ip)
				return false, time.Time{}
			}
			entry.lastSeen = now
		}
	}
	shard.mu.Unlock()
	return false, time.Time{}
}

func (s *IPBanStore) RecordFailure(ip string, failLimit int, banMinutes int) (bool, time.Time, int) {
	if failLimit <= 0 || banMinutes <= 0 {
		return false, time.Time{}, 0
	}
	now := time.Now()

	if s.maxEntries > 0 && atomic.LoadInt64(&s.totalEntries) >= int64(s.maxEntries) {
		s.evictGlobal(now)
	}

	shard := s.shardFor(ip)
	shard.mu.Lock()

	entry, ok := shard.entries[ip]
	if !ok {
		entry = &banEntry{lastSeen: now}
		shard.entries[ip] = entry
		atomic.AddInt64(&s.totalEntries, 1)
	}
	entry.lastSeen = now

	if now.Before(entry.bannedUntil) {
		bannedUntil := entry.bannedUntil
		failCount := entry.failCount
		lastSeen := entry.lastSeen
		shard.mu.Unlock()
		s.persistUpsert(ip, failCount, bannedUntil, lastSeen)
		return true, bannedUntil, failCount
	}
	entry.failCount++
	if entry.failCount >= failLimit {
		entry.bannedUntil = now.Add(time.Duration(banMinutes) * time.Minute)
		entry.failCount = 0
		bannedUntil := entry.bannedUntil
		lastSeen := entry.lastSeen
		shard.mu.Unlock()
		s.persistUpsert(ip, 0, bannedUntil, lastSeen)
		return true, bannedUntil, 0
	}
	failCount := entry.failCount
	lastSeen := entry.lastSeen
	shard.mu.Unlock()
	s.persistUpsert(ip, failCount, time.Time{}, lastSeen)
	return false, time.Time{}, failCount
}

func (s *IPBanStore) Reset(ip string) {
	shard := s.shardFor(ip)
	shard.mu.Lock()
	if _, ok := shard.entries[ip]; ok {
		delete(shard.entries, ip)
		atomic.AddInt64(&s.totalEntries, -1)
	}
	shard.mu.Unlock()
	s.persistDelete(ip)
}

func (s *IPBanStore) Stats(limit int) BanStats {
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}

	s.cleanup()
	now := time.Now()
	stats := BanStats{
		MaxEntries:         s.maxEntries,
		IdleTTLSeconds:     int(s.idleTTL.Seconds()),
		CleanupIntervalSec: int(s.cleanupInterval.Seconds()),
		Shards:             len(s.shards),
		EstimatedBytesNote: "Estimated only (does not include map/shard overhead).",
	}

	stats.TotalEntries = int(atomic.LoadInt64(&s.totalEntries))
	stats.SampleIPs = make([]BanIPInfo, 0, min(limit, stats.TotalEntries))

	var ipBytes int64
	for i := range s.shards {
		shard := &s.shards[i]
		shard.mu.RLock()
		for ip, entry := range shard.entries {
			banned := now.Before(entry.bannedUntil)
			if banned {
				stats.BannedEntries++
			}
			ipBytes += int64(len(ip))
			if len(stats.SampleIPs) < limit {
				info := BanIPInfo{
					IP:       ip,
					Banned:   banned,
					LastSeen: entry.lastSeen.UTC().Format(time.RFC3339),
				}
				if !entry.bannedUntil.IsZero() {
					info.BannedUntil = entry.bannedUntil.UTC().Format(time.RFC3339)
				}
				stats.SampleIPs = append(stats.SampleIPs, info)
			}
		}
		shard.mu.RUnlock()
	}

	stats.SampleTruncated = len(stats.SampleIPs) < stats.TotalEntries
	if stats.TotalEntries > 0 {
		estimatedPerIP := int64(32 + 16 + 24)
		estimatedPerIP += ipBytes / int64(stats.TotalEntries)
		stats.EstimatedBytesPerIP = estimatedPerIP
		stats.EstimatedBytes = estimatedPerIP * int64(stats.TotalEntries)
	}
	return stats
}

func (s *IPBanStore) Close() {
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
}

func (s *IPBanStore) startJanitor() {
	if s.cleanupInterval <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(s.cleanupInterval)
		defer ticker.Stop()
		for {
			select {
			case <-s.stopCh:
				return
			case <-ticker.C:
				s.cleanup()
			}
		}
	}()
}

func (s *IPBanStore) cleanup() {
	now := time.Now()
	var deleted int64
	var deletedIPs []string
	for i := range s.shards {
		shard := &s.shards[i]
		shard.mu.Lock()
		for ip, entry := range shard.entries {
			if s.isExpiredOrIdle(entry, now) {
				delete(shard.entries, ip)
				deleted++
				deletedIPs = append(deletedIPs, ip)
			}
		}
		shard.mu.Unlock()
	}
	if deleted > 0 {
		atomic.AddInt64(&s.totalEntries, -deleted)
	}
	for _, ip := range deletedIPs {
		s.persistDelete(ip)
	}

	if s.maxEntries > 0 && atomic.LoadInt64(&s.totalEntries) > int64(s.maxEntries) {
		for atomic.LoadInt64(&s.totalEntries) > int64(s.maxEntries) {
			if !s.evictOldestOnce() {
				return
			}
		}
	}
}

func (s *IPBanStore) isExpiredOrIdle(entry *banEntry, now time.Time) bool {
	if now.After(entry.bannedUntil) && entry.bannedUntil != (time.Time{}) {
		return true
	}
	if entry.failCount == 0 && entry.bannedUntil.IsZero() && s.idleTTL > 0 {
		return now.Sub(entry.lastSeen) >= s.idleTTL
	}
	if s.idleTTL > 0 && now.Sub(entry.lastSeen) >= s.idleTTL {
		return true
	}
	return false
}

func (s *IPBanStore) evictGlobal(now time.Time) {
	s.cleanup()
	if s.maxEntries <= 0 {
		return
	}
	for atomic.LoadInt64(&s.totalEntries) > int64(s.maxEntries) {
		if !s.evictOldestOnce() {
			return
		}
	}
}

func (s *IPBanStore) evictOldestOnce() bool {
	var oldestIP string
	var oldest time.Time
	oldestShard := -1

	for i := range s.shards {
		shard := &s.shards[i]
		shard.mu.RLock()
		for ip, entry := range shard.entries {
			if oldestIP == "" || entry.lastSeen.Before(oldest) {
				oldestIP = ip
				oldest = entry.lastSeen
				oldestShard = i
			}
		}
		shard.mu.RUnlock()
	}

	if oldestIP == "" || oldestShard < 0 {
		return false
	}

	shard := &s.shards[oldestShard]
	shard.mu.Lock()
	if _, ok := shard.entries[oldestIP]; ok {
		delete(shard.entries, oldestIP)
		shard.mu.Unlock()
		atomic.AddInt64(&s.totalEntries, -1)
		s.persistDelete(oldestIP)
		return true
	}
	shard.mu.Unlock()
	return false
}

func (s *IPBanStore) persistUpsert(ip string, failCount int, bannedUntil, lastSeen time.Time) {
	if s.persistor == nil {
		return
	}
	_ = s.persistor.UpsertBan(context.Background(), s.kind, ip, failCount, bannedUntil, lastSeen)
}

func (s *IPBanStore) persistDelete(ip string) {
	if s.persistor == nil {
		return
	}
	_ = s.persistor.DeleteBan(context.Background(), s.kind, ip)
}

func (s *IPBanStore) shardFor(ip string) *banShard {
	h := fnv32a(ip)
	return &s.shards[h&s.shardMask]
}

func fnv32a(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}

func defaultShardCount() int {
	c := runtime.GOMAXPROCS(0) * 4
	if c < 8 {
		c = 8
	}
	if c > 64 {
		c = 64
	}
	return nextPow2(c)
}

func nextPow2(n int) int {
	if n <= 1 {
		return 1
	}
	x := 1
	for x < n {
		x <<= 1
	}
	return x
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func getClientIP(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff != "" {
		parts := strings.Split(xff, ",")
		if len(parts) > 0 {
			ip := strings.TrimSpace(parts[0])
			if ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return host
	}
	return r.RemoteAddr
}

type BanError struct {
	Message    string
	RetryAfter time.Duration
}

func (e *BanError) Error() string { return e.Message }
