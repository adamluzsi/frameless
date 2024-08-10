package codemigrate

import (
	"context"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.llib.dev/frameless/ports/option"
)

type Migrator struct {
	Steps []MigrationStep
}

func (m Migrator) Migrate(ctx context.Context) error {
	var r MigrationResource

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
	Up func(ctx context.Context, r *MigrationResource) error
}

func (mr MigrationStep) MigrateUp(ctx context.Context, r *MigrationResource) error {
	if mr.Up == nil {
		return nil
	}
	return mr.Up(ctx, r)
}

type MigrationResource struct {
	// WDP is the working directory path, which is the current directory.
	WDP string
}

func (r MigrationResource) Grep(pattern string, opts ...GrepOption) ([]string, error) {
	var c GrepConfig = option.Use[GrepConfig](opts)
	var matchingLines []string

	var (
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

	err = filepath.Walk(r.WDP, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("Error walking %s: %v", path, err)
			return err
		}
		if !info.IsDir() {
			data, err := ioutil.ReadFile(path)
			if err != nil {
				log.Printf("Error reading file %s: %v", path, err)
				return err
			}
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				if re.MatchString(line) {
					matchingLines = append(matchingLines, line)
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return matchingLines, nil
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
