package relay

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// IPRestriction はIP制限の設定
type IPRestriction struct {
	allowedNets []*net.IPNet
}

// NewIPRestriction は新しいIP制限を作成する
func NewIPRestriction(cidrs []string) (*IPRestriction, error) {
	if len(cidrs) == 0 {
		return &IPRestriction{}, nil
	}

	nets := make([]*net.IPNet, 0, len(cidrs))
	for _, cidr := range cidrs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			// CIDRでない場合は単一IPとして扱う
			ip := net.ParseIP(cidr)
			if ip == nil {
				return nil, fmt.Errorf("invalid CIDR or IP: %s", cidr)
			}
			// /32 or /128 として扱う
			bits := 32
			if ip.To4() == nil {
				bits = 128
			}
			ipNet = &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}
		}
		nets = append(nets, ipNet)
	}

	return &IPRestriction{allowedNets: nets}, nil
}

// IsAllowed はIPが許可されているか確認する
func (ir *IPRestriction) IsAllowed(ip net.IP) bool {
	if len(ir.allowedNets) == 0 {
		return true // 制限なし
	}

	for _, ipNet := range ir.allowedNets {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

// Middleware はIP制限ミドルウェアを返す
func (ir *IPRestriction) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(ir.allowedNets) == 0 {
			next.ServeHTTP(w, r)
			return
		}

		ip := getClientIP(r)
		if ip == nil || !ir.IsAllowed(ip) {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// getClientIP はクライアントIPを取得する
func getClientIP(r *http.Request) net.IP {
	// X-Forwarded-For ヘッダーをチェック
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			ip := net.ParseIP(strings.TrimSpace(ips[0]))
			if ip != nil {
				return ip
			}
		}
	}

	// X-Real-IP ヘッダーをチェック
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		ip := net.ParseIP(xri)
		if ip != nil {
			return ip
		}
	}

	// RemoteAddr から取得
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return nil
	}
	return net.ParseIP(host)
}

// AccessController はアクセス制御
type AccessController struct {
	allowedSpaces   map[string]struct{}
	allowedProjects map[string]struct{}
}

// NewAccessController は新しいアクセス制御を作成する
func NewAccessController(spaces, projects []string) *AccessController {
	ac := &AccessController{
		allowedSpaces:   make(map[string]struct{}),
		allowedProjects: make(map[string]struct{}),
	}

	for _, s := range spaces {
		ac.allowedSpaces[s] = struct{}{}
	}
	for _, p := range projects {
		ac.allowedProjects[p] = struct{}{}
	}

	return ac
}

// CheckSpace はスペースが許可されているか確認する
func (ac *AccessController) CheckSpace(space string) error {
	if len(ac.allowedSpaces) == 0 {
		return nil // 制限なし
	}

	if _, ok := ac.allowedSpaces[space]; !ok {
		return fmt.Errorf("space '%s' is not allowed", space)
	}
	return nil
}

// CheckProject はプロジェクトが許可されているか確認する
func (ac *AccessController) CheckProject(project string) error {
	if len(ac.allowedProjects) == 0 || project == "" {
		return nil // 制限なしまたはプロジェクト指定なし
	}

	if _, ok := ac.allowedProjects[project]; !ok {
		return fmt.Errorf("project '%s' is not allowed", project)
	}
	return nil
}
