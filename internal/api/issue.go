package api

import (
	"context"

	"github.com/yacchi/backlog-cli/internal/backlog"
)

// Issue は課題情報
type Issue struct {
	ID             int          `json:"id"`
	ProjectID      int          `json:"projectId"`
	IssueKey       string       `json:"issueKey"`
	KeyID          int          `json:"keyId"`
	IssueType      IssueType    `json:"issueType"`
	Summary        string       `json:"summary"`
	Description    string       `json:"description"`
	Resolution     *Resolution  `json:"resolution"`
	Priority       Priority     `json:"priority"`
	Status         Status       `json:"status"`
	Assignee       *User        `json:"assignee"`
	Category       []Category   `json:"category"`
	Versions       []Version    `json:"versions"`
	Milestone      []Version    `json:"milestone"`
	StartDate      string       `json:"startDate"`
	DueDate        string       `json:"dueDate"`
	EstimatedHours float64      `json:"estimatedHours"`
	ActualHours    float64      `json:"actualHours"`
	ParentIssueID  *int         `json:"parentIssueId"`
	CreatedUser    User         `json:"createdUser"`
	Created        string       `json:"created"`
	UpdatedUser    *User        `json:"updatedUser"`
	Updated        string       `json:"updated"`
	Attachments    []Attachment `json:"attachments"`
	SharedFiles    []SharedFile `json:"sharedFiles"`
	Stars          []Star       `json:"stars"`
}

// Resolution は完了理由
type Resolution struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Priority は優先度
type Priority struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Attachment は添付ファイル
type Attachment struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	CreatedUser User   `json:"createdUser"`
	Created     string `json:"created"`
}

// SharedFile は共有ファイル
type SharedFile struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	Dir         string `json:"dir"`
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	CreatedUser User   `json:"createdUser"`
	Created     string `json:"created"`
	UpdatedUser *User  `json:"updatedUser"`
	Updated     string `json:"updated"`
}

// Star はスター
type Star struct {
	ID        int    `json:"id"`
	Comment   string `json:"comment"`
	URL       string `json:"url"`
	Title     string `json:"title"`
	Presenter User   `json:"presenter"`
	Created   string `json:"created"`
}

// IssueListOptions は課題一覧取得オプション
type IssueListOptions struct {
	ProjectIDs     []int
	IssueTypeIDs   []int
	CategoryIDs    []int
	VersionIDs     []int
	MilestoneIDs   []int
	StatusIDs      []int
	PriorityIDs    []int
	AssigneeIDs    []int
	CreatedUserIDs []int
	ResolutionIDs  []int
	ParentChild    int // 0: all, 1: exclude child, 2: child only, 3: not parent or child, 4: parent only
	Attachment     *bool
	SharedFile     *bool
	Sort           string
	Order          string // asc, desc
	Offset         int
	Count          int
	CreatedSince   string
	CreatedUntil   string
	UpdatedSince   string
	UpdatedUntil   string
	StartDateSince string
	StartDateUntil string
	DueDateSince   string
	DueDateUntil   string
	IDs            []int
	ParentIssueIDs []int
	Keyword        string
}

// GetIssues は課題一覧を取得する
func (c *Client) GetIssues(opts *IssueListOptions) ([]backlog.Issue, error) {
	params := backlog.GetIssuesParams{}
	if opts != nil {
		params.ProjectId = opts.ProjectIDs
		params.IssueTypeId = opts.IssueTypeIDs
		params.CategoryId = opts.CategoryIDs
		params.VersionId = opts.VersionIDs
		params.MilestoneId = opts.MilestoneIDs
		params.StatusId = opts.StatusIDs
		params.PriorityId = opts.PriorityIDs
		params.AssigneeId = opts.AssigneeIDs
		params.CreatedUserId = opts.CreatedUserIDs
		params.ResolutionId = opts.ResolutionIDs
		params.ID = opts.IDs
		params.ParentIssueId = opts.ParentIssueIDs

		if opts.ParentChild > 0 {
			params.ParentChild = backlog.NewOptInt(opts.ParentChild)
		}
		if opts.Attachment != nil {
			params.Attachment = backlog.NewOptBool(*opts.Attachment)
		}
		if opts.SharedFile != nil {
			params.SharedFile = backlog.NewOptBool(*opts.SharedFile)
		}
		if opts.Sort != "" {
			params.Sort = backlog.NewOptString(opts.Sort)
		}
		if opts.Order != "" {
			params.Order = backlog.NewOptString(opts.Order)
		}
		if opts.Offset > 0 {
			params.Offset = backlog.NewOptInt(opts.Offset)
		}
		if opts.Count > 0 {
			params.Count = backlog.NewOptInt(opts.Count)
		}
		if opts.Keyword != "" {
			params.Keyword = backlog.NewOptString(opts.Keyword)
		}
		if opts.CreatedSince != "" {
			params.CreatedSince = backlog.NewOptString(opts.CreatedSince)
		}
		if opts.CreatedUntil != "" {
			params.CreatedUntil = backlog.NewOptString(opts.CreatedUntil)
		}
		if opts.UpdatedSince != "" {
			params.UpdatedSince = backlog.NewOptString(opts.UpdatedSince)
		}
		if opts.UpdatedUntil != "" {
			params.UpdatedUntil = backlog.NewOptString(opts.UpdatedUntil)
		}
		if opts.StartDateSince != "" {
			params.StartDateSince = backlog.NewOptString(opts.StartDateSince)
		}
		if opts.StartDateUntil != "" {
			params.StartDateUntil = backlog.NewOptString(opts.StartDateUntil)
		}
		if opts.DueDateSince != "" {
			params.DueDateSince = backlog.NewOptString(opts.DueDateSince)
		}
		if opts.DueDateUntil != "" {
			params.DueDateUntil = backlog.NewOptString(opts.DueDateUntil)
		}
	}

	return c.backlogClient.GetIssues(context.TODO(), params)
}

