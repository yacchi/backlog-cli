package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

// FileInfo はプロジェクト共有ファイルのメタ情報
type FileInfo struct {
	ID          int    `json:"id"`
	Type        string `json:"type"` // "file" or "directory"
	Dir         string `json:"dir"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	CreatedUser User   `json:"createdUser"`
	Created     string `json:"created"`
	UpdatedUser *User  `json:"updatedUser"`
	Updated     string `json:"updated"`
}

// FileListOptions は共有ファイル一覧取得オプション
type FileListOptions struct {
	Order  string
	Offset int
	Count  int
}

// ListProjectFiles はプロジェクトの共有ファイル一覧を取得する
// dirPath はディレクトリのパス (例: "/" または "/docs/sub")
// パス区切り "/" をそのままリクエストするため raw HTTP で実装する
func (c *Client) ListProjectFiles(ctx context.Context, projectIDOrKey, dirPath string, opts *FileListOptions) ([]FileInfo, error) {
	// path の各セグメントをエスケープしつつ、区切り "/" は保持する
	segments := strings.Split(strings.TrimPrefix(dirPath, "/"), "/")
	escapedSegs := make([]string, 0, len(segments))
	for _, seg := range segments {
		if seg != "" {
			escapedSegs = append(escapedSegs, url.PathEscape(seg))
		}
	}
	encodedPath := strings.Join(escapedSegs, "/")

	apiPath := fmt.Sprintf("/projects/%s/files/metadata/%s", url.PathEscape(projectIDOrKey), encodedPath)

	query := url.Values{}
	if opts != nil {
		if opts.Order != "" {
			query.Set("order", opts.Order)
		}
		if opts.Offset > 0 {
			query.Set("offset", strconv.Itoa(opts.Offset))
		}
		if opts.Count > 0 {
			query.Set("count", strconv.Itoa(opts.Count))
		}
	}

	resp, err := c.Get(ctx, apiPath, query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if err := CheckResponse(resp); err != nil {
		return nil, err
	}

	var files []FileInfo
	if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
		return nil, err
	}
	return files, nil
}

// DownloadProjectFile はプロジェクト共有ファイルをダウンロードする
func (c *Client) DownloadProjectFile(ctx context.Context, projectIDOrKey string, sharedFileID int, w io.Writer) (string, int64, error) {
	path := fmt.Sprintf("/projects/%s/files/%d", url.PathEscape(projectIDOrKey), sharedFileID)
	return c.downloadRaw(ctx, path, w)
}
