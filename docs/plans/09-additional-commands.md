# Phase 09: 追加コマンド

## 目標

- `backlog issue edit` - 課題編集
- `backlog issue close` - 課題クローズ
- `backlog issue comment` - コメント追加
- `backlog pr list/view/create` - プルリクエスト
- `backlog wiki list/view/create` - Wiki

## 1. Issue Edit コマンド

### internal/cmd/issue/edit.go

```go
package issue

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/api"
	"github.com/yourorg/backlog-cli/internal/cmd"
	"github.com/yourorg/backlog-cli/internal/ui"
)

var editCmd = &cobra.Command{
	Use:   "edit <issue-key>",
	Short: "Edit an issue",
	Long: `Edit an existing issue.

Examples:
  backlog issue edit PROJ-123 --summary "New title"
  backlog issue edit PROJ-123 --assignee @me
  backlog issue edit PROJ-123 --status 2`,
	Args: cobra.ExactArgs(1),
	RunE: runEdit,
}

var (
	editSummary     string
	editDescription string
	editStatusID    int
	editPriorityID  int
	editAssignee    string
	editDueDate     string
	editComment     string
)

func init() {
	editCmd.Flags().StringVarP(&editSummary, "summary", "s", "", "New summary")
	editCmd.Flags().StringVarP(&editDescription, "description", "d", "", "New description")
	editCmd.Flags().IntVar(&editStatusID, "status", 0, "Status ID")
	editCmd.Flags().IntVar(&editPriorityID, "priority", 0, "Priority ID")
	editCmd.Flags().StringVarP(&editAssignee, "assignee", "a", "", "Assignee (user ID or @me)")
	editCmd.Flags().StringVar(&editDueDate, "due", "", "Due date (YYYY-MM-DD)")
	editCmd.Flags().StringVarP(&editComment, "comment", "c", "", "Comment to add")
	
	IssueCmd.AddCommand(editCmd)
}

func runEdit(c *cobra.Command, args []string) error {
	issueKey := args[0]
	
	client, resolved, err := cmd.GetAPIClient(c)
	if err != nil {
		return err
	}
	
	input := &api.UpdateIssueInput{}
	hasUpdate := false
	
	if editSummary != "" {
		input.Summary = &editSummary
		hasUpdate = true
	}
	if editDescription != "" {
		input.Description = &editDescription
		hasUpdate = true
	}
	if editStatusID > 0 {
		input.StatusID = &editStatusID
		hasUpdate = true
	}
	if editPriorityID > 0 {
		input.PriorityID = &editPriorityID
		hasUpdate = true
	}
	if editDueDate != "" {
		input.DueDate = &editDueDate
		hasUpdate = true
	}
	if editComment != "" {
		input.Comment = &editComment
		hasUpdate = true
	}
	
	// 担当者
	if editAssignee == "@me" {
		me, err := client.GetCurrentUser()
		if err != nil {
			return fmt.Errorf("failed to get current user: %w", err)
		}
		input.AssigneeID = &me.ID
		hasUpdate = true
	} else if editAssignee != "" {
		assigneeID, err := strconv.Atoi(editAssignee)
		if err != nil {
			return fmt.Errorf("invalid assignee ID: %s", editAssignee)
		}
		input.AssigneeID = &assigneeID
		hasUpdate = true
	}
	
	if !hasUpdate {
		return fmt.Errorf("no updates specified")
	}
	
	issue, err := client.UpdateIssue(issueKey, input)
	if err != nil {
		return fmt.Errorf("failed to update issue: %w", err)
	}
	
	ui.Success("Updated %s", issue.IssueKey)
	return nil
}
```

## 2. Issue Close コマンド

### internal/cmd/issue/close.go

```go
package issue

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/api"
	"github.com/yourorg/backlog-cli/internal/cmd"
	"github.com/yourorg/backlog-cli/internal/ui"
)

var closeCmd = &cobra.Command{
	Use:   "close <issue-key>",
	Short: "Close an issue",
	Long: `Close an issue by changing its status to "Closed".

