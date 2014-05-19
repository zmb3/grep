package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
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
// - parallelize for performance

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: grep [options] <PATTERN> [INPUT_FILES]\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(exitError)
		return
	}

	files := inputFiles(flag.Args()[1:])
	c := make(chan *match)

	// kick off a goroutine that performs the search and writes matches to c
	// (we either search stdin or a set of files)
	go func() {
		if len(files) == 0 {
			scanFile("stdin", os.Stdin, flag.Arg(0), c)
		} else {
			for _, filename := range files {
				file, err := os.Open(filename)
				if err != nil {
					continue
				}
				defer file.Close()
				scanFile(filename, file, flag.Arg(0), c)
			}
		}
		close(c)
	}()

	// display matching lines.  a match is considered anything that procudes output
	// (so if the invert flag is enabled, a match is actually a line that didn't
	// match the specified pattern)
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
		// for each glob match, add it to the search list if it is a regular file,
		// or recurse if the recurse flag is enabled and the match is a directory
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

// getFilesInDir returns a slice containing the names of all regular files
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

// scanFile reads the from the specified Reader and checks whether any
// of the lines match the specified pattern.  It writes any matches to the
// channel c.  scanFile returns a bool indicating whether a match was found,
// and an error (if one occurred).
func scanFile(filename string, rc io.Reader, pattern string, c chan *match) (bool, error) {
	scanner := bufio.NewScanner(rc)
	var matchFound bool = false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// convert to lower case if ignoreCase is enabled
		// TODO: might be faster to use strings.EqualFold()
		if *ignoreCase {
			line = strings.ToLower(line)
			pattern = strings.ToLower(pattern)
		}

		// we either look for a substring or an exact match
		// (depending on whether the "whole line" flag is enabled)
		var found bool
		if *wholeLine {
			found = line == pattern
		} else {
			found = strings.Contains(line, pattern)
		}

		// we return a match based on the find result and the invert flag
		if found != *invert {
			matchFound = true
			// if the string isn't valid utf8, we'll consider the file binary
			binary := !utf8.ValidString(line)
			if binary {
				line = "Binary File Matches"
			}
			result := &match{
				filename,
				line,
			}
			c <- result

			// we don't need multiple "binary file matches" messages
			if binary {
				break
			}
		}
	}
	return matchFound, nil
}
