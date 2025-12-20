# Phase 06: APIクライアント

## 目標

- Backlog API v2 クライアントの実装
- 自動トークン更新
- エラーハンドリング
- ページネーション対応

## 1. クライアント基盤

### internal/api/client.go

```go
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/yourorg/backlog-cli/internal/config"
)

// Client は Backlog API クライアント
type Client struct {
	space       string
	domain      string
	accessToken string
	httpClient  *http.Client
	
	// トークン更新用
	refreshToken string
	expiresAt    time.Time
	relayServer  string
	onTokenUpdate func(accessToken, refreshToken string, expiresAt time.Time)
}

// ClientOption はクライアントオプション
type ClientOption func(*Client)

// WithTokenRefresh はトークン自動更新を有効にする
func WithTokenRefresh(refreshToken, relayServer string, expiresAt time.Time, callback func(string, string, time.Time)) ClientOption {
	return func(c *Client) {
		c.refreshToken = refreshToken
		c.relayServer = relayServer
		c.expiresAt = expiresAt
		c.onTokenUpdate = callback
	}
}

// NewClient は新しいクライアントを作成する
func NewClient(space, domain, accessToken string, opts ...ClientOption) *Client {
	c := &Client{
		space:       space,
		domain:      domain,
		accessToken: accessToken,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
	
	for _, opt := range opts {
		opt(c)
	}
	
	return c
}

// NewClientFromConfig は設定からクライアントを作成する
func NewClientFromConfig(cfg *config.Config, resolved *config.ResolvedConfig) (*Client, error) {
	if resolved.Credential == nil {
		return nil, fmt.Errorf("not authenticated")
	}
	
	cred := resolved.Credential
	
	client := NewClient(
		resolved.Space,
		resolved.Domain,
		cred.AccessToken,
		WithTokenRefresh(
			cred.RefreshToken,
			resolved.RelayServer,
			cred.ExpiresAt,
			func(accessToken, refreshToken string, expiresAt time.Time) {
				// 設定ファイルを更新
				host := resolved.Space + "." + resolved.Domain
				if cfg.Client.Credentials == nil {
					cfg.Client.Credentials = make(map[string]config.Credential)
				}
				cfg.Client.Credentials[host] = config.Credential{
					AccessToken:  accessToken,
					RefreshToken: refreshToken,
					ExpiresAt:    expiresAt,
					UserID:       cred.UserID,
					UserName:     cred.UserName,
				}
				config.Save(cfg)
			},
		),
	)
	
	return client, nil
}

func (c *Client) baseURL() string {
	return fmt.Sprintf("https://%s.%s/api/v2", c.space, c.domain)
}

// ensureValidToken はトークンが有効か確認し、必要なら更新する
func (c *Client) ensureValidToken() error {
	if c.refreshToken == "" || c.relayServer == "" {
		return nil // 自動更新なし
	}
	
	// 有効期限の5分前に更新
	if time.Now().Add(5 * time.Minute).Before(c.expiresAt) {
		return nil // まだ有効
	}
	
	// トークン更新
	return c.doRefreshToken()
}

func (c *Client) doRefreshToken() error {
	reqBody := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": c.refreshToken,
		"domain":        c.domain,
		"space":         c.space,
	}
	
	body, _ := json.Marshal(reqBody)
	resp, err := c.httpClient.Post(
		c.relayServer+"/auth/token",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		return fmt.Errorf("token refresh request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token refresh failed with status %d", resp.StatusCode)
	}
	
	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("failed to parse token response: %w", err)
	}
	
	// 更新
	c.accessToken = tokenResp.AccessToken
	c.refreshToken = tokenResp.RefreshToken
	c.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	
	// コールバック
	if c.onTokenUpdate != nil {
		c.onTokenUpdate(c.accessToken, c.refreshToken, c.expiresAt)
	}
	
	return nil
}

// Request はAPIリクエストを実行する
func (c *Client) Request(method, path string, query url.Values, body interface{}) (*http.Response, error) {
	if err := c.ensureValidToken(); err != nil {
		return nil, fmt.Errorf("token refresh failed: %w", err)
	}
	
	u := c.baseURL() + path
	if query != nil && len(query) > 0 {
		u += "?" + query.Encode()
	}
	
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewReader(data)
	}
	
	req, err := http.NewRequest(method, u, reqBody)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	
	return c.httpClient.Do(req)
}

// Get はGETリクエストを実行する
func (c *Client) Get(path string, query url.Values) (*http.Response, error) {
	return c.Request("GET", path, query, nil)
}

// Post はPOSTリクエストを実行する
func (c *Client) Post(path string, body interface{}) (*http.Response, error) {
	return c.Request("POST", path, nil, body)
}

// Patch はPATCHリクエストを実行する
func (c *Client) Patch(path string, body interface{}) (*http.Response, error) {
	return c.Request("PATCH", path, nil, body)
}

// Delete はDELETEリクエストを実行する
func (c *Client) Delete(path string) (*http.Response, error) {
	return c.Request("DELETE", path, nil, nil)
}
```

