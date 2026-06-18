package cmdutil

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/api"
)

// NamedResolverOption is a candidate for name-to-ID resolution.
type NamedResolverOption struct {
	ID          int
	Label       string
	Aliases     []string
	Description string
}

func (o NamedResolverOption) displayText() string {
	if o.Description != "" {
		return o.Description
	}
	return o.Label
}

// ResolveNamedID resolves a single ID, exact name, alias, or fuzzy match.
// Resolution order: exact match → prefix match → contains match.
// If exactly one candidate matches at any stage, it is used.
// If multiple candidates match, they are presented as suggestions.
func ResolveNamedID(input, singular, plural string, options []NamedResolverOption) (int, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return 0, fmt.Errorf("%s value cannot be empty", singular)
	}

	if id, err := strconv.Atoi(value); err == nil {
		return id, nil
	}

	// Stage 1: exact match (case-insensitive)
	var exactMatches []NamedResolverOption
	for _, option := range options {
		if strings.EqualFold(option.Label, value) {
			exactMatches = append(exactMatches, option)
			continue
		}
		for _, alias := range option.Aliases {
			if strings.EqualFold(alias, value) {
				exactMatches = append(exactMatches, option)
				break
			}
		}
	}
	if len(exactMatches) == 1 {
		return exactMatches[0].ID, nil
	}
	if len(exactMatches) > 1 {
		return 0, ambiguousError(plural, value, exactMatches)
	}

	// Stage 2: prefix match (case-insensitive)
	lowerValue := strings.ToLower(value)
	var prefixMatches []NamedResolverOption
	for _, option := range options {
		if matchesPrefix(option, lowerValue) {
			prefixMatches = append(prefixMatches, option)
		}
	}
	if len(prefixMatches) == 1 {
		return prefixMatches[0].ID, nil
	}
	if len(prefixMatches) > 1 {
		return 0, didYouMeanError(singular, value, prefixMatches)
	}

	// Stage 3: contains match (case-insensitive)
	var containsMatches []NamedResolverOption
	for _, option := range options {
		if matchesContains(option, lowerValue) {
			containsMatches = append(containsMatches, option)
		}
	}
	if len(containsMatches) == 1 {
		return containsMatches[0].ID, nil
	}
	if len(containsMatches) > 1 {
		return 0, didYouMeanError(singular, value, containsMatches)
	}

	// No match at all
	lines := []string{fmt.Sprintf("%s not found: %s", singular, value)}
	if len(options) > 0 {
		lines = append(lines, "", fmt.Sprintf("Available %s:", plural))
		for _, option := range options {
			lines = append(lines, fmt.Sprintf("  %d # %s", option.ID, option.displayText()))
		}
	}
	return 0, errors.New(strings.Join(lines, "\n"))
}

func matchesPrefix(option NamedResolverOption, lowerValue string) bool {
	if strings.HasPrefix(strings.ToLower(option.Label), lowerValue) {
		return true
	}
	for _, alias := range option.Aliases {
		if strings.HasPrefix(strings.ToLower(alias), lowerValue) {
			return true
		}
	}
	return false
}

func matchesContains(option NamedResolverOption, lowerValue string) bool {
	if strings.Contains(strings.ToLower(option.Label), lowerValue) {
		return true
	}
	for _, alias := range option.Aliases {
		if strings.Contains(strings.ToLower(alias), lowerValue) {
			return true
		}
	}
	return false
}

func ambiguousError(plural, value string, matches []NamedResolverOption) error {
	lines := []string{fmt.Sprintf("multiple %s match %q:", plural, value)}
	for _, option := range matches {
		lines = append(lines, fmt.Sprintf("  %d # %s", option.ID, option.displayText()))
	}
	lines = append(lines, "", "Use a numeric ID to disambiguate.")
	return errors.New(strings.Join(lines, "\n"))
}

func didYouMeanError(singular, value string, matches []NamedResolverOption) error {
	lines := []string{fmt.Sprintf("%s not found: %s", singular, value)}
	lines = append(lines, "", "Did you mean:")
	for _, option := range matches {
		lines = append(lines, fmt.Sprintf("  %d # %s", option.ID, option.displayText()))
	}
	return errors.New(strings.Join(lines, "\n"))
}

