package cli

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"go.llib.dev/frameless/pkg/env"
	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/pkg/slicekit"
)

func isHelp(err error) bool {
	return errors.Is(err, flag.ErrHelp)
}

func toHelp(ctx context.Context, h Handler, err error) string {
	isHelpErr := isHelp(err)

	usage, usageErr := Usage(h, commandName(ctx))
	if usageErr != nil {
		panic(usageErr.Error())
	}
	var o = &bytes.Buffer{}

	printfln(o, usage)
	printfln(o)

	if !isHelpErr && err != nil {
		printfln(o, errorkit.WithoutTrace(err).Error())
	}

	return o.String()
}

type HelpUsage interface {
	Usage(pattern string) (string, error)
}

// Usage will generate a help usage message for a given handler on a given command request pattern/path.
func Usage(h Handler, command string) (string, error) {
	if u, ok := h.(HelpUsage); ok {
		return u.Usage(command)
	}
	return helpCreateUsage(h, command), nil
}

func (m Mux) helpLineBreak(w io.Writer, n int) {
	w.Write([]byte(strings.Repeat(lineSeparator, n)))
}

func (m Mux) helpUsage(w io.Writer) {
	var msg []string
	msg = append(msg, fmt.Sprintf("Usage: %s", m.getPath()))
	printfln(w, msg...)
}

func helpCreateUsage(h Handler, command string) string {
	const indent = "  "
	var lines []string

	ptr := reflect.New(reflect.TypeOf(h))
	ptr.Elem().Set(reflect.ValueOf(h))

	var (
		m     = metaFor(ptr)
		flags = m.Flags()
		args  = m.Args()
		envs  []*metaRef
	)

	for _, ref := range m.refs {
		if ref.IsEnv() {
			envs = append(envs, ref)
		}
	}

	var usage string
	usage += "Usage: " + command

	if 0 < len(flags) {
		usage += " [OPTION]..."
	}
	if 0 < len(args) {
		for _, arg := range args {
			usage += fmt.Sprintf(" [%s]", arg.Name)
		}
	}

	lines = append(lines, usage, "")

	if s, ok := h.(HelpSummary); ok {
		lines = append(lines, s.Summary(), "")
	}

	if 0 < len(flags) {
		lines = append(lines, "Options:")
		for _, flag := range m.Flags() {
			name, ok := slicekit.First(flag.Names)
			if !ok {
				continue
			}

			line := fmt.Sprintf("  -%s=[%s]", name, flag.Ref.V.StructField.Type.String())

			desc, ok := flag.Ref.LookupDescription()
			if ok && 0 < len(desc) {
				line += ": " + desc
			}

			if osEnvVarNames, ok := env.LookupFieldEnvNames(flag.Ref.V.StructField); ok && 0 < len(osEnvVarNames) {
				line += fmt.Sprintf(" (env: %s)", strings.Join(osEnvVarNames, ", "))
			}

			def, ok := flag.Ref.LookupDefault()
			if ok && 0 < len(def) {
				line += fmt.Sprintf(" (default: %q)", def)
			}

			if flag.Ref.IsRequired() {
				line += " [required]"
			}

			lines = append(lines, line)

			for i := 1; i < len(flag.Names); i++ {
				lines = append(lines, fmt.Sprintf("  -%s", flag.Names[i]))
			}
		}
	}

	if 0 < len(args) {
		if 0 < len(flags) {
			lines = append(lines, "") // empty line for seperation
		}
		lines = append(lines, "Arguments:")
		for _, arg := range args {
			line := fmt.Sprintf("  %s [%s]", arg.Name, arg.Ref.V.StructField.Type.String())

			if desc, ok := arg.Ref.LookupDescription(); ok {
				line += ": " + desc
			}

			defVal, ok := arg.Ref.LookupDefault()
			if ok {
				line += fmt.Sprintf(" (Default: %q)", defVal)
			}

			lines = append(lines, line)
		}
	}

	if 0 < len(envs) {
		lines = append(lines, "") // empty line for seperation
		lines = append(lines, "Environments:")
		for _, ref := range envs {
			refEnvs := ref.Envs()

			if len(refEnvs) == 0 {
				continue
			}

			var idLine string

			idLine = fmt.Sprintf("%s%s [%s]", indent, strings.Join(refEnvs, ", "), ref.Type().String())
			if ref.IsRequired() {
				idLine += " [required]"
			}

			lines = append(lines, idLine)

			linePrefix := strings.Repeat(indent, 3)

			var description string
			if desc, ok := ref.LookupDescription(); ok {
				description += desc
			}
			if defVal, ok := ref.LookupDefault(); ok {
				description = fmt.Sprintf("%s (default: %q)", description, defVal)
			}
			if 0 < len(description) {
				lines = append(lines, fmt.Sprintf("%s%s", linePrefix, description))
			}
			if ref.IsFlag() || ref.IsArg() {
				var subs string = fmt.Sprintf("%sSubstitute", linePrefix)
				if ref.IsFlag() {
					subs += fmt.Sprintf(" %s flag", strings.Join(ref.Flags(), " / "))
				}
				if argInd, ok := ref.ArgIndex(); ok {
					if ref.IsFlag() {
						subs += " or "
					}
					subs += fmt.Sprintf(" args[%d]", argInd)
				}
				lines = append(lines, subs)
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

		if entry.Handler != nil {
			if h, ok := (entry.Handler).(HelpSummary); ok {
				line += ": " + h.Summary()
			}
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
