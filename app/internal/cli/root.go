package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"go-proxy/internal/application"
	"go-proxy/pkg/textutil"
)

type Runner struct {
	App               *application.App
	In                io.Reader
	Out, Err          io.Writer
	Version, Revision string
	JSON, Yes         bool
	NoColor           bool
	Timeout           time.Duration
	mu                sync.Mutex
	executed          bool
	result            application.Result
	wroteResult       bool
	ioCtx             context.Context
	progressLine      *progressLine
}

func New(version, revision string, in io.Reader, out, stderr io.Writer) *Runner {
	r := &Runner{In: in, Out: out, Err: stderr, Version: version, Revision: revision, ioCtx: context.Background()}
	r.App = application.New(r.progress)
	return r
}

func (r *Runner) progress(message string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	line := r.progressLine
	if line == nil {
		fmt.Fprintln(contextWriter{ctx: r.ioCtx, writer: r.Err}, redact(message))
		return
	}
	line.show(redact(message))
}

// stopProgress ends the animated step, if one is running, and leaves it on
// screen as an ordinary line. Safe to call twice.
func (r *Runner) stopProgress() {
	r.mu.Lock()
	line := r.progressLine
	r.progressLine = nil
	r.mu.Unlock()
	line.finish()
}

func (r *Runner) confirm() error {
	if !r.Yes {
		return application.Invalid("this operation requires --confirm")
	}
	return nil
}

// confirming runs the command's own argument checks and then the --confirm
// check, both before RunE. Confirmation used to be checked inside the operation,
// which is after the operation has started: refusing to act is not something
// to report from inside the operation. Argument guidance stays first, because a
// reader who has not yet named what to remove needs the list, not the flag.
func (r *Runner) confirming(args cobra.PositionalArgs) cobra.PositionalArgs {
	return func(cmd *cobra.Command, values []string) error {
		if args != nil {
			if err := args(cmd, values); err != nil {
				return err
			}
		}
		return r.confirm()
	}
}

