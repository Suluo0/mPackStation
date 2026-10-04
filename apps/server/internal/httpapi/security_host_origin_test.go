package httpapi

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"mpackstation/internal/store"
)

/* 跨机(局域网)访问下的 Host/Origin 守卫回归。
   背景:前端 vite dev 绑 0.0.0.0,浏览器送到的 Host 是局域网地址,
   而代理 changeOrigin:false 原样透传,守卫若只认回环会把所有 /api 打死成 400。 */

func secHandler(t *testing.T) http.Handler {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "security.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewRouter(db, "test")
}

func secDo(t *testing.T, h http.Handler, method, path, host, origin string, write bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.Host = host
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	req.Header.Set("X-Request-ID", "sec-"+strings.ReplaceAll(host, ":", "-"))
	if write {
		req.Header.Set("X-MPack-Token", "test")
		req.Header.Set("Content-Type", "application/json")
	}
	res := httptest.NewRecorder()
	h.ServeHTTP(res, req)
	return res
}

func secGuardCode(t *testing.T, res *httptest.ResponseRecorder) string {
	t.Helper()
	b := res.Body.String()
	switch {
	case strings.Contains(b, `"invalid_host"`):
		return "invalid_host"
	case strings.Contains(b, `"invalid_origin"`):
		return "invalid_origin"
	}
	return ""
}

// 局域网真实访问链路必须能读到接口:Host 是本机私有地址。
func TestSecurityReadsAllowedFromPrivateHost(t *testing.T) {
	h := secHandler(t)
	for _, host := range []string{"192.168.0.100:5273", "10.144.144.4:5273", "172.16.3.4:5273", "[fd00::1]:5273"} {
		res := secDo(t, h, http.MethodGet, "/api/health", host, "", false)
		if code := secGuardCode(t, res); code != "" {
			t.Fatalf("Host %s blocked by %s (body=%s), want pass-through", host, code, res.Body.String())
		}
	}
}

// 写请求带同源局域网 Origin 时不得被判非法 Origin。
func TestSecurityWritesAllowedFromSameOriginLAN(t *testing.T) {
	h := secHandler(t)
	res := secDo(t, h, http.MethodPost, "/api/export-dirs", "192.168.0.100:5273", "http://192.168.0.100:5273", true)
	if code := secGuardCode(t, res); code != "" {
		t.Fatalf("same-origin LAN write blocked by %s (body=%s)", code, res.Body.String())
	}
}

// 白名单里的孤儿条目与公网地址必须继续拒绝:这是防 DNS rebinding 的正身。
func TestSecurityHostStillRejectsRebindingVectors(t *testing.T) {
	h := secHandler(t)
	for _, tc := range []struct{ name, host, origin string }{
		{"example_com_leftover", "example.com", ""},
		{"attacker_hostname", "evil.example", ""},
		{"rebind_same_origin", "evil.example:18871", "http://evil.example:18871"},
		{"public_ip_literal", "8.8.8.8:18871", ""},
	} {
		res := secDo(t, h, http.MethodGet, "/api/health", tc.host, tc.origin, false)
		if res.Code != http.StatusBadRequest || !strings.Contains(res.Body.String(), `"invalid_host"`) {
			t.Fatalf("%s: status=%d body=%s, want 400 invalid_host", tc.name, res.Code, res.Body.String())
		}
	}
}

// 回环访问的跨站 Origin 仍然拒绝(自定义头 + Origin 双保险不能退化)。
func TestSecurityCrossSiteOriginRejected(t *testing.T) {
	h := secHandler(t)
	res := secDo(t, h, http.MethodGet, "/api/health", "127.0.0.1:18871", "https://evil.example", false)
	if res.Code != http.StatusForbidden || !strings.Contains(res.Body.String(), `"invalid_origin"`) {
		t.Fatalf("status=%d body=%s, want 403 invalid_origin", res.Code, res.Body.String())
	}
}

// 自定义域名(mDNS/主机名)通过 MPACK_ALLOWED_HOSTS 显式放行。
func TestSecurityAllowedHostsEnv(t *testing.T) {
	t.Setenv("MPACK_ALLOWED_HOSTS", "mpack-box.local, other.host ")
	h := secHandler(t)
	res := secDo(t, h, http.MethodGet, "/api/health", "mpack-box.local:5273", "http://mpack-box.local:5273", false)
	if code := secGuardCode(t, res); code != "" {
		t.Fatalf("allowed host via env blocked by %s (body=%s)", code, res.Body.String())
	}
	res = secDo(t, h, http.MethodGet, "/api/health", "not-listed.local:5273", "", false)
	if res.Code != http.StatusBadRequest {
		t.Fatalf("unlisted host status=%d, want 400", res.Code)
	}
}

// MPACK_FRONTEND_ORIGIN 支持逗号多值,且不再因此打断回环默认放行。
func TestSecurityFrontendOriginListKeepsLoopback(t *testing.T) {
	t.Setenv("MPACK_FRONTEND_ORIGIN", "http://192.168.0.100:5273,http://mpack-box.local:5273")
	h := secHandler(t)
	for _, host := range []string{"127.0.0.1:18871", "localhost:18871"} {
		res := secDo(t, h, http.MethodPost, "/api/export-dirs", host, "http://127.0.0.1:5273", true)
		if code := secGuardCode(t, res); code != "" {
			t.Fatalf("loopback write with env origin set blocked by %s (body=%s)", code, res.Body.String())
		}
	}
	res := secDo(t, h, http.MethodPost, "/api/export-dirs", "127.0.0.1:18871", "http://mpack-box.local:5273", true)
	if code := secGuardCode(t, res); code != "" {
		t.Fatalf("listed cross-host origin blocked by %s (body=%s)", code, res.Body.String())
	}
	res = secDo(t, h, http.MethodPost, "/api/export-dirs", "127.0.0.1:18871", "http://evil.example:5273", true)
	if res.Code != http.StatusForbidden {
		t.Fatalf("unlisted origin status=%d body=%s, want 403", res.Code, res.Body.String())
	}
}
