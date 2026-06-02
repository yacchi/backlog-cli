package activity

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// activityType はアクティビティ種別のセマンティック名と Backlog の activityTypeId の対応。
// Backlog のアクティビティタイプ表 (type 1-26) に準拠する。
type activityType struct {
	Name string
	ID   int
}

// activityTypes はセマンティック名 ↔ activityTypeId のマッピング定義（表示順）。
var activityTypes = []activityType{
	{"issue-create", 1},
	{"issue-update", 2},
	{"issue-comment", 3},
	{"issue-delete", 4},
	{"wiki-create", 5},
	{"wiki-update", 6},
	{"wiki-delete", 7},
	{"file-add", 8},
	{"file-update", 9},
	{"file-delete", 10},
	{"svn-commit", 11},
	{"git-push", 12},
	{"git-repo-create", 13},
	{"issue-bulk-update", 14},
	{"project-user-add", 15},
	{"project-user-remove", 16},
	{"notify-add", 17},
	{"pr-add", 18},
	{"pr-update", 19},
	{"pr-comment", 20},
	{"pr-delete", 21},
	{"milestone-add", 22},
	{"milestone-update", 23},
	{"milestone-delete", 24},
	{"group-project-add", 25},
	{"group-project-remove", 26},
}

var (
	nameToID = func() map[string]int {
		m := make(map[string]int, len(activityTypes))
		for _, t := range activityTypes {
			m[t.Name] = t.ID
		}
		return m
	}()
	idToName = func() map[int]string {
		m := make(map[int]string, len(activityTypes))
		for _, t := range activityTypes {
			m[t.ID] = t.Name
		}
		return m
	}()
)

// defaultActivityTypes は --type 省略時に使う課題系のデフォルト種別。
var defaultActivityTypes = []string{"issue-create", "issue-update", "issue-comment"}

// ParseTypes はカンマ区切りのセマンティック名（または数値ID）を activityTypeId のスライスに変換する。
// 重複は除去し、未知の名前はエラーを返す。
func ParseTypes(csv string) ([]int, error) {
	seen := make(map[int]struct{})
	var ids []int
	for _, part := range strings.Split(csv, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		id, ok := nameToID[name]
		if !ok {
			// 数値 ID も許容する
			if n, err := strconv.Atoi(name); err == nil && n >= 1 && n <= 26 {
				id = n
			} else {
				return nil, fmt.Errorf("unknown activity type %q (see 'backlog activity list --help' for valid names)", name)
			}
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no valid activity type specified")
	}
	return ids, nil
}

// TypeName は activityTypeId を表示用のセマンティック名に変換する。未知の場合は "type-N" を返す。
func TypeName(id int) string {
	if name, ok := idToName[id]; ok {
		return name
	}
	return fmt.Sprintf("type-%d", id)
}

// TypeNames は利用可能なセマンティック名を ID 順で返す（ヘルプ表示用）。
func TypeNames() []string {
	names := make([]string, 0, len(activityTypes))
	ids := make([]int, 0, len(activityTypes))
	for _, t := range activityTypes {
		ids = append(ids, t.ID)
	}
	sort.Ints(ids)
	for _, id := range ids {
		names = append(names, fmt.Sprintf("%s(%d)", idToName[id], id))
	}
	return names
}