// ResolveNamedIDs resolves a comma-separated list of IDs or exact name aliases.
func ResolveNamedIDs(input, singular, plural string, options []NamedResolverOption) ([]int, error) {
	parts := strings.Split(input, ",")
	ids := make([]int, 0, len(parts))
	for _, part := range parts {
		id, err := ResolveNamedID(part, singular, plural, options)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// ResolveCategoryIDs resolves category IDs or exact names for a project.
func ResolveCategoryIDs(ctx context.Context, client *api.Client, projectKey, input string) ([]int, error) {
	if projectKey == "" && hasNonNumericToken(input) {
		return nil, errors.New("project is required to resolve category names")
	}

	categories, err := client.GetCategories(ctx, projectKey)
	if err != nil {
		return nil, err
	}

	options := make([]NamedResolverOption, len(categories))
	for i, category := range categories {
		options[i] = NamedResolverOption{
			ID:    category.ID,
			Label: category.Name,
		}
	}

	return ResolveNamedIDs(input, "category", "categories", options)
}

// ResolveCategory resolves a single category by ID or exact name.
func ResolveCategory(ctx context.Context, client *api.Client, projectKey, input string) (*api.Category, error) {
	categories, err := client.GetCategories(ctx, projectKey)
	if err != nil {
		return nil, err
	}

	options := make([]NamedResolverOption, len(categories))
	for i, category := range categories {
		options[i] = NamedResolverOption{
			ID:    category.ID,
			Label: category.Name,
		}
	}

	id, err := ResolveNamedID(input, "category", "categories", options)
	if err != nil {
		return nil, err
	}

	for i := range categories {
		if categories[i].ID == id {
			return &categories[i], nil
		}
	}
	return nil, nil
}

// ResolveMilestoneIDs resolves milestone IDs or exact names for a project.
func ResolveMilestoneIDs(ctx context.Context, client *api.Client, projectKey, input string) ([]int, error) {
	if projectKey == "" && hasNonNumericToken(input) {
		return nil, errors.New("project is required to resolve milestone names")
	}

	versions, err := client.GetVersions(ctx, projectKey)
	if err != nil {
		return nil, err
	}

	options := make([]NamedResolverOption, len(versions))
	for i, version := range versions {
		options[i] = NamedResolverOption{
			ID:    version.ID,
			Label: version.Name,
		}
	}

	return ResolveNamedIDs(input, "milestone", "milestones", options)
}

// ResolveMilestone resolves a single milestone by ID or exact name.
func ResolveMilestone(ctx context.Context, client *api.Client, projectKey, input string) (*api.Version, error) {
	versions, err := client.GetVersions(ctx, projectKey)
	if err != nil {
		return nil, err
	}

	options := make([]NamedResolverOption, len(versions))
	for i, version := range versions {
		options[i] = NamedResolverOption{
			ID:    version.ID,
			Label: version.Name,
		}
	}

	id, err := ResolveNamedID(input, "milestone", "milestones", options)
	if err != nil {
		return nil, err
	}

	for i := range versions {
		if versions[i].ID == id {
			return &versions[i], nil
		}
	}
	return nil, nil
}

// issueTypeAliases maps Japanese issue type names to common English equivalents.
var issueTypeAliases = map[string][]string{
	"バグ":   {"bug"},
	"タスク":  {"task"},
	"要望":   {"feature", "enhancement", "request"},
	"その他":  {"other"},
	"障害対応": {"incident"},
}

// ResolveIssueTypeIDs resolves issue type IDs or exact names for a project.
func ResolveIssueTypeIDs(ctx context.Context, client *api.Client, projectKey, input string) ([]int, error) {
	if projectKey == "" && hasNonNumericToken(input) {
		return nil, errors.New("project is required to resolve issue type names")
	}

	issueTypes, err := client.GetIssueTypes(ctx, projectKey)
	if err != nil {
		return nil, err
	}

	options := make([]NamedResolverOption, len(issueTypes))
	for i, issueType := range issueTypes {
		options[i] = NamedResolverOption{
			ID:      issueType.ID,
			Label:   issueType.Name,
			Aliases: issueTypeAliases[issueType.Name],
		}
	}

	return ResolveNamedIDs(input, "issue type", "issue types", options)
}

// ResolveIssueType resolves a single issue type by ID or exact name.
func ResolveIssueType(ctx context.Context, client *api.Client, projectKey, input string) (*api.IssueType, error) {
	issueTypes, err := client.GetIssueTypes(ctx, projectKey)
	if err != nil {
		return nil, err
	}

	options := make([]NamedResolverOption, len(issueTypes))
	for i, issueType := range issueTypes {
		options[i] = NamedResolverOption{
			ID:      issueType.ID,
			Label:   issueType.Name,
			Aliases: issueTypeAliases[issueType.Name],
		}
	}

	id, err := ResolveNamedID(input, "issue type", "issue types", options)
	if err != nil {
		return nil, err
	}

	for i := range issueTypes {
		if issueTypes[i].ID == id {
			return &issueTypes[i], nil
		}
	}
	return nil, nil
}

// ResolveStatusIDs resolves status IDs or exact names for a project.
func ResolveStatusIDs(ctx context.Context, client *api.Client, projectKey, input string) ([]int, error) {
	if projectKey == "" && hasNonNumericToken(input) {
		return nil, errors.New("project is required to resolve status names")
	}

	statuses, err := client.GetStatuses(ctx, projectKey)
	if err != nil {
		return nil, err
	}

	options := make([]NamedResolverOption, len(statuses))
	for i, status := range statuses {
		options[i] = NamedResolverOption{
			ID:    status.ID,
			Label: status.Name,
		}
	}

	return ResolveNamedIDs(input, "status", "statuses", options)
}

// priorityAliases maps Japanese priority names to common English equivalents.
var priorityAliases = map[string][]string{
	"高": {"high"},
	"中": {"medium", "normal"},
	"低": {"low"},
}

// ResolvePriorityIDs resolves priority IDs or exact names (space-scoped).
func ResolvePriorityIDs(ctx context.Context, client *api.Client, input string) ([]int, error) {
	priorities, err := client.GetPriorities(ctx)
	if err != nil {
		return nil, err
	}

	options := make([]NamedResolverOption, len(priorities))
	for i, priority := range priorities {
		options[i] = NamedResolverOption{
			ID:      priority.ID.Value,
			Label:   priority.Name.Value,
			Aliases: priorityAliases[priority.Name.Value],
		}
	}

	return ResolveNamedIDs(input, "priority", "priorities", options)
}

// ResolveResolutionIDs resolves resolution IDs or exact names (space-scoped).
func ResolveResolutionIDs(ctx context.Context, client *api.Client, input string) ([]int, error) {
	resolutions, err := client.GetResolutions(ctx)
	if err != nil {
		return nil, err
	}

	options := make([]NamedResolverOption, len(resolutions))
	for i, resolution := range resolutions {
		options[i] = NamedResolverOption{
			ID:    resolution.ID.Value,
			Label: resolution.Name.Value,
		}
	}

	return ResolveNamedIDs(input, "resolution", "resolutions", options)
}

// ResolveVersionIDs resolves version IDs or exact names for a project.
func ResolveVersionIDs(ctx context.Context, client *api.Client, projectKey, input string) ([]int, error) {
	if projectKey == "" && hasNonNumericToken(input) {
		return nil, errors.New("project is required to resolve version names")
	}

	versions, err := client.GetVersions(ctx, projectKey)
	if err != nil {
		return nil, err
	}

	options := make([]NamedResolverOption, len(versions))
	for i, version := range versions {
		options[i] = NamedResolverOption{
			ID:    version.ID,
			Label: version.Name,
		}
	}

	return ResolveNamedIDs(input, "version", "versions", options)
}

// ResolveIssueIDs resolves a comma-separated list of issue IDs or issue keys
// (e.g. "123,PROJ-45") into numeric issue IDs.
func ResolveIssueIDs(ctx context.Context, client *api.Client, input string) ([]int, error) {
	parts := strings.Split(input, ",")
	ids := make([]int, 0, len(parts))
	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}
		if id, err := strconv.Atoi(value); err == nil {
			ids = append(ids, id)
			continue
		}
		issue, err := client.GetIssue(ctx, value)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve issue %q: %w", value, err)
		}
		if !issue.ID.IsSet() {
			return nil, fmt.Errorf("issue %q has no ID", value)
		}
		ids = append(ids, issue.ID.Value)
	}
	return ids, nil
}