## 2. エラーハンドリング

### internal/api/error.go

```go
package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// APIError は Backlog API エラー
type APIError struct {
	StatusCode int
	Errors     []ErrorDetail `json:"errors"`
}

// ErrorDetail はエラー詳細
type ErrorDetail struct {
	Message  string `json:"message"`
	Code     int    `json:"code"`
	MoreInfo string `json:"moreInfo"`
}

func (e *APIError) Error() string {
	if len(e.Errors) > 0 {
		return fmt.Sprintf("Backlog API error: %s (code: %d)", e.Errors[0].Message, e.Errors[0].Code)
	}
	return fmt.Sprintf("Backlog API error: status %d", e.StatusCode)
}

// CheckResponse はレスポンスをチェックし、エラーがあれば返す
func CheckResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	
	apiErr := &APIError{StatusCode: resp.StatusCode}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return apiErr
	}
	
	// Backlog API のエラー形式をパース
	var errResp struct {
		Errors []ErrorDetail `json:"errors"`
	}
	if json.Unmarshal(body, &errResp) == nil {
		apiErr.Errors = errResp.Errors
	}
	
	return apiErr
}

// DecodeResponse はレスポンスをデコードする
func DecodeResponse(resp *http.Response, v interface{}) error {
	if err := CheckResponse(resp); err != nil {
		return err
	}
	
	if v == nil {
		return nil
	}
	
	return json.NewDecoder(resp.Body).Decode(v)
}
```

## 3. ユーザーAPI

### internal/api/user.go

```go
package api

import (
	"fmt"
)

// User はユーザー情報
type User struct {
	ID          int         `json:"id"`
	UserID      string      `json:"userId"`
	Name        string      `json:"name"`
	RoleType    int         `json:"roleType"`
	Lang        string      `json:"lang"`
	MailAddress string      `json:"mailAddress"`
	NulabAccount *NulabAccount `json:"nulabAccount"`
	LastLoginTime string    `json:"lastLoginTime"`
}

// NulabAccount は Nulab アカウント情報
type NulabAccount struct {
	NulabID  string `json:"nulabId"`
	Name     string `json:"name"`
	UniqueID string `json:"uniqueId"`
}

// GetCurrentUser は認証ユーザーの情報を取得する
func (c *Client) GetCurrentUser() (*User, error) {
	resp, err := c.Get("/users/myself", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var user User
	if err := DecodeResponse(resp, &user); err != nil {
		return nil, err
	}
	
	return &user, nil
}

// GetUser はユーザー情報を取得する
func (c *Client) GetUser(userID int) (*User, error) {
	resp, err := c.Get(fmt.Sprintf("/users/%d", userID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var user User
	if err := DecodeResponse(resp, &user); err != nil {
		return nil, err
	}
	
	return &user, nil
}
```

## 4. プロジェクトAPI

### internal/api/project.go

