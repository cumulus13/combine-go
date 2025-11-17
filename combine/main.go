package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	VERSION        = "2.1.0"
	MAX_FILE_SIZE  = 100 * 1024 * 1024 // 100MB
	BUFFER_SIZE    = 8192
)

// CommentStyle defines how to format comments for different file types
type CommentStyle struct {
	SingleLine string
	BlockStart string
	BlockEnd   string
}

// Config holds command-line arguments
type Config struct {
	Patterns        []string
	Output          string
	Excludes        []string
	Root            string
	NoSeparator     bool
	Encoding        string
	NewlineType     string
	MaxSize         int64
	IgnoreGitignore bool
	DryRun          bool
	Verbose         bool
	Debug           bool
}

// FileInfo holds information about processed files
type FileInfo struct {
	Path   string
	Reason string
}

// Comment styles by extension
var commentStyles = map[string]CommentStyle{
	// # comments
	".py":   {SingleLine: "#"},
	".rb":   {SingleLine: "#"},
	".sh":   {SingleLine: "#"},
	".bash": {SingleLine: "#"},
	".zsh":  {SingleLine: "#"},
	".yaml": {SingleLine: "#"},
	".yml":  {SingleLine: "#"},
	".toml": {SingleLine: "#"},
	".conf": {SingleLine: "#"},
	".ini":  {SingleLine: "#"},
	".r":    {SingleLine: "#"},
	".pl":   {SingleLine: "#"},
	".pm":   {SingleLine: "#"},

	// // comments
	".js":    {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},
	".ts":    {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},
	".jsx":   {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},
	".tsx":   {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},
	".java":  {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},
	".c":     {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},
	".cpp":   {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},
	".cc":    {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},
	".h":     {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},
	".hpp":   {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},
	".cs":    {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},
	".go":    {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},
	".swift": {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},
	".kt":    {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},
	".scala": {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},
	".rs":    {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},
	".dart":  {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},
	".php":   {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},

	// Markup
	".html": {BlockStart: "<!--", BlockEnd: "-->"},
	".xml":  {BlockStart: "<!--", BlockEnd: "-->"},
	".svg":  {BlockStart: "<!--", BlockEnd: "-->"},
	".css":  {BlockStart: "/*", BlockEnd: "*/"},
	".scss": {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},
	".sass": {SingleLine: "//"},
	".less": {SingleLine: "//", BlockStart: "/*", BlockEnd: "*/"},

	// Others
	".sql":  {SingleLine: "--", BlockStart: "/*", BlockEnd: "*/"},
	".lisp": {SingleLine: ";"},
	".clj":  {SingleLine: ";"},
	".scm":  {SingleLine: ";"},
	".lua":  {SingleLine: "--", BlockStart: "--[[", BlockEnd: "]]"},
	".bat":  {SingleLine: "REM"},
	".cmd":  {SingleLine: "REM"},
	".vb":   {SingleLine: "'"},
	".m":    {SingleLine: "%"},
	".tex":  {SingleLine: "%"},
	".txt":  {SingleLine: "#"},
	".md":   {BlockStart: "<!--", BlockEnd: "-->"},
	".rst":  {SingleLine: ".."},
}

// Binary file extensions
var binaryExtensions = map[string]bool{
	".exe": true, ".dll": true, ".so": true, ".dylib": true, ".bin": true, ".dat": true,
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".bmp": true, ".ico": true,
	".mp3": true, ".mp4": true, ".wav": true, ".avi": true, ".mov": true, ".flv": true,
	".zip": true, ".tar": true, ".gz": true, ".bz2": true, ".7z": true, ".rar": true,
	".pdf": true, ".doc": true, ".docx": true, ".xls": true, ".xlsx": true,
	".ppt": true, ".pptx": true, ".pyc": true, ".pyo": true, ".class": true,
	".o": true, ".obj": true,
}

