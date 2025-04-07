package main

import (
	"fmt"
	"regexp"
	"strings"
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
	
	// 测试带查询参数的导入路径
	fmt.Println("\n==================")
	fmt.Println("测试带查询参数的导入路径")
	fmt.Println("==================")
	
	queryTestStr := `import "/scheduler@^0.25.0?target=es2022";
import * as React from "/react@19.0.0?target=es2022";
import { useState } from "/react@19.0.0/es2022/react.mjs?target=es2022";`
	
	// 测试当前正则表达式是否能匹配带查询参数的路径
	fmt.Println("\n当前正则表达式对带查询参数路径的匹配结果:")
	queryMatches := dependencyRegex.FindAllStringSubmatch(queryTestStr, -1)
	if len(queryMatches) > 0 {
		for i, match := range queryMatches {
			fmt.Printf("匹配 #%d: %v\n", i+1, match)
		}
	} else {
		fmt.Println("没有匹配到任何内容 - 当前正则表达式无法处理带查询参数的路径")
	}
	
	// 针对带查询参数路径的改进正则表达式 1 - 使用(.+?)替代(\/[@\w\d\.\-]+\/[^"']+)
	queryRegex1 := regexp.MustCompile(`(?:import\s*\*?\s*as\s*[^"']*\s*from|import\s*\{[^}]*\}\s*from|import|export\s*\*\s*from|export\s*\{\s*[^}]*\}\s*from)\s*["'](\/[^"']+)["']`)
	queryMatches1 := queryRegex1.FindAllStringSubmatch(queryTestStr, -1)
	
	fmt.Println("\n改进正则表达式1对带查询参数路径的匹配结果:")
	if len(queryMatches1) > 0 {
		for i, match := range queryMatches1 {
			fmt.Printf("匹配 #%d: %v\n", i+1, match)
		}
	} else {
		fmt.Println("没有匹配到任何内容")
	}
	
	// 针对带查询参数路径的改进正则表达式 2 - 更完整的匹配
	queryRegex2 := regexp.MustCompile(`(?:import\s*\*?\s*as\s*[^"']*\s*from|import\s*\{[^}]*\}\s*from|import|export\s*\*\s*from|export\s*\{\s*[^}]*\}\s*from)\s*["'](\/[@\w\d\.\-]+(?:\/[^"'?]+)?(?:\?[^"']*)?)["']`)
	queryMatches2 := queryRegex2.FindAllStringSubmatch(queryTestStr, -1)
	
	fmt.Println("\n改进正则表达式2对带查询参数路径的匹配结果:")
	if len(queryMatches2) > 0 {
		for i, match := range queryMatches2 {
			fmt.Printf("匹配 #%d: %v\n", i+1, match)
		}
	} else {
		fmt.Println("没有匹配到任何内容")
	}
	
	// 对findDeepDependencies函数中使用的正则表达式进行测试
	findDeepRegex := regexp.MustCompile(`(?:import\s*\*?\s*as\s*[^"']*\s*from|import\s*\{[^}]*\}\s*from|import|export\s*\*\s*from|export\s*\{\s*[^}]*\}\s*from)\s*["'](\/[@\w\d\.\-]+\/[^"']+)["']`)
	findDeepMatches := findDeepRegex.FindAllStringSubmatch(queryTestStr, -1)
	
	fmt.Println("\nfindDeepDependencies函数使用的正则表达式对带查询参数路径的匹配结果:")
	if len(findDeepMatches) > 0 {
		for i, match := range findDeepMatches {
			fmt.Printf("匹配 #%d: %v\n", i+1, match)
		}
	} else {
		fmt.Println("没有匹配到任何内容 - findDeepDependencies函数无法处理带查询参数的路径")
	}
	
	// 推荐的最终完整正则表达式
	finalRegex := regexp.MustCompile(`(?:import\s*\*?\s*as\s*[^"']*\s*from|import\s*\{[^}]*\}\s*from|import|export\s*\*\s*from|export\s*\{\s*[^}]*\}\s*from)\s*["'](\/[^"']+)["']`)
	finalMatches := finalRegex.FindAllStringSubmatch(queryTestStr, -1)
	
	fmt.Println("\n推荐的最终正则表达式匹配结果:")
	if len(finalMatches) > 0 {
		for i, match := range finalMatches {
			fmt.Printf("匹配 #%d: %v\n", i+1, match)
			
			// 打印出修复后的路径处理逻辑示例
			path := match[1]
			fmt.Printf("  - 原始路径: %s\n", path)
			
			// 分离路径和查询参数
			var query string
			if strings.Contains(path, "?") {
				pathParts := strings.SplitN(path, "?", 2)
				path = pathParts[0]
				query = "?" + pathParts[1]
				fmt.Printf("  - 分离后的路径: %s\n", path)
				fmt.Printf("  - 查询参数: %s\n", query)
			}
		}
	} else {
		fmt.Println("没有匹配到任何内容")
	}
} 