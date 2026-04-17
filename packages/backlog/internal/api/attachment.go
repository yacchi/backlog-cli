package api

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
)

// extractFilename は Content-Disposition ヘッダーからファイル名を抽出する
func extractFilename(resp *http.Response) string {
	cd := resp.Header.Get("Content-Disposition")
	if cd == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(cd)
	if err != nil {
		for _, part := range strings.Split(cd, ";") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "filename=") {
				return strings.Trim(strings.TrimPrefix(part, "filename="), `"`)
			}
		}
		return ""
	}
	if name, ok := params["filename"]; ok {
		return name
	}
	if name, ok := params["filename*"]; ok {
		return name
	}
	return ""
}

// UploadedAttachment は POST /space/attachment のレスポンス
type UploadedAttachment struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	CreatedUser User   `json:"createdUser"`
	Created     string `json:"created"`
}

// UploadSpaceAttachment はファイルをスペース添付としてアップロードする
// 返された ID を issue create/edit/wiki attachment などで使用する
func (c *Client) UploadSpaceAttachment(ctx context.Context, filename string, r io.Reader) (*UploadedAttachment, error) {
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	fw, err := w.CreateFormFile("file", filename)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(fw, r); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}

	resp, err := c.RawRequest(ctx, "POST", "/api/v2/space/attachment", nil, body, w.FormDataContentType())
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if err := CheckResponse(resp); err != nil {
		return nil, err
	}

	var out UploadedAttachment
	if err := DecodeResponse(resp, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// downloadRaw は octet-stream ダウンロードの共通ヘルパー
// Content-Disposition からファイル名を取得し、データを w に書き込む
func (c *Client) downloadRaw(ctx context.Context, path string, w io.Writer) (filename string, size int64, err error) {
	resp, err := c.Get(ctx, path, nil)
	if err != nil {
		return "", 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if err := CheckResponse(resp); err != nil {
		return "", 0, err
	}

	filename = extractFilename(resp)
	size, err = io.Copy(w, resp.Body)
	return filename, size, err
}