func (r *Runner) leaf(use, short string, args cobra.PositionalArgs, fn func(context.Context, *cobra.Command, []string) (application.Result, error)) *cobra.Command {
	cmd := &cobra.Command{Use: use, Short: short, Args: args}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		r.executed = true
		stream, _ := cmd.Flags().GetBool("follow")
		stream = stream || cmd.Name() == "watchdog"
		if stream && r.JSON {
			return application.Invalid("--json is not available for streaming commands")
		}
		if r.Timeout < 0 || cmd.Flags().Changed("timeout") && r.Timeout == 0 {
			return application.Invalid("--timeout must be positive")
		}
		ctx := cmd.Context()
		cancel := func() {}
		if !stream || r.Timeout > 0 {
			timeout := r.Timeout
			if timeout == 0 {
				timeout = 2500 * time.Millisecond
				if mutation(cmd) {
					timeout = 2 * time.Minute
				}
				// Keyed on the full path, not the verb: `add` is also a fast
				// user or chain operation, while these three stream a download
				// or wait on certificate issuance.
				switch cmd.CommandPath() {
				case "gproxy protocol add", "gproxy cert ensure":
					timeout = 5 * time.Minute
				case "gproxy route test":
					// One sing-box rule-set match per set until one
					// matches, two at a time: a user with every preset
					// is a second or two on a small host.
					timeout = 10 * time.Second
				}
				if cmd.Name() == "update" && mutation(cmd) {
					timeout = 5 * time.Minute
				}
			}
			ctx, cancel = context.WithTimeout(ctx, timeout)
		}
		defer cancel()
		r.mu.Lock()
		r.ioCtx = ctx
		r.mu.Unlock()
		if mutation(cmd) {
			// The animation is the terminal's answer to a step that takes
			// minutes; a stream that cannot be rewritten gets the five-second
			// line it has always had, so a captured log reads the same as before.
			animate := colorEnabled(r.Err, r.NoColor)
			r.mu.Lock()
			r.progressLine = newProgressLine(contextWriter{ctx: ctx, writer: r.Err}, animate)
			r.mu.Unlock()
			defer r.stopProgress()
			// No opening "starting <command>" line: it repeated the command
			// just typed. The first step an operation reports opens the
			// progress, and a fast one reports none.
			if !animate {
				done := make(chan struct{})
				defer close(done)
				go func() {
					ticker := time.NewTicker(5 * time.Second)
					defer ticker.Stop()
					for {
						select {
						case <-ticker.C:
							r.progress("operation in progress")
						case <-done:
							return
						case <-ctx.Done():
							return
						}
					}
				}()
			}
		}
		result, err := fn(ctx, cmd, args)
		// Before anything is written to stdout or stderr: the animated line is
		// rewritten in place, so a result printed over it would interleave.
		r.stopProgress()
		r.result = result
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if result.Silent {
			return nil
		}
		if result.Raw != nil {
			r.wroteResult = true
			_, err = (contextWriter{ctx: ctx, writer: r.Out}).Write(result.Raw)
			return err
		}
		r.wroteResult = true
		if r.JSON {
			if data, ok := result.Data.(json.RawMessage); ok {
				return writeEnvelope(contextWriter{ctx: ctx, writer: r.Out}, result.Changed, data)
			}
			return json.NewEncoder(contextWriter{ctx: ctx, writer: r.Out}).Encode(map[string]any{"ok": true, "changed": result.Changed, "data": result.Data})
		}
		// Colour is decided from r.Out, not from the wrapper written to: the
		// wrapper carries cancellation and is never an *os.File, so asking it
		// would disable colour unconditionally. Both target the same descriptor.
		out := contextWriter{ctx: ctx, writer: r.Out}
		if render(out, palette{on: colorEnabled(r.Out, r.NoColor)}, cmd.CommandPath(), result.Data) {
			return nil
		}
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(result.Data)
	}
	return cmd
}

// writeEnvelope streams an already encoded payload in the envelope encoding/json produces for the
// result map. Re-encoding it would copy and revalidate the whole stream, which a capacity-scale
// export cannot afford.
func writeEnvelope(w io.Writer, changed bool, data json.RawMessage) error {
	header := `{"changed":false,"data":`
	if changed {
		header = `{"changed":true,"data":`
	}
	for _, chunk := range [][]byte{[]byte(header), data, []byte(`,"ok":true}` + "\n")} {
		if _, err := w.Write(chunk); err != nil {
			return err
		}
	}
	return nil
}

func mutation(cmd *cobra.Command) bool {
	if check, _ := cmd.Flags().GetBool("check"); check {
		return false
	}
	if preview, _ := cmd.Flags().GetBool("preview"); preview {
		return false
	}
	switch cmd.Name() {
	case "init", "add", "remove", "delete", "rename", "set", "modify", "clear", "apply", "release", "enable", "disable", "ensure", "update", "uninstall", "start", "stop", "restart", "sync-dns":
		return true
	}
	return false
}

