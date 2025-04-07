package main

import (
	"fmt"
	"regexp"
)

func main() {
	// 测试字符串
	testStr := `import*as __0$ from"/react@19.0.0/es2022/react.mjs";
import{decode as il}from"/turbo-stream@2.4.0/es2022/turbo-stream.mjs";
import Q from"/axios@1.6.2/index.js";
import * as React from "/react@19.0.0?target=es2022";
import { useState } from "/react@19.0.0/es2022/react.mjs?target=es2022";
import Ql,{useContext as tV,useEffect as rV}from"/react@19.0.0/es2022/react.mjs";
import*as Q from"/react@19.0.0/es2022/react.mjs"; `
	
	// 原始正则表达式
	originalFindDeepRegex := regexp.MustCompile(`(?:import\s*\*?\s*as\s*[^"']*\s*from|import\s*\{[^}]*\}\s*from|import\s+[^"'\s]+\s+from|import|export\s*\*\s*from|export\s*\{\s*[^}]*\}\s*from)\s*["']((?:\/|\.[\.\/]).*?)["']`)
	originalProcessRegex := regexp.MustCompile(`(?:import\s*\*?\s*as\s*[^"']*\s*from|import\s*\{[^}]*\}\s*from|import\s+[^"'\s]+\s+from|import|export\s*\*\s*from|export\s*\{\s*[^}]*\}\s*from)\s*["'](\/.+?)["']`)
	
	// 改进的正则表达式 - 添加支持复杂多导入情况
	// 添加对 import Ql,{...} from 模式的支持
	improvedFindDeepRegex := regexp.MustCompile(`(?:import\s*\*?\s*as\s*[^"']*\s*from|import\s*[^"'\s]+\s*,\s*\{[^}]*\}\s*from|import\s*\{[^}]*\}\s*from|import\s+[^"'\s]+\s+from|import|export\s*\*\s*from|export\s*\{\s*[^}]*\}\s*from)\s*["']((?:\/|\.[\.\/]).*?)["']`)
	improvedProcessRegex := regexp.MustCompile(`(?:import\s*\*?\s*as\s*[^"']*\s*from|import\s*[^"'\s]+\s*,\s*\{[^}]*\}\s*from|import\s*\{[^}]*\}\s*from|import\s+[^"'\s]+\s+from|import|export\s*\*\s*from|export\s*\{\s*[^}]*\}\s*from)\s*["'](\/.+?)["']`)
	
	fmt.Println("=====================================================")
	fmt.Println("测试原始 vs 改进的正则表达式")
	fmt.Println("=====================================================")
	complexImportStr := `import Ql,{useContext as tV,useEffect as rV}from"/react@19.0.0/es2022/react.mjs";`
	
	fmt.Println("\n原始 findDeepDependencies 正则表达式对复杂多导入的匹配结果:")
	originalFindMatches := originalFindDeepRegex.FindAllStringSubmatch(complexImportStr, -1)
	if len(originalFindMatches) > 0 {
		for i, match := range originalFindMatches {
			fmt.Printf("匹配 #%d: %v\n", i+1, match)
		}
	} else {
		fmt.Println("没有匹配到任何内容 - 原始正则表达式无法处理复杂多导入情况")
	}
	
	fmt.Println("\n改进的 findDeepDependencies 正则表达式对复杂多导入的匹配结果:")
	improvedFindMatches := improvedFindDeepRegex.FindAllStringSubmatch(complexImportStr, -1)
	if len(improvedFindMatches) > 0 {
		for i, match := range improvedFindMatches {
			fmt.Printf("匹配 #%d: %v\n", i+1, match)
		}
	} else {
		fmt.Println("没有匹配到任何内容 - 改进的正则表达式仍无法处理复杂多导入情况")
	}
	
	fmt.Println("\n原始 processWrapperContent 正则表达式对复杂多导入的匹配结果:")
	originalProcessMatches := originalProcessRegex.FindAllStringSubmatch(complexImportStr, -1)
	if len(originalProcessMatches) > 0 {
		for i, match := range originalProcessMatches {
			fmt.Printf("匹配 #%d: %v\n", i+1, match)
		}
	} else {
		fmt.Println("没有匹配到任何内容 - 原始正则表达式无法处理复杂多导入情况")
	}
	
	fmt.Println("\n改进的 processWrapperContent 正则表达式对复杂多导入的匹配结果:")
	improvedProcessMatches := improvedProcessRegex.FindAllStringSubmatch(complexImportStr, -1)
	if len(improvedProcessMatches) > 0 {
		for i, match := range improvedProcessMatches {
			fmt.Printf("匹配 #%d: %v\n", i+1, match)
		}
	} else {
		fmt.Println("没有匹配到任何内容 - 改进的正则表达式仍无法处理复杂多导入情况")
	}
	
	fmt.Println("\n=====================================================")
	fmt.Println("对整个测试字符串进行匹配")
	fmt.Println("=====================================================")
	
	fmt.Println("\n原始 findDeepDependencies 正则表达式对所有测试用例的匹配结果:")
	allOriginalFindMatches := originalFindDeepRegex.FindAllStringSubmatch(testStr, -1)
	fmt.Printf("匹配数量: %d\n", len(allOriginalFindMatches))
	
	fmt.Println("\n改进的 findDeepDependencies 正则表达式对所有测试用例的匹配结果:")
	allImprovedFindMatches := improvedFindDeepRegex.FindAllStringSubmatch(testStr, -1)
	fmt.Printf("匹配数量: %d\n", len(allImprovedFindMatches))
	for i, match := range allImprovedFindMatches {
		fmt.Printf("匹配 #%d: %v\n", i+1, match)
	}
} 