```go
package api

import (
	"fmt"
	"net/url"
	"strconv"
)

// Project はプロジェクト情報
type Project struct {
	ID                                int    `json:"id"`
	ProjectKey                        string `json:"projectKey"`
	Name                              string `json:"name"`
	ChartEnabled                      bool   `json:"chartEnabled"`
	UseResolvedForChart               bool   `json:"useResolvedForChart"`
	SubtaskingEnabled                 bool   `json:"subtaskingEnabled"`
	ProjectLeaderCanEditProjectLeader bool   `json:"projectLeaderCanEditProjectLeader"`
	UseWiki                           bool   `json:"useWiki"`
	UseFileSharing                    bool   `json:"useFileSharing"`
	UseWikiTreeView                   bool   `json:"useWikiTreeView"`
	UseOriginalImageSizeAtWiki        bool   `json:"useOriginalImageSizeAtWiki"`
	TextFormattingRule                string `json:"textFormattingRule"`
	Archived                          bool   `json:"archived"`
	DisplayOrder                      int    `json:"displayOrder"`
	UseDevAttributes                  bool   `json:"useDevAttributes"`
}

// GetProjects はプロジェクト一覧を取得する
func (c *Client) GetProjects() ([]Project, error) {
	query := url.Values{}
	query.Set("archived", "false")
	
	resp, err := c.Get("/projects", query)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var projects []Project
	if err := DecodeResponse(resp, &projects); err != nil {
		return nil, err
	}
	
	return projects, nil
}

// GetProject はプロジェクト情報を取得する
func (c *Client) GetProject(projectIDOrKey string) (*Project, error) {
	resp, err := c.Get(fmt.Sprintf("/projects/%s", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var project Project
	if err := DecodeResponse(resp, &project); err != nil {
		return nil, err
	}
	
	return &project, nil
}

// IssueType は課題種別
type IssueType struct {
	ID           int    `json:"id"`
	ProjectID    int    `json:"projectId"`
	Name         string `json:"name"`
	Color        string `json:"color"`
	DisplayOrder int    `json:"displayOrder"`
}

// GetIssueTypes は課題種別一覧を取得する
func (c *Client) GetIssueTypes(projectIDOrKey string) ([]IssueType, error) {
	resp, err := c.Get(fmt.Sprintf("/projects/%s/issueTypes", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var issueTypes []IssueType
	if err := DecodeResponse(resp, &issueTypes); err != nil {
		return nil, err
	}
	
	return issueTypes, nil
}

// Category はカテゴリー
type Category struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	DisplayOrder int    `json:"displayOrder"`
}

// GetCategories はカテゴリー一覧を取得する
func (c *Client) GetCategories(projectIDOrKey string) ([]Category, error) {
	resp, err := c.Get(fmt.Sprintf("/projects/%s/categories", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var categories []Category
	if err := DecodeResponse(resp, &categories); err != nil {
		return nil, err
	}
	
	return categories, nil
}

// Version はバージョン/マイルストーン
type Version struct {
	ID             int    `json:"id"`
	ProjectID      int    `json:"projectId"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	StartDate      string `json:"startDate"`
	ReleaseDueDate string `json:"releaseDueDate"`
	Archived       bool   `json:"archived"`
	DisplayOrder   int    `json:"displayOrder"`
}