func (r *Runner) Root() *cobra.Command {
	root := &cobra.Command{Use: "gproxy", Short: "go-proxy: non-interactive proxy management", SilenceUsage: true, SilenceErrors: true, Args: cobra.NoArgs}
	root.SetIn(r.In)
	root.SetOut(r.Out)
	root.SetErr(r.Err)
	root.PersistentFlags().BoolVar(&r.JSON, "json", false, "write a structured JSON result")
	root.PersistentFlags().BoolVar(&r.Yes, "confirm", false, "confirm the selected destructive operation")
	root.PersistentFlags().DurationVar(&r.Timeout, "timeout", 0, "override the operation deadline, such as 30s")
	root.PersistentFlags().BoolVar(&r.NoColor, "no-color", false, "disable colour in human-readable output")
	root.RunE = func(cmd *cobra.Command, args []string) error { return cmd.Help() }
	root.AddCommand(r.leaf("version", "show the build version", cobra.NoArgs, func(ctx context.Context, c *cobra.Command, args []string) (application.Result, error) {
		if !r.JSON {
			return application.Result{Raw: []byte("go-proxy " + r.Version + "\n")}, nil
		}
		revision := r.Revision
		if revision == "" {
			if info, ok := debug.ReadBuildInfo(); ok {
				for _, setting := range info.Settings {
					if setting.Key == "vcs.revision" {
						revision = setting.Value
					}
				}
			}
		}
		return application.Result{Data: map[string]any{"name": "go-proxy", "command": "gproxy", "version": r.Version, "revision": revision}}, nil
	}))
	registerProtocol(r, root)
	registerUser(r, root)
	registerSub(r, root)
	registerSystem(r, root)
	registerNetwork(r, root)
	registerRouting(r, root)
	registerInspect(r, root)
	var groups func(*cobra.Command)
	groups = func(c *cobra.Command) {
		if !c.Runnable() && c.HasSubCommands() {
			c.Args = cobra.NoArgs
			c.RunE = func(cmd *cobra.Command, args []string) error {
				names := []string{}
				for _, child := range cmd.Commands() {
					if child.IsAvailableCommand() || child.Name() == "help" {
						names = append(names, child.Name())
					}
				}
				if len(names) == 0 {
					return application.Invalid(cmd.CommandPath() + " requires a subcommand")
				}
				return application.Invalid(cmd.CommandPath() + " requires a subcommand: " + strings.Join(names, ", "))
			}
		}
		for _, child := range c.Commands() {
			groups(child)
		}
	}
	// Add the completion command now rather than letting cobra add it during
	// ExecuteC. Added late it would escape the groups walk below, so bare
	// `gproxy completion` would exit 0 and print help to stdout even under
	// --json, which is the contract violation u-2-129 fixed for every other
	// group.
	// Tab lists bare names, the way most commands complete; what each one
	// does is --help's to say.
	root.CompletionOptions.DisableDescriptions = true
	root.InitDefaultCompletionCmd()
	groups(root)
	registerCompletions(root)
	root.SetHelpFunc(r.help)
	return root
}

func (r *Runner) Run(ctx context.Context, args []string) int {
	r.executed = false
	r.result = application.Result{}
	r.wroteResult = false
	root := r.Root()
	helpCtx, helpCancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer helpCancel()
	root.SetOut(contextWriter{ctx: helpCtx, writer: r.Out})
	root.SetErr(contextWriter{ctx: helpCtx, writer: r.Err})
	root.SetArgs(args)
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if arg == "--json" || arg == "--json=true" {
			r.JSON = true
		}
		if arg == "--json=false" {
			r.JSON = false
		}
	}
	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	code := 1
	detail := &application.Error{Code: "operation_failed", Message: redact(err.Error())}
	var appErr *application.Error
	if errors.As(err, &appErr) {
		copy := *appErr
		detail = &copy
		detail.Message = redact(detail.Message)
	}
	if !r.executed {
		detail.Code = "invalid_argument"
	}
	if detail.Code == "invalid_argument" {
		code = 2
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		code = 130
		detail.Code = "cancelled"
		detail.Message = "operation cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		detail.Code = "timeout"
		detail.Message = "operation exceeded its deadline"
	}
	var data any
	if detail.Data != nil {
		data = detail.Data
	}
	errorCtx, errorCancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer errorCancel()
	r.mu.Lock()
	r.ioCtx = errorCtx
	r.mu.Unlock()
	if detail.JSONMessage != "" && r.JSON {
		detail.Message = detail.JSONMessage
	}
	if r.JSON && !r.wroteResult {
		_ = json.NewEncoder(contextWriter{ctx: errorCtx, writer: r.Out}).Encode(map[string]any{"ok": false, "changed": r.result.Changed || detail.Changed, "data": data, "error": detail})
	} else {
		// Colour is applied after redaction, which strips control sequences.
		p := palette{on: colorEnabled(r.Err, r.NoColor)}
		w := contextWriter{ctx: errorCtx, writer: r.Err}
		// Guidance -- an error carrying the commands that complete it --
		// prints only those commands: the error line said the same thing
		// again. An error with nothing to show after it keeps its line. The
		// JSON envelope carries the message either way.
		var listed *listedError
		if errors.As(err, &listed) && listed.list != nil {
			listed.list(w, p)
		}
		if detail.Message != "" && len(detail.Hint) == 0 {
			fmt.Fprintln(w, p.bad("error:")+" "+redact(detail.Message))
		}
		for _, line := range detail.Hint {
			fmt.Fprintln(w, commandHint(p, redactValues(line)))
		}
	}
	return code
}