// ParseParentChild parses a parent-child filter value into the Backlog API
// parentChild parameter (0: all, 1: exclude child, 2: child only,
// 3: neither parent nor child, 4: parent only).
func ParseParentChild(input string) (int, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return 0, nil
	}
	if n, err := strconv.Atoi(value); err == nil {
		if n < 0 || n > 4 {
			return 0, fmt.Errorf("invalid parent-child value: %d (must be 0-4)", n)
		}
		return n, nil
	}
	switch strings.ToLower(value) {
	case "all":
		return 0, nil
	case "exclude-child", "not-child":
		return 1, nil
	case "child":
		return 2, nil
	case "none", "neither":
		return 3, nil
	case "parent":
		return 4, nil
	default:
		return 0, fmt.Errorf("invalid parent-child value: %q (must be all, exclude-child, child, parent, none, or 0-4)", value)
	}
}

// ResolveUserID resolves a space-level user using @me, numeric ID, userId, or
// display name. Unlike ResolveProjectUserID this is not scoped to a project, so
// it is suitable for resources like notifications that are not project-bound.
func ResolveUserID(ctx context.Context, client *api.Client, input string) (int, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return 0, fmt.Errorf("user value cannot be empty")
	}
	if value == "@me" {
		me, err := client.GetCurrentUser(ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to get current user: %w", err)
		}
		return me.ID.Value, nil
	}
	if id, err := strconv.Atoi(value); err == nil {
		return id, nil
	}

	users, err := client.GetUsers(ctx)
	if err != nil {
		return 0, err
	}

	options := make([]NamedResolverOption, len(users))
	for i, user := range users {
		var aliases []string
		description := user.Name.Value
		if user.UserId.Value != "" {
			aliases = []string{user.UserId.Value}
			description = fmt.Sprintf("%s (%s)", user.Name.Value, user.UserId.Value)
		}
		options[i] = NamedResolverOption{
			ID:          user.ID.Value,
			Label:       user.Name.Value,
			Aliases:     aliases,
			Description: description,
		}
	}

	return ResolveNamedID(value, "user", "users", options)
}

