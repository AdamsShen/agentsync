// Package ask 交互询问：分发目标多选、冲突三选一。
// M0 用纯文本 TUI（无第三方依赖），后续可换 bubbletea。
package ask

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// MultiSelect 多选询问。candidates 为可选项，默认全部选中。
// 返回选中的下标集合；用户输入空回车 = 全选。
func MultiSelect(prompt string, candidates []string, defaults []string) ([]string, error) {
	if !isTTY() {
		// 无 TTY：默认全选（调用方负责按 no_tty_policy 处理）
		return defaults, nil
	}
	fmt.Printf("\n%s\n", prompt)
	// 编号 + 标注默认
	idxDefault := map[int]bool{}
	for _, d := range defaults {
		for i, c := range candidates {
			if c == d {
				idxDefault[i] = true
			}
		}
	}
	fmt.Println("  [0] ⭐ 全部")
	for i, c := range candidates {
		mark := " "
		if idxDefault[i] {
			mark = "✓"
		}
		fmt.Printf("  [%d] %s %s\n", i+1, mark, c)
	}
	fmt.Printf("输入编号（逗号分隔；空回车=全部）：")
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && len(line) == 0 {
		return defaults, nil
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return defaults, nil
	}
	if line == "0" {
		return candidates, nil
	}
	parts := strings.FieldsFunc(line, func(r rune) bool { return r == ',' || r == ' ' })
	selected := []string{}
	seen := map[int]bool{}
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n < 1 || n > len(candidates) {
			continue
		}
		if seen[n] {
			continue
		}
		seen[n] = true
		selected = append(selected, candidates[n-1])
	}
	sort.Strings(selected)
	return selected, nil
}

// Confirm 是/否确认
func Confirm(prompt string, def bool) bool {
	if !isTTY() {
		return def
	}
	disp := "[y/N]"
	if def {
		disp = "[Y/n]"
	}
	fmt.Printf("%s %s: ", prompt, disp)
	reader := bufio.NewReader(os.Stdin)
	line, _ := reader.ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line == "" {
		return def
	}
	return line == "y" || line == "yes"
}

func isTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