// Known text file extensions
var textExtensions = map[string]bool{
	".js": true, ".ts": true, ".jsx": true, ".tsx": true, ".json": true,
	".html": true, ".htm": true, ".xml": true, ".css": true, ".scss": true,
	".sass": true, ".less": true, ".md": true, ".txt": true, ".csv": true,
	".py": true, ".rb": true, ".java": true, ".c": true, ".cpp": true,
	".h": true, ".hpp": true, ".go": true, ".rs": true, ".php": true,
	".sh": true, ".bash": true, ".zsh": true, ".bat": true, ".cmd": true,
	".ps1": true, ".yaml": true, ".yml": true, ".toml": true, ".ini": true,
	".conf": true, ".cfg": true, ".sql": true, ".r": true, ".m": true,
	".pl": true, ".pm": true, ".lua": true, ".swift": true, ".kt": true,
	".dart": true, ".vue": true, ".svelte": true, ".astro": true,
	".cs": true, ".vb": true, ".fs": true, ".lisp": true, ".clj": true,
	".scm": true, ".scala": true, ".erl": true, ".ex": true, ".exs": true,
	".dockerfile": true, ".gitignore": true, ".env": true, ".editorconfig": true,
	".rst": true, ".adoc": true, ".textile": true, ".org": true,
}

func main() {
	config := parseFlags()

	if config.Debug {
		config.Verbose = true
	}

	// Validate root directory
	rootInfo, err := os.Stat(config.Root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Root directory does not exist: %s\n", config.Root)
		os.Exit(1)
	}
	if !rootInfo.IsDir() {
		fmt.Fprintf(os.Stderr, "Error: Root path is not a directory: %s\n", config.Root)
		os.Exit(1)
	}

	// Load gitignore patterns
	var gitignorePatterns []string
	if !config.IgnoreGitignore {
		gitignorePatterns = loadGitignore(config.Root, config.Verbose)
	}

	// Combine all exclusion patterns
	allExcludes := append(config.Excludes, gitignorePatterns...)

	// Find files
	if config.Verbose {
		fmt.Println("Searching for files...")
	}

	files, skipped := findFiles(config.Root, config.Patterns, allExcludes, config.MaxSize, config.Verbose)

	// Print summary
	printSummary(config, files, skipped)

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "Error: No files found matching the patterns")
		os.Exit(1)
	}

	// Dry run mode
	if config.DryRun {
		fmt.Println("Dry-run mode: No files were modified")
		os.Exit(0)
	}

	// Combine files
	if config.Verbose {
		fmt.Println("Combining files...")
	}

	exitCode := combineFiles(config, files)
	os.Exit(exitCode)
}

func parseFlags() *Config {
	config := &Config{}

	var patterns string
	var excludes string

	flag.StringVar(&patterns, "p", "", "Glob patterns (comma-separated), e.g., \"*.py,*.txt\"")
	flag.StringVar(&config.Output, "o", "", "Output file path (required)")
	flag.StringVar(&excludes, "e", "", "Exclude patterns (comma-separated)")
	flag.StringVar(&config.Root, "root", ".", "Root directory to search")
	flag.BoolVar(&config.NoSeparator, "no-separator", false, "Don't add separators between files")
	flag.StringVar(&config.Encoding, "encoding", "utf-8", "Output file encoding")
	flag.StringVar(&config.NewlineType, "newline", "lf", "Newline type: lf, crlf, cr")
	flag.Int64Var(&config.MaxSize, "max-size", MAX_FILE_SIZE, "Maximum file size in bytes")
	flag.BoolVar(&config.IgnoreGitignore, "ignore-gitignore", false, "Don't read .gitignore")
	flag.BoolVar(&config.DryRun, "dry-run", false, "Preview without writing")
	flag.BoolVar(&config.Verbose, "v", false, "Verbose output")
	flag.BoolVar(&config.Debug, "debug", false, "Debug mode")

	version := flag.Bool("version", false, "Show version")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "combine v%s - Combine multiple files matching glob patterns\n\n", VERSION)
		fmt.Fprintf(os.Stderr, "Usage: combine -p PATTERNS -o OUTPUT [options]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  combine -p \"*.py\" -o combined.py\n")
		fmt.Fprintf(os.Stderr, "  combine -p \"*.go,*.mod\" -o project.txt\n")
		fmt.Fprintf(os.Stderr, "  combine -p \"**/*.js\" -o bundle.js -e \"node_modules,dist\"\n")
		fmt.Fprintf(os.Stderr, "  combine -p \"src/**/*.cpp\" -o output.cpp --dry-run\n")
	}

	flag.Parse()

	if *version {
		fmt.Printf("combine v%s\n", VERSION)
		os.Exit(0)
	}

	if patterns == "" || config.Output == "" {
		flag.Usage()
		os.Exit(1)
	}

	// Parse patterns
	config.Patterns = strings.Split(patterns, ",")
	for i := range config.Patterns {
		config.Patterns[i] = strings.TrimSpace(config.Patterns[i])
	}

	// Parse excludes
	if excludes != "" {
		config.Excludes = strings.Split(excludes, ",")
		for i := range config.Excludes {
			config.Excludes[i] = strings.TrimSpace(config.Excludes[i])
		}
	}

	// Validate newline type
	config.NewlineType = strings.ToLower(config.NewlineType)

	return config
}