Examples:
  backlog issue close PROJ-123
  backlog issue close PROJ-123 --resolution 0
  backlog issue close PROJ-123 --comment "Fixed in v1.2"`,
	Args: cobra.ExactArgs(1),
	RunE: runClose,
}

var (
	closeResolutionID int
	closeComment      string
)

func init() {
	closeCmd.Flags().IntVar(&closeResolutionID, "resolution", 0, "Resolution ID")
	closeCmd.Flags().StringVarP(&closeComment, "comment", "c", "", "Comment to add")
	
	IssueCmd.AddCommand(closeCmd)
}

func runClose(c *cobra.Command, args []string) error {
	issueKey := args[0]
	
	client, resolved, err := cmd.GetAPIClient(c)
	if err != nil {
		return err
	}
	
	// 現在の課題を取得
	issue, err := client.GetIssue(issueKey)
	if err != nil {
		return fmt.Errorf("failed to get issue: %w", err)
	}
	
	// プロジェクトのステータスを取得してCloseステータスを探す
	statuses, err := client.GetStatuses(strconv.Itoa(issue.ProjectID))
	if err != nil {
		return fmt.Errorf("failed to get statuses: %w", err)
	}
	
	var closedStatusID int
	for _, s := range statuses {
		// "完了" または "Closed" を探す
		if s.Name == "完了" || s.Name == "Closed" || s.Name == "Done" {
			closedStatusID = s.ID
			break
		}
	}
	
	if closedStatusID == 0 {
		// 見つからない場合は最後のステータスを使用
		if len(statuses) > 0 {
			closedStatusID = statuses[len(statuses)-1].ID
		} else {
			return fmt.Errorf("could not find closed status")
		}
	}
	
	input := &api.UpdateIssueInput{
		StatusID: &closedStatusID,
	}
	
	if closeResolutionID > 0 {
		input.ResolutionID = &closeResolutionID
	}
	if closeComment != "" {
		input.Comment = &closeComment
	}
	
	issue, err = client.UpdateIssue(issueKey, input)
	if err != nil {
		return fmt.Errorf("failed to close issue: %w", err)
	}
	
	ui.Success("Closed %s", issue.IssueKey)
	return nil
}
```

## 3. Issue Comment コマンド

### internal/cmd/issue/comment.go

```go
package issue

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/cmd"
	"github.com/yourorg/backlog-cli/internal/ui"
)

var commentCmd = &cobra.Command{
	Use:   "comment <issue-key> [message]",
	Short: "Add a comment to an issue",
	Long: `Add a comment to an issue.

If no message is provided, opens an editor.

Examples:
  backlog issue comment PROJ-123 "This is fixed"
  backlog issue comment PROJ-123 --editor`,
	Args: cobra.MinimumNArgs(1),
	RunE: runComment,
}

var commentEditor bool

func init() {
	commentCmd.Flags().BoolVarP(&commentEditor, "editor", "e", false, "Open editor")
	
	IssueCmd.AddCommand(commentCmd)
}

func runComment(c *cobra.Command, args []string) error {
	issueKey := args[0]
	
	var message string
	if len(args) > 1 {
		message = args[1]
	}
	
	client, _, err := cmd.GetAPIClient(c)
	if err != nil {
		return err
	}
	
	// メッセージ取得
	if message == "" {
		if commentEditor {
			message, err = openEditor("")
			if err != nil {
				return fmt.Errorf("failed to open editor: %w", err)
			}
		} else {
			message, err = ui.Input("Comment:", "")
			if err != nil {
				return err
			}
		}
	}
	
	if message == "" {
		return fmt.Errorf("comment cannot be empty")
	}
	
	comment, err := client.AddComment(issueKey, message, nil)
	if err != nil {
		return fmt.Errorf("failed to add comment: %w", err)
	}
	
	ui.Success("Added comment #%d to %s", comment.ID, issueKey)
	return nil
}
```

## 4. Pull Request API

### internal/api/pullrequest.go

