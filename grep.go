package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
)

var (
	recurse    = flag.Bool("r", false, "For each directory operand, read and process all files in the directory, recursively")
	ignoreCase = flag.Bool("i", false, "Ignore case distinctions in both the pattern and input files")
	invert     = flag.Bool("v", false, "Invert the sense of matching, to select non matching lines")
	wholeLine  = flag.Bool("x", false, "Select only those matches that exactly match the whole line")
)

// Program exit codes
const (
	exitMatchesFound int = 0
	exitNoMatches    int = 1
	exitError        int = 2
)

// A match represents a line in a particular file that matched the search pattern.
type match struct {
	file string
	line string
}

func (m *match) String() string {
	return m.file + ": " + m.line
}

// TODO
// - don't try to print contents of binary files
// - handle different text encodings?
// - regex patterns (not just text)
// - various flags
// - search stdin if no input files
// - parallelize for performance
// - wild card search (go\*.go)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: grep [options] PATTERN INPUT_FILES\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 2 {
		flag.Usage()
		os.Exit(exitError)
		return
	}

	fmt.Println(recurse, ignoreCase, invert, wholeLine)

	files := inputFiles(flag.Args()[1:])

	//fmt.Printf("files: %v\n", files)
	c := make(chan *match)

	// search each file, line-by-line, writing results to c
	go func() {
		for _, file := range files {
			_, _ = scanFile(file, flag.Arg(0), c)
		}
		close(c)
	}()

	// display matching lines
	matchFound := false
	for result := range c {
		if !matchFound {
			matchFound = true
		}
		fmt.Println(result)
	}

	var exit int
	if matchFound {
		exit = exitMatchesFound
	} else {
		exit = exitNoMatches
	}
	os.Exit(exit)
}

// inputFiles generates the list of all files that must be searched,
// given a particular set of input arguments.
func inputFiles(input []string) []string {
	var result []string
	// first get all the files in this directory that match the pattern
	for _, glob := range input {
		items, err := filepath.Glob(glob)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error %s\n", err.Error())
			continue
		}
		if items == nil {
			fmt.Fprintf(os.Stderr, "No match for %s\n", glob)
			continue
		}
		// for each glob match, add the regular files, and optionally recurse into subdirectories
		for _, file := range items {
			fileInfo, err := os.Stat(file)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s\n", err.Error())
				continue
			}
			if fileInfo.Mode().IsRegular() {
				result = append(result, file)
			} else if fileInfo.Mode().IsDir() && *recurse {
				files, err := getFilesInDir(file, true)
				if err == nil {
					result = append(result, files...)
				}
			}
		}
	}
	return result
}

// getFilesInDir returns a slice containing the names of all files
// in a particular directory, optionally recursing into subdirectories.
// It does not follow symbolic links.
func getFilesInDir(dir string, recurse bool) ([]string, error) {
	infos, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var results []string
	for _, item := range infos {
		if item.Mode().IsRegular() {
			results = append(results, path.Join(dir, item.Name()))
		} else if item.IsDir() && recurse {
			subdir, err := getFilesInDir(path.Join(dir, item.Name()), true)
			if err != nil {
				// TODO: ignore??
				continue
			}
			results = append(results, subdir...)
		}
	}
	return results, nil
}

// scanFile reads the specified file and checks whether any of the lines
// match the specified pattern.  It writes any matches to the channel c.
// scanFile returns a bool indicating whether a match was found, and
// an error (if one occurred).
func scanFile(filename string, pattern string, c chan *match) (bool, error) {
	file, err := os.Open(filename)
	if err != nil {
		return false, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	var matchFound bool = false
	for scanner.Scan() {
		line := scanner.Text()
		found := strings.Contains(line, pattern)
		if found != *invert {
			matchFound = true
			result := &match{
				filename,
				strings.TrimSpace(line),
			}
			c <- result
		}
	}
	return matchFound, nil
}