func loadGitignore(root string, verbose bool) []string {
	patterns := []string{}
	gitignorePath := filepath.Join(root, ".gitignore")

	file, err := os.Open(gitignorePath)
	if err != nil {
		return patterns
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}

	if verbose {
		fmt.Printf("Loaded %d patterns from .gitignore\n", len(patterns))
	}

	return patterns
}

func matchExcluded(path, root string, patterns []string) bool {
	relPath, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}

	relPath = filepath.ToSlash(relPath)

	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		
		// Direct substring match
		if strings.Contains(relPath, pattern) {
			return true
		}

		// Pattern matching
		matched, _ := filepath.Match(pattern, filepath.Base(relPath))
		if matched {
			return true
		}

		// Check parent directories
		parts := strings.Split(relPath, "/")
		for _, part := range parts {
			if part == strings.TrimSuffix(pattern, "/") {
				return true
			}
		}
	}

	return false
}

func isBinaryFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))

	// Check binary extensions
	if binaryExtensions[ext] {
		return true
	}

	// Check text extensions
	if textExtensions[ext] {
		return false
	}

	// Check content
	file, err := os.Open(path)
	if err != nil {
		return true
	}
	defer file.Close()

	buffer := make([]byte, BUFFER_SIZE)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return true
	}

	buffer = buffer[:n]

	// Empty file is text
	if n == 0 {
		return false
	}

	// Check for null bytes
	if bytes.Contains(buffer, []byte{0}) {
		return true
	}

	// Check ratio of non-printable characters
	nonPrintable := 0
	for _, b := range buffer {
		if b < 32 && b != 9 && b != 10 && b != 13 {
			nonPrintable++
		}
	}

	ratio := float64(nonPrintable) / float64(len(buffer))
	return ratio > 0.3
}

func findFiles(root string, patterns []string, excludes []string, maxSize int64, verbose bool) ([]string, []FileInfo) {
	allFiles := make(map[string]bool)
	var skipped []FileInfo

	// Collect files from all patterns
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(root, pattern))
		if err == nil {
			for _, match := range matches {
				allFiles[match] = true
			}
		}

		// Also support recursive patterns
		err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return nil
			}

			matched, _ := filepath.Match(pattern, filepath.Base(path))
			if matched {
				allFiles[path] = true
			}

			return nil
		})
	}

	// Convert to sorted slice
	var files []string
	for file := range allFiles {
		files = append(files, file)
	}
	sort.Strings(files)

	// Filter and validate
	var results []string
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			skipped = append(skipped, FileInfo{file, fmt.Sprintf("Cannot stat: %v", err)})
			continue
		}

		if !info.Mode().IsRegular() {
			continue
		}

		// Check exclusions
		if matchExcluded(file, root, excludes) {
			skipped = append(skipped, FileInfo{file, "Matched exclusion pattern"})
			continue
		}

		// Check file size
		if info.Size() > maxSize {
			skipped = append(skipped, FileInfo{file, fmt.Sprintf("Too large (%.1f MB)", float64(info.Size())/1024/1024)})
			continue
		}

		// Check if binary
		if isBinaryFile(file) {
			skipped = append(skipped, FileInfo{file, "Binary file"})
			continue
		}

		results = append(results, file)
	}

	return results, skipped
}

func getCommentStyle(path string) CommentStyle {
	ext := strings.ToLower(filepath.Ext(path))
	if style, ok := commentStyles[ext]; ok {
		return style
	}
	return CommentStyle{SingleLine: "#"}
}

func createSeparator(path, root string, index int, style CommentStyle) string {
	relPath, _ := filepath.Rel(root, path)
	relPath = filepath.ToSlash(relPath)
	timestamp := time.Now().Format("2006-01-02 15:04:05")

	separator := "\n"

	if style.BlockStart != "" && style.BlockEnd != "" {
		separator += fmt.Sprintf("%s\n FILE %d: %s\n Combined at: %s\n%s\n\n",
			style.BlockStart, index, relPath, timestamp, style.BlockEnd)
	} else if style.SingleLine != "" {
		line := strings.Repeat("=", 70)
		separator += fmt.Sprintf("%s %s\n%s FILE %d: %s\n%s Combined at: %s\n%s %s\n\n",
			style.SingleLine, line,
			style.SingleLine, index, relPath,
			style.SingleLine, timestamp,
			style.SingleLine, line)
	} else {
		line := strings.Repeat("=", 70)
		separator += fmt.Sprintf("%s\n FILE %d: %s\n%s\n\n", line, index, relPath, line)
	}

	return separator
}