// GetVersions はバージョン一覧を取得する
func (c *Client) GetVersions(projectIDOrKey string) ([]Version, error) {
	resp, err := c.Get(fmt.Sprintf("/projects/%s/versions", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var versions []Version
	if err := DecodeResponse(resp, &versions); err != nil {
		return nil, err
	}
	
	return versions, nil
}

// Status はステータス
type Status struct {
	ID           int    `json:"id"`
	ProjectID    int    `json:"projectId"`
	Name         string `json:"name"`
	Color        string `json:"color"`
	DisplayOrder int    `json:"displayOrder"`
}

// GetStatuses はステータス一覧を取得する
func (c *Client) GetStatuses(projectIDOrKey string) ([]Status, error) {
	resp, err := c.Get(fmt.Sprintf("/projects/%s/statuses", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var statuses []Status
	if err := DecodeResponse(resp, &statuses); err != nil {
		return nil, err
	}
	
	return statuses, nil
}

// GetProjectUsers はプロジェクトユーザー一覧を取得する
func (c *Client) GetProjectUsers(projectIDOrKey string) ([]User, error) {
	resp, err := c.Get(fmt.Sprintf("/projects/%s/users", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var users []User
	if err := DecodeResponse(resp, &users); err != nil {
		return nil, err
	}
	
	return users, nil
}
```

## 5. 課題API

### internal/api/issue.go

```go
package api

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Issue は課題情報
type Issue struct {
	ID             int            `json:"id"`
	ProjectID      int            `json:"projectId"`
	IssueKey       string         `json:"issueKey"`
	KeyID          int            `json:"keyId"`
	IssueType      IssueType      `json:"issueType"`
	Summary        string         `json:"summary"`
	Description    string         `json:"description"`
	Resolution     *Resolution    `json:"resolution"`
	Priority       Priority       `json:"priority"`
	Status         Status         `json:"status"`
	Assignee       *User          `json:"assignee"`
	Category       []Category     `json:"category"`
	Versions       []Version      `json:"versions"`
	Milestone      []Version      `json:"milestone"`
	StartDate      string         `json:"startDate"`
	DueDate        string         `json:"dueDate"`
	EstimatedHours float64        `json:"estimatedHours"`
	ActualHours    float64        `json:"actualHours"`
	ParentIssueID  *int           `json:"parentIssueId"`
	CreatedUser    User           `json:"createdUser"`
	Created        string         `json:"created"`
	UpdatedUser    *User          `json:"updatedUser"`
	Updated        string         `json:"updated"`
	Attachments    []Attachment   `json:"attachments"`
	SharedFiles    []SharedFile   `json:"sharedFiles"`
	Stars          []Star         `json:"stars"`
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
	ProjectIDs    []int
	IssueTypeIDs  []int
	CategoryIDs   []int
	VersionIDs    []int
	MilestoneIDs  []int
	StatusIDs     []int
	PriorityIDs   []int
	AssigneeIDs   []int
	CreatedUserIDs []int
	ResolutionIDs []int
	ParentChild   int // 0: all, 1: exclude child, 2: child only, 3: not parent or child, 4: parent only
	Attachment    *bool
	SharedFile    *bool
	Sort          string
	Order         string // asc, desc
	Offset        int
	Count         int
	CreatedSince  string
	CreatedUntil  string
	UpdatedSince  string
	UpdatedUntil  string
	StartDateSince string
	StartDateUntil string
	DueDateSince  string
	DueDateUntil  string
	IDs           []int
	ParentIssueIDs []int
	Keyword       string
}

// ToQuery はクエリパラメータに変換する
func (o *IssueListOptions) ToQuery() url.Values {
	q := url.Values{}
	
	for _, id := range o.ProjectIDs {
		q.Add("projectId[]", strconv.Itoa(id))
	}
	for _, id := range o.IssueTypeIDs {
		q.Add("issueTypeId[]", strconv.Itoa(id))
	}
	for _, id := range o.CategoryIDs {
		q.Add("categoryId[]", strconv.Itoa(id))
	}
	for _, id := range o.VersionIDs {
		q.Add("versionId[]", strconv.Itoa(id))
	}
	for _, id := range o.MilestoneIDs {
		q.Add("milestoneId[]", strconv.Itoa(id))
	}
	for _, id := range o.StatusIDs {
		q.Add("statusId[]", strconv.Itoa(id))
	}
	for _, id := range o.PriorityIDs {
		q.Add("priorityId[]", strconv.Itoa(id))
	}
	for _, id := range o.AssigneeIDs {
		q.Add("assigneeId[]", strconv.Itoa(id))
	}
	
	if o.ParentChild > 0 {
		q.Set("parentChild", strconv.Itoa(o.ParentChild))
	}
	if o.Sort != "" {
		q.Set("sort", o.Sort)
	}
	if o.Order != "" {
		q.Set("order", o.Order)
	}
	if o.Offset > 0 {
		q.Set("offset", strconv.Itoa(o.Offset))
	}
	if o.Count > 0 {
		q.Set("count", strconv.Itoa(o.Count))
	}
	if o.Keyword != "" {
		q.Set("keyword", o.Keyword)
	}
	
	return q
}

// GetIssues は課題一覧を取得する
func (c *Client) GetIssues(opts *IssueListOptions) ([]Issue, error) {
	var query url.Values
	if opts != nil {
		query = opts.ToQuery()
	}
	
	resp, err := c.Get("/issues", query)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var issues []Issue
	if err := DecodeResponse(resp, &issues); err != nil {
		return nil, err
	}
	
	return issues, nil
}

// GetIssue は課題を取得する
func (c *Client) GetIssue(issueIDOrKey string) (*Issue, error) {
	resp, err := c.Get(fmt.Sprintf("/issues/%s", issueIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var issue Issue
	if err := DecodeResponse(resp, &issue); err != nil {
		return nil, err
	}
	
	return &issue, nil
}

// CreateIssueInput は課題作成の入力
type CreateIssueInput struct {
	ProjectID      int      `json:"projectId"`
	Summary        string   `json:"summary"`
	IssueTypeID    int      `json:"issueTypeId"`
	PriorityID     int      `json:"priorityId"`
	Description    string   `json:"description,omitempty"`
	StartDate      string   `json:"startDate,omitempty"`
	DueDate        string   `json:"dueDate,omitempty"`
	EstimatedHours float64  `json:"estimatedHours,omitempty"`
	ActualHours    float64  `json:"actualHours,omitempty"`
	CategoryIDs    []int    `json:"categoryId,omitempty"`
	VersionIDs     []int    `json:"versionId,omitempty"`
	MilestoneIDs   []int    `json:"milestoneId,omitempty"`
	AssigneeID     int      `json:"assigneeId,omitempty"`
	ParentIssueID  int      `json:"parentIssueId,omitempty"`
}

// CreateIssue は課題を作成する
func (c *Client) CreateIssue(input *CreateIssueInput) (*Issue, error) {
	// URLエンコード形式で送信
	data := url.Values{}
	data.Set("projectId", strconv.Itoa(input.ProjectID))
	data.Set("summary", input.Summary)
	data.Set("issueTypeId", strconv.Itoa(input.IssueTypeID))
	data.Set("priorityId", strconv.Itoa(input.PriorityID))
	
	if input.Description != "" {
		data.Set("description", input.Description)
	}
	if input.StartDate != "" {
		data.Set("startDate", input.StartDate)
	}
	if input.DueDate != "" {
		data.Set("dueDate", input.DueDate)
	}
	if input.AssigneeID > 0 {
		data.Set("assigneeId", strconv.Itoa(input.AssigneeID))
	}
	for _, id := range input.CategoryIDs {
		data.Add("categoryId[]", strconv.Itoa(id))
	}
	for _, id := range input.MilestoneIDs {
		data.Add("milestoneId[]", strconv.Itoa(id))
	}
	
	resp, err := c.PostForm("/issues", data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var issue Issue
	if err := DecodeResponse(resp, &issue); err != nil {
		return nil, err
	}
	
	return &issue, nil
}

// UpdateIssueInput は課題更新の入力
type UpdateIssueInput struct {
	Summary        *string  `json:"summary,omitempty"`
	Description    *string  `json:"description,omitempty"`
	StatusID       *int     `json:"statusId,omitempty"`
	ResolutionID   *int     `json:"resolutionId,omitempty"`
	StartDate      *string  `json:"startDate,omitempty"`
	DueDate        *string  `json:"dueDate,omitempty"`
	EstimatedHours *float64 `json:"estimatedHours,omitempty"`
	ActualHours    *float64 `json:"actualHours,omitempty"`
	AssigneeID     *int     `json:"assigneeId,omitempty"`
	CategoryIDs    []int    `json:"categoryId,omitempty"`
	VersionIDs     []int    `json:"versionId,omitempty"`
	MilestoneIDs   []int    `json:"milestoneId,omitempty"`
	PriorityID     *int     `json:"priorityId,omitempty"`
	IssueTypeID    *int     `json:"issueTypeId,omitempty"`
	Comment        *string  `json:"comment,omitempty"`
}

// UpdateIssue は課題を更新する
func (c *Client) UpdateIssue(issueIDOrKey string, input *UpdateIssueInput) (*Issue, error) {
	resp, err := c.Patch(fmt.Sprintf("/issues/%s", issueIDOrKey), input)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var issue Issue
	if err := DecodeResponse(resp, &issue); err != nil {
		return nil, err
	}
	
	return &issue, nil
}

// PostForm はフォーム形式でPOSTする
func (c *Client) PostForm(path string, data url.Values) (*http.Response, error) {
	if err := c.ensureValidToken(); err != nil {
		return nil, fmt.Errorf("token refresh failed: %w", err)
	}
	
	req, err := http.NewRequest("POST", c.baseURL()+path, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Authorization", "Bearer "+c.accessToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	
	return c.httpClient.Do(req)
}
```

## 6. コメントAPI

### internal/api/comment.go

```go
package api

import (
	"fmt"
	"net/url"
	"strconv"
)

// Comment はコメント
type Comment struct {
	ID            int          `json:"id"`
	Content       string       `json:"content"`
	ChangeLog     []ChangeLog  `json:"changeLog"`
	CreatedUser   User         `json:"createdUser"`
	Created       string       `json:"created"`
	Updated       string       `json:"updated"`
	Stars         []Star       `json:"stars"`
	Notifications []Notification `json:"notifications"`
}

// ChangeLog は変更ログ
type ChangeLog struct {
	Field         string `json:"field"`
	NewValue      string `json:"newValue"`
	OriginalValue string `json:"originalValue"`
}

// Notification は通知
type Notification struct {
	ID            int  `json:"id"`
	AlreadyRead   bool `json:"alreadyRead"`
	Reason        int  `json:"reason"`
	User          User `json:"user"`
	ResourceAlreadyRead bool `json:"resourceAlreadyRead"`
}

// GetComments は課題のコメント一覧を取得する
func (c *Client) GetComments(issueIDOrKey string, opts *CommentListOptions) ([]Comment, error) {
	var query url.Values
	if opts != nil {
		query = opts.ToQuery()
	}
	
	resp, err := c.Get(fmt.Sprintf("/issues/%s/comments", issueIDOrKey), query)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var comments []Comment
	if err := DecodeResponse(resp, &comments); err != nil {
		return nil, err
	}
	
	return comments, nil
}

// CommentListOptions はコメント一覧取得オプション
type CommentListOptions struct {
	MinID int
	MaxID int
	Count int
	Order string
}

// ToQuery はクエリパラメータに変換する
func (o *CommentListOptions) ToQuery() url.Values {
	q := url.Values{}
	if o.MinID > 0 {
		q.Set("minId", strconv.Itoa(o.MinID))
	}
	if o.MaxID > 0 {
		q.Set("maxId", strconv.Itoa(o.MaxID))
	}
	if o.Count > 0 {
		q.Set("count", strconv.Itoa(o.Count))
	}
	if o.Order != "" {
		q.Set("order", o.Order)
	}
	return q
}

// AddComment は課題にコメントを追加する
func (c *Client) AddComment(issueIDOrKey string, content string, notifiedUserIDs []int) (*Comment, error) {
	data := url.Values{}
	data.Set("content", content)
	for _, id := range notifiedUserIDs {
		data.Add("notifiedUserId[]", strconv.Itoa(id))
	}
	
	resp, err := c.PostForm(fmt.Sprintf("/issues/%s/comments", issueIDOrKey), data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var comment Comment
	if err := DecodeResponse(resp, &comment); err != nil {
		return nil, err
	}
	
	return &comment, nil
}
```

## 完了条件

- [ ] APIクライアントが作成できる
- [ ] トークンが自動更新される
- [ ] ユーザー情報が取得できる
- [ ] プロジェクト一覧が取得できる
- [ ] 課題一覧が取得できる
- [ ] 課題が作成できる
- [ ] コメントが追加できる
- [ ] APIエラーが適切にハンドリングされる

## 次のステップ

`07-issue-commands.md` に進んで課題コマンドを実装してください。
