package cli

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"sort"
	"strings"

	"go.llib.dev/frameless/pkg/env"
	"go.llib.dev/frameless/pkg/slicekit"
)

func isHelp(err error) bool {
	return errors.Is(err, flag.ErrHelp)
}

func toHelp(h Handler, err error) string {
	isHelpErr := isHelp(err)

	usage, usageErr := Usage(h, execName())
	if usageErr != nil {
		panic(usageErr.Error())
	}
	var o = &bytes.Buffer{}

	printfln(o, usage)
	printfln(o)

	if !isHelpErr && err != nil {
		printfln(o, err.Error())
	}

	return o.String()
}

type HelpUsage interface {
	Usage(pattern string) (string, error)
}

// Usage will generate a help usage message for a given handler on a given command request pattern/path.
func Usage(h Handler, pattern string) (string, error) {
	if u, ok := h.(HelpUsage); ok {
		return u.Usage(pattern)
	}
	var meta *structMeta
	m, ok, err := structMetaFor(h)
	if err != nil {
		return "", err
	}
	if ok {
		meta = &m
	}
	return helpCreateUsage(h, meta, pattern), nil
}

func (m Mux) helpLineBreak(w io.Writer, n int) {
	w.Write([]byte(strings.Repeat(lineSeparator, n)))
}

func (m Mux) helpUsage(w io.Writer) {
	var msg []string
	msg = append(msg, fmt.Sprintf("Usage: %s", m.getPath()))
	printfln(w, msg...)
}

func helpUsageOf(w io.Writer, h Handler, meta *structMeta, path string) {
	printfln(w, helpCreateUsage(h, meta, path))
}

func helpCreateUsage(h Handler, meta *structMeta, path string) string {
	var lines []string

	var usage string
	usage += "Usage: " + path

	if meta != nil {
		if 0 < len(meta.Flags) {
			usage += " [OPTION]..."
		}
		if 0 < len(meta.Args) {
			for _, arg := range meta.Args {
				usage += fmt.Sprintf(" [%s]", arg.Name)
			}
		}
	}

	lines = append(lines, usage, "")

	if s, ok := h.(HelpSummary); ok {
		lines = append(lines, s.Summary(), "")
	}

	if meta != nil {
		if 0 < len(meta.Flags) {
			lines = append(lines, "Options:")
			for _, flag := range meta.Flags {
				name, ok := slicekit.First(flag.Names)
				if !ok {
					continue
				}

				line := fmt.Sprintf("  -%s=[%s]", name, flag.StructField.Type.String())
				if 0 < len(flag.Desc) {
					line += ": " + flag.Desc
				}

				if osEnvVarNames, ok := env.LookupFieldEnvNames(flag.StructField); ok && 0 < len(osEnvVarNames) {
					line += fmt.Sprintf(" (env: %s)", strings.Join(osEnvVarNames, ", "))
				}

				if 0 < len(flag.Default) {
					line += fmt.Sprintf(" (default: %s)", flag.Default)
				}

				lines = append(lines, line)

				for i := 1; i < len(flag.Names); i++ {
					lines = append(lines, fmt.Sprintf("  -%s", flag.Names[i]))
				}
			}
		}
		if 0 < len(meta.Args) {
			if 0 < len(meta.Flags) {
				lines = append(lines, "") // empty line for seperation
			}
			lines = append(lines, "Arguments:")
			for _, arg := range meta.Args {
				line := fmt.Sprintf("  %s [%s]", arg.Name, arg.StructField.Type.String())
				if 0 < len(arg.Desc) {
					line += ": " + arg.Desc
				}
				if 0 < len(arg.Default) {
					line += fmt.Sprintf(" (Default: %s)", arg.Default)
				}

				lines = append(lines, line)
			}
		}
	}

	return strings.Join(lines, lineSeparator)
}

func (m Mux) helpCommands(w io.Writer) {
	var msg []string

	var cmds []string
	for name, entry := range m.entries() {
		var line string
		line = " - " + name

		if h, ok := entry.Handler.(HelpSummary); ok {
			line += ": " + h.Summary()
		}

		cmds = append(cmds, line)
	}

	if 0 < len(cmds) {
		msg = append(msg, "Commands:")
		sort.Strings(cmds)
		msg = append(msg, cmds...)
	}

	printfln(w, msg...)
}