func getNewline(newlineType string) string {
	switch strings.ToLower(newlineType) {
	case "crlf", "\\r\\n":
		return "\r\n"
	case "cr", "\\r":
		return "\r"
	default:
		return "\n"
	}
}

func combineFiles(config *Config, files []string) int {
	// Remove output file from input files
	absOutput, _ := filepath.Abs(config.Output)
	var filteredFiles []string
	for _, file := range files {
		absFile, _ := filepath.Abs(file)
		if absFile != absOutput {
			filteredFiles = append(filteredFiles, file)
		}
	}
	files = filteredFiles

	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "Error: No files to combine after filtering")
		return 1
	}

	// Create output directory if needed
	outputDir := filepath.Dir(config.Output)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot create output directory: %v\n", err)
		return 2
	}

	// Open output file
	outFile, err := os.Create(config.Output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Cannot create output file: %v\n", err)
		return 2
	}
	defer outFile.Close()

	writer := bufio.NewWriter(outFile)
	defer writer.Flush()

	newline := getNewline(config.NewlineType)
	successCount := 0
	errorCount := 0

	for idx, filePath := range files {
		if config.Verbose {
			fmt.Printf("Processing [%d/%d]: %s\n", idx+1, len(files), filepath.Base(filePath))
		}

		// Read file
		content, err := os.ReadFile(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Skipped %s: %v\n", filePath, err)
			errorCount++
			continue
		}

		// Add separator
		if !config.NoSeparator {
			style := getCommentStyle(filePath)
			separator := createSeparator(filePath, config.Root, idx+1, style)
			writer.WriteString(separator)
		}

		// Write content
		writer.Write(content)

		// Ensure newline at end
		if len(content) > 0 && !bytes.HasSuffix(content, []byte(newline)) {
			writer.WriteString(newline)
		}

		successCount++
	}

	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Printf("SUCCESS: Combined %d files into %s\n", successCount, config.Output)
	if errorCount > 0 {
		fmt.Printf("WARNING: %d files were skipped due to errors\n", errorCount)
	}
	fmt.Println(strings.Repeat("=", 70))

	return 0
}

func printSummary(config *Config, files []string, skipped []FileInfo) {
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("COMBINE FILES - SUMMARY")
	fmt.Println(strings.Repeat("=", 70))
	fmt.Printf("Root directory    : %s\n", config.Root)
	fmt.Printf("Output file       : %s\n", config.Output)
	fmt.Printf("Search patterns   : %s\n", strings.Join(config.Patterns, ", "))
	fmt.Printf("Files found       : %d\n", len(files))
	fmt.Printf("Files excluded    : %d\n", len(skipped))
	if config.DryRun {
		fmt.Printf("Mode              : DRY-RUN (no changes)\n")
	} else {
		fmt.Printf("Mode              : EXECUTION\n")
	}
	fmt.Println(strings.Repeat("=", 70))

	if len(skipped) > 0 {
		fmt.Println("\nEXCLUDED FILES (showing first 15):")
		limit := len(skipped)
		if limit > 15 {
			limit = 15
		}
		for i := 0; i < limit; i++ {
			relPath, _ := filepath.Rel(config.Root, skipped[i].Path)
			fmt.Printf("  × %s\n", relPath)
			fmt.Printf("    Reason: %s\n", skipped[i].Reason)
		}
		if len(skipped) > 15 {
			fmt.Printf("  ... and %d more files\n\n", len(skipped)-15)
		}
	}

	if config.DryRun && len(files) > 0 {
		fmt.Println("\nFILES TO BE COMBINED (showing first 20):")
		limit := len(files)
		if limit > 20 {
			limit = 20
		}
		for i := 0; i < limit; i++ {
			relPath, _ := filepath.Rel(config.Root, files[i])
			info, _ := os.Stat(files[i])
			sizeKB := float64(info.Size()) / 1024
			fmt.Printf("  ✓ %s (%.1f KB)\n", relPath, sizeKB)
		}
		if len(files) > 20 {
			fmt.Printf("  ... and %d more files\n", len(files)-20)
		}
		fmt.Printf("\nTotal: %d files will be combined\n", len(files))
	}
}