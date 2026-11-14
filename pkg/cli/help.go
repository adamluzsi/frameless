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
	"go.llib.dev/frameless/pkg/validate"
)

func isHelp(err error) bool {
	return errors.Is(err, flag.ErrHelp)
}

func toHelp(ctx context.Context, h Handler, err error) string {
	usage, usageErr := Usage(h, commandName(ctx))
	if usageErr != nil {
		panic(usageErr.Error())
	}
	var o = &bytes.Buffer{}

	printfln(o, strings.TrimRight(usage, lineSeparator))

	if err != nil && !isHelp(err) {
		printfln(o) // a blank line between the help and the error
		// TODO: we could integrate a check here to see if Handler implements an ErrorHandler to handle validation issues
		if eii, ok := errorkit.As[ErrInvalidInput](err); ok {
			defaultInvalidInputErrorHandler(ctx, o, eii)
		} else {
			printfln(o, errorkit.WithoutTrace(err).Error())
		}
	}

	// the caller ends the help with the line break, so no blank lines trail it.
	return strings.TrimRight(o.String(), lineSeparator)
}

func defaultInvalidInputErrorHandler(ctx context.Context, w io.Writer, eii ErrInvalidInput) {
	printfln(w, "Invalid input:")
	if tag, ok := eii.ArgTag(); ok && tag.Begin != nil {
		printfln(w, "\targ"+tag.PositionExpr())
	}
	if flags := eii.Flags(); 0 < len(flags) {
		printfln(w, fmt.Sprintf("\tflag: %s", strings.Join(flags, ", ")))
	}
	if envs := eii.Envs(); 0 < len(envs) {
		printfln(w, fmt.Sprintf("\tenv: %s", strings.Join(envs, ", ")))
	}
	if eii.Err != nil {
		printfln(w)
		if verr, ok := errorkit.As[validate.Error](eii); ok {
			if verr.Cause != nil {
				printfln(w, errorkit.WithoutTrace(verr.Cause).Error())
			}
		} else {
			printfln(w, errorkit.WithoutTrace(eii.Err).Error())
		}
	}
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

// help returns the help of the mux: its usage, summary, description and commands.
// Like the help of a command, a blank line separates the sections, and no line break ends it.
func (m *ServeMux) help(ctx context.Context) string {
	usage := "Usage: " + execName()
	if cmd, ok := ctxCommandName.Lookup(ctx); ok {
		usage += " " + cmd
	}
	var sections = []string{usage}
	if 0 < len(m.summary) {
		sections = append(sections, m.summary)
	}
	if 0 < len(m.description) {
		sections = append(sections, m.description)
	}
	if cmds := m.helpCommands(); 0 < len(cmds) {
		sections = append(sections, strings.Join(append([]string{"Commands:"}, cmds...), lineSeparator))
	}
	return strings.Join(sections, lineSeparator+lineSeparator)
}

func helpCreateUsage(h Handler, command string) string {
	const indent = "  "

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

	// sections are separated by a blank line
	var sections []string
	section := func(lines ...string) {
		sections = append(sections, strings.Join(lines, lineSeparator))
	}

	var usage string
	usage += "Usage: " + execName()
	if 0 < len(command) {
		usage += " " + command
	}
	if 0 < len(flags) {
		usage += " [OPTION]..."
	}
	if 0 < len(args) {
		for _, arg := range args {
			if tag, ok := arg.Ref.ArgTag(); ok && !arg.Variadic && tag.Begin != nil && tag.End != nil {
				// Range argument: expand to individual placeholders (e.g., VS1 VS2 VS3)
				for i := *tag.Begin; i < *tag.End; i++ {
					usage += fmt.Sprintf(" %s%d", arg.Name, i+1)
				}
			} else {
				usage += fmt.Sprintf(" %s", arg.Name)
			}
		}
	}

	section(usage)

	if s, ok := h.(HelpSummary); ok && 0 < len(s.Summary()) {
		section(s.Summary())
	}

	if s, ok := h.(HelpDescription); ok && 0 < len(s.Description()) {
		section(s.Description())
	}

	if 0 < len(flags) {
		lines := []string{"Options:"}
		for _, flag := range m.Flags() {
			name, ok := slicekit.First(flag.Names)
			if !ok {
				continue
			}

			line := fmt.Sprintf("  -%s=[%s]", name, flag.Ref.Node.StructField.Type.String())

			desc, ok := flag.Ref.LookupDescription()
			if ok && 0 < len(desc) {
				line += ": " + desc
			}

			if osEnvVarNames, ok := env.LookupFieldEnvNames(flag.Ref.Node.StructField); ok && 0 < len(osEnvVarNames) {
				line += fmt.Sprintf(" (env: %s)", strings.Join(osEnvVarNames, ", "))
			}

			if flag.Ref.IsTime() {
				line += fmt.Sprintf(" (layout: %q)", flag.Ref.TimeLayout())
			}

			def, ok := flag.Ref.LookupDefault()
			if ok && 0 < len(def) {
				line += fmt.Sprintf(" (default: %q)", def)
			}

			if sep := flag.Ref.Separator; sep != "" {
				line += fmt.Sprintf(" (seperator: %s)", sep)
			}

			if flag.Ref.IsRequired() {
				line += " [required]"
			}

			lines = append(lines, line)
			for i := 1; i < len(flag.Names); i++ {
				lines = append(lines, fmt.Sprintf("    -%s", flag.Names[i]))
			}
		}
		section(lines...)
	}

	if 0 < len(args) {
		lines := []string{"Arguments:"}
		for _, arg := range args {
			line := fmt.Sprintf("  %s [%s]", arg.Name, arg.Ref.Node.StructField.Type.String())

			if desc, ok := arg.Ref.LookupDescription(); ok {
				line += ": " + desc
			}

			if arg.Ref.IsTime() {
				line += fmt.Sprintf(" (layout: %q)", arg.Ref.TimeLayout())
			}

			defVal, ok := arg.Ref.LookupDefault()
			if ok {
				line += fmt.Sprintf(" (Default: %q)", defVal)
			}

			lines = append(lines, line)
		}
		section(lines...)
	}

	if 0 < len(envs) {
		lines := []string{"Environments:"}
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
			if ref.IsTime() {
				description = fmt.Sprintf("%s (layout: %q)", description, ref.TimeLayout())
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
				if tag, ok := ref.ArgTag(); ok && tag.Begin != nil {
					if ref.IsFlag() {
						subs += " or "
					}
					subs += " args" + tag.PositionExpr()
				}
				lines = append(lines, subs)
			}

		}
		section(lines...)
	}

	return strings.Join(sections, lineSeparator+lineSeparator)
}

// helpCommands returns the sorted lines of the commands list, one for each registered name.
func (m *ServeMux) helpCommands() []string {
	var cmds []string
	for name, entry := range m.entries() {
		var summary string
		if entry.Handler != nil {
			if h, ok := (entry.Handler).(HelpSummary); ok {
				summary = h.Summary()
			}
		} else if entry.Mux != nil {
			summary = entry.Mux.summary
		}

		line := " - " + name
		if 0 < len(summary) {
			line += ": " + summary
		}
		cmds = append(cmds, line)
	}
	sort.Strings(cmds)
	return cmds
}
