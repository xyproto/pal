package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/iafan/cwalk"
	"github.com/xyproto/files"
)

type Findings struct {
	git          *Git
	mut          sync.Mutex
	regularFiles []string
	ignoredFiles []string
	infoMap      map[string]os.FileInfo
	dirList      []string
	fileList     []string
	printMap     map[time.Time]string
}

func NewFindings() *Findings {
	var findings Findings
	findings.regularFiles = make([]string, 0)
	findings.ignoredFiles = make([]string, 0)
	findings.infoMap = make(map[string]os.FileInfo)
	return &findings
}

func SplitPath(path string) []string {
	return strings.Split(path, string(filepath.Separator))
}

func Examine(path string, respectIgnoreFiles, respectHiddenFiles bool, maxDepth int) (*Findings, error) {
	if !files.IsDir(path) {
		return nil, fmt.Errorf("not a path: %s", path)
	}

	findings := NewFindings()

	var ignoreMut sync.Mutex
	var extraIgnoredFiles []string
	var wg sync.WaitGroup

	walkFunc := func(path string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("os.Lstat error: %v", err)
		}
		if path == "" {
			return nil // skip
		}
		parts := SplitPath(path)
		if len(parts) == 0 {
			return fmt.Errorf("no path given: %s", path)
		} else if len(parts) > maxDepth {
			return filepath.SkipDir // skip this directory
		}
		head := strings.ToLower(parts[0])
		if head == "vendor" {
			if respectIgnoreFiles {
				// Store the ignored file
				wg.Add(1)
				go func() {
					defer wg.Done()
					findings.mut.Lock()
					findings.ignoredFiles = append(findings.ignoredFiles, path)
					findings.infoMap[path] = fileInfo
					findings.mut.Unlock()
				}()
			} else {
				// Store a regular file
				wg.Add(1)
				go func() {
					defer wg.Done()
					findings.mut.Lock()
					findings.regularFiles = append(findings.regularFiles, path)
					findings.infoMap[path] = fileInfo
					findings.mut.Unlock()
				}()
			}
			return nil // skip
		}
		if head == ".git" {
			if respectIgnoreFiles {
				// Store the ignored file
				wg.Add(1)
				go func() {
					defer wg.Done()
					findings.mut.Lock()
					findings.ignoredFiles = append(findings.ignoredFiles, path)
					findings.infoMap[path] = fileInfo
					findings.mut.Unlock()
				}()
			} else {
				// Store a regular file
				wg.Add(1)
				go func() {
					defer wg.Done()
					findings.mut.Lock()
					findings.regularFiles = append(findings.regularFiles, path)
					findings.infoMap[path] = fileInfo
					findings.mut.Unlock()
				}()
			}
			// Find and store Git info
			foundGit := false
			findings.mut.Lock()
			foundGit = findings.git != nil
			findings.mut.Unlock()
			if !foundGit {
				wg.Add(1)
				go func() {
					defer wg.Done()
					git, err := NewGit(head) // pass in the path to the .git directory
					if err != nil {
						return // don't store the git struct in the findings
					}
					findings.mut.Lock()
					findings.git = git
					findings.mut.Unlock()
				}()
			}
			return nil // skip
		}
		if respectHiddenFiles && len(head) > 1 && strings.HasPrefix(head, ".") {
			// Store the ignored file
			wg.Add(1)
			go func() {
				defer wg.Done()
				findings.mut.Lock()
				findings.ignoredFiles = append(findings.ignoredFiles, path)
				findings.infoMap[path] = fileInfo
				findings.mut.Unlock()
			}()
			return nil // skip
		}
		if respectIgnoreFiles && (head == ".ignore" || head == ".gitignore") {
			if extraIgnoredFilesData, err := os.ReadFile(head); err == nil { // success
				lines := strings.Split(string(extraIgnoredFilesData), "\n")
				for _, line := range lines {
					trimmedLine := strings.TrimSpace(line)
					if trimmedLine != "" && !strings.HasPrefix(trimmedLine, "#") {
						ignoreMut.Lock()
						extraIgnoredFiles = append(extraIgnoredFiles, trimmedLine)
						ignoreMut.Unlock()
					}
				}
			}
		}
		if respectHiddenFiles && len(head) > 1 && strings.HasPrefix(head, ".") {
			ignoreMut.Lock()
			extraIgnoredFiles = append(extraIgnoredFiles, head)
			ignoreMut.Unlock()
		}
		// Store a regular file
		wg.Add(1)
		go func() {
			defer wg.Done()
			findings.mut.Lock()
			findings.regularFiles = append(findings.regularFiles, path)
			findings.infoMap[path] = fileInfo
			findings.mut.Unlock()
		}()
		return nil // all good
	}

	if err := cwalk.Walk(path, walkFunc); err != nil {
		return nil, err
	}

	hasS := func(xs []string, x string) (bool, int) {
		for i, e := range xs {
			if x == e {
				return true, i
			}
		}
		return false, -1
	}

	wg.Wait() // The mutexes above are not needed after this point

	for _, extraIgnoredFile := range extraIgnoredFiles {
		if ok, index := hasS(findings.regularFiles, extraIgnoredFile); ok {
			// delete extraIgnoredFile from findings.regular by appending two sliced string slices
			findings.regularFiles = append(findings.regularFiles[:index], findings.regularFiles[index+1:]...)
			// add extraIgnoredFile to findings.ignoredFiles
			findings.ignoredFiles = append(findings.ignoredFiles, extraIgnoredFile)
		}
	}

	return findings, nil
}
