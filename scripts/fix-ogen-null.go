//go:build ignore

package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// ogen生成コードのnull処理を追加するスクリプト
// Backlog APIはnullableフィールドが多いため、Optional型のDecodeメソッドに
// nullチェックを追加する必要がある

func main() {
	targetFile := "packages/backlog/internal/gen/backlog/oas_json_gen.go"

	content, err := os.ReadFile(targetFile)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	original := string(content)
	modified := original

	// 修正対象の型リスト
	types := []string{
		"OptNulabAccount",
		"OptString",
		"OptUser",
		"OptIssue",
		"OptIssueType",
		"OptPRStatus",
		"OptPriority",
		"OptStatus",
	}

	for _, typeName := range types {
		modified = addNullCheck(modified, typeName)
	}

	if modified == original {
		fmt.Println("No changes needed")
		return
	}

	if err := os.WriteFile(targetFile, []byte(modified), 0644); err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Successfully fixed null handling in ogen generated code")
}

func addNullCheck(content, typeName string) string {
	// Decodeメソッドを探す正規表現
	// パターン: func (o *TypeName) Decode(d *jx.Decoder) error {
	//            if o == nil {
	//                return errors.New("...")
	//            }
	//            o.Set = true
	pattern := fmt.Sprintf(
		`(// Decode decodes [^\n]+ from json\.\nfunc \(o \*%s\) Decode\(d \*jx\.Decoder\) error \{\n\tif o == nil \{\n\t\treturn errors\.New\("[^"]+"\)\n\t\})(\n\to\.Set = true)`,
		regexp.QuoteMeta(typeName),
	)

	nullCheck := `
	if d.Next() == jx.Null {
		if err := d.Null(); err != nil {
			return err
		}
		return nil
	}`

	re := regexp.MustCompile(pattern)
	if re.MatchString(content) {
		// すでにnullチェックが含まれているか確認
		if strings.Contains(content, fmt.Sprintf("func (o *%s) Decode(d *jx.Decoder) error {\n\tif o == nil", typeName)) &&
			!strings.Contains(content, fmt.Sprintf("func (o *%s) Decode(d *jx.Decoder) error {\n\tif o == nil {\n\t\treturn errors.New", typeName)+`"`+"\n\t}\n\tif d.Next() == jx.Null") {
			// nullチェックを追加
			return re.ReplaceAllString(content, "$1"+nullCheck+"$2")
		}
	}

	return content
}
