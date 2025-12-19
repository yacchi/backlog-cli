package relay

import (
	"net/http"
	"sync"
	"time"
)

// RateLimiter はレートリミッター
type RateLimiter struct {
	enabled bool
	rate    int // リクエスト/分
	burst   int // バースト許容数

	mu      sync.Mutex
	clients map[string]*clientRate
}

type clientRate struct {
	tokens    float64
	lastCheck time.Time
}

// NewRateLimiter は新しいレートリミッターを作成する
func NewRateLimiter(enabled bool, requestsPerMinute, burst int) *RateLimiter {
	return &RateLimiter{
		enabled: enabled,
		rate:    requestsPerMinute,
		burst:   burst,
		clients: make(map[string]*clientRate),
	}
}

// Allow はリクエストを許可するか確認する
func (rl *RateLimiter) Allow(clientIP string) bool {
	if !rl.enabled {
		return true
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cr, exists := rl.clients[clientIP]

	if !exists {
		rl.clients[clientIP] = &clientRate{
			tokens:    float64(rl.burst - 1),
			lastCheck: now,
		}
		return true
	}

	// トークンを補充
	elapsed := now.Sub(cr.lastCheck).Minutes()
	cr.tokens += elapsed * float64(rl.rate)
	if cr.tokens > float64(rl.burst) {
		cr.tokens = float64(rl.burst)
	}
	cr.lastCheck = now

	// トークンを消費
	if cr.tokens >= 1 {
		cr.tokens--
		return true
	}

	return false
}

// Middleware はレートリミットミドルウェアを返す
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.enabled {
			next.ServeHTTP(w, r)
			return
		}

		ip := getClientIP(r)
		if ip == nil {
			next.ServeHTTP(w, r)
			return
		}

		if !rl.Allow(ip.String()) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Cleanup は古いエントリを削除する（定期実行用）
func (rl *RateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	threshold := time.Now().Add(-10 * time.Minute)
	for ip, cr := range rl.clients {
		if cr.lastCheck.Before(threshold) {
			delete(rl.clients, ip)
		}
	}
}