```go
package api

import (
	"fmt"
	"net/url"
	"strconv"
)

// PullRequest はプルリクエスト
type PullRequest struct {
	ID           int       `json:"id"`
	ProjectID    int       `json:"projectId"`
	RepositoryID int       `json:"repositoryId"`
	Number       int       `json:"number"`
	Summary      string    `json:"summary"`
	Description  string    `json:"description"`
	Base         string    `json:"base"`
	Branch       string    `json:"branch"`
	Status       PRStatus  `json:"status"`
	Assignee     *User     `json:"assignee"`
	Issue        *Issue    `json:"issue"`
	BaseCommit   string    `json:"baseCommit"`
	BranchCommit string    `json:"branchCommit"`
	CloseAt      string    `json:"closeAt"`
	MergeAt      string    `json:"mergeAt"`
	CreatedUser  User      `json:"createdUser"`
	Created      string    `json:"created"`
	UpdatedUser  *User     `json:"updatedUser"`
	Updated      string    `json:"updated"`
}

// PRStatus はPRステータス
type PRStatus struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Repository はリポジトリ
type Repository struct {
	ID          int    `json:"id"`
	ProjectID   int    `json:"projectId"`
	Name        string `json:"name"`
	Description string `json:"description"`
	HookURL     string `json:"hookUrl"`
	HTTPURL     string `json:"httpUrl"`
	SSHURL      string `json:"sshUrl"`
	DisplayOrder int   `json:"displayOrder"`
	PushedAt    string `json:"pushedAt"`
	CreatedUser User   `json:"createdUser"`
	Created     string `json:"created"`
	UpdatedUser *User  `json:"updatedUser"`
	Updated     string `json:"updated"`
}

// GetRepositories はリポジトリ一覧を取得する
func (c *Client) GetRepositories(projectIDOrKey string) ([]Repository, error) {
	resp, err := c.Get(fmt.Sprintf("/projects/%s/git/repositories", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var repos []Repository
	if err := DecodeResponse(resp, &repos); err != nil {
		return nil, err
	}
	
	return repos, nil
}

// GetPullRequests はプルリクエスト一覧を取得する
func (c *Client) GetPullRequests(projectIDOrKey, repoIDOrName string, opts *PRListOptions) ([]PullRequest, error) {
	var query url.Values
	if opts != nil {
		query = opts.ToQuery()
	}
	
	resp, err := c.Get(fmt.Sprintf("/projects/%s/git/repositories/%s/pullRequests", projectIDOrKey, repoIDOrName), query)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var prs []PullRequest
	if err := DecodeResponse(resp, &prs); err != nil {
		return nil, err
	}
	
	return prs, nil
}

// PRListOptions はPR一覧取得オプション
type PRListOptions struct {
	StatusIDs  []int
	AssigneeIDs []int
	IssueIDs   []int
	CreatedUserIDs []int
	Offset     int
	Count      int
}

// ToQuery はクエリパラメータに変換する
func (o *PRListOptions) ToQuery() url.Values {
	q := url.Values{}
	for _, id := range o.StatusIDs {
		q.Add("statusId[]", strconv.Itoa(id))
	}
	for _, id := range o.AssigneeIDs {
		q.Add("assigneeId[]", strconv.Itoa(id))
	}
	if o.Offset > 0 {
		q.Set("offset", strconv.Itoa(o.Offset))
	}
	if o.Count > 0 {
		q.Set("count", strconv.Itoa(o.Count))
	}
	return q
}

// GetPullRequest はプルリクエストを取得する
func (c *Client) GetPullRequest(projectIDOrKey, repoIDOrName string, number int) (*PullRequest, error) {
	resp, err := c.Get(fmt.Sprintf("/projects/%s/git/repositories/%s/pullRequests/%d", projectIDOrKey, repoIDOrName, number), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var pr PullRequest
	if err := DecodeResponse(resp, &pr); err != nil {
		return nil, err
	}
	
	return &pr, nil
}
```

## 5. PR コマンド

### internal/cmd/pr/pr.go

```go
package pr

import (
	"github.com/spf13/cobra"
)

var PRCmd = &cobra.Command{
	Use:     "pr",
	Aliases: []string{"pull-request"},
	Short:   "Manage pull requests",
	Long:    "Work with Backlog Git pull requests.",
}

func init() {
	PRCmd.AddCommand(listCmd)
	PRCmd.AddCommand(viewCmd)
}
```

### internal/cmd/pr/list.go

```go
package pr

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/cmd"
	"github.com/yourorg/backlog-cli/internal/ui"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List pull requests",
	Long: `List pull requests in a repository.

Examples:
  backlog pr list --repo myrepo
  backlog pr list --repo myrepo --status 1`,
	RunE: runList,
}

var (
	listRepo   string
	listStatus int
	listLimit  int
)

func init() {
	listCmd.Flags().StringVarP(&listRepo, "repo", "r", "", "Repository name (required)")
	listCmd.Flags().IntVar(&listStatus, "status", 0, "Filter by status (1=Open, 2=Closed, 3=Merged)")
	listCmd.Flags().IntVarP(&listLimit, "limit", "l", 20, "Maximum number to show")
	listCmd.MarkFlagRequired("repo")
}

func runList(c *cobra.Command, args []string) error {
	client, resolved, err := cmd.GetAPIClient(c)
	if err != nil {
		return err
	}
	
	if err := cmd.RequireProject(resolved); err != nil {
		return err
	}
	
	opts := &api.PRListOptions{
		Count: listLimit,
	}
	if listStatus > 0 {
		opts.StatusIDs = []int{listStatus}
	}
	
	prs, err := client.GetPullRequests(resolved.Project, listRepo, opts)
	if err != nil {
		return fmt.Errorf("failed to get pull requests: %w", err)
	}
	
	if len(prs) == 0 {
		fmt.Println("No pull requests found")
		return nil
	}
	
	table := ui.NewTable("#", "STATUS", "AUTHOR", "BRANCH", "SUMMARY")
	
	for _, pr := range prs {
		status := pr.Status.Name
		switch pr.Status.ID {
		case 1:
			status = ui.Green("Open")
		case 2:
			status = ui.Red("Closed")
		case 3:
			status = ui.Blue("Merged")
		}
		
		table.AddRow(
			fmt.Sprintf("%d", pr.Number),
			status,
			pr.CreatedUser.Name,
			pr.Branch,
			truncate(pr.Summary, 40),
		)
	}
	
	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
	return nil
}
```

