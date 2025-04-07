package main

import (
	"fmt"
	"regexp"
)

func main() {
	// 测试字符串
	testStr := `import*as __0$ from"/react@19.0.0/es2022/react.mjs";
import{decode as il}from"/turbo-stream@2.4.0/es2022/turbo-stream.mjs";`
	
	// 当前代码中使用的正则表达式
	dependencyRegex := regexp.MustCompile(`(?:import|export\s*\*\s*from|export\s*\{\s*[^}]*\}\s*from)\s*["'](\/[@\w\d\.\-]+\/[^"']+)["']`)
	matches := dependencyRegex.FindAllStringSubmatch(testStr, -1)
	
	fmt.Println("当前正则表达式匹配结果:")
	if len(matches) > 0 {
		for i, match := range matches {
			fmt.Printf("匹配 #%d: %v\n", i+1, match)
		}
	} else {
		fmt.Println("没有匹配到任何内容")
	}
	
	// 修改后的正则表达式，更宽松地匹配import语句
	improvedRegex := regexp.MustCompile(`import\s*(?:\*?\s*as\s*[^"']*|\{[^}]*\})\s*from\s*["'](\/[@\w\d\.\-]+\/[^"']+)["']`)
	improvedMatches := improvedRegex.FindAllStringSubmatch(testStr, -1)
	
	fmt.Println("\n改进后的正则表达式匹配结果:")
	if len(improvedMatches) > 0 {
		for i, match := range improvedMatches {
			fmt.Printf("匹配 #%d: %v\n", i+1, match)
		}
	} else {
		fmt.Println("没有匹配到任何内容")
	}
	
	// 更全面的正则表达式
	completeRegex := regexp.MustCompile(`(?:import\s*\*?\s*as\s*[^"']*\s*from|import\s*\{[^}]*\}\s*from|import|export\s*\*\s*from|export\s*\{\s*[^}]*\}\s*from)\s*["'](\/[@\w\d\.\-]+\/[^"']+)["']`)
	completeMatches := completeRegex.FindAllStringSubmatch(testStr, -1)
	
	fmt.Println("\n全面改进的正则表达式匹配结果:")
	if len(completeMatches) > 0 {
		for i, match := range completeMatches {
			fmt.Printf("匹配 #%d: %v\n", i+1, match)
		}
	} else {
		fmt.Println("没有匹配到任何内容")
	}
} 