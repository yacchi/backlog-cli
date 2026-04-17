package api

import (
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/gen/backlog"
)

// Document はドキュメント
type Document struct {
	ID          string        `json:"id"`
	ProjectID   int           `json:"projectId"`
	Title       string        `json:"title"`
	StatusID    int           `json:"statusId"`
	Emoji       string        `json:"emoji"`
	Tags        []DocumentTag `json:"tags"`
	Attachments []Attachment  `json:"attachments"`
	CreatedUser User          `json:"createdUser"`
	Created     string        `json:"created"`
	UpdatedUser *User         `json:"updatedUser"`
	Updated     string        `json:"updated"`
}

// DocumentDetail はコンテンツ本文を含むドキュメント詳細
type DocumentDetail struct {
	Document
	Plain string `json:"plain"`
	JSON  string `json:"json"`
}

// DocumentTag はドキュメントタグ
type DocumentTag struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// DocumentTreeNode はドキュメントツリーノード（再帰構造）
type DocumentTreeNode struct {
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Emoji    string             `json:"emoji"`
	Children []DocumentTreeNode `json:"children"`
}

// DocumentTree はドキュメントツリー
type DocumentTree struct {
	ProjectID  int               `json:"projectId"`
	ActiveTree *DocumentTreeNode `json:"activeTree"`
	TrashTree  *DocumentTreeNode `json:"trashTree"`
}

// DocumentComment はドキュメントコメント
type DocumentComment struct {
	ID          int    `json:"id"`
	DocumentID  string `json:"documentId"`
	StatusID    int    `json:"statusId"`
	Content     string `json:"content"`
	Plain       string `json:"plain"`
	CommentType string `json:"commentType"`
	CreatedUser User   `json:"createdUser"`
	Created     string `json:"created"`
	UpdatedUser *User  `json:"updatedUser"`
	Updated     string `json:"updated"`
}

// DocumentListOptions はドキュメント一覧取得オプション
type DocumentListOptions struct {
	ProjectIDs []int
	Keyword    string
	Sort       string
	Order      string
	Offset     int
	Count      int
}

// CreateDocumentInput はドキュメント作成入力
type CreateDocumentInput struct {
	ProjectID int
	Title     string
	Content   string
	Emoji     string
	ParentID  string
	AddLast   bool
}

func convertUser(u backlog.User) User {
	user := User{
		ID:       u.ID.Or(0),
		UserID:   u.UserId.Or(""),
		Name:     u.Name.Or(""),
		RoleType: u.RoleType.Or(0),
		Lang:     u.Lang.Or(""),
	}
	if na, ok := u.NulabAccount.Get(); ok {
		user.NulabAccount = &NulabAccount{
			NulabID:  na.NulabId.Or(""),
			Name:     na.Name.Or(""),
			UniqueID: na.UniqueId.Or(""),
		}
	}
	return user
}

func convertAttachment(a backlog.Attachment) Attachment {
	att := Attachment{
		ID:   a.ID.Or(0),
		Name: a.Name.Or(""),
		Size: int64(a.Size.Or(0)),
	}
	if u, ok := a.CreatedUser.Get(); ok {
		att.CreatedUser = convertUser(u)
	}
	att.Created = a.Created.Or("")
	return att
}

func convertDocument(d backlog.Document) Document {
	doc := Document{
		ID:        d.ID.Or(""),
		ProjectID: d.ProjectId.Or(0),
		Title:     d.Title.Or(""),
		StatusID:  d.StatusId.Or(0),
		Emoji:     d.Emoji.Value,
		Created:   d.Created.Or(""),
		Updated:   d.Updated.Value,
	}
	for _, t := range d.Tags {
		doc.Tags = append(doc.Tags, DocumentTag{ID: t.ID.Or(0), Name: t.Name.Or("")})
	}
	for _, a := range d.Attachments {
		doc.Attachments = append(doc.Attachments, convertAttachment(a))
	}
	if u, ok := d.CreatedUser.Get(); ok {
		doc.CreatedUser = convertUser(u)
	}
	if u, ok := d.UpdatedUser.Get(); ok {
		cu := convertUser(u)
		doc.UpdatedUser = &cu
	}
	return doc
}