var secretField = regexp.MustCompile(`(?i)(psk|password|private_key|private-key|shadow-tls-password)(["']?\s*[:=]\s*["']?)([^,\s"}]+)`)
var urlUser = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^/@\s]+@`)

func redact(s string) string { return strings.TrimSpace(redactValues(s)) }

// redactValues applies the same redaction without trimming, so guidance keeps
// the indentation that makes it readable as a list. Control sequences go with
// the secrets: an error message can quote the output of systemctl or nft, and
// --json carries no escape sequence under any condition. Line breaks survive,
// because a relayed message is sometimes several lines.
func redactValues(s string) string {
	s = secretField.ReplaceAllString(s, "${1}${2}<redacted>")
	s = urlUser.ReplaceAllString(s, "${1}<redacted>@")
	return textutil.CleanText(s)
}

// help replaces cobra's help template with this CLI's layout: one line per
// command or flag, its explanation as a bracketed note after one space, in
// lowercase and the note colour, and no blank lines between the sections.
func (r *Runner) help(cmd *cobra.Command, _ []string) {
	w := cmd.OutOrStdout()
	p := palette{on: colorEnabled(r.Out, r.NoColor)}
	about := cmd.Long
	if about == "" {
		about = cmd.Short
	}
	for _, line := range strings.Split(about, "\n") {
		if strings.TrimSpace(line) != "" {
			fmt.Fprintln(w, strings.TrimRight(line, " \t"))
		}
	}
	fmt.Fprintln(w, "Usage:")
	if cmd.Runnable() {
		fmt.Fprintln(w, "  "+cmd.UseLine())
	}
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintln(w, "  "+cmd.CommandPath()+" [command]")
		fmt.Fprintln(w, "Available Commands:")
		for _, child := range cmd.Commands() {
			if child.IsAvailableCommand() || child.Name() == "help" {
				fmt.Fprintln(w, "  "+child.Name()+" "+note(p, child.Short))
			}
		}
	}
	writeHelpFlags(w, p, "Flags:", cmd.LocalFlags())
	writeHelpFlags(w, p, "Global Flags:", cmd.InheritedFlags())
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintf(w, "Use \"%s [command] --help\" for more information about a command.\n", cmd.CommandPath())
	}
}

// writeHelpFlags lists flags as "--name type (what it does; default x)".
// Shorthands are not shown: --help is the only one, and one spelling per
// flag reads more plainly.
func writeHelpFlags(w io.Writer, p palette, title string, flags *pflag.FlagSet) {
	lines := []string{}
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}
		name, usage := pflag.UnquoteUsage(flag)
		line := "--" + flag.Name
		if name != "" {
			line += " " + name
		}
		switch flag.DefValue {
		case "", "false", "0", "0s", "[]":
		default:
			usage += "; default " + flag.DefValue
		}
		lines = append(lines, "  "+line+" "+note(p, usage))
	})
	if len(lines) == 0 {
		return
	}
	fmt.Fprintln(w, title)
	for _, line := range lines {
		fmt.Fprintln(w, line)
	}
}
