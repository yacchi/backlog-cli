package summary

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/yacchi/backlog-cli/packages/backlog/internal/config"
	"github.com/yacchi/backlog-cli/packages/backlog/internal/debug"
)

// Provider はAI要約プロバイダーのインターフェース
type Provider interface {
	// Summarize はプロンプトと入力テキストを受け取り、要約結果を返す
	Summarize(ctx context.Context, prompt, input string) (string, error)
}

// CommandInfo はコマンドプロバイダーの情報を提供するインターフェース
type CommandInfo interface {
	// GetCommandInfo はコマンドと引数の文字列表現を返す
	GetCommandInfo() string
}

// CommandProvider は外部コマンドを使用するAIプロバイダー
type CommandProvider struct {
	Command   string
	Args      []string
	UseStdin  bool
	UseStdout bool
	Timeout   time.Duration
}

// NewCommandProvider は設定からCommandProviderを作成する
func NewCommandProvider(cfg *config.ResolvedAISummary) (*CommandProvider, error) {
	provider := cfg.GetActiveProvider()
	if provider == nil {
		return nil, fmt.Errorf("provider %q not found", cfg.Provider)
	}

	// コマンドの存在確認
	if _, err := exec.LookPath(provider.Command); err != nil {
		return nil, fmt.Errorf("command %q not found: %w", provider.Command, err)
	}

	timeout := cfg.TimeoutDuration()
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	return &CommandProvider{
		Command:   provider.Command,
		Args:      provider.Args,
		UseStdin:  provider.UseStdin,
		UseStdout: provider.UseStdout,
		Timeout:   timeout,
	}, nil
}

// Summarize はプロンプトと入力テキストを外部コマンドに渡して要約を取得する
func (p *CommandProvider) Summarize(ctx context.Context, prompt, input string) (string, error) {
	// タイムアウト付きコンテキスト
	ctx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()

	// プロンプトと入力を結合
	fullPrompt := prompt + "\n\n" + input

	// 引数の置換
	args := p.substituteArgs(fullPrompt)

	debug.Log("AI summary: executing command",
		"command", p.Command,
		"args", args,
		"use_stdin", p.UseStdin,
		"timeout", p.Timeout,
		"input_length", len(fullPrompt),
	)
	debug.Log("AI summary: prompt content",
		"prompt", fullPrompt,
	)

	// コマンド作成
	cmd := exec.CommandContext(ctx, p.Command, args...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// stdinを使う場合
	if p.UseStdin {
		cmd.Stdin = strings.NewReader(fullPrompt)
	}

	// 実行
	startTime := time.Now()
	if err := cmd.Run(); err != nil {
		elapsed := time.Since(startTime)
		debug.Log("AI summary: command failed",
			"elapsed", elapsed,
			"error", err,
			"stderr", stderr.String(),
		)
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("command timed out after %v", p.Timeout)
		}
		return "", fmt.Errorf("command failed: %w\nstderr: %s", err, stderr.String())
	}

	elapsed := time.Since(startTime)
	output := strings.TrimSpace(stdout.String())
	debug.Log("AI summary: command completed",
		"elapsed", elapsed,
		"output_length", len(output),
	)
	debug.Log("AI summary: output content",
		"output", output,
	)

	if p.UseStdout {
		return output, nil
	}

	// stdoutを使わない場合（ファイル出力の場合）
	// この実装では常にstdoutを使用する
	return output, nil
}

// substituteArgs は引数内のプレースホルダーを置換する
func (p *CommandProvider) substituteArgs(fullPrompt string) []string {
	args := make([]string, len(p.Args))
	for i, arg := range p.Args {
		// $PROMPT_WITH_INPUT を実際のプロンプトに置換
		// ただし、stdinを使う場合は置換しない
		if !p.UseStdin && strings.Contains(arg, "$PROMPT_WITH_INPUT") {
			args[i] = strings.ReplaceAll(arg, "$PROMPT_WITH_INPUT", fullPrompt)
		} else {
			args[i] = arg
		}
	}
	return args
}

// SummarizeWithFiles はファイル入出力を使用して要約を取得する
// プロバイダーがファイルベースの入出力をサポートしている場合に使用
func (p *CommandProvider) SummarizeWithFiles(ctx context.Context, prompt, input string) (string, error) {
	// タイムアウト付きコンテキスト
	ctx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()

	// プロンプトと入力を結合
	fullPrompt := prompt + "\n\n" + input

	// 入力ファイル作成
	inputFile, err := os.CreateTemp("", "backlog-summary-input-*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create input file: %w", err)
	}
	defer func() { _ = os.Remove(inputFile.Name()) }()

	if _, err := inputFile.WriteString(fullPrompt); err != nil {
		_ = inputFile.Close()
		return "", fmt.Errorf("failed to write input file: %w", err)
	}
	_ = inputFile.Close()

	// 出力ファイルパス
	outputFile, err := os.CreateTemp("", "backlog-summary-output-*.txt")
	if err != nil {
		return "", fmt.Errorf("failed to create output file: %w", err)
	}
	outputPath := outputFile.Name()
	_ = outputFile.Close()
	defer func() { _ = os.Remove(outputPath) }()

	// 引数の置換
	args := p.substituteFileArgs(inputFile.Name(), outputPath)

	// コマンド作成
	cmd := exec.CommandContext(ctx, p.Command, args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// 実行
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("command timed out after %v", p.Timeout)
		}
		return "", fmt.Errorf("command failed: %w\nstderr: %s", err, stderr.String())
	}

	// 出力ファイル読み込み
	output, err := os.ReadFile(outputPath)
	if err != nil {
		return "", fmt.Errorf("failed to read output file: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// substituteFileArgs はファイルパスのプレースホルダーを置換する
func (p *CommandProvider) substituteFileArgs(inputPath, outputPath string) []string {
	args := make([]string, len(p.Args))
	for i, arg := range p.Args {
		arg = strings.ReplaceAll(arg, "$INPUT_FILE", inputPath)
		arg = strings.ReplaceAll(arg, "$OUTPUT_FILE", outputPath)
		args[i] = arg
	}
	return args
}

// GetCommandInfo はコマンドと引数の文字列表現を返す
func (p *CommandProvider) GetCommandInfo() string {
	if len(p.Args) == 0 {
		return p.Command
	}
	// 長い引数は省略
	var displayArgs []string
	for _, arg := range p.Args {
		if len(arg) > 50 {
			displayArgs = append(displayArgs, arg[:47]+"...")
		} else {
			displayArgs = append(displayArgs, arg)
		}
	}
	return p.Command + " " + strings.Join(displayArgs, " ")
}