func convertDocumentDetail(d *backlog.DocumentDetail) *DocumentDetail {
	base := convertDocument(backlog.Document{
		ID:          d.ID,
		ProjectId:   d.ProjectId,
		Title:       d.Title,
		StatusId:    d.StatusId,
		Emoji:       d.Emoji,
		Tags:        d.Tags,
		Attachments: d.Attachments,
		CreatedUser: d.CreatedUser,
		Created:     d.Created,
		UpdatedUser: d.UpdatedUser,
		Updated:     d.Updated,
	})
	return &DocumentDetail{
		Document: base,
		Plain:    d.Plain.Value,
		JSON:     d.JSON.Value,
	}
}

func convertDocumentTreeNode(n backlog.DocumentTreeNode) DocumentTreeNode {
	node := DocumentTreeNode{
		ID:    n.ID.Or(""),
		Name:  n.Name.Or(""),
		Emoji: n.Emoji.Value,
	}
	for _, c := range n.Children {
		node.Children = append(node.Children, convertDocumentTreeNode(c))
	}
	return node
}

// GetDocuments はドキュメント一覧を取得する
func (c *Client) GetDocuments(ctx context.Context, opts *DocumentListOptions) ([]Document, error) {
	params := backlog.GetDocumentsParams{}
	if opts != nil {
		params.ProjectId = opts.ProjectIDs
		params.Offset = opts.Offset
		if opts.Keyword != "" {
			params.Keyword = backlog.NewOptString(opts.Keyword)
		}
		if opts.Sort != "" {
			params.Sort = backlog.NewOptString(opts.Sort)
		}
		if opts.Order != "" {
			params.Order = backlog.NewOptString(opts.Order)
		}
		if opts.Count > 0 {
			params.Count = backlog.NewOptInt(opts.Count)
		}
	}

	res, err := c.backlogClient.GetDocuments(ctx, params)
	if err != nil {
		return nil, err
	}

	docs := make([]Document, 0, len(res))
	for _, d := range res {
		docs = append(docs, convertDocument(d))
	}
	return docs, nil
}

// GetDocumentCount はドキュメント数を取得する
func (c *Client) GetDocumentCount(ctx context.Context, projectIDOrKey string) (int, error) {
	res, err := c.backlogClient.GetDocumentsCount(ctx, backlog.GetDocumentsCountParams{
		ProjectIdOrKey: projectIDOrKey,
	})
	if err != nil {
		return 0, err
	}
	if res.Count.IsSet() {
		return res.Count.Value, nil
	}
	return 0, nil
}

// GetDocumentTree はドキュメントツリーを取得する
func (c *Client) GetDocumentTree(ctx context.Context, projectIDOrKey string) (*DocumentTree, error) {
	res, err := c.backlogClient.GetDocumentTree(ctx, backlog.GetDocumentTreeParams{
		ProjectIdOrKey: projectIDOrKey,
	})
	if err != nil {
		return nil, err
	}

	tree := &DocumentTree{
		ProjectID: res.ProjectId.Or(0),
	}
	if n, ok := res.ActiveTree.Get(); ok {
		converted := convertDocumentTreeNode(n)
		tree.ActiveTree = &converted
	}
	if n, ok := res.TrashTree.Get(); ok {
		converted := convertDocumentTreeNode(n)
		tree.TrashTree = &converted
	}
	return tree, nil
}

// GetDocument はドキュメントを取得する
func (c *Client) GetDocument(ctx context.Context, documentID string) (*DocumentDetail, error) {
	if c.cache != nil {
		var doc DocumentDetail
		key := fmt.Sprintf("document:%s.%s:%s", c.space, c.domain, documentID)
		if ok, _ := c.cache.Get(key, &doc); ok {
			return &doc, nil
		}
	}

	res, err := c.backlogClient.GetDocument(ctx, backlog.GetDocumentParams{
		DocumentId: documentID,
	})
	if err != nil {
		return nil, err
	}

	doc := convertDocumentDetail(res)

	if c.cache != nil {
		key := fmt.Sprintf("document:%s.%s:%s", c.space, c.domain, documentID)
		_ = c.cache.Set(key, doc, c.cacheTTL)
	}

	return doc, nil
}