// GetIssuesCount は課題数を取得する
func (c *Client) GetIssuesCount(opts *IssueListOptions) (int, error) {
	params := backlog.GetIssuesCountParams{}
	if opts != nil {
		params.ProjectId = opts.ProjectIDs
		params.StatusId = opts.StatusIDs
	}

	res, err := c.backlogClient.GetIssuesCount(context.TODO(), params)
	if err != nil {
		return 0, err
	}

	if res.Count.IsSet() {
		return res.Count.Value, nil
	}
	return 0, nil
}

// GetIssue は課題を取得する
func (c *Client) GetIssue(issueIDOrKey string) (*backlog.Issue, error) {
	return c.backlogClient.GetIssue(context.TODO(), backlog.GetIssueParams{
		IssueIdOrKey: issueIDOrKey,
	})
}

// CreateIssueInput は課題作成の入力
type CreateIssueInput struct {
	ProjectID      int
	Summary        string
	IssueTypeID    int
	PriorityID     int
	Description    string
	StartDate      string
	DueDate        string
	EstimatedHours float64
	ActualHours    float64
	CategoryIDs    []int
	VersionIDs     []int
	MilestoneIDs   []int
	AssigneeID     int
	ParentIssueID  int
}

// CreateIssue は課題を作成する
func (c *Client) CreateIssue(input *CreateIssueInput) (*backlog.Issue, error) {
	req := backlog.CreateIssueReq{
		ProjectId:   input.ProjectID,
		Summary:     input.Summary,
		IssueTypeId: input.IssueTypeID,
		PriorityId:  input.PriorityID,
		CategoryId:  input.CategoryIDs,
		VersionId:   input.VersionIDs,
		MilestoneId: input.MilestoneIDs,
	}

	if input.Description != "" {
		req.Description = backlog.NewOptString(input.Description)
	}
	if input.StartDate != "" {
		req.StartDate = backlog.NewOptString(input.StartDate)
	}
	if input.DueDate != "" {
		req.DueDate = backlog.NewOptString(input.DueDate)
	}
	if input.EstimatedHours > 0 {
		req.EstimatedHours = backlog.NewOptFloat64(input.EstimatedHours)
	}
	if input.ActualHours > 0 {
		req.ActualHours = backlog.NewOptFloat64(input.ActualHours)
	}
	if input.AssigneeID > 0 {
		req.AssigneeId = backlog.NewOptInt(input.AssigneeID)
	}
	if input.ParentIssueID > 0 {
		req.ParentIssueId = backlog.NewOptInt(input.ParentIssueID)
	}

	return c.backlogClient.CreateIssue(context.TODO(), backlog.NewOptCreateIssueReq(req))
}

// UpdateIssueInput は課題更新の入力
type UpdateIssueInput struct {
	Summary        *string
	Description    *string
	StatusID       *int
	ResolutionID   *int
	StartDate      *string
	DueDate        *string
	EstimatedHours *float64
	ActualHours    *float64
	AssigneeID     *int
	CategoryIDs    []int
	VersionIDs     []int
	MilestoneIDs   []int
	PriorityID     *int
	IssueTypeID    *int
	Comment        *string
}

// UpdateIssue は課題を更新する
func (c *Client) UpdateIssue(issueIDOrKey string, input *UpdateIssueInput) (*backlog.Issue, error) {
	req := backlog.UpdateIssueReq{}

	if input.Summary != nil {
		req.Summary = backlog.NewOptString(*input.Summary)
	}
	if input.Description != nil {
		req.Description = backlog.NewOptString(*input.Description)
	}
	if input.StatusID != nil {
		req.StatusId = backlog.NewOptInt(*input.StatusID)
	}
	if input.ResolutionID != nil {
		req.ResolutionId = backlog.NewOptInt(*input.ResolutionID)
	}
	if input.PriorityID != nil {
		req.PriorityId = backlog.NewOptInt(*input.PriorityID)
	}
	if input.IssueTypeID != nil {
		req.IssueTypeId = backlog.NewOptInt(*input.IssueTypeID)
	}
	if input.StartDate != nil {
		req.StartDate = backlog.NewOptString(*input.StartDate)
	}
	if input.DueDate != nil {
		req.DueDate = backlog.NewOptString(*input.DueDate)
	}
	if input.EstimatedHours != nil {
		req.EstimatedHours = backlog.NewOptFloat64(*input.EstimatedHours)
	}
	if input.ActualHours != nil {
		req.ActualHours = backlog.NewOptFloat64(*input.ActualHours)
	}
	if input.AssigneeID != nil {
		req.AssigneeId = backlog.NewOptInt(*input.AssigneeID)
	}
	if input.Comment != nil {
		req.Comment = backlog.NewOptString(*input.Comment)
	}

	req.CategoryId = input.CategoryIDs
	req.VersionId = input.VersionIDs
	req.MilestoneId = input.MilestoneIDs

	return c.backlogClient.UpdateIssue(context.TODO(), backlog.NewOptUpdateIssueReq(req), backlog.UpdateIssueParams{
		IssueIdOrKey: issueIDOrKey,
	})
}

// DeleteIssue は課題を削除する
func (c *Client) DeleteIssue(issueIDOrKey string) (*backlog.Issue, error) {
	return c.backlogClient.DeleteIssue(context.TODO(), backlog.DeleteIssueParams{
		IssueIdOrKey: issueIDOrKey,
	})
}