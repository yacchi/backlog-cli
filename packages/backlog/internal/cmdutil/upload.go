package cmdutil

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/ui"
)

// UploadFiles は複数ファイルを並行アップロードし、添付IDのスライスを返す
// 入力順と同じ順序で結果を返す。いずれかのファイルが失敗した場合はエラーを返す。
func UploadFiles(ctx context.Context, client *api.Client, filePaths []string) ([]int, error) {
	if len(filePaths) == 0 {
		return nil, nil
	}

	if len(filePaths) == 1 {
		id, err := uploadSingleFile(ctx, client, filePaths[0])
		if err != nil {
			return nil, err
		}
		return []int{id}, nil
	}

	type result struct {
		id  int
		err error
	}
	results := make([]result, len(filePaths))
	var wg sync.WaitGroup

	for i, fp := range filePaths {
		wg.Add(1)
		go func(idx int, filePath string) {
			defer wg.Done()
			id, err := uploadSingleFile(ctx, client, filePath)
			results[idx] = result{id: id, err: err}
		}(i, fp)
	}
	wg.Wait()

	ids := make([]int, len(filePaths))
	for i, r := range results {
		if r.err != nil {
			return nil, r.err
		}
		ids[i] = r.id
	}
	return ids, nil
}

func uploadSingleFile(ctx context.Context, client *api.Client, filePath string) (int, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("failed to open %s: %w", filePath, err)
	}
	defer func() { _ = f.Close() }()

	name := filepath.Base(filePath)

	var r io.Reader = f
	if info, statErr := f.Stat(); statErr == nil {
		r = ui.NewProgressReader(f, "Uploading "+name, info.Size())
	}

	up, err := client.UploadSpaceAttachment(ctx, name, r)
	if err != nil {
		return 0, fmt.Errorf("failed to upload %s: %w", filePath, err)
	}
	return up.ID, nil
}
