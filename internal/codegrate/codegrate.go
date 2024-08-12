package codegrate

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"go.llib.dev/frameless/ports/option"
)

type Migrator struct {
	// VersionFileName is the file name that contains the versions
	VersionFileName string

	// Steps contain the migration steps
	Steps map[Version]MigrationStep
}

func (m Migrator) Migrate(ctx context.Context) error {
	var r Resource

	wdp, err := m.findRootDirectory(ctx)
	if err != nil {
		return err
	}
	r.WDP = wdp

	for _, step := range m.Steps {
		if err := step.MigrateUp(ctx, &r); err != nil {
			return err
		}
	}
	return nil
}

func (m Migrator) findRootDirectory(ctx context.Context) (string, error) {
	return "", nil
}

type MigrationStep struct {
	Up func(ctx context.Context, r *Resource) error
}

func (mr MigrationStep) MigrateUp(ctx context.Context, r *Resource) error {
	if mr.Up == nil {
		return nil
	}
	return mr.Up(ctx, r)
}

type Resource struct {
	// WDP is the working directory path, which is the current directory.
	WDP string
}

type GrepMatch struct {
	FilePath    string
	LineNumber  int
	LineContent string
}

func (r Resource) Grep(pattern string, opts ...GrepOption) ([]GrepMatch, error) {
	var (
		c   GrepConfig = option.Use[GrepConfig](opts)
		re  *regexp.Regexp
		err error
	)
	if c.FixedStrings {
		re, err = regexp.Compile(`\m` + pattern + `\M`) // regexp.QuoteMeta()
	} else {
		re, err = regexp.Compile(pattern)
	}
	if err != nil {
		return nil, err
	}

	var matches []GrepMatch
	err = filepath.Walk(r.WDP, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() { // skip dir
			return nil
		}

		file, err := os.Open(path)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		scanner.Split(bufio.ScanLines)
		for lineNumber := 1; scanner.Scan(); lineNumber++ {
			line := scanner.Text()

			if re.MatchString(line) {
				matches = append(matches, GrepMatch{
					FilePath:    path,
					LineNumber:  lineNumber,
					LineContent: line,
				})
			}
		}
		return scanner.Err()
	})
	if err != nil {
		return nil, err
	}

	return matches, nil
}

type GrepOption interface {
	option.Option[GrepConfig]
}

type GrepConfig struct {
	// FixedStrings will make Grep interpret pattern as a set of fixed strings.
	FixedStrings bool
}

func (c GrepConfig) Configure(t *GrepConfig) {
	option.Configure[GrepConfig](c, t)
}
