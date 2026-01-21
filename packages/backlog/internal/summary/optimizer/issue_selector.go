package optimizer

import (
	"context"
	"fmt"
	"strings"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
	backlog "github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
)

// IssueSelector は課題選定機能を提供する
type IssueSelector struct {
	client *api.Client
}

// NewIssueSelector は新しいIssueSelectorを作成する
func NewIssueSelector(client *api.Client) *IssueSelector {
	return &IssueSelector{client: client}
}

// CandidateIssue は選定候補の課題情報
type CandidateIssue struct {
	Key            string
	Summary        string
	Description    string
	IssueType      string
	Status         string
	CommentCount   int
	DescriptionLen int
	HasComments    bool
}

// FetchCandidateIssues は指定プロジェクトから候補課題を取得する
func (s *IssueSelector) FetchCandidateIssues(ctx context.Context, projectKeys []string, limit int) ([]CandidateIssue, error) {
	// 課題一覧取得オプション
	// 最近更新された課題を優先するため、updated降順でソート
	opts := &api.IssueListOptions{
		Count: limit,
		Sort:  "updated",
		Order: "desc",
	}

	// プロジェクトが指定されている場合
	if len(projectKeys) > 0 {
		projectIDs := make([]int, 0, len(projectKeys))
		for _, projectKey := range projectKeys {
			// プロジェクト情報を取得してIDを取得
			project, err := s.client.GetProject(ctx, projectKey)
			if err != nil {
				return nil, fmt.Errorf("failed to get project %s: %w", projectKey, err)
			}
			projectIDs = append(projectIDs, project.ID)
		}
		opts.ProjectIDs = projectIDs
	}

	issues, err := s.client.GetIssues(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch issues: %w", err)
	}

	candidates := make([]CandidateIssue, 0, len(issues))
	for _, issue := range issues {
		desc := issue.GetDescription().Value
		issueType := getIssueTypeName(issue.GetIssueType())
		status := getStatusName(issue.GetStatus())
		candidates = append(candidates, CandidateIssue{
			Key:            issue.GetIssueKey().Value,
			Summary:        issue.GetSummary().Value,
			Description:    desc,
			IssueType:      issueType,
			Status:         status,
			DescriptionLen: len(desc),
			HasComments:    false, // コメント数は別途取得が必要
		})
	}

	return candidates, nil
}

// getIssueTypeName は OptIssueType から課題タイプ名を取得する
func getIssueTypeName(opt backlog.OptIssueType) string {
	if !opt.IsSet() {
		return ""
	}
	return opt.Value.GetName().Value
}

// getStatusName は OptStatus からステータス名を取得する
func getStatusName(opt backlog.OptStatus) string {
	if !opt.IsSet() {
		return ""
	}
	return opt.Value.GetName().Value
}

// FetchIssueWithComments は課題とそのコメントを取得する
func (s *IssueSelector) FetchIssueWithComments(ctx context.Context, issueKey string, commentLimit int) (*CandidateIssue, []string, error) {
	// 課題詳細を取得
	issue, err := s.client.GetIssue(ctx, issueKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch issue %s: %w", issueKey, err)
	}

	desc := issue.GetDescription().Value
	issueType := getIssueTypeName(issue.GetIssueType())
	status := getStatusName(issue.GetStatus())
	candidate := &CandidateIssue{
		Key:            issue.GetIssueKey().Value,
		Summary:        issue.GetSummary().Value,
		Description:    desc,
		IssueType:      issueType,
		Status:         status,
		DescriptionLen: len(desc),
	}

	// コメントを取得
	opts := &api.CommentListOptions{
		Count: commentLimit,
		Order: "desc",
	}

	comments, err := s.client.GetComments(ctx, issueKey, opts)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch comments for %s: %w", issueKey, err)
	}

	commentTexts := make([]string, 0, len(comments))
	// 逆順で追加（古い順）
	for i := len(comments) - 1; i >= 0; i-- {
		content := comments[i].Content
		if content != "" {
			commentTexts = append(commentTexts, content)
		}
	}

	candidate.CommentCount = len(commentTexts)
	candidate.HasComments = len(commentTexts) > 0

	return candidate, commentTexts, nil
}

// FormatCandidatesForSelection は評価モデル用に候補課題をフォーマットする
func FormatCandidatesForSelection(candidates []CandidateIssue) string {
	var result string
	for i, c := range candidates {
		result += fmt.Sprintf("%d. %s\n", i+1, c.Key)
		result += fmt.Sprintf("   タイプ: %s\n", c.IssueType)
		result += fmt.Sprintf("   ステータス: %s\n", c.Status)
		result += fmt.Sprintf("   タイトル: %s\n", c.Summary)
		// 説明文の先頭部分を含める（選定の判断材料として）
		if c.Description != "" {
			descPreview := truncateDescription(c.Description, 200)
			result += fmt.Sprintf("   説明文: %s\n", descPreview)
		}
		result += fmt.Sprintf("   説明文長: %d文字\n", c.DescriptionLen)
		if c.HasComments {
			result += fmt.Sprintf("   コメント数: %d\n", c.CommentCount)
		}
		result += "\n"
	}
	return result
}

// truncateDescription は説明文を指定文字数で切り詰める
func truncateDescription(desc string, maxLen int) string {
	// 改行を空白に置換してプレビュー用に整形
	desc = strings.ReplaceAll(desc, "\n", " ")
	desc = strings.ReplaceAll(desc, "\r", "")

	runes := []rune(desc)
	if len(runes) <= maxLen {
		return desc
	}
	return string(runes[:maxLen]) + "..."
}