## 6. Wiki API

### internal/api/wiki.go

```go
package api

import (
	"fmt"
	"net/url"
	"strconv"
)

// Wiki はWikiページ
type Wiki struct {
	ID          int          `json:"id"`
	ProjectID   int          `json:"projectId"`
	Name        string       `json:"name"`
	Content     string       `json:"content"`
	Tags        []WikiTag    `json:"tags"`
	Attachments []Attachment `json:"attachments"`
	SharedFiles []SharedFile `json:"sharedFiles"`
	Stars       []Star       `json:"stars"`
	CreatedUser User         `json:"createdUser"`
	Created     string       `json:"created"`
	UpdatedUser *User        `json:"updatedUser"`
	Updated     string       `json:"updated"`
}

// WikiTag はWikiタグ
type WikiTag struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// GetWikis はWiki一覧を取得する
func (c *Client) GetWikis(projectIDOrKey string) ([]Wiki, error) {
	resp, err := c.Get(fmt.Sprintf("/wikis?projectIdOrKey=%s", projectIDOrKey), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var wikis []Wiki
	if err := DecodeResponse(resp, &wikis); err != nil {
		return nil, err
	}
	
	return wikis, nil
}

// GetWiki はWikiページを取得する
func (c *Client) GetWiki(wikiID int) (*Wiki, error) {
	resp, err := c.Get(fmt.Sprintf("/wikis/%d", wikiID), nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var wiki Wiki
	if err := DecodeResponse(resp, &wiki); err != nil {
		return nil, err
	}
	
	return &wiki, nil
}

// CreateWikiInput はWiki作成の入力
type CreateWikiInput struct {
	ProjectID   int    `json:"projectId"`
	Name        string `json:"name"`
	Content     string `json:"content"`
	MailNotify  bool   `json:"mailNotify"`
}

// CreateWiki はWikiページを作成する
func (c *Client) CreateWiki(input *CreateWikiInput) (*Wiki, error) {
	data := url.Values{}
	data.Set("projectId", strconv.Itoa(input.ProjectID))
	data.Set("name", input.Name)
	data.Set("content", input.Content)
	if input.MailNotify {
		data.Set("mailNotify", "true")
	}
	
	resp, err := c.PostForm("/wikis", data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	var wiki Wiki
	if err := DecodeResponse(resp, &wiki); err != nil {
		return nil, err
	}
	
	return &wiki, nil
}
```

## 7. Wiki コマンド

### internal/cmd/wiki/wiki.go

```go
package wiki

import (
	"github.com/spf13/cobra"
)

var WikiCmd = &cobra.Command{
	Use:   "wiki",
	Short: "Manage Wiki pages",
	Long:  "Work with Backlog Wiki pages.",
}

func init() {
	WikiCmd.AddCommand(listCmd)
	WikiCmd.AddCommand(viewCmd)
	WikiCmd.AddCommand(createCmd)
}
```

### internal/cmd/wiki/list.go

```go
package wiki

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/yourorg/backlog-cli/internal/cmd"
	"github.com/yourorg/backlog-cli/internal/ui"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List Wiki pages",
	RunE:  runList,
}

func runList(c *cobra.Command, args []string) error {
	client, resolved, err := cmd.GetAPIClient(c)
	if err != nil {
		return err
	}
	
	if err := cmd.RequireProject(resolved); err != nil {
		return err
	}
	
	wikis, err := client.GetWikis(resolved.Project)
	if err != nil {
		return fmt.Errorf("failed to get wikis: %w", err)
	}
	
	if len(wikis) == 0 {
		fmt.Println("No wiki pages found")
		return nil
	}
	
	table := ui.NewTable("ID", "NAME", "UPDATED")
	
	for _, wiki := range wikis {
		table.AddRow(
			fmt.Sprintf("%d", wiki.ID),
			wiki.Name,
			formatDate(wiki.Updated),
		)
	}
	
	table.RenderWithColor(os.Stdout, ui.IsColorEnabled())
	return nil
}
```

## 完了条件

- [ ] `backlog issue edit` で課題を編集できる
- [ ] `backlog issue close` で課題をクローズできる
- [ ] `backlog issue comment` でコメントを追加できる
- [ ] `backlog pr list` でPR一覧が表示される
- [ ] `backlog pr view` でPR詳細が表示される
- [ ] `backlog wiki list` でWiki一覧が表示される
- [ ] `backlog wiki view` でWiki内容が表示される
- [ ] `backlog wiki create` でWikiが作成できる

## 次のステップ

`10-improvements.md` に進んで改善・仕上げを行ってください。
