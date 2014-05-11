package main

import (
	"bufio"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path"
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
// - read symlinks in input files
// - parallelize for performance
// - wild card search (go\*.go)

func main() {

	flag.Parse()

	if flag.NArg() < 2 {
		printUsage()
		os.Exit(exitError)
		return
	}

	fmt.Println(recurse, ignoreCase, invert, wholeLine)

	files := inputFiles(flag.Args()[1:])
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

func printUsage() {
	fmt.Fprintf(os.Stderr, "Usage: grep [options] PATTERN INPUT_FILES\n")
	fmt.Fprintf(os.Stderr, "Options:\n")
	flag.PrintDefaults()
}

// inputFiles generates the list of all files that must be searched,
// given a particular set of input arguments.
func inputFiles(input []string) []string {
	var result []string
	for _, item := range input {
		info, err := os.Stat(item)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
			continue
		}
		// we handle 3 cases:  1) regular files, 2) directories, 3) symlinks
		if info.Mode().IsRegular() {
			result = append(result, item)
		} else if info.Mode().IsDir() {
			subdir, err := getFilesInDir(item, *recurse)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %s\n", err.Error())
				continue
			}
			result = append(result, subdir...)
		} else if info.Mode()&os.ModeSymlink > 0 {
			// TODO follow symlink
		} else {
			fmt.Fprintf(os.Stderr, "Skipping %s (unknown file type)\n", info.Name())
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
				fmt.Fprintf(os.Stderr, "Error getting dir: %s\n", err.Error())
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
		if line := scanner.Text(); strings.Contains(line, pattern) {
			matchFound = true
			result := &match{
				filename,
				line,
			}
			c <- result
		}
	}
	return matchFound, nil
}