// CreateDocument はドキュメントを作成する
func (c *Client) CreateDocument(ctx context.Context, input *CreateDocumentInput) (*Document, error) {
	req := backlog.OptCreateDocumentReq{
		Set: true,
		Value: backlog.CreateDocumentReq{
			ProjectId: input.ProjectID,
		},
	}
	if input.Title != "" {
		req.Value.Title = backlog.NewOptString(input.Title)
	}
	if input.Content != "" {
		req.Value.Content = backlog.NewOptString(input.Content)
	}
	if input.Emoji != "" {
		req.Value.Emoji = backlog.NewOptString(input.Emoji)
	}
	if input.ParentID != "" {
		req.Value.ParentId = backlog.NewOptString(input.ParentID)
	}
	if input.AddLast {
		req.Value.AddLast = backlog.NewOptBool(true)
	}

	res, err := c.backlogClient.CreateDocument(ctx, req)
	if err != nil {
		return nil, err
	}

	doc := convertDocument(*res)
	return &doc, nil
}

// DeleteDocument はドキュメントを削除する
func (c *Client) DeleteDocument(ctx context.Context, documentID string) (*Document, error) {
	res, err := c.backlogClient.DeleteDocument(ctx, backlog.DeleteDocumentParams{
		DocumentId: documentID,
	})
	if err != nil {
		return nil, err
	}

	doc := convertDocument(*res)
	return &doc, nil
}

// GetDocumentComments はドキュメントコメントを取得する
func (c *Client) GetDocumentComments(ctx context.Context, documentID string) ([]DocumentComment, error) {
	res, err := c.backlogClient.GetDocumentComments(ctx, backlog.GetDocumentCommentsParams{
		DocumentId: documentID,
	})
	if err != nil {
		return nil, err
	}

	comments := make([]DocumentComment, 0, len(res))
	for _, cm := range res {
		c2 := DocumentComment{
			ID:          cm.ID.Or(0),
			DocumentID:  cm.DocumentId.Or(""),
			StatusID:    cm.StatusId.Or(0),
			Content:     cm.Content.Value,
			Plain:       cm.Plain.Value,
			CommentType: cm.CommentType.Value,
			Created:     cm.Created.Or(""),
			Updated:     cm.Updated.Value,
		}
		if u, ok := cm.CreatedUser.Get(); ok {
			c2.CreatedUser = convertUser(u)
		}
		if u, ok := cm.UpdatedUser.Get(); ok {
			cu := convertUser(u)
			c2.UpdatedUser = &cu
		}
		comments = append(comments, c2)
	}
	return comments, nil
}

// AddDocumentTags はドキュメントにタグを追加する
func (c *Client) AddDocumentTags(ctx context.Context, documentID string, tagNames []string) ([]DocumentTag, error) {
	req := backlog.OptAddDocumentTagsReq{
		Set: true,
		Value: backlog.AddDocumentTagsReq{
			TagNames: tagNames,
		},
	}
	res, err := c.backlogClient.AddDocumentTags(ctx, req, backlog.AddDocumentTagsParams{
		DocumentId: documentID,
	})
	if err != nil {
		return nil, err
	}

	tags := make([]DocumentTag, 0, len(res))
	for _, t := range res {
		tags = append(tags, DocumentTag{ID: t.ID.Or(0), Name: t.Name.Or("")})
	}
	return tags, nil
}

// RemoveDocumentTags はドキュメントからタグを削除する
func (c *Client) RemoveDocumentTags(ctx context.Context, documentID string, tagNames []string) ([]DocumentTag, error) {
	req := backlog.OptRemoveDocumentTagsReq{
		Set: true,
		Value: backlog.RemoveDocumentTagsReq{
			TagNames: tagNames,
		},
	}
	res, err := c.backlogClient.RemoveDocumentTags(ctx, req, backlog.RemoveDocumentTagsParams{
		DocumentId: documentID,
	})
	if err != nil {
		return nil, err
	}

	tags := make([]DocumentTag, 0, len(res))
	for _, t := range res {
		tags = append(tags, DocumentTag{ID: t.ID.Or(0), Name: t.Name.Or("")})
	}
	return tags, nil
}

// DownloadDocumentAttachment はドキュメント添付ファイルをダウンロードする
// Content-Disposition ヘッダーからファイル名を取得し、データを w に書き込む
func (c *Client) DownloadDocumentAttachment(ctx context.Context, documentID string, attachmentID int, w io.Writer) (filename string, size int64, err error) {
	resp, err := c.Get(ctx, fmt.Sprintf("/documents/%s/attachments/%d", documentID, attachmentID), nil)
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

func extractFilename(resp *http.Response) string {
	cd := resp.Header.Get("Content-Disposition")
	if cd == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(cd)
	if err != nil {
		// フォールバック: filename= を直接探す
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
