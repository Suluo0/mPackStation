package provider

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
)

// DownloadToFile 把直连 URL（mrpack manifest 的 CDN 地址等）下载到本地文件，
// 返回实际 sha1 与字节数。导入管道专用：mrpack 清单只给直链，不给版本号，
// 走不了按版本查询的 Download 流程。调用方（service 层）负责重试与目录创建。
func DownloadToFile(ctx context.Context, rawURL, destPath string) (string, int64, error) {
	if rawURL == "" {
		return "", 0, ErrNotFound
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if e != nil {
		return "", 0, ErrNotFound
	}
	resp, e := http.DefaultClient.Do(req)
	if e != nil {
		return "", 0, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("%w: download returned HTTP %d", ErrUnavailable, resp.StatusCode)
	}
	const maxBytes = 512 * 1024 * 1024
	tmp := destPath + ".part"
	f, e := os.Create(tmp)
	if e != nil {
		return "", 0, errFileWrite(e)
	}
	hash := sha1.New()
	size, e := io.Copy(io.MultiWriter(f, hash), io.LimitReader(resp.Body, maxBytes+1))
	closeErr := f.Close()
	if e != nil {
		_ = os.Remove(tmp)
		return "", 0, ErrUnavailable
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return "", 0, errFileWrite(closeErr)
	}
	if size > maxBytes {
		_ = os.Remove(tmp)
		return "", 0, fmt.Errorf("download exceeds %d MB limit", maxBytes/(1024*1024))
	}
	if e := os.Rename(tmp, destPath); e != nil {
		_ = os.Remove(tmp)
		return "", 0, errFileWrite(e)
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

// FetchCFFile 按 CurseForge 的 projectID/fileID 下载单个文件（完整清单导入专用，
// CF 的 zip 清单只给这对 ID 不给直链；需要已配置 key 的适配器）。
func (h *HTTPAdapter) FetchCFFile(ctx context.Context, projectID, fileID int64, destPath string) (string, int64, error) {
	u := fmt.Sprintf("%s/v1/mods/%d/files/%d/download", h.base.String(), projectID, fileID)
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if e != nil {
		return "", 0, ErrNotFound
	}
	if h.token != "" {
		req.Header.Set("x-api-key", h.token)
	}
	resp, e := h.client.Do(req)
	if e != nil {
		return "", 0, ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("%w: curseforge file download returned HTTP %d", ErrUnavailable, resp.StatusCode)
	}
	const maxBytes = 512 * 1024 * 1024
	tmp := destPath + ".part"
	f, e := os.Create(tmp)
	if e != nil {
		return "", 0, errFileWrite(e)
	}
	hash := sha1.New()
	size, e := io.Copy(io.MultiWriter(f, hash), io.LimitReader(resp.Body, maxBytes+1))
	closeErr := f.Close()
	if e != nil {
		_ = os.Remove(tmp)
		return "", 0, ErrUnavailable
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return "", 0, errFileWrite(closeErr)
	}
	if size > maxBytes {
		_ = os.Remove(tmp)
		return "", 0, fmt.Errorf("download exceeds %d MB limit", maxBytes/(1024*1024))
	}
	if e := os.Rename(tmp, destPath); e != nil {
		_ = os.Remove(tmp)
		return "", 0, errFileWrite(e)
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func errFileWrite(e error) error {
	return fmt.Errorf("write download file: %w", e)
}