// ResolveProjectUserID resolves a project user using @me, numeric ID, userId, or display name.
func ResolveProjectUserID(ctx context.Context, client *api.Client, projectKey, input string) (int, error) {
	return resolveProjectUserID(ctx, client, projectKey, input, "user", "users")
}

// ResolveProjectUserIDs resolves comma-separated project users.
func ResolveProjectUserIDs(ctx context.Context, client *api.Client, projectKey, input string) ([]int, error) {
	parts := strings.Split(input, ",")
	ids := make([]int, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		id, err := ResolveProjectUserID(ctx, client, projectKey, part)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// ResolveProjectAssigneeID resolves an assignee using @me, numeric ID, userId, or display name.
func ResolveProjectAssigneeID(ctx context.Context, client *api.Client, projectKey, input string) (int, error) {
	return resolveProjectUserID(ctx, client, projectKey, input, "assignee", "assignees")
}

// ResolveProjectAuthorID resolves an author using @me, numeric ID, userId, or display name.
func ResolveProjectAuthorID(ctx context.Context, client *api.Client, projectKey, input string) (int, error) {
	return resolveProjectUserID(ctx, client, projectKey, input, "author", "authors")
}

func resolveProjectUserID(ctx context.Context, client *api.Client, projectKey, input, singular, plural string) (int, error) {
	value := strings.TrimSpace(input)
	if value == "" {
		return 0, fmt.Errorf("%s value cannot be empty", singular)
	}
	if value == "@me" {
		me, err := client.GetCurrentUser(ctx)
		if err != nil {
			return 0, fmt.Errorf("failed to get current user: %w", err)
		}
		return me.ID.Value, nil
	}
	if id, err := strconv.Atoi(value); err == nil {
		return id, nil
	}
	if projectKey == "" {
		// プロジェクト未指定時（-p all 等）はスペースレベルのユーザー一覧にフォールバック
		spaceUsers, err := client.GetUsers(ctx)
		if err != nil {
			return 0, err
		}
		options := make([]NamedResolverOption, len(spaceUsers))
		for i, u := range spaceUsers {
			var aliases []string
			description := u.Name.Value
			if u.UserId.Value != "" {
				aliases = []string{u.UserId.Value}
				description = fmt.Sprintf("%s (%s)", u.Name.Value, u.UserId.Value)
			}
			options[i] = NamedResolverOption{
				ID:          u.ID.Value,
				Label:       u.Name.Value,
				Aliases:     aliases,
				Description: description,
			}
		}
		return ResolveNamedID(value, singular, plural, options)
	}

	users, err := client.GetProjectUsers(ctx, projectKey)
	if err != nil {
		return 0, err
	}

	options := make([]NamedResolverOption, len(users))
	for i, user := range users {
		options[i] = NamedResolverOption{
			ID:          user.ID,
			Label:       user.Name,
			Aliases:     projectUserAliases(user),
			Description: describeProjectUser(user),
		}
	}

	return ResolveNamedID(value, singular, plural, options)
}

func projectUserAliases(user api.User) []string {
	if user.UserID == "" {
		return nil
	}
	return []string{user.UserID}
}

func describeProjectUser(user api.User) string {
	if user.UserID == "" {
		return user.Name
	}
	return fmt.Sprintf("%s (%s)", user.Name, user.UserID)
}

func hasNonNumericToken(input string) bool {
	for _, part := range strings.Split(input, ",") {
		if _, err := strconv.Atoi(strings.TrimSpace(part)); err != nil {
			return true
		}
	}
	return false
}
