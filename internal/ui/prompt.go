package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlecAivazis/survey/v2"
)

// Select は選択肢から1つを選ばせる
func Select(message string, options []string) (string, error) {
	var result string
	prompt := &survey.Select{
		Message: message,
		Options: options,
	}
	if err := survey.AskOne(prompt, &result); err != nil {
		return "", err
	}
	return result, nil
}

// SelectOption は説明付きの選択肢
type SelectOption struct {
	Value       string
	Description string
}

// SelectWithDesc は説明付きの選択肢から1つを選ばせる
func SelectWithDesc(message string, options []SelectOption) (string, error) {
	labels := make([]string, len(options))
	valueMap := make(map[string]string)

	for i, opt := range options {
		if opt.Description != "" {
			labels[i] = opt.Value + " - " + opt.Description
		} else {
			labels[i] = opt.Value
		}
		valueMap[labels[i]] = opt.Value
	}

	var result string
	prompt := &survey.Select{
		Message: message,
		Options: labels,
	}
	if err := survey.AskOne(prompt, &result); err != nil {
		return "", err
	}
	return valueMap[result], nil
}

// Input はテキスト入力を受け付ける
func Input(message string, defaultValue string) (string, error) {
	var result string
	prompt := &survey.Input{
		Message: message,
		Default: defaultValue,
	}
	if err := survey.AskOne(prompt, &result); err != nil {
		return "", err
	}
	return result, nil
}

// Confirm は確認プロンプトを表示する
func Confirm(message string, defaultValue bool) (bool, error) {
	var result bool
	prompt := &survey.Confirm{
		Message: message,
		Default: defaultValue,
	}
	if err := survey.AskOne(prompt, &result); err != nil {
		return false, err
	}
	return result, nil
}

// Password はパスワード入力を受け付ける（入力は非表示）
func Password(message string) (string, error) {
	var result string
	prompt := &survey.Password{
		Message: message,
	}
	if err := survey.AskOne(prompt, &result); err != nil {
		return "", err
	}
	return result, nil
}

// SelectDirectory はディレクトリを対話的に選択する
// 親ディレクトリへの移動、子ディレクトリへの移動、現在のディレクトリの選択ができる
func SelectDirectory(startDir string) (string, error) {
	currentDir := startDir

	for {
		// 選択肢を構築
		options := []string{
			"[Select this directory]",
			"..",
		}

		// サブディレクトリを取得
		entries, err := os.ReadDir(currentDir)
		if err != nil {
			return "", err
		}

		var subdirs []string
		for _, entry := range entries {
			if entry.IsDir() {
				subdirs = append(subdirs, entry.Name()+"/")
			}
		}
		options = append(options, subdirs...)

		// 現在のディレクトリを表示
		fmt.Printf("\nCurrent: %s\n", currentDir)

		var selected string
		prompt := &survey.Select{
			Message: "Select directory:",
			Options: options,
		}
		if err := survey.AskOne(prompt, &selected); err != nil {
			return "", err
		}

		switch selected {
		case "[Select this directory]":
			return currentDir, nil
		case "..":
			parent := filepath.Dir(currentDir)
			if parent != currentDir {
				currentDir = parent
			}
		default:
			// trailing "/" を除去
			dirName := strings.TrimSuffix(selected, "/")
			currentDir = filepath.Join(currentDir, dirName)
		}
	}
}
